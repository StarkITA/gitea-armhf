// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build ignore

package main

import (
	"encoding/json"
	"io/fs"
	"os"
	goPath "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// regexp is based on go-license, excluding README and NOTICE
// https://github.com/google/go-licenses/blob/master/licenses/find.go
var licenseRe = regexp.MustCompile(`^(?i)((UN)?LICEN(S|C)E|COPYING).*$`)

type LicenseEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	LicenseText string `json:"licenseText"`
}

func main() {
	base, out := os.Args[1], os.Args[2]

	paths := []string{}
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !licenseRe.MatchString(entry.Name()) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		panic(err)
	}

	sort.Strings(paths)

	entries := []LicenseEntry{}
	for _, path := range paths {
		path := filepath.ToSlash(path)

		licenseText, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}

		path = strings.Replace(path, base+"/", "", 1)
		name := goPath.Dir(path)

		// There might be a bug somewhere in go-licenses that sometimes interprets the
		// root package as "." and sometimes as "code.gitea.io/gitea". Workaround by
		// removing both of them for the sake of stable output.
		if name == "." || name == "code.gitea.io/gitea" {
			continue
		}

		entries = append(entries, LicenseEntry{
			Name:        name,
			Path:        path,
			LicenseText: string(licenseText),
		})
	}

	jsonBytes, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(out, jsonBytes, 0o644)
	if err != nil {
		panic(err)
	}
}
