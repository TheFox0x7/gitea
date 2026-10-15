// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"log/slog"
	"strings"
)

// Level is an alias of slog.Level so config and handlers interoperate freely.
type Level = slog.Level

const (
	LevelTrace Level = slog.LevelDebug - 4
	LevelDebug Level = slog.LevelDebug
	LevelInfo  Level = slog.LevelInfo
	LevelWarn  Level = slog.LevelWarn
	LevelError Level = slog.LevelError

	LevelCritical       = LevelError // most logger frameworks don't support CRITICAL
	LevelFatal    Level = 12
	LevelNone     Level = 16
)

var levelNames = map[Level]string{
	LevelTrace: "trace",
	LevelDebug: "debug",
	LevelInfo:  "info",
	LevelWarn:  "warn",
	LevelError: "error",
	LevelFatal: "fatal",
	LevelNone:  "none",
}

var levelsFromString = map[string]Level{
	"trace":   LevelTrace,
	"debug":   LevelDebug,
	"info":    LevelInfo,
	"warn":    LevelWarn,
	"warning": LevelWarn,
	"error":   LevelError,
	"fatal":   LevelFatal,
	"none":    LevelNone,
}

// LevelName returns the lower-case Gitea name of the level, falling back to
// slog's own rendering (e.g. "DEBUG+2") for unmapped values.
func LevelName(l Level) string {
	if s, ok := levelNames[l]; ok {
		return s
	}
	return l.String()
}

// LevelFromString parses a level name, accepting "warning" as WARN.
func LevelFromString(s string) (Level, bool) {
	l, ok := levelsFromString[strings.ToLower(strings.TrimSpace(s))]
	if !ok {
		return LevelInfo, false
	}
	return l, true
}

// levelColors is also exposed as LevelColors for the compatibility shim.
var levelColors = map[Level][]ColorAttribute{
	LevelTrace: {Bold, FgCyan},
	LevelDebug: {Bold, FgBlue},
	LevelInfo:  {Bold, FgGreen},
	LevelWarn:  {Bold, FgYellow},
	LevelError: {Bold, FgRed},
	LevelFatal: {Bold, BgRed},
}

// LevelColors returns the ANSI color attributes for a log level.
func LevelColors(l Level) []ColorAttribute {
	return levelColorAttrs(l)
}

func levelColorAttrs(l Level) []ColorAttribute {
	if attrs, ok := levelColors[l]; ok {
		return attrs
	}
	return []ColorAttribute{Reset}
}
