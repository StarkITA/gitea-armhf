// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	ini "gopkg.in/ini.v1"
)

func Test_getStorageCustomType(t *testing.T) {
	iniStr := `
[attachment]
STORAGE_TYPE = my_minio
MINIO_BUCKET = gitea-attachment

[storage.my_minio]
STORAGE_TYPE = minio
MINIO_ENDPOINT = my_minio:9000
`
	Cfg, _ = ini.Load([]byte(iniStr))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "minio", storage.Type)
	assert.EqualValues(t, "my_minio:9000", storage.Section.Key("MINIO_ENDPOINT").String())
	assert.EqualValues(t, "gitea-attachment", storage.Section.Key("MINIO_BUCKET").String())
}

func Test_getStorageNameSectionOverridesTypeSection(t *testing.T) {
	iniStr := `
[attachment]
STORAGE_TYPE = minio

[storage.attachments]
MINIO_BUCKET = gitea-attachment

[storage.minio]
MINIO_BUCKET = gitea
`
	Cfg, _ = ini.Load([]byte(iniStr))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "minio", storage.Type)
	assert.EqualValues(t, "gitea-attachment", storage.Section.Key("MINIO_BUCKET").String())
}

func Test_getStorageTypeSectionOverridesStorageSection(t *testing.T) {
	iniStr := `
[attachment]
STORAGE_TYPE = minio

[storage.minio]
MINIO_BUCKET = gitea-minio

[storage]
MINIO_BUCKET = gitea
`
	Cfg, _ = ini.Load([]byte(iniStr))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "minio", storage.Type)
	assert.EqualValues(t, "gitea-minio", storage.Section.Key("MINIO_BUCKET").String())
}

func Test_getStorageSpecificOverridesStorage(t *testing.T) {
	iniStr := `
[attachment]
STORAGE_TYPE = minio
MINIO_BUCKET = gitea-attachment

[storage.attachments]
MINIO_BUCKET = gitea

[storage]
STORAGE_TYPE = local
`
	Cfg, _ = ini.Load([]byte(iniStr))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "minio", storage.Type)
	assert.EqualValues(t, "gitea-attachment", storage.Section.Key("MINIO_BUCKET").String())
}

func Test_getStorageGetDefaults(t *testing.T) {
	Cfg, _ = ini.Load([]byte(""))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "gitea", storage.Section.Key("MINIO_BUCKET").String())
}

func Test_getStorageMultipleName(t *testing.T) {
	iniStr := `
[lfs]
MINIO_BUCKET = gitea-lfs

[attachment]
MINIO_BUCKET = gitea-attachment

[storage]
MINIO_BUCKET = gitea-storage
`
	Cfg, _ = ini.Load([]byte(iniStr))

	{
		sec := Cfg.Section("attachment")
		storageType := sec.Key("STORAGE_TYPE").MustString("")
		storage := getStorage("attachments", storageType, sec)

		assert.EqualValues(t, "gitea-attachment", storage.Section.Key("MINIO_BUCKET").String())
	}
	{
		sec := Cfg.Section("lfs")
		storageType := sec.Key("STORAGE_TYPE").MustString("")
		storage := getStorage("lfs", storageType, sec)

		assert.EqualValues(t, "gitea-lfs", storage.Section.Key("MINIO_BUCKET").String())
	}
	{
		sec := Cfg.Section("avatar")
		storageType := sec.Key("STORAGE_TYPE").MustString("")
		storage := getStorage("avatars", storageType, sec)

		assert.EqualValues(t, "gitea-storage", storage.Section.Key("MINIO_BUCKET").String())
	}
}

func Test_getStorageUseOtherNameAsType(t *testing.T) {
	iniStr := `
[attachment]
STORAGE_TYPE = lfs

[storage.lfs]
MINIO_BUCKET = gitea-storage
`
	Cfg, _ = ini.Load([]byte(iniStr))

	{
		sec := Cfg.Section("attachment")
		storageType := sec.Key("STORAGE_TYPE").MustString("")
		storage := getStorage("attachments", storageType, sec)

		assert.EqualValues(t, "gitea-storage", storage.Section.Key("MINIO_BUCKET").String())
	}
	{
		sec := Cfg.Section("lfs")
		storageType := sec.Key("STORAGE_TYPE").MustString("")
		storage := getStorage("lfs", storageType, sec)

		assert.EqualValues(t, "gitea-storage", storage.Section.Key("MINIO_BUCKET").String())
	}
}

func Test_getStorageInheritStorageType(t *testing.T) {
	iniStr := `
[storage]
STORAGE_TYPE = minio
`
	Cfg, _ = ini.Load([]byte(iniStr))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "minio", storage.Type)
}

func Test_getStorageInheritNameSectionType(t *testing.T) {
	iniStr := `
[storage.attachments]
STORAGE_TYPE = minio
`
	Cfg, _ = ini.Load([]byte(iniStr))

	sec := Cfg.Section("attachment")
	storageType := sec.Key("STORAGE_TYPE").MustString("")
	storage := getStorage("attachments", storageType, sec)

	assert.EqualValues(t, "minio", storage.Type)
}
