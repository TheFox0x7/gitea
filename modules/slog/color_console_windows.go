// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package slog

import (
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/windows"
)

func enableVTMode(console windows.Handle) bool {
	mode := uint32(0)
	err := windows.GetConsoleMode(console, &mode)
	if err != nil {
		return false
	}
	mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	err = windows.SetConsoleMode(console, mode)
	return err == nil
}

func init() {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		CanColorStdout = enableVTMode(windows.Stdout)
	} else {
		CanColorStdout = isatty.IsCygwinTerminal(os.Stderr.Fd())
	}

	if isatty.IsTerminal(os.Stderr.Fd()) {
		CanColorStderr = enableVTMode(windows.Stderr)
	} else {
		CanColorStderr = isatty.IsCygwinTerminal(os.Stderr.Fd())
	}
}
