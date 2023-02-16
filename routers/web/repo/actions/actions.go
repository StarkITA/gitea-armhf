// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package actions

import (
	"net/http"

	actions_model "code.gitea.io/gitea/models/actions"
	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unit"
	"code.gitea.io/gitea/modules/actions"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/convert"
)

const (
	tplListActions base.TplName = "repo/actions/list"
	tplViewActions base.TplName = "repo/actions/view"
)

// MustEnableActions check if actions are enabled in settings
func MustEnableActions(ctx *context.Context) {
	if !setting.Actions.Enabled {
		ctx.NotFound("MustEnableActions", nil)
		return
	}

	if unit.TypeActions.UnitGlobalDisabled() {
		ctx.NotFound("MustEnableActions", nil)
		return
	}

	if ctx.Repo.Repository != nil {
		if !ctx.Repo.CanRead(unit.TypeActions) {
			ctx.NotFound("MustEnableActions", nil)
			return
		}
	}
}

func List(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("actions.actions")
	ctx.Data["PageIsActions"] = true

	var workflows git.Entries
	if empty, err := ctx.Repo.GitRepo.IsEmpty(); err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	} else if !empty {
		defaultBranch, err := ctx.Repo.GitRepo.GetDefaultBranch()
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		commit, err := ctx.Repo.GitRepo.GetBranchCommit(defaultBranch)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
		workflows, err = actions.ListWorkflows(commit)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, err.Error())
			return
		}
	}

	ctx.Data["workflows"] = workflows
	ctx.Data["RepoLink"] = ctx.Repo.Repository.Link()

	page := ctx.FormInt("page")
	if page <= 0 {
		page = 1
	}

	workflow := ctx.FormString("workflow")
	ctx.Data["CurWorkflow"] = workflow

	opts := actions_model.FindRunOptions{
		ListOptions: db.ListOptions{
			Page:     page,
			PageSize: convert.ToCorrectPageSize(ctx.FormInt("limit")),
		},
		RepoID:           ctx.Repo.Repository.ID,
		WorkflowFileName: workflow,
	}

	// open counts
	opts.IsClosed = util.OptionalBoolFalse
	numOpenRuns, err := actions_model.CountRuns(ctx, opts)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Data["NumOpenActionRuns"] = numOpenRuns

	// closed counts
	opts.IsClosed = util.OptionalBoolTrue
	numClosedRuns, err := actions_model.CountRuns(ctx, opts)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Data["NumClosedActionRuns"] = numClosedRuns

	opts.IsClosed = util.OptionalBoolNone
	if ctx.FormString("state") == "closed" {
		opts.IsClosed = util.OptionalBoolTrue
		ctx.Data["IsShowClosed"] = true
	} else {
		opts.IsClosed = util.OptionalBoolFalse
	}
	runs, total, err := actions_model.FindRuns(ctx, opts)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	for _, run := range runs {
		run.Repo = ctx.Repo.Repository
	}

	if err := runs.LoadTriggerUser(ctx); err != nil {
		ctx.Error(http.StatusInternalServerError, err.Error())
		return
	}

	ctx.Data["Runs"] = runs

	pager := context.NewPagination(int(total), opts.PageSize, opts.Page, 5)
	pager.SetDefaultParams(ctx)
	ctx.Data["Page"] = pager

	ctx.HTML(http.StatusOK, tplListActions)
}
