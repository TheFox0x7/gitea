// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"gitea.dev/modules/slog"
)

// Level is the level of the logger. The numeric values are slog's values, not
// the historical iota values of the old logging system.
type Level = slog.Level

const (
	// UNDEFINED means "no level set"; it equals NONE (there is no free numeric
	// slot in slog's level range). The only in-tree comparison is against an
	// unset router log level, which is never NONE.
	UNDEFINED = NONE

	TRACE    = slog.LevelTrace
	DEBUG    = slog.LevelDebug
	INFO     = slog.LevelInfo
	WARN     = slog.LevelWarn
	ERROR    = slog.LevelError
	CRITICAL = slog.LevelError // most logger frameworks don't support CRITICAL
	FATAL    = slog.LevelFatal
	NONE     = slog.LevelNone
)

// Deprecated: use slog.LevelName.
func LevelName(l Level) string {
	return slog.LevelName(l)
}

// Deprecated: use slog.LevelFromString.
func LevelFromString(level string) Level {
	l, _ := slog.LevelFromString(level)
	return l
}

// Deprecated: use slog.LevelColors.
func ColorAttributes(l Level) []ColorAttribute {
	return slog.LevelColors(l)
}
