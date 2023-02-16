// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package install

import (
	"context"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/svg"
	"code.gitea.io/gitea/modules/translation"
	"code.gitea.io/gitea/routers/common"
)

// PreloadSettings preloads the configuration to check if we need to run install
func PreloadSettings(ctx context.Context) bool {
	setting.LoadAllowEmpty()
	if !setting.InstallLock {
		log.Info("AppPath: %s", setting.AppPath)
		log.Info("AppWorkPath: %s", setting.AppWorkPath)
		log.Info("Custom path: %s", setting.CustomPath)
		log.Info("Log path: %s", setting.LogRootPath)
		log.Info("Configuration file: %s", setting.CustomConf)
		log.Info("Prepare to run install page")
		translation.InitLocales(ctx)
		if setting.EnableSQLite3 {
			log.Info("SQLite3 is supported")
		}
		setting.InitDBConfig()
		setting.NewServicesForInstall()
		svg.Init()
	}

	return !setting.InstallLock
}

// reloadSettings reloads the existing settings and starts up the database
func reloadSettings(ctx context.Context) {
	setting.LoadFromExisting()
	setting.InitDBConfig()
	if setting.InstallLock {
		if err := common.InitDBEngine(ctx); err == nil {
			log.Info("ORM engine initialization successful!")
		} else {
			log.Fatal("ORM engine initialization failed: %v", err)
		}
		svg.Init()
	}
}
