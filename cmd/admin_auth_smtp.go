// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"strings"

	auth_model "code.gitea.io/gitea/models/auth"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/auth/source/smtp"

	"github.com/urfave/cli/v3"
)

// withSmtpFlags creates list of smtp specific flags
// isSetup toggles mandatory parameters on for setup scenario
func withSmtpFlags(isSetup bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.StringFlag{Name: "host", Usage: "SMTP Host", Required: isSetup},
		&cli.IntFlag{Name: "port", Usage: "SMTP Port", Required: isSetup},
		&cli.BoolFlag{Name: "disable-helo", Usage: "Disable SMTP helo."},
		&cli.BoolFlag{
			Name:  "force-smtps",
			Usage: "SMTPS is always used on port 465. Set this to force SMTPS on other ports.",
		},
		&cli.BoolFlag{Name: "skip-verify", Usage: "Skip TLS verify."}, // Problably could be global
		&cli.StringFlag{
			Name:  "auth-type",
			Value: "PLAIN",
			Usage: "SMTP Authentication Type (PLAIN/LOGIN/CRAM-MD5)",
			Validator: func(auth string) error {
				validAuthTypes := []string{"PLAIN", "LOGIN", "CRAM-MD5"}
				if !util.SliceContainsString(validAuthTypes, strings.ToUpper(auth)) {
					return errors.New("Auth must be one of PLAIN/LOGIN/CRAM-MD5")
				}
				return nil
			},
		},

		&cli.StringFlag{
			Name:  "helo-hostname",
			Usage: "Hostname sent with HELO. Leave blank to send current hostname",
		},
		&cli.StringFlag{
			Name:  "allowed-domains", // move to allow domain and multiple uses?
			Usage: "Leave empty to allow all domains. Separate multiple domains with a comma (',')",
		},
	}
	return withCommonAuthFlags(flags, isSetup)
}

func microcmdAuthUpdateSMTP() *cli.Command {
	return &cli.Command{
		Name:  "update-smtp",
		Usage: "Update existing SMTP authentication source",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return newAuthService().runUpdateSMTP(ctx, cmd)
		},
		Flags: withSmtpFlags(false),
	}
}

func microcmdAuthAddSMTP() *cli.Command {
	return &cli.Command{
		Name:  "add-smtp",
		Usage: "Add new SMTP authentication source",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return newAuthService().runAddSMTP(ctx, cmd)
		},
		Flags: withSmtpFlags(true),
	}
}

func parseSMTPConfig(c *cli.Command, conf *smtp.Source) error {
	if c.IsSet("auth-type") {
		conf.Auth = c.String("auth-type")
	}
	if c.IsSet("host") {
		conf.Host = c.String("host")
	}
	if c.IsSet("port") {
		conf.Port = c.Int("port")
	}
	if c.IsSet("allowed-domains") {
		conf.AllowedDomains = c.String("allowed-domains")
	}
	if c.IsSet("force-smtps") {
		conf.ForceSMTPS = c.Bool("force-smtps")
	}
	if c.IsSet("skip-verify") {
		conf.SkipVerify = c.Bool("skip-verify")
	}
	if c.IsSet("helo-hostname") {
		conf.HeloHostname = c.String("helo-hostname")
	}
	if c.IsSet("disable-helo") {
		conf.DisableHelo = c.Bool("disable-helo")
	}
	return nil
}

func (a *authService) runAddSMTP(ctx context.Context, c *cli.Command) error {
	if err := a.initDB(ctx); err != nil {
		return err
	}

	active := true
	if c.IsSet("active") {
		active = c.Bool("active")
	}

	var smtpConfig smtp.Source
	if err := parseSMTPConfig(c, &smtpConfig); err != nil {
		return err
	}

	// If not set default to PLAIN
	if len(smtpConfig.Auth) == 0 {
		smtpConfig.Auth = "PLAIN"
	}

	return a.createAuthSource(ctx, &auth_model.Source{
		Type:            auth_model.SMTP,
		Name:            c.String("name"),
		IsActive:        active,
		Cfg:             &smtpConfig,
		TwoFactorPolicy: util.Iif(c.Bool("skip-local-2fa"), "skip", ""),
	})
}

func (a *authService) runUpdateSMTP(ctx context.Context, c *cli.Command) error {
	if !c.IsSet("id") {
		return errors.New("--id flag is missing")
	}

	if err := a.initDB(ctx); err != nil {
		return err
	}

	source, err := a.getAuthSourceByID(ctx, c.Int64("id"))
	if err != nil {
		return err
	}

	smtpConfig := source.Cfg.(*smtp.Source)

	if err := parseSMTPConfig(c, smtpConfig); err != nil {
		return err
	}

	if c.IsSet("name") {
		source.Name = c.String("name")
	}

	if c.IsSet("active") {
		source.IsActive = c.Bool("active")
	}

	source.Cfg = smtpConfig
	source.TwoFactorPolicy = util.Iif(c.Bool("skip-local-2fa"), "skip", "")
	return a.updateAuthSource(ctx, source)
}
