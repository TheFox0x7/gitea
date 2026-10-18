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

	configured atomic.Bool // false until an Apply gives this logger real sinks
}

func newLogger(mgr *Manager, name string) *Logger {
	l := &Logger{mgr: mgr, name: name}
	l.handler.Store(newMultiHandlerChildren([]child{bufferChild{l: l}}))
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
	sl := newSlogLogger(l.handler.Load(), l.name)
	// only fill an empty slot: swapHandler always stores a fresh pair, so a
	// winning CAS here can never cache a stale handler
	l.slogger.CompareAndSwap(nil, sl)
	return l.slogger.Load()
}

// newSlogLogger wraps the tree with the stack injector when a structured sink
// configured a stacktrace level (the stdlib has no stack mechanism).
func newSlogLogger(h *multiHandler, name string) *slog.Logger {
	var handler slog.Handler = h
	if h.structuredStackLevel != LevelNone {
		handler = &stackHandler{inner: h, level: h.structuredStackLevel}
	}
	return slog.New(handler).With(slog.String("logger", name))
}

func (l *Logger) swapHandler(h *multiHandler) {
	l.handler.Store(h)
	l.slogger.Store(newSlogLogger(h, l.name))
}

func (l *Logger) GetLevel() Level {
	return l.handler.Load().level
}

// SinkNames returns the names of the sinks attached to this logger.
func (l *Logger) SinkNames() []string {
	return l.handler.Load().sinkNames()
}

func (l *Logger) LevelEnabled(level Level) bool {
	return l.handler.Load().Enabled(context.Background(), level)
}

// Log logs a printf-style message at the given level. skip is the number of
// stack frames to skip for caller resolution, 1 meaning the caller of Log.
//
// Deprecated: printf-style logging produces no structured output; use Slog() instead.
//
//go:noinline
func (l *Logger) Log(skip int, level Level, format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(skip+2, pcs[:])
	l.logPrintf(pcs[0], level, format, args, "", skip+2)
}

// LogPC logs with an explicitly captured PC, for compatibility wrappers whose
// own frames must not skew caller attribution. The stack trace starts one
// frame above the caller of this method.
//
//go:noinline
func (l *Logger) LogPC(pc uintptr, level Level, stackSkip int, format string, args ...any) {
	l.logPrintf(pc, level, format, args, "", stackSkip)
}

// Deprecated: use Slog().Debug(...) instead.
//
//go:noinline
func (l *Logger) Trace(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelTrace, format, args, "", 1)
}

// Deprecated: use Slog().Debug(...) instead.
//
//go:noinline
func (l *Logger) Debug(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelDebug, format, args, "", 1)
}

// Deprecated: use Slog().Info(...) instead.
func (l *Logger) Info(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelInfo, format, args, "", 1)
}

// Deprecated: use Slog().Warn(...) instead.
//
//go:noinline
func (l *Logger) Warn(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelWarn, format, args, "", 1)
}

// Deprecated: use Slog().Error(...) instead.
//
//go:noinline
func (l *Logger) Error(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelError, format, args, "", 1)
}

// Deprecated: use Slog().Error(...) instead.
//
//go:noinline
func (l *Logger) Critical(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelCritical, format, args, "", 1)
}

// Fatal logs at FATAL, flushes the manager and exits the process. If no sink
// accepted the event (before Apply, or level filtered), the message is echoed
// to stderr so an early fatal is never lost.
// Deprecated: use Slog().Log(ctx, LevelFatal, ...) plus explicit exit handling instead.
//
//go:noinline
func (l *Logger) Fatal(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	accepted := l.logPrintf(pcs[0], LevelFatal, format, args, "", 2)
	// the bootstrap buffer accepts events but is not a sink: an early fatal
	// must still be echoed to stderr, since Close discards the buffer
	if !accepted || len(l.handler.Load().sinkStates()) == 0 {
		FallbackErrorf(format, args...)
	}
	l.mgr.Close()
	OsExiter(1)
}

// LogWithCaller logs a printf-style message with a prefilled caller name
// (e.g. a route pattern) overriding caller resolution.
//
//go:noinline
func (l *Logger) LogWithCaller(skip int, level Level, caller, format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(skip+2, pcs[:])
	l.logPrintf(pcs[0], level, format, args, caller, skip+2)
}

// LogWithCallerPC logs with a prefilled caller name and an explicitly captured
// PC, for compatibility wrappers whose own frames must not skew attribution.
//
//go:noinline
func (l *Logger) LogWithCallerPC(pc uintptr, level Level, caller string, stackSkip int, format string, args ...any) {
	l.logPrintf(pc, level, format, args, caller, stackSkip)
}

// logPrintf is the printf pipeline: level gate (one atomic OR check), arg
// resolution on the caller goroutine (masking, LogStringer, ColoredValue),
// then dispatch. The pc is used for caller/source attribution; stackSkip
// positions the stack trace relative to logPrintf. Raw format+args and the
// stacktrace travel beside the record as *hiddenInfo, never as record attrs.
// It reports whether any sink accepted the event.
func (l *Logger) logPrintf(pc uintptr, level Level, format string, args []any, prefillCaller string, stackSkip int) bool {
	h := l.handler.Load()
	if !h.Enabled(context.Background(), level) {
		return false
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
	if prefillCaller != "" {
		if hi == nil {
			hi = &hiddenInfo{}
		}
		hi.caller = prefillCaller
	}
	if level >= h.stacktraceLevel {
		if hi == nil {
			hi = &hiddenInfo{}
		}
		hi.stack = captureStack(stackSkip + 1)
	}
	if h.needsSource && (hi == nil || hi.caller == "") {
		if hi == nil {
			hi = &hiddenInfo{}
		}
		// resolve once, all legacy sinks share it
		hi.prefillSource(&rec)
	}
	h.handle(rec, hi)
	return true
}
