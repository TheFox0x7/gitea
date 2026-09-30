// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"net/http"
	"os"

	gossh "golang.org/x/crypto/ssh"

	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/templates"
	"gitea.dev/services/context"
)

const tplInstanceInfo templates.TplName = "misc/instance"

type SSHHostKey struct {
	Algorithm   string
	Fingerprint string
}

func loadSSHHostKeyFingerprints() []SSHHostKey {
	keys := make([]SSHHostKey, 0, len(setting.SSH.ServerHostKeys))
	for _, keyFile := range setting.SSH.ServerHostKeys {
		pemBytes, err := os.ReadFile(keyFile)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Warn("Unable to read SSH host key %s: %v", keyFile, err)
			}
			continue
		}
		signer, err := gossh.ParsePrivateKey(pemBytes)
		if err != nil {
			log.Warn("Unable to parse SSH host key %s: %v", keyFile, err)
			continue
		}
		keys = append(keys, SSHHostKey{
			Algorithm:   signer.PublicKey().Type(),
			Fingerprint: gossh.FingerprintSHA256(signer.PublicKey()),
		})
	}
	return keys
}

func InstanceInfo(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("misc.instance_info")
	ctx.Data["PageIsInstanceInfo"] = true
	ctx.Data["AppVer"] = setting.AppVer
	ctx.Data["AppBuiltWith"] = setting.AppBuiltWith

	ctx.Data["SSHEnabled"] = !setting.SSH.Disabled
	ctx.Data["SSHCloneDomain"] = setting.SSH.Domain
	ctx.Data["SSHClonePort"] = setting.SSH.Port
	ctx.Data["SSHCloneUser"] = setting.SSH.User
	ctx.Data["SSHHostKeys"] = loadSSHHostKeyFingerprints()

	ctx.Data["ActionsEnabled"] = setting.Actions.Enabled
	ctx.Data["PackagesEnabled"] = setting.Packages.Enabled
	ctx.Data["LFSEnabled"] = setting.LFS.StartServer
	ctx.Data["MailEnabled"] = setting.MailService != nil
	ctx.Data["FederationEnabled"] = setting.Federation.Enabled

	ctx.HTML(http.StatusOK, tplInstanceInfo)
}
