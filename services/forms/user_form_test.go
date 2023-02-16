// Copyright 2018 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package forms

import (
	"strconv"
	"testing"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/modules/setting"

	"github.com/stretchr/testify/assert"
)

func TestRegisterForm_IsDomainAllowed_Empty(t *testing.T) {
	_ = setting.Service

	setting.Service.EmailDomainWhitelist = []string{}

	form := RegisterForm{}

	assert.True(t, form.IsEmailDomainAllowed())
}

func TestRegisterForm_IsDomainAllowed_InvalidEmail(t *testing.T) {
	_ = setting.Service

	setting.Service.EmailDomainWhitelist = []string{"gitea.io"}

	tt := []struct {
		email string
	}{
		{"securitygieqqq"},
		{"hdudhdd"},
	}

	for _, v := range tt {
		form := RegisterForm{Email: v.email}

		assert.False(t, form.IsEmailDomainAllowed())
	}
}

func TestRegisterForm_IsDomainAllowed_WhitelistedEmail(t *testing.T) {
	_ = setting.Service

	setting.Service.EmailDomainWhitelist = []string{"gitea.io"}

	tt := []struct {
		email string
		valid bool
	}{
		{"security@gitea.io", true},
		{"security@gITea.io", true},
		{"hdudhdd", false},
		{"seee@example.com", false},
	}

	for _, v := range tt {
		form := RegisterForm{Email: v.email}

		assert.Equal(t, v.valid, form.IsEmailDomainAllowed())
	}
}

func TestRegisterForm_IsDomainAllowed_BlocklistedEmail(t *testing.T) {
	_ = setting.Service

	setting.Service.EmailDomainWhitelist = []string{}
	setting.Service.EmailDomainBlocklist = []string{"gitea.io"}

	tt := []struct {
		email string
		valid bool
	}{
		{"security@gitea.io", false},
		{"security@gitea.example", true},
		{"hdudhdd", true},
	}

	for _, v := range tt {
		form := RegisterForm{Email: v.email}

		assert.Equal(t, v.valid, form.IsEmailDomainAllowed())
	}
}

func TestNewAccessTokenForm_GetScope(t *testing.T) {
	tests := []struct {
		form        NewAccessTokenForm
		scope       auth_model.AccessTokenScope
		expectedErr error
	}{
		{
			form:  NewAccessTokenForm{Name: "test", Scope: []string{"repo"}},
			scope: "repo",
		},
		{
			form:  NewAccessTokenForm{Name: "test", Scope: []string{"repo", "user"}},
			scope: "repo,user",
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			scope, err := test.form.GetScope()
			assert.Equal(t, test.expectedErr, err)
			assert.Equal(t, test.scope, scope)
		})
	}
}
