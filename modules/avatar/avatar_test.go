// Copyright 2016 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package avatar

import (
	"os"
	"testing"

	"code.gitea.io/gitea/modules/setting"

	"github.com/stretchr/testify/assert"
)

func Test_RandomImageSize(t *testing.T) {
	_, err := RandomImageSize(0, []byte("gitea@local"))
	assert.Error(t, err)

	_, err = RandomImageSize(64, []byte("gitea@local"))
	assert.NoError(t, err)
}

func Test_RandomImage(t *testing.T) {
	_, err := RandomImage([]byte("gitea@local"))
	assert.NoError(t, err)
}

func Test_PrepareWithPNG(t *testing.T) {
	setting.Avatar.MaxWidth = 4096
	setting.Avatar.MaxHeight = 4096

	data, err := os.ReadFile("testdata/avatar.png")
	assert.NoError(t, err)

	imgPtr, err := Prepare(data)
	assert.NoError(t, err)

	assert.Equal(t, 290, (*imgPtr).Bounds().Max.X)
	assert.Equal(t, 290, (*imgPtr).Bounds().Max.Y)
}

func Test_PrepareWithJPEG(t *testing.T) {
	setting.Avatar.MaxWidth = 4096
	setting.Avatar.MaxHeight = 4096

	data, err := os.ReadFile("testdata/avatar.jpeg")
	assert.NoError(t, err)

	imgPtr, err := Prepare(data)
	assert.NoError(t, err)

	assert.Equal(t, 290, (*imgPtr).Bounds().Max.X)
	assert.Equal(t, 290, (*imgPtr).Bounds().Max.Y)
}

func Test_PrepareWithInvalidImage(t *testing.T) {
	setting.Avatar.MaxWidth = 5
	setting.Avatar.MaxHeight = 5

	_, err := Prepare([]byte{})
	assert.EqualError(t, err, "DecodeConfig: image: unknown format")
}

func Test_PrepareWithInvalidImageSize(t *testing.T) {
	setting.Avatar.MaxWidth = 5
	setting.Avatar.MaxHeight = 5

	data, err := os.ReadFile("testdata/avatar.png")
	assert.NoError(t, err)

	_, err = Prepare(data)
	assert.EqualError(t, err, "Image width is too large: 10 > 5")
}
