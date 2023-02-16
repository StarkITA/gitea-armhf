// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package private includes all internal routes. The package name internal is ideal but Golang is not allowed, so we use private as package name instead.
package private

import (
	"fmt"
	"net/http"

	repo_model "code.gitea.io/gitea/models/repo"
	gitea_context "code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/private"
)

// ________          _____             .__   __
// \______ \   _____/ ____\____   __ __|  |_/  |_
//  |    |  \_/ __ \   __\\__  \ |  |  \  |\   __\
//  |    `   \  ___/|  |   / __ \|  |  /  |_|  |
// /_______  /\___  >__|  (____  /____/|____/__|
//         \/     \/           \/
// __________                             .__
// \______   \____________    ____   ____ |  |__
//  |    |  _/\_  __ \__  \  /    \_/ ___\|  |  \
//  |    |   \ |  | \// __ \|   |  \  \___|   Y  \
//  |______  / |__|  (____  /___|  /\___  >___|  /
//         \/             \/     \/     \/     \/

// SetDefaultBranch updates the default branch
func SetDefaultBranch(ctx *gitea_context.PrivateContext) {
	ownerName := ctx.Params(":owner")
	repoName := ctx.Params(":repo")
	branch := ctx.Params(":branch")

	ctx.Repo.Repository.DefaultBranch = branch
	if err := ctx.Repo.GitRepo.SetDefaultBranch(ctx.Repo.Repository.DefaultBranch); err != nil {
		if !git.IsErrUnsupportedVersion(err) {
			ctx.JSON(http.StatusInternalServerError, private.Response{
				Err: fmt.Sprintf("Unable to set default branch on repository: %s/%s Error: %v", ownerName, repoName, err),
			})
			return
		}
	}

	if err := repo_model.UpdateDefaultBranch(ctx.Repo.Repository); err != nil {
		ctx.JSON(http.StatusInternalServerError, private.Response{
			Err: fmt.Sprintf("Unable to set default branch on repository: %s/%s Error: %v", ownerName, repoName, err),
		})
		return
	}
	ctx.PlainText(http.StatusOK, "success")
}
