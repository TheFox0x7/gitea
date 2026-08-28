// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

// Package-level printf-style logging on the default logger, kept for gradual
// migration from modules/log. Prefer Slog()-based structured logging.

// Log logs a printf-style message on the default logger. skip is the number of
// stack frames to skip, 1 meaning the caller of this function.
//
// Deprecated: use Default().Slog() instead.
func Log(skip int, level Level, format string, args ...any) {
	Default().Log(skip+1, level, format, args...)
}

// Deprecated: use Default().Slog().Debug(...) instead.
func Trace(format string, args ...any) {
	Log(1, LevelTrace, format, args...)
}

// Deprecated: use Default().Slog().Debug(...) instead.
func Debug(format string, args ...any) {
	Log(1, LevelDebug, format, args...)
}

// Deprecated: use Default().Slog().Info(...) instead.
func Info(format string, args ...any) {
	Log(1, LevelInfo, format, args...)
}

// Deprecated: use Default().Slog().Warn(...) instead.
func Warn(format string, args ...any) {
	Log(1, LevelWarn, format, args...)
}

// Deprecated: use Default().Slog().Error(...) instead.
func Error(format string, args ...any) {
	Log(1, LevelError, format, args...)
}

// Deprecated: use Default().Slog().Error(...) instead.
func Critical(format string, args ...any) {
	Log(1, LevelCritical, format, args...)
}

// Fatal logs at FATAL on the default logger, flushes and exits.
// Deprecated: handle fatal errors explicitly instead.
func Fatal(format string, args ...any) {
	Log(1, LevelFatal, format, args...)
	GetManager().Close()
	OsExiter(1)
}

// GetLevel returns the level of the default logger.
//
// Deprecated: use Default().GetLevel() or Slog().Enabled(...) instead.
func GetLevel() Level {
	return Default().GetLevel()
}

// IsTrace reports whether trace logging is enabled on the default logger.
//
// Deprecated: use Default().LevelEnabled(LevelTrace) instead.
func IsTrace() bool {
	return Default().LevelEnabled(LevelTrace)
}

// IsDebug reports whether debug logging is enabled on the default logger.
//
// Deprecated: use Default().LevelEnabled(LevelDebug) instead.
func IsDebug() bool {
	return Default().LevelEnabled(LevelDebug)
}
