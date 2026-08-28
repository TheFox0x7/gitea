// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"log/slog"
	"strings"
)

// hiddenInfo carries printf-path metadata beside a record: the multiHandler
// passes it directly to its sink frontends, so it never becomes a record attr
// and stdlib-format sinks have nothing to strip.
type hiddenInfo struct {
	caller string // prefilled caller name (overrides Record.PC resolution)
	file   string // prefilled file name
	line   int    // prefilled line number

	format     string // printf format, for colorized legacy re-rendering
	args       []any  // printf args, for colorized legacy re-rendering
	stacktrace string // pre-computed stack trace
}

// prefillSource resolves the record's caller once so all legacy sinks share it
func (hi *hiddenInfo) prefillSource(rec *slog.Record) {
	if src := rec.Source(); src != nil {
		hi.caller = strings.ReplaceAll(src.Function, "[...]", "") + "()"
		hi.file = strings.TrimPrefix(src.File, projectPackagePrefix)
		hi.line = src.Line
	}
}
