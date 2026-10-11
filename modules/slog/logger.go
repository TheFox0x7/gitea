// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"context"
	"log/slog"
	"runtime"
	"slices"
	"sync/atomic"
	"time"
)

// Logger is a named logger: a facade over a *slog.Logger with a per-name
// handler tree that can be swapped by the Manager on reconfiguration.
type Logger struct {
	mgr  *Manager
	name string

	handler atomic.Pointer[multiHandler]
	slogger atomic.Pointer[slog.Logger]
}

func newLogger(mgr *Manager, name string) *Logger {
	l := &Logger{mgr: mgr, name: name}
	l.handler.Store(newMultiHandler(nil))
	return l
}

func (l *Logger) Name() string {
	return l.name
}

// Slog returns this logger as a *slog.Logger for structured, context-aware
// logging. It carries a "logger" attr with the logger's name. Re-fetch it
// after a reconfiguration: a previously returned value stays bound to the
// handler tree it was created with.
func (l *Logger) Slog() *slog.Logger {
	if sl := l.slogger.Load(); sl != nil {
		return sl
	}
	sl := slog.New(l.handler.Load()).With(slog.String("logger", l.name))
	// only fill an empty slot: swapHandler always stores a fresh pair, so a
	// winning CAS here can never cache a stale handler
	l.slogger.CompareAndSwap(nil, sl)
	return l.slogger.Load()
}

func (l *Logger) swapHandler(h *multiHandler) {
	l.handler.Store(h)
	l.slogger.Store(slog.New(h).With(slog.String("logger", l.name)))
}

func (l *Logger) GetLevel() Level {
	return l.handler.Load().level
}

func (l *Logger) LevelEnabled(level Level) bool {
	return l.handler.Load().Enabled(context.Background(), level)
}

// Log logs a printf-style message at the given level. skip is the number of
// stack frames to skip for caller resolution, 1 meaning the caller of Log.
//
// Deprecated: printf-style logging produces no structured output; use Slog() instead.
func (l *Logger) Log(skip int, level Level, format string, args ...any) {
	l.logPrintf(skip, level, format, args)
}

// Deprecated: use Slog().Debug(...) instead.
func (l *Logger) Trace(format string, args ...any) {
	l.logPrintf(1, LevelTrace, format, args)
}

// Deprecated: use Slog().Debug(...) instead.
func (l *Logger) Debug(format string, args ...any) {
	l.logPrintf(1, LevelDebug, format, args)
}

// Deprecated: use Slog().Info(...) instead.
func (l *Logger) Info(format string, args ...any) {
	l.logPrintf(1, LevelInfo, format, args)
}

// Deprecated: use Slog().Warn(...) instead.
func (l *Logger) Warn(format string, args ...any) {
	l.logPrintf(1, LevelWarn, format, args)
}

// Deprecated: use Slog().Error(...) instead.
func (l *Logger) Error(format string, args ...any) {
	l.logPrintf(1, LevelError, format, args)
}

// Deprecated: use Slog().Error(...) instead.
func (l *Logger) Critical(format string, args ...any) {
	l.logPrintf(1, LevelCritical, format, args)
}

// Fatal logs at FATAL, flushes the manager and exits the process. If no sink
// accepted the event (before Apply, or level filtered), the message is echoed
// to stderr so an early fatal is never lost.
// Deprecated: use Slog().Log(ctx, LevelFatal, ...) plus explicit exit handling instead.
func (l *Logger) Fatal(format string, args ...any) {
	if !l.logPrintf(1, LevelFatal, format, args) {
		FallbackErrorf(format, args...)
	}
	l.mgr.Close()
	OsExiter(1)
}

// logPrintf is the printf pipeline: level gate (one atomic OR check), lazy PC
// capture, arg resolution on the caller goroutine (masking, LogStringer,
// ColoredValue), then dispatch. Raw format+args and the stacktrace travel
// beside the record as *hiddenInfo, never as record attrs. It reports whether
// any sink accepted the event.
func (l *Logger) logPrintf(skip int, level Level, format string, args []any) bool {
	h := l.handler.Load()
	if !h.Enabled(context.Background(), level) {
		return false
	}

	var pc uintptr
	if h.needsSource {
		var pcs [1]uintptr
		// skip [runtime.Callers, logPrintf, Log] then the user-supplied skip
		runtime.Callers(skip+2, pcs[:])
		pc = pcs[0]
	}

	// resolve args on this goroutine; clone before the first in-place write
	// since a caller may pass a reused slice to Log explicitly
	mutated := false
	for i, v := range args {
		var nv any
		if cv, ok := v.(*ColoredValue); ok {
			if ls := asLogStringer(cv.v); ls != nil {
				cv.v = logStringFormatter{v: ls} // mutate through the pointer
			}
			continue
		}
		if ls := asLogStringer(v); ls != nil {
			nv = logStringFormatter{v: ls}
		} else if s, ok := v.(string); ok {
			if masked, changed := protectSensitiveInfo(s); changed {
				nv = masked
			}
		}
		if nv != nil {
			if !mutated {
				args = slices.Clone(args)
				mutated = true
			}
			args[i] = nv
		}
	}

	// when every sink re-renders colorized from the raw args, the uncolored
	// pre-render would be discarded, so leave the message empty
	var msg string
	if h.needsMessage {
		msg = colorSprintf(false, format, args...)
	}
	rec := slog.NewRecord(time.Now(), level, msg, pc)

	var hi *hiddenInfo
	if h.wantsRawArgs {
		hi = &hiddenInfo{format: format, args: args}
	}
	if level >= h.stacktraceLevel {
		if hi == nil {
			hi = &hiddenInfo{}
		}
		hi.stacktrace = Stack(skip + 1)
	}
	if h.needsSource {
		if hi == nil {
			hi = &hiddenInfo{}
		}
		// resolve once, all legacy sinks share it
		hi.prefillSource(&rec)
	}
	h.handle(rec, hi)
	return true
}
