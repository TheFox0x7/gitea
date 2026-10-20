// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package log provides Gitea's historical logging API as a compatibility shim
// over the slog-based engine (modules/slog). All functions are thin forwards;
// new code should use modules/slog directly (slog.GetLogger returns a stdlib
// *slog.Logger).
package log

import (
	"runtime"

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

// logger forwards the legacy Logger API to the slog engine. Public methods
// are //go:noinline and capture their caller PC themselves, so caller
// attribution never depends on skip arithmetic through this shim.
type logger struct {
	p *slog.PrintfLogger
}

var _ Logger = (*logger)(nil)

//go:noinline
func (l *logger) Log(skip int, event *Event, format string, v ...any) {
	level, pc := eventLevelPC(event)
	l.p.LogWithCallerPC(pc, level, eventCaller(event), skip+2, format, v...)
}

func (l *logger) GetLevel() Level {
	return l.p.GetLevel()
}

func (l *logger) LevelEnabled(level Level) bool {
	return l.p.LevelEnabled(level)
}

// IsEnabled reports whether any sink would accept an event
func (l *logger) IsEnabled() bool {
	level := l.p.GetLevel()
	return level != NONE && level < FATAL
}

//go:noinline
func (l *logger) Trace(format string, v ...any) {
	l.p.LogPC(shimPC(), TRACE, 3, format, v...)
}

//go:noinline
func (l *logger) Debug(format string, v ...any) {
	l.p.LogPC(shimPC(), DEBUG, 3, format, v...)
}

//go:noinline
func (l *logger) Info(format string, v ...any) {
	l.p.LogPC(shimPC(), INFO, 3, format, v...)
}

//go:noinline
func (l *logger) Warn(format string, v ...any) {
	l.p.LogPC(shimPC(), WARN, 3, format, v...)
}

//go:noinline
func (l *logger) Error(format string, v ...any) {
	l.p.LogPC(shimPC(), ERROR, 3, format, v...)
}

//go:noinline
func (l *logger) Critical(format string, v ...any) {
	l.p.LogPC(shimPC(), CRITICAL, 3, format, v...)
}

// shimPC returns the PC of the caller of the calling shim function. The
// calling function is //go:noinline, so the physical frame counting is
// deterministic.
func shimPC() uintptr {
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // 0=Callers, 1=shimPC, 2=shim fn, 3=caller
	return pcs[0]
}

func eventLevelPC(event *Event) (Level, uintptr) {
	if event == nil {
		return INFO, 0
	}
	return event.Level, event.PC
}

func eventCaller(event *Event) string {
	if event == nil {
		return ""
	}
	return event.Caller
}
