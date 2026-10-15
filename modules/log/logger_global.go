// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"os"

	"gitea.dev/modules/slog"
)

// FallbackErrorf is the last chance to show an error if the logger has internal errors
func FallbackErrorf(format string, args ...any) {
	slog.FallbackErrorf(format, args...)
}

// Deprecated: use slog.GetLevel.
func GetLevel() Level {
	return GetLogger(DEFAULT).GetLevel()
}

// Deprecated: use slog.Log.
func Log(skip int, level Level, format string, v ...any) {
	// codeql[disable-next-line=go/clear-text-logging]
	GetLogger(DEFAULT).Log(skip+1, &Event{Level: level}, format, v...)
}

// Deprecated: use slog.Log with slog.LevelTrace.
func Trace(format string, v ...any) {
	Log(2, TRACE, format, v...)
}

// Deprecated: use slog.GetLogger(...).LevelEnabled(slog.LevelTrace).
func IsTrace() bool {
	return GetLevel() <= TRACE
}

// Deprecated: use slog.Log with slog.LevelDebug.
func Debug(format string, v ...any) {
	Log(2, DEBUG, format, v...)
}

// Deprecated: use slog.Log with slog.LevelInfo.
func Info(format string, v ...any) {
	Log(2, INFO, format, v...)
}

// Deprecated: use slog.Log with slog.LevelWarn.
func Warn(format string, v ...any) {
	Log(2, WARN, format, v...)
}

// Deprecated: use slog.Log with slog.LevelError.
func Error(format string, v ...any) {
	Log(2, ERROR, format, v...)
}

// Deprecated: use slog.Log with slog.LevelError.
func ErrorWithSkip(skip int, format string, v ...any) {
	Log(skip+2, ERROR, format, v...)
}

var OsExiter = os.Exit

// Fatal records fatal log and exit process
// Deprecated: handle fatal errors explicitly instead.
func Fatal(format string, v ...any) {
	Log(2, FATAL, format, v...)
	GetManager().Close()
	OsExiter(1)
}

// Deprecated: use slog.GetLogger.
func GetLogger(name string) Logger {
	return GetManager().GetLogger(name)
}

// Deprecated: use slog.GetLogger(...).GetLevel() != slog.LevelNone.
func IsLoggerEnabled(name string) bool {
	return GetManager().GetLogger(name).IsEnabled()
}

// SetupStderrLogger installs a console logger writing to stderr, it is used
// before the configuration is loaded (e.g. for CLI commands).
// Deprecated: use slog.Apply with slog.StderrConfig.
func SetupStderrLogger(loggerName, writerName string, level Level) {
	slogLevel, _ := slog.LevelFromString(level.String())
	if err := slog.GetManager().Apply(slog.StderrConfig(loggerName, slogLevel)); err != nil {
		slog.FallbackErrorf("unable to apply slog config: %v", err)
	}
}
