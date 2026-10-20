// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"os"
	"runtime"

	"gitea.dev/modules/slog"
)

// FallbackErrorf is the last chance to show an error if the logger has internal errors
func FallbackErrorf(format string, args ...any) {
	slog.FallbackErrorf(format, args...)
}

// Deprecated: use slog.GetLevel.
func GetLevel() Level {
	return slog.DefaultPrintfLogger().GetLevel()
}

// Deprecated: use slog.Log.
//
//go:noinline
func Log(skip int, level Level, format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(skip+2, pcs[:]) // 0=Callers, 1=this fn, 2=caller
	slog.DefaultPrintfLogger().LogWithCallerPC(pcs[0], level, "", skip+3, format, v...)
}

// Deprecated: use slog.Log with slog.LevelTrace.
//
//go:noinline
func Trace(format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller
	slog.DefaultPrintfLogger().LogPC(pcs[0], TRACE, 5, format, v...)
}

// Deprecated: use slog.GetLogger(...).LevelEnabled(slog.LevelTrace).
func IsTrace() bool {
	return GetLevel() <= TRACE
}

// Deprecated: use slog.Log with slog.LevelDebug.
//
//go:noinline
func Debug(format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	slog.DefaultPrintfLogger().LogPC(pcs[0], DEBUG, 5, format, v...)
}

// Deprecated: use slog.Log with slog.LevelInfo.
//
//go:noinline
func Info(format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	slog.DefaultPrintfLogger().LogPC(pcs[0], INFO, 5, format, v...)
}

// Deprecated: use slog.Log with slog.LevelWarn.
//
//go:noinline
func Warn(format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	slog.DefaultPrintfLogger().LogPC(pcs[0], WARN, 5, format, v...)
}

// Deprecated: use slog.Log with slog.LevelError.
//
//go:noinline
func Error(format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	slog.DefaultPrintfLogger().LogPC(pcs[0], ERROR, 5, format, v...)
}

// Deprecated: use slog.Log with slog.LevelError and an explicit skip.
//
//go:noinline
func ErrorWithSkip(skip int, format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	slog.DefaultPrintfLogger().LogPC(pcs[0], ERROR, 5+skip, format, v...)
}

var OsExiter = os.Exit

// Fatal records fatal log and exit process
// Deprecated: handle fatal errors explicitly instead.
//
//go:noinline
func Fatal(format string, v ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	slog.DefaultPrintfLogger().LogPC(pcs[0], FATAL, 5, format, v...)
	GetManager().Close()
	OsExiter(1)
}

// Deprecated: use slog.GetLogger.
func GetLogger(name string) Logger {
	return GetManager().GetLogger(name)
}

// Deprecated: use slog.GetLogger(...).LevelEnabled(slog.LevelInfo).
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
