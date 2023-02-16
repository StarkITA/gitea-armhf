// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	ini "gopkg.in/ini.v1"
)

func TestParseMailerConfig(t *testing.T) {
	iniFile := ini.Empty()
	kases := map[string]*Mailer{
		"smtp.mydomain.com": {
			SMTPAddr: "smtp.mydomain.com",
			SMTPPort: "465",
		},
		"smtp.mydomain.com:123": {
			SMTPAddr: "smtp.mydomain.com",
			SMTPPort: "123",
		},
		":123": {
			SMTPAddr: "127.0.0.1",
			SMTPPort: "123",
		},
	}
	for host, kase := range kases {
		t.Run(host, func(t *testing.T) {
			iniFile.DeleteSection("mailer")
			sec := iniFile.Section("mailer")
			sec.NewKey("ENABLED", "true")
			sec.NewKey("HOST", host)

			// Check mailer setting
			parseMailerConfig(iniFile)

			assert.EqualValues(t, kase.SMTPAddr, MailService.SMTPAddr)
			assert.EqualValues(t, kase.SMTPPort, MailService.SMTPPort)
		})
	}
}
