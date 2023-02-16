// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/util"
)

var _ ObjectStorage = &LocalStorage{}

// LocalStorageType is the type descriptor for local storage
const LocalStorageType Type = "local"

// LocalStorageConfig represents the configuration for a local storage
type LocalStorageConfig struct {
	Path          string `ini:"PATH"`
	TemporaryPath string `ini:"TEMPORARY_PATH"`
}

// LocalStorage represents a local files storage
type LocalStorage struct {
	ctx    context.Context
	dir    string
	tmpdir string
}

// NewLocalStorage returns a local files
func NewLocalStorage(ctx context.Context, cfg interface{}) (ObjectStorage, error) {
	configInterface, err := toConfig(LocalStorageConfig{}, cfg)
	if err != nil {
		return nil, err
	}
	config := configInterface.(LocalStorageConfig)

	log.Info("Creating new Local Storage at %s", config.Path)
	if err := os.MkdirAll(config.Path, os.ModePerm); err != nil {
		return nil, err
	}

	if config.TemporaryPath == "" {
		config.TemporaryPath = config.Path + "/tmp"
	}

	return &LocalStorage{
		ctx:    ctx,
		dir:    config.Path,
		tmpdir: config.TemporaryPath,
	}, nil
}

func (l *LocalStorage) buildLocalPath(p string) string {
	return filepath.Join(l.dir, path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))[1:])
}

// Open a file
func (l *LocalStorage) Open(path string) (Object, error) {
	return os.Open(l.buildLocalPath(path))
}

// Save a file
func (l *LocalStorage) Save(path string, r io.Reader, size int64) (int64, error) {
	p := l.buildLocalPath(path)
	if err := os.MkdirAll(filepath.Dir(p), os.ModePerm); err != nil {
		return 0, err
	}

	// Create a temporary file to save to
	if err := os.MkdirAll(l.tmpdir, os.ModePerm); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(l.tmpdir, "upload-*")
	if err != nil {
		return 0, err
	}
	tmpRemoved := false
	defer func() {
		if !tmpRemoved {
			_ = util.Remove(tmp.Name())
		}
	}()

	n, err := io.Copy(tmp, r)
	if err != nil {
		return 0, err
	}

	if err := tmp.Close(); err != nil {
		return 0, err
	}

	if err := util.Rename(tmp.Name(), p); err != nil {
		return 0, err
	}
	// Golang's tmp file (os.CreateTemp) always have 0o600 mode, so we need to change the file to follow the umask (as what Create/MkDir does)
	// but we don't want to make these files executable - so ensure that we mask out the executable bits
	if err := util.ApplyUmask(p, os.ModePerm&0o666); err != nil {
		return 0, err
	}

	tmpRemoved = true

	return n, nil
}

// Stat returns the info of the file
func (l *LocalStorage) Stat(path string) (os.FileInfo, error) {
	return os.Stat(l.buildLocalPath(path))
}

// Delete delete a file
func (l *LocalStorage) Delete(path string) error {
	return util.Remove(l.buildLocalPath(path))
}

// URL gets the redirect URL to a file
func (l *LocalStorage) URL(path, name string) (*url.URL, error) {
	return nil, ErrURLNotSupported
}

// IterateObjects iterates across the objects in the local storage
func (l *LocalStorage) IterateObjects(fn func(path string, obj Object) error) error {
	return filepath.WalkDir(l.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-l.ctx.Done():
			return l.ctx.Err()
		default:
		}
		if path == l.dir {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(l.dir, path)
		if err != nil {
			return err
		}
		obj, err := os.Open(path)
		if err != nil {
			return err
		}
		defer obj.Close()
		return fn(relPath, obj)
	})
}

func init() {
	RegisterStorageType(LocalStorageType, NewLocalStorage)
}
