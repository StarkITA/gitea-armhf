// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package main

/*
Checkout a PR and load the tests data into sqlite database
*/

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"code.gitea.io/gitea/models/db"
	"code.gitea.io/gitea/models/unittest"
	gitea_git "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/graceful"
	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/markup/external"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/routers"
	markup_service "code.gitea.io/gitea/services/markup"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"xorm.io/xorm"
)

var codeFilePath = "contrib/pr/checkout.go"

func runPR() {
	log.Printf("[PR] Starting gitea ...\n")
	curDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	setting.SetCustomPathAndConf("", "", "")
	setting.LoadAllowEmpty()

	setting.RepoRootPath, err = os.MkdirTemp(os.TempDir(), "repos")
	if err != nil {
		log.Fatalf("TempDir: %v\n", err)
	}
	setting.AppDataPath, err = os.MkdirTemp(os.TempDir(), "appdata")
	if err != nil {
		log.Fatalf("TempDir: %v\n", err)
	}
	setting.AppWorkPath = curDir
	setting.StaticRootPath = curDir
	setting.GravatarSource = "https://secure.gravatar.com/avatar/"
	setting.AppURL = "http://localhost:8080/"
	setting.HTTPPort = "8080"
	setting.SSH.Domain = "localhost"
	setting.SSH.Port = 3000
	setting.InstallLock = true
	setting.SecretKey = "9pCviYTWSb"
	setting.InternalToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYmYiOjE0OTI3OTU5ODN9.OQkH5UmzID2XBdwQ9TAI6Jj2t1X-wElVTjbE7aoN4I8"
	curUser, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	setting.RunUser = curUser.Username

	log.Printf("[PR] Loading fixtures data ...\n")
	//models.LoadConfigs()
	/*
		setting.Database.Type = "sqlite3"
		setting.Database.Path = ":memory:"
		setting.Database.Timeout = 500
	*/
	dbCfg := setting.Cfg.Section("database")
	dbCfg.NewKey("DB_TYPE", "sqlite3")
	dbCfg.NewKey("PATH", ":memory:")

	routers.InitGitServices()
	setting.Database.LogSQL = true
	// x, err = xorm.NewEngine("sqlite3", "file::memory:?cache=shared")

	db.InitEngineWithMigration(context.Background(), func(_ *xorm.Engine) error {
		return nil
	})
	db.HasEngine = true
	// x.ShowSQL(true)
	err = unittest.InitFixtures(
		unittest.FixturesOptions{
			Dir: path.Join(curDir, "models/fixtures/"),
		},
	)
	if err != nil {
		fmt.Printf("Error initializing test database: %v\n", err)
		os.Exit(1)
	}
	unittest.LoadFixtures()
	util.RemoveAll(setting.RepoRootPath)
	util.RemoveAll(repo_module.LocalCopyPath())
	unittest.CopyDir(path.Join(curDir, "tests/gitea-repositories-meta"), setting.RepoRootPath)

	log.Printf("[PR] Setting up router\n")
	// routers.GlobalInit()
	external.RegisterRenderers()
	markup.Init(markup_service.ProcessorHelper())
	c := routers.NormalRoutes(graceful.GetManager().HammerContext())

	log.Printf("[PR] Ready for testing !\n")
	log.Printf("[PR] Login with user1, user2, user3, ... with pass: password\n")
	/*
		log.Info("Listen: %v://%s%s", setting.Protocol, listenAddr, setting.AppSubURL)

		if setting.LFS.StartServer {
			log.Info("LFS server enabled")
		}

		if setting.EnablePprof {
			go func() {
				log.Info("Starting pprof server on localhost:6060")
				log.Info("%v", http.ListenAndServe("localhost:6060", nil))
			}()
		}
	*/

	// Start the server
	http.ListenAndServe(":8080", c)

	log.Printf("[PR] Cleaning up ...\n")
	/*
		if err = util.RemoveAll(setting.Indexer.IssuePath); err != nil {
			fmt.Printf("util.RemoveAll: %v\n", err)
			os.Exit(1)
		}
		if err = util.RemoveAll(setting.Indexer.RepoPath); err != nil {
			fmt.Printf("Unable to remove repo indexer: %v\n", err)
			os.Exit(1)
		}
	*/
	if err = util.RemoveAll(setting.RepoRootPath); err != nil {
		log.Fatalf("util.RemoveAll: %v\n", err)
	}
	if err = util.RemoveAll(setting.AppDataPath); err != nil {
		log.Fatalf("util.RemoveAll: %v\n", err)
	}
}

func main() {
	runPRFlag := flag.Bool("run", false, "Run the PR code")
	flag.Parse()
	if *runPRFlag {
		runPR()
		return
	}

	// To force checkout (e.g. Windows complains about unclean work tree) set env variable FORCE=true
	force, err := strconv.ParseBool(os.Getenv("FORCE"))
	if err != nil {
		force = false
	}

	// Otherwise checkout PR
	if len(os.Args) != 2 {
		log.Fatal("Need only one arg: the PR number")
	}
	pr := os.Args[1]

	codeFilePath = filepath.FromSlash(codeFilePath) // Convert to running OS

	// Copy this file if it will not exist in the PR branch
	dat, err := os.ReadFile(codeFilePath)
	if err != nil {
		log.Fatalf("Failed to cache this code file : %v", err)
	}

	repo, err := git.PlainOpen(".")
	if err != nil {
		log.Fatalf("Failed to open the repo : %v", err)
	}

	// Find remote upstream
	remotes, err := repo.Remotes()
	if err != nil {
		log.Fatalf("Failed to list remotes of repo : %v", err)
	}
	remoteUpstream := "origin" // Default
	for _, r := range remotes {
		if r.Config().URLs[0] == "https://github.com/go-gitea/gitea.git" ||
			r.Config().URLs[0] == "https://github.com/go-gitea/gitea" ||
			r.Config().URLs[0] == "git@github.com:go-gitea/gitea.git" { // fetch at index 0
			remoteUpstream = r.Config().Name
			break
		}
	}

	branch := fmt.Sprintf("pr-%s-%d", pr, time.Now().Unix())
	branchRef := plumbing.NewBranchReferenceName(branch)

	log.Printf("Fetching PR #%s in %s\n", pr, branch)
	if runtime.GOOS == "windows" {
		// Use git cli command for windows
		runCmd("git", "fetch", remoteUpstream, fmt.Sprintf("pull/%s/head:%s", pr, branch))
	} else {
		ref := fmt.Sprintf("%s%s/head:%s", gitea_git.PullPrefix, pr, branchRef)
		err = repo.Fetch(&git.FetchOptions{
			RemoteName: remoteUpstream,
			RefSpecs: []config.RefSpec{
				config.RefSpec(ref),
			},
		})
		if err != nil {
			log.Fatalf("Failed to fetch %s from %s : %v", ref, remoteUpstream, err)
		}
	}

	tree, err := repo.Worktree()
	if err != nil {
		log.Fatalf("Failed to parse git tree : %v", err)
	}
	log.Printf("Checkout PR #%s in %s\n", pr, branch)
	err = tree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Force:  force,
	})
	if err != nil {
		log.Fatalf("Failed to checkout %s : %v", branch, err)
	}

	// Copy this file if not exist
	if _, err := os.Stat(codeFilePath); os.IsNotExist(err) {
		err = os.MkdirAll(filepath.Dir(codeFilePath), 0o755)
		if err != nil {
			log.Fatalf("Failed to duplicate this code file in PR : %v", err)
		}
		err = os.WriteFile(codeFilePath, dat, 0o644)
		if err != nil {
			log.Fatalf("Failed to duplicate this code file in PR : %v", err)
		}
	}
	// Force build of js, css, bin, ...
	runCmd("make", "build")
	// Start with integration test
	runCmd("go", "run", "-mod", "vendor", "-tags", "sqlite sqlite_unlock_notify", codeFilePath, "-run")
}

func runCmd(cmd ...string) {
	log.Printf("Executing : %s ...\n", cmd)
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		log.Panicln(err)
	}
	if err := c.Wait(); err != nil {
		log.Panicln(err)
	}
}
