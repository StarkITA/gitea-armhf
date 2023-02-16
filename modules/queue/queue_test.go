// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package queue

import (
	"testing"

	"code.gitea.io/gitea/modules/json"

	"github.com/stretchr/testify/assert"
)

type testData struct {
	TestString string
	TestInt    int
}

func TestToConfig(t *testing.T) {
	cfg := testData{
		TestString: "Config",
		TestInt:    10,
	}
	exemplar := testData{}

	cfg2I, err := toConfig(exemplar, cfg)
	assert.NoError(t, err)
	cfg2, ok := (cfg2I).(testData)
	assert.True(t, ok)
	assert.NotEqual(t, cfg2, exemplar)
	assert.Equal(t, &cfg, &cfg2)
	cfgString, err := json.Marshal(cfg)
	assert.NoError(t, err)

	cfg3I, err := toConfig(exemplar, cfgString)
	assert.NoError(t, err)
	cfg3, ok := (cfg3I).(testData)
	assert.True(t, ok)
	assert.Equal(t, cfg.TestString, cfg3.TestString)
	assert.Equal(t, cfg.TestInt, cfg3.TestInt)
	assert.NotEqual(t, cfg3, exemplar)
}
