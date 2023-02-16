// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build bindata

package options

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/util"
)

var directories = make(directorySet)

// Dir returns all files from bindata or custom directory.
func Dir(name string) ([]string, error) {
	if directories.Filled(name) {
		return directories.Get(name), nil
	}

	var result []string

	customDir := path.Join(setting.CustomPath, "options", name)
	isDir, err := util.IsDir(customDir)
	if err != nil {
		return []string{}, fmt.Errorf("unable to check if custom directory %q is a directory. %w", customDir, err)
	}
	if isDir {
		files, err := util.StatDir(customDir, true)
		if err != nil {
			return []string{}, fmt.Errorf("unable to read custom directory %q. %w", customDir, err)
		}

		result = append(result, files...)
	}

	files, err := AssetDir(name)
	if err != nil {
		return []string{}, fmt.Errorf("unable to read embedded directory %q. %w", name, err)
	}

	result = append(result, files...)
	return directories.AddAndGet(name, result), nil
}

func AssetDir(dirName string) ([]string, error) {
	d, err := Assets.Open(dirName)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	files, err := d.Readdir(-1)
	if err != nil {
		return nil, err
	}
	results := make([]string, 0, len(files))
	for _, file := range files {
		results = append(results, file.Name())
	}
	return results, nil
}

// Locale reads the content of a specific locale from bindata or custom path.
func Locale(name string) ([]byte, error) {
	return fileFromDir(path.Join("locale", name))
}

// WalkLocales reads the content of a specific locale from static or custom path.
func WalkLocales(callback func(path, name string, d fs.DirEntry, err error) error) error {
	if err := walkAssetDir(filepath.Join(setting.CustomPath, "options", "locale"), callback); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to walk locales. Error: %w", err)
	}
	return nil
}

// Readme reads the content of a specific readme from bindata or custom path.
func Readme(name string) ([]byte, error) {
	return fileFromDir(path.Join("readme", name))
}

// Gitignore reads the content of a gitignore locale from bindata or custom path.
func Gitignore(name string) ([]byte, error) {
	return fileFromDir(path.Join("gitignore", name))
}

// License reads the content of a specific license from bindata or custom path.
func License(name string) ([]byte, error) {
	return fileFromDir(path.Join("license", name))
}

// Labels reads the content of a specific labels from static or custom path.
func Labels(name string) ([]byte, error) {
	return fileFromDir(path.Join("label", name))
}

// fileFromDir is a helper to read files from bindata or custom path.
func fileFromDir(name string) ([]byte, error) {
	customPath := path.Join(setting.CustomPath, "options", name)

	isFile, err := util.IsFile(customPath)
	if err != nil {
		log.Error("Unable to check if %s is a file. Error: %v", customPath, err)
	}
	if isFile {
		return os.ReadFile(customPath)
	}

	f, err := Assets.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(f)
}

func Asset(name string) ([]byte, error) {
	f, err := Assets.Open("/" + name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func AssetNames() []string {
	realFS := Assets.(vfsgen۰FS)
	results := make([]string, 0, len(realFS))
	for k := range realFS {
		results = append(results, k[1:])
	}
	return results
}

func AssetIsDir(name string) (bool, error) {
	if f, err := Assets.Open("/" + name); err != nil {
		return false, err
	} else {
		defer f.Close()
		if fi, err := f.Stat(); err != nil {
			return false, err
		} else {
			return fi.IsDir(), nil
		}
	}
}

// IsDynamic will return false when using embedded data (-tags bindata)
func IsDynamic() bool {
	return false
}
