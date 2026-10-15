// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package log provides Gitea's historical logging API as a compatibility shim
// over the slog-based engine (modules/slog). All functions are thin forwards;
// new code should use modules/slog directly (prefer Logger.Slog() for
// structured logging).
package log

import (
	"gitea.dev/modules/slog"
)

// BaseLogger provides the basic logging functions
type BaseLogger interface {
	Log(skip int, event *Event, format string, v ...any)
	GetLevel() Level
}

// LevelLogger provides level-related logging functions
type LevelLogger interface {
	LevelEnabled(level Level) bool

	Trace(format string, v ...any)
	Debug(format string, v ...any)
	Info(format string, v ...any)
	Warn(format string, v ...any)
	Error(format string, v ...any)
	Critical(format string, v ...any)
}

// Logger provides the full logging functions
type Logger interface {
	BaseLogger
	LevelLogger

	IsEnabled() bool
}

type LogStringer = slog.LogStringer //nolint:revive // export stutter, kept for compatibility

// logger forwards the legacy Logger API to the slog engine.
type logger struct {
	l *slog.Logger
}

var _ Logger = (*logger)(nil)

func (l *logger) Log(skip int, event *Event, format string, v ...any) {
	// +2: skip this shim and the facade; pinned by TestCallerAttribution
	if event != nil && event.Caller != "" {
		l.l.LogWithCaller(skip+2, event.Level, event.Caller, format, v...)
		return
	}
	l.l.Log(skip+2, event.Level, format, v...)
}

func (l *logger) GetLevel() Level {
	return l.l.GetLevel()
}

func (l *logger) LevelEnabled(level Level) bool {
	return l.l.LevelEnabled(level)
}

// IsEnabled reports whether any sink would accept an event
func (l *logger) IsEnabled() bool {
	level := l.l.GetLevel()
	return level != slog.LevelNone && level < slog.LevelFatal
}

// SinkNames returns the names of the sinks attached to this logger.
func (l *logger) SinkNames() []string {
	return l.l.SinkNames()
}

func (l *logger) Trace(format string, v ...any) {
	l.l.Log(2, slog.LevelTrace, format, v...)
}

func (l *logger) Debug(format string, v ...any) {
	l.l.Log(2, slog.LevelDebug, format, v...)
}

func (l *logger) Info(format string, v ...any) {
	l.l.Log(2, slog.LevelInfo, format, v...)
}

func (l *logger) Warn(format string, v ...any) {
	l.l.Log(2, slog.LevelWarn, format, v...)
}

func (l *logger) Error(format string, v ...any) {
	l.l.Log(2, slog.LevelError, format, v...)
}

func (l *logger) Critical(format string, v ...any) {
	l.l.Log(2, slog.LevelCritical, format, v...)
}
