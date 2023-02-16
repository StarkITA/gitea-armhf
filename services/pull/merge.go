// Copyright 2019 The Gitea Authors.
// All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/models/db"
	git_model "code.gitea.io/gitea/models/git"
	issues_model "code.gitea.io/gitea/models/issues"
	access_model "code.gitea.io/gitea/models/perm/access"
	pull_model "code.gitea.io/gitea/models/pull"
	repo_model "code.gitea.io/gitea/models/repo"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/cache"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/notification"
	"code.gitea.io/gitea/modules/references"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/timeutil"
	asymkey_service "code.gitea.io/gitea/services/asymkey"
	issue_service "code.gitea.io/gitea/services/issue"
)

// GetDefaultMergeMessage returns default message used when merging pull request
func GetDefaultMergeMessage(ctx context.Context, baseGitRepo *git.Repository, pr *issues_model.PullRequest, mergeStyle repo_model.MergeStyle) (message, body string, err error) {
	if err := pr.LoadHeadRepo(ctx); err != nil {
		return "", "", err
	}
	if err := pr.LoadBaseRepo(ctx); err != nil {
		return "", "", err
	}
	if pr.BaseRepo == nil {
		return "", "", repo_model.ErrRepoNotExist{ID: pr.BaseRepoID}
	}

	if err := pr.LoadIssue(ctx); err != nil {
		return "", "", err
	}

	isExternalTracker := pr.BaseRepo.UnitEnabled(ctx, unit.TypeExternalTracker)
	issueReference := "#"
	if isExternalTracker {
		issueReference = "!"
	}

	if mergeStyle != "" {
		templateFilepath := fmt.Sprintf(".gitea/default_merge_message/%s_TEMPLATE.md", strings.ToUpper(string(mergeStyle)))
		commit, err := baseGitRepo.GetBranchCommit(pr.BaseRepo.DefaultBranch)
		if err != nil {
			return "", "", err
		}
		templateContent, err := commit.GetFileContent(templateFilepath, setting.Repository.PullRequest.DefaultMergeMessageSize)
		if err != nil {
			if !git.IsErrNotExist(err) {
				return "", "", err
			}
		} else {
			vars := map[string]string{
				"BaseRepoOwnerName":      pr.BaseRepo.OwnerName,
				"BaseRepoName":           pr.BaseRepo.Name,
				"BaseBranch":             pr.BaseBranch,
				"HeadRepoOwnerName":      "",
				"HeadRepoName":           "",
				"HeadBranch":             pr.HeadBranch,
				"PullRequestTitle":       pr.Issue.Title,
				"PullRequestDescription": pr.Issue.Content,
				"PullRequestPosterName":  pr.Issue.Poster.Name,
				"PullRequestIndex":       strconv.FormatInt(pr.Index, 10),
				"PullRequestReference":   fmt.Sprintf("%s%d", issueReference, pr.Index),
			}
			if pr.HeadRepo != nil {
				vars["HeadRepoOwnerName"] = pr.HeadRepo.OwnerName
				vars["HeadRepoName"] = pr.HeadRepo.Name
			}
			refs, err := pr.ResolveCrossReferences(ctx)
			if err == nil {
				closeIssueIndexes := make([]string, 0, len(refs))
				closeWord := "close"
				if len(setting.Repository.PullRequest.CloseKeywords) > 0 {
					closeWord = setting.Repository.PullRequest.CloseKeywords[0]
				}
				for _, ref := range refs {
					if ref.RefAction == references.XRefActionCloses {
						if err := ref.LoadIssue(ctx); err != nil {
							return "", "", err
						}
						closeIssueIndexes = append(closeIssueIndexes, fmt.Sprintf("%s %s%d", closeWord, issueReference, ref.Issue.Index))
					}
				}
				if len(closeIssueIndexes) > 0 {
					vars["ClosingIssues"] = strings.Join(closeIssueIndexes, ", ")
				} else {
					vars["ClosingIssues"] = ""
				}
			}
			message, body = expandDefaultMergeMessage(templateContent, vars)
			return message, body, nil
		}
	}

	// Squash merge has a different from other styles.
	if mergeStyle == repo_model.MergeStyleSquash {
		return fmt.Sprintf("%s (%s%d)", pr.Issue.Title, issueReference, pr.Issue.Index), "", nil
	}

	if pr.BaseRepoID == pr.HeadRepoID {
		return fmt.Sprintf("Merge pull request '%s' (%s%d) from %s into %s", pr.Issue.Title, issueReference, pr.Issue.Index, pr.HeadBranch, pr.BaseBranch), "", nil
	}

	if pr.HeadRepo == nil {
		return fmt.Sprintf("Merge pull request '%s' (%s%d) from <deleted>:%s into %s", pr.Issue.Title, issueReference, pr.Issue.Index, pr.HeadBranch, pr.BaseBranch), "", nil
	}

	return fmt.Sprintf("Merge pull request '%s' (%s%d) from %s:%s into %s", pr.Issue.Title, issueReference, pr.Issue.Index, pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseBranch), "", nil
}

func expandDefaultMergeMessage(template string, vars map[string]string) (message, body string) {
	message = strings.TrimSpace(template)
	if splits := strings.SplitN(message, "\n", 2); len(splits) == 2 {
		message = splits[0]
		body = strings.TrimSpace(splits[1])
	}
	mapping := func(s string) string { return vars[s] }
	return os.Expand(message, mapping), os.Expand(body, mapping)
}

// Merge merges pull request to base repository.
// Caller should check PR is ready to be merged (review and status checks)
func Merge(ctx context.Context, pr *issues_model.PullRequest, doer *user_model.User, baseGitRepo *git.Repository, mergeStyle repo_model.MergeStyle, expectedHeadCommitID, message string, wasAutoMerged bool) error {
	if err := pr.LoadHeadRepo(ctx); err != nil {
		log.Error("LoadHeadRepo: %v", err)
		return fmt.Errorf("LoadHeadRepo: %w", err)
	} else if err := pr.LoadBaseRepo(ctx); err != nil {
		log.Error("LoadBaseRepo: %v", err)
		return fmt.Errorf("LoadBaseRepo: %w", err)
	}

	pullWorkingPool.CheckIn(fmt.Sprint(pr.ID))
	defer pullWorkingPool.CheckOut(fmt.Sprint(pr.ID))

	// Removing an auto merge pull and ignore if not exist
	if err := pull_model.DeleteScheduledAutoMerge(ctx, pr.ID); err != nil && !db.IsErrNotExist(err) {
		return err
	}

	prUnit, err := pr.BaseRepo.GetUnit(ctx, unit.TypePullRequests)
	if err != nil {
		log.Error("pr.BaseRepo.GetUnit(unit.TypePullRequests): %v", err)
		return err
	}
	prConfig := prUnit.PullRequestsConfig()

	// Check if merge style is correct and allowed
	if !prConfig.IsMergeStyleAllowed(mergeStyle) {
		return models.ErrInvalidMergeStyle{ID: pr.BaseRepo.ID, Style: mergeStyle}
	}

	defer func() {
		go AddTestPullRequestTask(doer, pr.BaseRepo.ID, pr.BaseBranch, false, "", "")
	}()

	// Run the merge in the hammer context to prevent cancellation
	hammerCtx := graceful.GetManager().HammerContext()

	pr.MergedCommitID, err = rawMerge(hammerCtx, pr, doer, mergeStyle, expectedHeadCommitID, message)
	if err != nil {
		return err
	}

	pr.MergedUnix = timeutil.TimeStampNow()
	pr.Merger = doer
	pr.MergerID = doer.ID

	if _, err := pr.SetMerged(hammerCtx); err != nil {
		log.Error("SetMerged [%d]: %v", pr.ID, err)
	}

	if err := pr.LoadIssue(hammerCtx); err != nil {
		log.Error("LoadIssue [%d]: %v", pr.ID, err)
	}

	if err := pr.Issue.LoadRepo(hammerCtx); err != nil {
		log.Error("LoadRepo for issue [%d]: %v", pr.ID, err)
	}
	if err := pr.Issue.Repo.GetOwner(hammerCtx); err != nil {
		log.Error("GetOwner for PR [%d]: %v", pr.ID, err)
	}

	if wasAutoMerged {
		notification.NotifyAutoMergePullRequest(hammerCtx, doer, pr)
	} else {
		notification.NotifyMergePullRequest(hammerCtx, doer, pr)
	}

	// Reset cached commit count
	cache.Remove(pr.Issue.Repo.GetCommitsCountCacheKey(pr.BaseBranch, true))

	// Resolve cross references
	refs, err := pr.ResolveCrossReferences(hammerCtx)
	if err != nil {
		log.Error("ResolveCrossReferences: %v", err)
		return nil
	}

	for _, ref := range refs {
		if err = ref.LoadIssue(hammerCtx); err != nil {
			return err
		}
		if err = ref.Issue.LoadRepo(hammerCtx); err != nil {
			return err
		}
		close := ref.RefAction == references.XRefActionCloses
		if close != ref.Issue.IsClosed {
			if err = issue_service.ChangeStatus(ref.Issue, doer, pr.MergedCommitID, close); err != nil {
				// Allow ErrDependenciesLeft
				if !issues_model.IsErrDependenciesLeft(err) {
					return err
				}
			}
		}
	}
	return nil
}

// rawMerge perform the merge operation without changing any pull information in database
func rawMerge(ctx context.Context, pr *issues_model.PullRequest, doer *user_model.User, mergeStyle repo_model.MergeStyle, expectedHeadCommitID, message string) (string, error) {
	// Clone base repo.
	tmpBasePath, err := createTemporaryRepo(ctx, pr)
	if err != nil {
		log.Error("CreateTemporaryPath: %v", err)
		return "", err
	}
	defer func() {
		if err := repo_module.RemoveTemporaryPath(tmpBasePath); err != nil {
			log.Error("Merge: RemoveTemporaryPath: %s", err)
		}
	}()

	baseBranch := "base"
	trackingBranch := "tracking"
	stagingBranch := "staging"

	if expectedHeadCommitID != "" {
		trackingCommitID, _, err := git.NewCommand(ctx, "show-ref", "--hash").AddDynamicArguments(git.BranchPrefix + trackingBranch).RunStdString(&git.RunOpts{Dir: tmpBasePath})
		if err != nil {
			log.Error("show-ref[%s] --hash refs/heads/trackingn: %v", tmpBasePath, git.BranchPrefix+trackingBranch, err)
			return "", fmt.Errorf("getDiffTree: %w", err)
		}
		if strings.TrimSpace(trackingCommitID) != expectedHeadCommitID {
			return "", models.ErrSHADoesNotMatch{
				GivenSHA:   expectedHeadCommitID,
				CurrentSHA: trackingCommitID,
			}
		}
	}

	var outbuf, errbuf strings.Builder

	// Enable sparse-checkout
	sparseCheckoutList, err := getDiffTree(ctx, tmpBasePath, baseBranch, trackingBranch)
	if err != nil {
		log.Error("getDiffTree(%s, %s, %s): %v", tmpBasePath, baseBranch, trackingBranch, err)
		return "", fmt.Errorf("getDiffTree: %w", err)
	}

	infoPath := filepath.Join(tmpBasePath, ".git", "info")
	if err := os.MkdirAll(infoPath, 0o700); err != nil {
		log.Error("Unable to create .git/info in %s: %v", tmpBasePath, err)
		return "", fmt.Errorf("Unable to create .git/info in tmpBasePath: %w", err)
	}

	sparseCheckoutListPath := filepath.Join(infoPath, "sparse-checkout")
	if err := os.WriteFile(sparseCheckoutListPath, []byte(sparseCheckoutList), 0o600); err != nil {
		log.Error("Unable to write .git/info/sparse-checkout file in %s: %v", tmpBasePath, err)
		return "", fmt.Errorf("Unable to write .git/info/sparse-checkout file in tmpBasePath: %w", err)
	}

	gitConfigCommand := func() *git.Command {
		return git.NewCommand(ctx, "config", "--local")
	}

	// Switch off LFS process (set required, clean and smudge here also)
	if err := gitConfigCommand().AddArguments("filter.lfs.process", "").
		Run(&git.RunOpts{
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git config [filter.lfs.process -> <> ]: %v\n%s\n%s", err, outbuf.String(), errbuf.String())
		return "", fmt.Errorf("git config [filter.lfs.process -> <> ]: %w\n%s\n%s", err, outbuf.String(), errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	if err := gitConfigCommand().AddArguments("filter.lfs.required", "false").
		Run(&git.RunOpts{
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git config [filter.lfs.required -> <false> ]: %v\n%s\n%s", err, outbuf.String(), errbuf.String())
		return "", fmt.Errorf("git config [filter.lfs.required -> <false> ]: %w\n%s\n%s", err, outbuf.String(), errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	if err := gitConfigCommand().AddArguments("filter.lfs.clean", "").
		Run(&git.RunOpts{
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git config [filter.lfs.clean -> <> ]: %v\n%s\n%s", err, outbuf.String(), errbuf.String())
		return "", fmt.Errorf("git config [filter.lfs.clean -> <> ]: %w\n%s\n%s", err, outbuf.String(), errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	if err := gitConfigCommand().AddArguments("filter.lfs.smudge", "").
		Run(&git.RunOpts{
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git config [filter.lfs.smudge -> <> ]: %v\n%s\n%s", err, outbuf.String(), errbuf.String())
		return "", fmt.Errorf("git config [filter.lfs.smudge -> <> ]: %w\n%s\n%s", err, outbuf.String(), errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	if err := gitConfigCommand().AddArguments("core.sparseCheckout", "true").
		Run(&git.RunOpts{
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git config [core.sparseCheckout -> true ]: %v\n%s\n%s", err, outbuf.String(), errbuf.String())
		return "", fmt.Errorf("git config [core.sparsecheckout -> true]: %w\n%s\n%s", err, outbuf.String(), errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	// Read base branch index
	if err := git.NewCommand(ctx, "read-tree", "HEAD").
		Run(&git.RunOpts{
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git read-tree HEAD: %v\n%s\n%s", err, outbuf.String(), errbuf.String())
		return "", fmt.Errorf("Unable to read base branch in to the index: %w\n%s\n%s", err, outbuf.String(), errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	sig := doer.NewGitSig()
	committer := sig

	// Determine if we should sign. If no signKeyID, use --no-gpg-sign to countermand the sign config (from gitconfig)
	var signArgs git.TrustedCmdArgs
	sign, signKeyID, signer, _ := asymkey_service.SignMerge(ctx, pr, doer, tmpBasePath, "HEAD", trackingBranch)
	if sign {
		if pr.BaseRepo.GetTrustModel() == repo_model.CommitterTrustModel || pr.BaseRepo.GetTrustModel() == repo_model.CollaboratorCommitterTrustModel {
			committer = signer
		}
		signArgs = git.ToTrustedCmdArgs([]string{"-S" + signKeyID})
	} else {
		signArgs = append(signArgs, "--no-gpg-sign")
	}

	commitTimeStr := time.Now().Format(time.RFC3339)

	// Because this may call hooks we should pass in the environment
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME="+sig.Name,
		"GIT_AUTHOR_EMAIL="+sig.Email,
		"GIT_AUTHOR_DATE="+commitTimeStr,
		"GIT_COMMITTER_NAME="+committer.Name,
		"GIT_COMMITTER_EMAIL="+committer.Email,
		"GIT_COMMITTER_DATE="+commitTimeStr,
	)

	// Merge commits.
	switch mergeStyle {
	case repo_model.MergeStyleMerge:
		cmd := git.NewCommand(ctx, "merge", "--no-ff", "--no-commit").AddDynamicArguments(trackingBranch)
		if err := runMergeCommand(pr, mergeStyle, cmd, tmpBasePath); err != nil {
			log.Error("Unable to merge tracking into base: %v", err)
			return "", err
		}

		if err := commitAndSignNoAuthor(ctx, pr, message, signArgs, tmpBasePath, env); err != nil {
			log.Error("Unable to make final commit: %v", err)
			return "", err
		}
	case repo_model.MergeStyleRebase:
		fallthrough
	case repo_model.MergeStyleRebaseUpdate:
		fallthrough
	case repo_model.MergeStyleRebaseMerge:
		// Checkout head branch
		if err := git.NewCommand(ctx, "checkout", "-b").AddDynamicArguments(stagingBranch, trackingBranch).
			Run(&git.RunOpts{
				Dir:    tmpBasePath,
				Stdout: &outbuf,
				Stderr: &errbuf,
			}); err != nil {
			log.Error("git checkout base prior to merge post staging rebase [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
			return "", fmt.Errorf("git checkout base prior to merge post staging rebase  [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
		}
		outbuf.Reset()
		errbuf.Reset()

		// Rebase before merging
		if err := git.NewCommand(ctx, "rebase").AddDynamicArguments(baseBranch).
			Run(&git.RunOpts{
				Dir:    tmpBasePath,
				Stdout: &outbuf,
				Stderr: &errbuf,
			}); err != nil {
			// Rebase will leave a REBASE_HEAD file in .git if there is a conflict
			if _, statErr := os.Stat(filepath.Join(tmpBasePath, ".git", "REBASE_HEAD")); statErr == nil {
				var commitSha string
				ok := false
				failingCommitPaths := []string{
					filepath.Join(tmpBasePath, ".git", "rebase-apply", "original-commit"), // Git < 2.26
					filepath.Join(tmpBasePath, ".git", "rebase-merge", "stopped-sha"),     // Git >= 2.26
				}
				for _, failingCommitPath := range failingCommitPaths {
					if _, statErr := os.Stat(failingCommitPath); statErr == nil {
						commitShaBytes, readErr := os.ReadFile(failingCommitPath)
						if readErr != nil {
							// Abandon this attempt to handle the error
							log.Error("git rebase staging on to base [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
							return "", fmt.Errorf("git rebase staging on to base [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
						}
						commitSha = strings.TrimSpace(string(commitShaBytes))
						ok = true
						break
					}
				}
				if !ok {
					log.Error("Unable to determine failing commit sha for this rebase message. Cannot cast as models.ErrRebaseConflicts.")
					log.Error("git rebase staging on to base [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
					return "", fmt.Errorf("git rebase staging on to base [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
				}
				log.Debug("RebaseConflict at %s [%s:%s -> %s:%s]: %v\n%s\n%s", commitSha, pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
				return "", models.ErrRebaseConflicts{
					Style:     mergeStyle,
					CommitSHA: commitSha,
					StdOut:    outbuf.String(),
					StdErr:    errbuf.String(),
					Err:       err,
				}
			}
			log.Error("git rebase staging on to base [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
			return "", fmt.Errorf("git rebase staging on to base [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
		}
		outbuf.Reset()
		errbuf.Reset()

		// not need merge, just update by rebase. so skip
		if mergeStyle == repo_model.MergeStyleRebaseUpdate {
			break
		}

		// Checkout base branch again
		if err := git.NewCommand(ctx, "checkout").AddDynamicArguments(baseBranch).
			Run(&git.RunOpts{
				Dir:    tmpBasePath,
				Stdout: &outbuf,
				Stderr: &errbuf,
			}); err != nil {
			log.Error("git checkout base prior to merge post staging rebase [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
			return "", fmt.Errorf("git checkout base prior to merge post staging rebase  [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
		}
		outbuf.Reset()
		errbuf.Reset()

		cmd := git.NewCommand(ctx, "merge")
		if mergeStyle == repo_model.MergeStyleRebase {
			cmd.AddArguments("--ff-only")
		} else {
			cmd.AddArguments("--no-ff", "--no-commit")
		}
		cmd.AddDynamicArguments(stagingBranch)

		// Prepare merge with commit
		if err := runMergeCommand(pr, mergeStyle, cmd, tmpBasePath); err != nil {
			log.Error("Unable to merge staging into base: %v", err)
			return "", err
		}
		if mergeStyle == repo_model.MergeStyleRebaseMerge {
			if err := commitAndSignNoAuthor(ctx, pr, message, signArgs, tmpBasePath, env); err != nil {
				log.Error("Unable to make final commit: %v", err)
				return "", err
			}
		}
	case repo_model.MergeStyleSquash:
		// Merge with squash
		cmd := git.NewCommand(ctx, "merge", "--squash").AddDynamicArguments(trackingBranch)
		if err := runMergeCommand(pr, mergeStyle, cmd, tmpBasePath); err != nil {
			log.Error("Unable to merge --squash tracking into base: %v", err)
			return "", err
		}

		if err = pr.Issue.LoadPoster(ctx); err != nil {
			log.Error("LoadPoster: %v", err)
			return "", fmt.Errorf("LoadPoster: %w", err)
		}
		sig := pr.Issue.Poster.NewGitSig()
		if setting.Repository.PullRequest.AddCoCommitterTrailers && committer.String() != sig.String() {
			// add trailer
			message += fmt.Sprintf("\nCo-authored-by: %s\nCo-committed-by: %s\n", sig.String(), sig.String())
		}
		if err := git.NewCommand(ctx, "commit").
			AddArguments(signArgs...).
			AddOptionFormat("--author='%s <%s>'", sig.Name, sig.Email).
			AddOptionValues("-m", message).
			Run(&git.RunOpts{
				Env:    env,
				Dir:    tmpBasePath,
				Stdout: &outbuf,
				Stderr: &errbuf,
			}); err != nil {
			log.Error("git commit [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
			return "", fmt.Errorf("git commit [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
		}
		outbuf.Reset()
		errbuf.Reset()
	default:
		return "", models.ErrInvalidMergeStyle{ID: pr.BaseRepo.ID, Style: mergeStyle}
	}

	// OK we should cache our current head and origin/headbranch
	mergeHeadSHA, err := git.GetFullCommitID(ctx, tmpBasePath, "HEAD")
	if err != nil {
		return "", fmt.Errorf("Failed to get full commit id for HEAD: %w", err)
	}
	mergeBaseSHA, err := git.GetFullCommitID(ctx, tmpBasePath, "original_"+baseBranch)
	if err != nil {
		return "", fmt.Errorf("Failed to get full commit id for origin/%s: %w", pr.BaseBranch, err)
	}
	mergeCommitID, err := git.GetFullCommitID(ctx, tmpBasePath, baseBranch)
	if err != nil {
		return "", fmt.Errorf("Failed to get full commit id for the new merge: %w", err)
	}

	// Now it's questionable about where this should go - either after or before the push
	// I think in the interests of data safety - failures to push to the lfs should prevent
	// the merge as you can always remerge.
	if setting.LFS.StartServer {
		if err := LFSPush(ctx, tmpBasePath, mergeHeadSHA, mergeBaseSHA, pr); err != nil {
			return "", err
		}
	}

	var headUser *user_model.User
	err = pr.HeadRepo.GetOwner(ctx)
	if err != nil {
		if !user_model.IsErrUserNotExist(err) {
			log.Error("Can't find user: %d for head repository - %v", pr.HeadRepo.OwnerID, err)
			return "", err
		}
		log.Error("Can't find user: %d for head repository - defaulting to doer: %s - %v", pr.HeadRepo.OwnerID, doer.Name, err)
		headUser = doer
	} else {
		headUser = pr.HeadRepo.Owner
	}

	var pushCmd *git.Command
	if mergeStyle == repo_model.MergeStyleRebaseUpdate {
		// force push the rebase result to head branch
		env = repo_module.FullPushingEnvironment(
			headUser,
			doer,
			pr.HeadRepo,
			pr.HeadRepo.Name,
			pr.ID,
		)
		pushCmd = git.NewCommand(ctx, "push", "-f", "head_repo").AddDynamicArguments(stagingBranch + ":" + git.BranchPrefix + pr.HeadBranch)
	} else {
		env = repo_module.FullPushingEnvironment(
			headUser,
			doer,
			pr.BaseRepo,
			pr.BaseRepo.Name,
			pr.ID,
		)
		pushCmd = git.NewCommand(ctx, "push", "origin").AddDynamicArguments(baseBranch + ":" + git.BranchPrefix + pr.BaseBranch)
	}

	// Push back to upstream.
	// TODO: this cause an api call to "/api/internal/hook/post-receive/...",
	//       that prevents us from doint the whole merge in one db transaction
	if err := pushCmd.Run(&git.RunOpts{
		Env:    env,
		Dir:    tmpBasePath,
		Stdout: &outbuf,
		Stderr: &errbuf,
	}); err != nil {
		if strings.Contains(errbuf.String(), "non-fast-forward") {
			return "", &git.ErrPushOutOfDate{
				StdOut: outbuf.String(),
				StdErr: errbuf.String(),
				Err:    err,
			}
		} else if strings.Contains(errbuf.String(), "! [remote rejected]") {
			err := &git.ErrPushRejected{
				StdOut: outbuf.String(),
				StdErr: errbuf.String(),
				Err:    err,
			}
			err.GenerateMessage()
			return "", err
		}
		return "", fmt.Errorf("git push: %s", errbuf.String())
	}
	outbuf.Reset()
	errbuf.Reset()

	return mergeCommitID, nil
}

func commitAndSignNoAuthor(ctx context.Context, pr *issues_model.PullRequest, message string, signArgs git.TrustedCmdArgs, tmpBasePath string, env []string) error {
	var outbuf, errbuf strings.Builder
	if err := git.NewCommand(ctx, "commit").AddArguments(signArgs...).AddOptionValues("-m", message).
		Run(&git.RunOpts{
			Env:    env,
			Dir:    tmpBasePath,
			Stdout: &outbuf,
			Stderr: &errbuf,
		}); err != nil {
		log.Error("git commit [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
		return fmt.Errorf("git commit [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
	}
	return nil
}

func runMergeCommand(pr *issues_model.PullRequest, mergeStyle repo_model.MergeStyle, cmd *git.Command, tmpBasePath string) error {
	var outbuf, errbuf strings.Builder
	if err := cmd.Run(&git.RunOpts{
		Dir:    tmpBasePath,
		Stdout: &outbuf,
		Stderr: &errbuf,
	}); err != nil {
		// Merge will leave a MERGE_HEAD file in the .git folder if there is a conflict
		if _, statErr := os.Stat(filepath.Join(tmpBasePath, ".git", "MERGE_HEAD")); statErr == nil {
			// We have a merge conflict error
			log.Debug("MergeConflict [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
			return models.ErrMergeConflicts{
				Style:  mergeStyle,
				StdOut: outbuf.String(),
				StdErr: errbuf.String(),
				Err:    err,
			}
		} else if strings.Contains(errbuf.String(), "refusing to merge unrelated histories") {
			log.Debug("MergeUnrelatedHistories [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
			return models.ErrMergeUnrelatedHistories{
				Style:  mergeStyle,
				StdOut: outbuf.String(),
				StdErr: errbuf.String(),
				Err:    err,
			}
		}
		log.Error("git merge [%s:%s -> %s:%s]: %v\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
		return fmt.Errorf("git merge [%s:%s -> %s:%s]: %w\n%s\n%s", pr.HeadRepo.FullName(), pr.HeadBranch, pr.BaseRepo.FullName(), pr.BaseBranch, err, outbuf.String(), errbuf.String())
	}

	return nil
}

var escapedSymbols = regexp.MustCompile(`([*[?! \\])`)

func getDiffTree(ctx context.Context, repoPath, baseBranch, headBranch string) (string, error) {
	getDiffTreeFromBranch := func(repoPath, baseBranch, headBranch string) (string, error) {
		var outbuf, errbuf strings.Builder
		// Compute the diff-tree for sparse-checkout
		if err := git.NewCommand(ctx, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root").AddDynamicArguments(baseBranch, headBranch).
			Run(&git.RunOpts{
				Dir:    repoPath,
				Stdout: &outbuf,
				Stderr: &errbuf,
			}); err != nil {
			return "", fmt.Errorf("git diff-tree [%s base:%s head:%s]: %s", repoPath, baseBranch, headBranch, errbuf.String())
		}
		return outbuf.String(), nil
	}

	scanNullTerminatedStrings := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexByte(data, '\x00'); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	}

	list, err := getDiffTreeFromBranch(repoPath, baseBranch, headBranch)
	if err != nil {
		return "", err
	}

	// Prefixing '/' for each entry, otherwise all files with the same name in subdirectories would be matched.
	out := bytes.Buffer{}
	scanner := bufio.NewScanner(strings.NewReader(list))
	scanner.Split(scanNullTerminatedStrings)
	for scanner.Scan() {
		filepath := scanner.Text()
		// escape '*', '?', '[', spaces and '!' prefix
		filepath = escapedSymbols.ReplaceAllString(filepath, `\$1`)
		// no necessary to escape the first '#' symbol because the first symbol is '/'
		fmt.Fprintf(&out, "/%s\n", filepath)
	}

	return out.String(), nil
}

// IsUserAllowedToMerge check if user is allowed to merge PR with given permissions and branch protections
func IsUserAllowedToMerge(ctx context.Context, pr *issues_model.PullRequest, p access_model.Permission, user *user_model.User) (bool, error) {
	if user == nil {
		return false, nil
	}

	pb, err := git_model.GetFirstMatchProtectedBranchRule(ctx, pr.BaseRepoID, pr.BaseBranch)
	if err != nil {
		return false, err
	}

	if (p.CanWrite(unit.TypeCode) && pb == nil) || (pb != nil && git_model.IsUserMergeWhitelisted(ctx, pb, user.ID, p)) {
		return true, nil
	}

	return false, nil
}

// CheckPullBranchProtections checks whether the PR is ready to be merged (reviews and status checks)
func CheckPullBranchProtections(ctx context.Context, pr *issues_model.PullRequest, skipProtectedFilesCheck bool) (err error) {
	if err = pr.LoadBaseRepo(ctx); err != nil {
		return fmt.Errorf("LoadBaseRepo: %w", err)
	}

	pb, err := git_model.GetFirstMatchProtectedBranchRule(ctx, pr.BaseRepoID, pr.BaseBranch)
	if err != nil {
		return fmt.Errorf("LoadProtectedBranch: %v", err)
	}
	if pb == nil {
		return nil
	}

	isPass, err := IsPullCommitStatusPass(ctx, pr)
	if err != nil {
		return err
	}
	if !isPass {
		return models.ErrDisallowedToMerge{
			Reason: "Not all required status checks successful",
		}
	}

	if !issues_model.HasEnoughApprovals(ctx, pb, pr) {
		return models.ErrDisallowedToMerge{
			Reason: "Does not have enough approvals",
		}
	}
	if issues_model.MergeBlockedByRejectedReview(ctx, pb, pr) {
		return models.ErrDisallowedToMerge{
			Reason: "There are requested changes",
		}
	}
	if issues_model.MergeBlockedByOfficialReviewRequests(ctx, pb, pr) {
		return models.ErrDisallowedToMerge{
			Reason: "There are official review requests",
		}
	}

	if issues_model.MergeBlockedByOutdatedBranch(pb, pr) {
		return models.ErrDisallowedToMerge{
			Reason: "The head branch is behind the base branch",
		}
	}

	if skipProtectedFilesCheck {
		return nil
	}

	if pb.MergeBlockedByProtectedFiles(pr.ChangedProtectedFiles) {
		return models.ErrDisallowedToMerge{
			Reason: "Changed protected files",
		}
	}

	return nil
}

// MergedManually mark pr as merged manually
func MergedManually(pr *issues_model.PullRequest, doer *user_model.User, baseGitRepo *git.Repository, commitID string) error {
	pullWorkingPool.CheckIn(fmt.Sprint(pr.ID))
	defer pullWorkingPool.CheckOut(fmt.Sprint(pr.ID))

	if err := db.WithTx(db.DefaultContext, func(ctx context.Context) error {
		if err := pr.LoadBaseRepo(ctx); err != nil {
			return err
		}
		prUnit, err := pr.BaseRepo.GetUnit(ctx, unit.TypePullRequests)
		if err != nil {
			return err
		}
		prConfig := prUnit.PullRequestsConfig()

		// Check if merge style is correct and allowed
		if !prConfig.IsMergeStyleAllowed(repo_model.MergeStyleManuallyMerged) {
			return models.ErrInvalidMergeStyle{ID: pr.BaseRepo.ID, Style: repo_model.MergeStyleManuallyMerged}
		}

		if len(commitID) < git.SHAFullLength {
			return fmt.Errorf("Wrong commit ID")
		}

		commit, err := baseGitRepo.GetCommit(commitID)
		if err != nil {
			if git.IsErrNotExist(err) {
				return fmt.Errorf("Wrong commit ID")
			}
			return err
		}
		commitID = commit.ID.String()

		ok, err := baseGitRepo.IsCommitInBranch(commitID, pr.BaseBranch)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("Wrong commit ID")
		}

		pr.MergedCommitID = commitID
		pr.MergedUnix = timeutil.TimeStamp(commit.Author.When.Unix())
		pr.Status = issues_model.PullRequestStatusManuallyMerged
		pr.Merger = doer
		pr.MergerID = doer.ID

		var merged bool
		if merged, err = pr.SetMerged(ctx); err != nil {
			return err
		} else if !merged {
			return fmt.Errorf("SetMerged failed")
		}
		return nil
	}); err != nil {
		return err
	}

	notification.NotifyMergePullRequest(baseGitRepo.Ctx, doer, pr)
	log.Info("manuallyMerged[%d]: Marked as manually merged into %s/%s by commit id: %s", pr.ID, pr.BaseRepo.Name, pr.BaseBranch, commitID)
	return nil
}
