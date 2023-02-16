// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package private

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/setting"
)

// Git environment variables
const (
	GitAlternativeObjectDirectories = "GIT_ALTERNATE_OBJECT_DIRECTORIES"
	GitObjectDirectory              = "GIT_OBJECT_DIRECTORY"
	GitQuarantinePath               = "GIT_QUARANTINE_PATH"
	GitPushOptionCount              = "GIT_PUSH_OPTION_COUNT"
)

// GitPushOptions is a wrapper around a map[string]string
type GitPushOptions map[string]string

// GitPushOptions keys
const (
	GitPushOptionRepoPrivate  = "repo.private"
	GitPushOptionRepoTemplate = "repo.template"
)

// Bool checks for a key in the map and parses as a boolean
func (g GitPushOptions) Bool(key string, def bool) bool {
	if val, ok := g[key]; ok {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return def
}

// HookOptions represents the options for the Hook calls
type HookOptions struct {
	OldCommitIDs                    []string
	NewCommitIDs                    []string
	RefFullNames                    []string
	UserID                          int64
	UserName                        string
	GitObjectDirectory              string
	GitAlternativeObjectDirectories string
	GitQuarantinePath               string
	GitPushOptions                  GitPushOptions
	PullRequestID                   int64
	DeployKeyID                     int64 // if the pusher is a DeployKey, then UserID is the repo's org user.
	IsWiki                          bool
	ActionPerm                      int
}

// SSHLogOption ssh log options
type SSHLogOption struct {
	IsError bool
	Message string
}

// HookPostReceiveResult represents an individual result from PostReceive
type HookPostReceiveResult struct {
	Results      []HookPostReceiveBranchResult
	RepoWasEmpty bool
	Err          string
}

// HookPostReceiveBranchResult represents an individual branch result from PostReceive
type HookPostReceiveBranchResult struct {
	Message bool
	Create  bool
	Branch  string
	URL     string
}

// HookProcReceiveResult represents an individual result from ProcReceive
type HookProcReceiveResult struct {
	Results []HookProcReceiveRefResult
	Err     string
}

// HookProcReceiveRefResult represents an individual result from ProcReceive
type HookProcReceiveRefResult struct {
	OldOID       string
	NewOID       string
	Ref          string
	OriginalRef  string
	IsForcePush  bool
	IsNotMatched bool
	Err          string
}

// HookPreReceive check whether the provided commits are allowed
func HookPreReceive(ctx context.Context, ownerName, repoName string, opts HookOptions) (int, string) {
	reqURL := setting.LocalURL + fmt.Sprintf("api/internal/hook/pre-receive/%s/%s",
		url.PathEscape(ownerName),
		url.PathEscape(repoName),
	)
	req := newInternalRequest(ctx, reqURL, "POST")
	req = req.Header("Content-Type", "application/json")
	jsonBytes, _ := json.Marshal(opts)
	req.Body(jsonBytes)
	req.SetTimeout(60*time.Second, time.Duration(60+len(opts.OldCommitIDs))*time.Second)
	resp, err := req.Response()
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Unable to contact gitea: %v", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, decodeJSONError(resp).Err
	}

	return http.StatusOK, ""
}

// HookPostReceive updates services and users
func HookPostReceive(ctx context.Context, ownerName, repoName string, opts HookOptions) (*HookPostReceiveResult, string) {
	reqURL := setting.LocalURL + fmt.Sprintf("api/internal/hook/post-receive/%s/%s",
		url.PathEscape(ownerName),
		url.PathEscape(repoName),
	)

	req := newInternalRequest(ctx, reqURL, "POST")
	req = req.Header("Content-Type", "application/json")
	req.SetTimeout(60*time.Second, time.Duration(60+len(opts.OldCommitIDs))*time.Second)
	jsonBytes, _ := json.Marshal(opts)
	req.Body(jsonBytes)
	resp, err := req.Response()
	if err != nil {
		return nil, fmt.Sprintf("Unable to contact gitea: %v", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeJSONError(resp).Err
	}
	res := &HookPostReceiveResult{}
	_ = json.NewDecoder(resp.Body).Decode(res)

	return res, ""
}

// HookProcReceive proc-receive hook
func HookProcReceive(ctx context.Context, ownerName, repoName string, opts HookOptions) (*HookProcReceiveResult, error) {
	reqURL := setting.LocalURL + fmt.Sprintf("api/internal/hook/proc-receive/%s/%s",
		url.PathEscape(ownerName),
		url.PathEscape(repoName),
	)

	req := newInternalRequest(ctx, reqURL, "POST")
	req = req.Header("Content-Type", "application/json")
	req.SetTimeout(60*time.Second, time.Duration(60+len(opts.OldCommitIDs))*time.Second)
	jsonBytes, _ := json.Marshal(opts)
	req.Body(jsonBytes)
	resp, err := req.Response()
	if err != nil {
		return nil, fmt.Errorf("Unable to contact gitea: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(decodeJSONError(resp).Err)
	}
	res := &HookProcReceiveResult{}
	_ = json.NewDecoder(resp.Body).Decode(res)

	return res, nil
}

// SetDefaultBranch will set the default branch to the provided branch for the provided repository
func SetDefaultBranch(ctx context.Context, ownerName, repoName, branch string) error {
	reqURL := setting.LocalURL + fmt.Sprintf("api/internal/hook/set-default-branch/%s/%s/%s",
		url.PathEscape(ownerName),
		url.PathEscape(repoName),
		url.PathEscape(branch),
	)
	req := newInternalRequest(ctx, reqURL, "POST")
	req = req.Header("Content-Type", "application/json")

	req.SetTimeout(60*time.Second, 60*time.Second)
	resp, err := req.Response()
	if err != nil {
		return fmt.Errorf("Unable to contact gitea: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Error returned from gitea: %v", decodeJSONError(resp).Err)
	}
	return nil
}

// SSHLog sends ssh error log response
func SSHLog(ctx context.Context, isErr bool, msg string) error {
	reqURL := setting.LocalURL + "api/internal/ssh/log"
	req := newInternalRequest(ctx, reqURL, "POST")
	req = req.Header("Content-Type", "application/json")

	jsonBytes, _ := json.Marshal(&SSHLogOption{
		IsError: isErr,
		Message: msg,
	})
	req.Body(jsonBytes)

	req.SetTimeout(60*time.Second, 60*time.Second)
	resp, err := req.Response()
	if err != nil {
		return fmt.Errorf("unable to contact gitea: %w", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Error returned from gitea: %v", decodeJSONError(resp).Err)
	}
	return nil
}
