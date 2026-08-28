// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

// CanColorStdout reports if we can color the Stdout.
// Terminal sniffing only happens on Windows in color_console_windows.go.
var CanColorStdout = true

// CanColorStderr reports if we can color the Stderr
var CanColorStderr = true
