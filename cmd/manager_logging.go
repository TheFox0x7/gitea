// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"os"

	"gitea.dev/modules/private"

	"github.com/urfave/cli/v3"
)

func newLoggingCommand() *cli.Command {
	return &cli.Command{
		Name:  "logging",
		Usage: "Adjust logging commands",
		Commands: []*cli.Command{
			{
				Name:  "release-and-reopen",
				Usage: "Cause Gitea to release and re-open files used for logging",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name: "debug",
					},
				},
				Action: runReleaseReopenLogging,
			}, {
				Name:  "log-sql",
				Usage: "Set LogSQL",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name: "debug",
					},
					&cli.BoolFlag{
						Name:  "off",
						Usage: "Switch off SQL logging",
					},
				},
				Action: runSetLogSQL,
			},
		},
	}
}

func runReleaseReopenLogging(ctx context.Context, c *cli.Command) error {
	setup(ctx, c.Bool("debug"))
	userMsg := private.ReleaseReopenLogging(ctx)
	_, _ = fmt.Fprintln(os.Stdout, userMsg)
	return nil
}

func runSetLogSQL(ctx context.Context, c *cli.Command) error {
	setup(ctx, c.Bool("debug"))

	extra := private.SetLogSQL(ctx, !c.Bool("off"))
	return handleCliResponseExtra(extra)
}
