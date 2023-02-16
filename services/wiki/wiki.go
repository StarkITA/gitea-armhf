// Copyright 2015 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	repo_model "code.gitea.io/gitea/models/repo"
	system_model "code.gitea.io/gitea/models/system"
	"code.gitea.io/gitea/models/unit"
	user_model "code.gitea.io/gitea/models/user"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/sync"
	"code.gitea.io/gitea/modules/util"
	asymkey_service "code.gitea.io/gitea/services/asymkey"
)

var (
	reservedWikiNames = []string{"_pages", "_new", "_edit", "raw"}
	// TODO: use clustered lock (unique queue? or *abuse* cache)
	wikiWorkingPool = sync.NewExclusivePool()
)

const (
	DefaultRemote = "origin"
	DefaultBranch = "master"
)

func nameAllowed(name string) error {
	if util.SliceContainsString(reservedWikiNames, name) {
		return repo_model.ErrWikiReservedName{
			Title: name,
		}
	}
	return nil
}

// NameToSubURL converts a wiki name to its corresponding sub-URL.
func NameToSubURL(name string) string {
	return url.PathEscape(strings.ReplaceAll(name, " ", "-"))
}

// NormalizeWikiName normalizes a wiki name
func NormalizeWikiName(name string) string {
	return strings.ReplaceAll(name, "-", " ")
}

// NameToFilename converts a wiki name to its corresponding filename.
func NameToFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "-")
	return url.QueryEscape(name) + ".md"
}

// FilenameToName converts a wiki filename to its corresponding page name.
func FilenameToName(filename string) (string, error) {
	if !strings.HasSuffix(filename, ".md") {
		return "", repo_model.ErrWikiInvalidFileName{
			FileName: filename,
		}
	}
	basename := filename[:len(filename)-3]
	unescaped, err := url.QueryUnescape(basename)
	if err != nil {
		return "", err
	}
	return NormalizeWikiName(unescaped), nil
}

// InitWiki initializes a wiki for repository,
// it does nothing when repository already has wiki.
func InitWiki(ctx context.Context, repo *repo_model.Repository) error {
	if repo.HasWiki() {
		return nil
	}

	if err := git.InitRepository(ctx, repo.WikiPath(), true); err != nil {
		return fmt.Errorf("InitRepository: %w", err)
	} else if err = repo_module.CreateDelegateHooks(repo.WikiPath()); err != nil {
		return fmt.Errorf("createDelegateHooks: %w", err)
	} else if _, _, err = git.NewCommand(ctx, "symbolic-ref", "HEAD", git.BranchPrefix+DefaultBranch).RunStdString(&git.RunOpts{Dir: repo.WikiPath()}); err != nil {
		return fmt.Errorf("unable to set default wiki branch to master: %w", err)
	}
	return nil
}

// prepareWikiFileName try to find a suitable file path with file name by the given raw wiki name.
// return: existence, prepared file path with name, error
func prepareWikiFileName(gitRepo *git.Repository, wikiName string) (bool, string, error) {
	unescaped := wikiName + ".md"
	escaped := NameToFilename(wikiName)

	// Look for both files
	filesInIndex, err := gitRepo.LsTree(DefaultBranch, unescaped, escaped)
	if err != nil {
		if strings.Contains(err.Error(), "Not a valid object name master") {
			return false, escaped, nil
		}
		log.Error("%v", err)
		return false, escaped, err
	}

	foundEscaped := false
	for _, filename := range filesInIndex {
		switch filename {
		case unescaped:
			// if we find the unescaped file return it
			return true, unescaped, nil
		case escaped:
			foundEscaped = true
		}
	}

	// If not return whether the escaped file exists, and the escaped filename to keep backwards compatibility.
	return foundEscaped, escaped, nil
}

// updateWikiPage adds a new page or edits an existing page in repository wiki.
func updateWikiPage(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, oldWikiName, newWikiName, content, message string, isNew bool) (err error) {
	if err = nameAllowed(newWikiName); err != nil {
		return err
	}
	wikiWorkingPool.CheckIn(fmt.Sprint(repo.ID))
	defer wikiWorkingPool.CheckOut(fmt.Sprint(repo.ID))

	if err = InitWiki(ctx, repo); err != nil {
		return fmt.Errorf("InitWiki: %w", err)
	}

	hasMasterBranch := git.IsBranchExist(ctx, repo.WikiPath(), DefaultBranch)

	basePath, err := repo_module.CreateTemporaryPath("update-wiki")
	if err != nil {
		return err
	}
	defer func() {
		if err := repo_module.RemoveTemporaryPath(basePath); err != nil {
			log.Error("Merge: RemoveTemporaryPath: %s", err)
		}
	}()

	cloneOpts := git.CloneRepoOptions{
		Bare:   true,
		Shared: true,
	}

	if hasMasterBranch {
		cloneOpts.Branch = DefaultBranch
	}

	if err := git.Clone(ctx, repo.WikiPath(), basePath, cloneOpts); err != nil {
		log.Error("Failed to clone repository: %s (%v)", repo.FullName(), err)
		return fmt.Errorf("Failed to clone repository: %s (%w)", repo.FullName(), err)
	}

	gitRepo, err := git.OpenRepository(ctx, basePath)
	if err != nil {
		log.Error("Unable to open temporary repository: %s (%v)", basePath, err)
		return fmt.Errorf("Failed to open new temporary repository in: %s %w", basePath, err)
	}
	defer gitRepo.Close()

	if hasMasterBranch {
		if err := gitRepo.ReadTreeToIndex("HEAD"); err != nil {
			log.Error("Unable to read HEAD tree to index in: %s %v", basePath, err)
			return fmt.Errorf("Unable to read HEAD tree to index in: %s %w", basePath, err)
		}
	}

	isWikiExist, newWikiPath, err := prepareWikiFileName(gitRepo, newWikiName)
	if err != nil {
		return err
	}

	if isNew {
		if isWikiExist {
			return repo_model.ErrWikiAlreadyExist{
				Title: newWikiPath,
			}
		}
	} else {
		// avoid check existence again if wiki name is not changed since gitRepo.LsFiles(...) is not free.
		isOldWikiExist := true
		oldWikiPath := newWikiPath
		if oldWikiName != newWikiName {
			isOldWikiExist, oldWikiPath, err = prepareWikiFileName(gitRepo, oldWikiName)
			if err != nil {
				return err
			}
		}

		if isOldWikiExist {
			err := gitRepo.RemoveFilesFromIndex(oldWikiPath)
			if err != nil {
				log.Error("%v", err)
				return err
			}
		}
	}

	// FIXME: The wiki doesn't have lfs support at present - if this changes need to check attributes here

	objectHash, err := gitRepo.HashObject(strings.NewReader(content))
	if err != nil {
		log.Error("%v", err)
		return err
	}

	if err := gitRepo.AddObjectToIndex("100644", objectHash, newWikiPath); err != nil {
		log.Error("%v", err)
		return err
	}

	tree, err := gitRepo.WriteTree()
	if err != nil {
		log.Error("%v", err)
		return err
	}

	commitTreeOpts := git.CommitTreeOpts{
		Message: message,
	}

	committer := doer.NewGitSig()

	sign, signingKey, signer, _ := asymkey_service.SignWikiCommit(ctx, repo.WikiPath(), doer)
	if sign {
		commitTreeOpts.KeyID = signingKey
		if repo.GetTrustModel() == repo_model.CommitterTrustModel || repo.GetTrustModel() == repo_model.CollaboratorCommitterTrustModel {
			committer = signer
		}
	} else {
		commitTreeOpts.NoGPGSign = true
	}
	if hasMasterBranch {
		commitTreeOpts.Parents = []string{"HEAD"}
	}

	commitHash, err := gitRepo.CommitTree(doer.NewGitSig(), committer, tree, commitTreeOpts)
	if err != nil {
		log.Error("%v", err)
		return err
	}

	if err := git.Push(gitRepo.Ctx, basePath, git.PushOptions{
		Remote: DefaultRemote,
		Branch: fmt.Sprintf("%s:%s%s", commitHash.String(), git.BranchPrefix, DefaultBranch),
		Env: repo_module.FullPushingEnvironment(
			doer,
			doer,
			repo,
			repo.Name+".wiki",
			0,
		),
	}); err != nil {
		log.Error("%v", err)
		if git.IsErrPushOutOfDate(err) || git.IsErrPushRejected(err) {
			return err
		}
		return fmt.Errorf("Push: %w", err)
	}

	return nil
}

// AddWikiPage adds a new wiki page with a given wikiPath.
func AddWikiPage(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, wikiName, content, message string) error {
	return updateWikiPage(ctx, doer, repo, "", wikiName, content, message, true)
}

// EditWikiPage updates a wiki page identified by its wikiPath,
// optionally also changing wikiPath.
func EditWikiPage(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, oldWikiName, newWikiName, content, message string) error {
	return updateWikiPage(ctx, doer, repo, oldWikiName, newWikiName, content, message, false)
}

// DeleteWikiPage deletes a wiki page identified by its path.
func DeleteWikiPage(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, wikiName string) (err error) {
	wikiWorkingPool.CheckIn(fmt.Sprint(repo.ID))
	defer wikiWorkingPool.CheckOut(fmt.Sprint(repo.ID))

	if err = InitWiki(ctx, repo); err != nil {
		return fmt.Errorf("InitWiki: %w", err)
	}

	basePath, err := repo_module.CreateTemporaryPath("update-wiki")
	if err != nil {
		return err
	}
	defer func() {
		if err := repo_module.RemoveTemporaryPath(basePath); err != nil {
			log.Error("Merge: RemoveTemporaryPath: %s", err)
		}
	}()

	if err := git.Clone(ctx, repo.WikiPath(), basePath, git.CloneRepoOptions{
		Bare:   true,
		Shared: true,
		Branch: DefaultBranch,
	}); err != nil {
		log.Error("Failed to clone repository: %s (%v)", repo.FullName(), err)
		return fmt.Errorf("Failed to clone repository: %s (%w)", repo.FullName(), err)
	}

	gitRepo, err := git.OpenRepository(ctx, basePath)
	if err != nil {
		log.Error("Unable to open temporary repository: %s (%v)", basePath, err)
		return fmt.Errorf("Failed to open new temporary repository in: %s %w", basePath, err)
	}
	defer gitRepo.Close()

	if err := gitRepo.ReadTreeToIndex("HEAD"); err != nil {
		log.Error("Unable to read HEAD tree to index in: %s %v", basePath, err)
		return fmt.Errorf("Unable to read HEAD tree to index in: %s %w", basePath, err)
	}

	found, wikiPath, err := prepareWikiFileName(gitRepo, wikiName)
	if err != nil {
		return err
	}
	if found {
		err := gitRepo.RemoveFilesFromIndex(wikiPath)
		if err != nil {
			return err
		}
	} else {
		return os.ErrNotExist
	}

	// FIXME: The wiki doesn't have lfs support at present - if this changes need to check attributes here

	tree, err := gitRepo.WriteTree()
	if err != nil {
		return err
	}
	message := "Delete page '" + wikiName + "'"
	commitTreeOpts := git.CommitTreeOpts{
		Message: message,
		Parents: []string{"HEAD"},
	}

	committer := doer.NewGitSig()

	sign, signingKey, signer, _ := asymkey_service.SignWikiCommit(ctx, repo.WikiPath(), doer)
	if sign {
		commitTreeOpts.KeyID = signingKey
		if repo.GetTrustModel() == repo_model.CommitterTrustModel || repo.GetTrustModel() == repo_model.CollaboratorCommitterTrustModel {
			committer = signer
		}
	} else {
		commitTreeOpts.NoGPGSign = true
	}

	commitHash, err := gitRepo.CommitTree(doer.NewGitSig(), committer, tree, commitTreeOpts)
	if err != nil {
		return err
	}

	if err := git.Push(gitRepo.Ctx, basePath, git.PushOptions{
		Remote: DefaultRemote,
		Branch: fmt.Sprintf("%s:%s%s", commitHash.String(), git.BranchPrefix, DefaultBranch),
		Env:    repo_module.PushingEnvironment(doer, repo),
	}); err != nil {
		if git.IsErrPushOutOfDate(err) || git.IsErrPushRejected(err) {
			return err
		}
		return fmt.Errorf("Push: %w", err)
	}

	return nil
}

// DeleteWiki removes the actual and local copy of repository wiki.
func DeleteWiki(ctx context.Context, repo *repo_model.Repository) error {
	if err := repo_model.UpdateRepositoryUnits(repo, nil, []unit.Type{unit.TypeWiki}); err != nil {
		return err
	}

	system_model.RemoveAllWithNotice(ctx, "Delete repository wiki", repo.WikiPath())
	return nil
}
