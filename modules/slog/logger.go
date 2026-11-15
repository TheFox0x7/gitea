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

// logger is the internal named-logger state: the swappable handler tree plus
// the stable stdlib logger handed to callers (see dynamicHandler). It is not
// exposed: GetLogger returns the stdlib logger directly.
type logger struct {
	mgr  *Manager
	name string

	handler atomic.Pointer[multiHandler]

	std atomic.Pointer[slog.Logger]

	configured atomic.Bool // false until an Apply gives this logger real sinks
}

func newLogger(mgr *Manager, name string) *logger {
	l := &logger{mgr: mgr, name: name}
	l.handler.Store(newMultiHandlerChildren([]child{bufferChild{l: l}}))
	l.std.Store(slog.New(&dynamicHandler{l: l}).With(slog.String("logger", name)))
	return l
}

func (l *logger) stdLogger() *slog.Logger {
	return l.std.Load()
}

func (l *logger) swapHandler(h *multiHandler) {
	l.handler.Store(h)
}

// GetLevel returns the lowest sink level of the logger's current tree,
// computed live so runtime SetSinkLevel changes are reflected; LevelNone when
// the logger has no sinks (still buffering).
func (l *logger) GetLevel() Level {
	level := LevelNone
	for _, st := range l.handler.Load().sinkStates() {
		if lv := st.level.Level(); lv < level {
			level = lv
		}
	}
	return level
}

func (l *logger) LevelEnabled(level Level) bool {
	return l.handler.Load().Enabled(context.Background(), level)
}

// dynamicHandler is the bridge between the stdlib logger and the swappable
// handler tree: Handle/Enabled always read the current tree, so a stdlib
// logger handed to a caller (or to a downstream library) stays valid for the
// process lifetime, across reconfigurations. With-state is replayed onto each
// tree generation and cached per generation.
type dynamicHandler struct {
	l      *logger
	attrs  []slog.Attr
	groups []string
	cache  atomic.Pointer[derivedHandler]
}

type derivedHandler struct {
	tree *multiHandler
	h    slog.Handler
}

var _ slog.Handler = (*dynamicHandler)(nil)

func (h *dynamicHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.current().Enabled(ctx, level)
}

func (h *dynamicHandler) Handle(ctx context.Context, r slog.Record) error {
	tree := h.l.handler.Load()
	// the stdlib has no stack mechanism: inject the structured field here when
	// a json/logfmt sink configured a stacktrace level. +1 skips the wrapper
	// frame of the entry method (slog.(*Logger).Info/…), so the first captured
	// frame is the caller of the logging call.
	if lvl := tree.structuredStackLevel; lvl != LevelNone && r.Level >= lvl {
		r.AddAttrs(slog.String("stacktrace", formatStackFrames(captureStack(4))))
	}
	return h.currentFor(tree).Handle(ctx, r)
}

func (h *dynamicHandler) current() slog.Handler {
	return h.currentFor(h.l.handler.Load())
}

func (h *dynamicHandler) currentFor(tree *multiHandler) slog.Handler {
	if c := h.cache.Load(); c != nil && c.tree == tree {
		return c.h
	}
	var hh slog.Handler = tree
	for _, g := range h.groups {
		hh = hh.WithGroup(g)
	}
	if len(h.attrs) > 0 {
		hh = hh.WithAttrs(h.attrs)
	}
	h.cache.Store(&derivedHandler{tree: tree, h: hh})
	return hh
}

func (h *dynamicHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := &dynamicHandler{l: h.l, groups: h.groups}
	n.attrs = append(slices.Clone(h.attrs), attrs...)
	return n
}

func (h *dynamicHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := &dynamicHandler{l: h.l, attrs: h.attrs}
	n.groups = append(slices.Clone(h.groups), name)
	return n
}

// Log logs a printf-style message at the given level. skip is the number of
// stack frames to skip for caller resolution, 1 meaning the caller of Log.
//
// Deprecated: printf-style logging produces no structured output; use the
// stdlib logger returned by GetLogger instead.
//
//go:noinline
func (l *logger) Log(skip int, level Level, format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(skip+2, pcs[:])
	l.logPrintf(pcs[0], level, format, args, "", skip+2)
}

// LogPC logs with an explicitly captured PC, for compatibility wrappers whose
// own frames must not skew caller attribution. The stack trace starts one
// frame above the caller of this method.
//
//go:noinline
func (l *logger) LogPC(pc uintptr, level Level, stackSkip int, format string, args ...any) {
	l.logPrintf(pc, level, format, args, "", stackSkip)
}

// Deprecated: printf-style logging produces no structured output.
//
//go:noinline
func (l *logger) Trace(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelTrace, format, args, "", 1)
}

// Deprecated: printf-style logging produces no structured output.
//
//go:noinline
func (l *logger) Debug(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelDebug, format, args, "", 1)
}

// Deprecated: printf-style logging produces no structured output.
func (l *logger) Info(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelInfo, format, args, "", 1)
}

// Deprecated: printf-style logging produces no structured output.
//
//go:noinline
func (l *logger) Warn(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelWarn, format, args, "", 1)
}

// Deprecated: printf-style logging produces no structured output.
//
//go:noinline
func (l *logger) Error(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelError, format, args, "", 1)
}

// Deprecated: printf-style logging produces no structured output.
//
//go:noinline
func (l *logger) Critical(format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // 0=Callers, 1=this fn, 2=caller (inline-stable)
	l.logPrintf(pcs[0], LevelCritical, format, args, "", 1)
}

// Fatal logs at FATAL, flushes the manager and exits the process. If no sink
// accepted the event (before Apply, or level filtered), the message is echoed
// to stderr so an early fatal is never lost.
// Deprecated: handle fatal errors explicitly instead.
//
//go:noinline
func (l *logger) Fatal(format string, args ...any) {
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
func (l *logger) LogWithCaller(skip int, level Level, caller, format string, args ...any) {
	var pcs [1]uintptr
	runtime.Callers(skip+2, pcs[:])
	l.logPrintf(pcs[0], level, format, args, caller, skip+2)
}

// LogWithCallerPC logs with a prefilled caller name and an explicitly captured
// PC, for compatibility wrappers whose own frames must not skew attribution.
//
//go:noinline
func (l *logger) LogWithCallerPC(pc uintptr, level Level, caller string, stackSkip int, format string, args ...any) {
	l.logPrintf(pc, level, format, args, caller, stackSkip)
}

// logPrintf is the printf pipeline: level gate (one atomic OR check), arg
// resolution on the caller goroutine (masking, LogStringer, ColoredValue),
// then dispatch. The pc is used for caller/source attribution; stackSkip
// positions the stack trace relative to logPrintf. Raw format+args and the
// stacktrace travel beside the record as *hiddenInfo, never as record attrs.
// It reports whether any sink accepted the event.
func (l *logger) logPrintf(pc uintptr, level Level, format string, args []any, prefillCaller string, stackSkip int) bool {
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
			// wrap LogStringer inside ColoredValue without mutating the
			// caller's value: replace with a fresh ColoredValue
			if ls := asLogStringer(cv.v); ls != nil {
				nv = NewColoredValue(logStringFormatter{v: ls}, cv.colors...)
			}
		} else if ls := asLogStringer(v); ls != nil {
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
		// +1 for logPrintf, +1 for the entry method (Error/LogPC/…): the first
		// captured frame is the caller of the logging method
		hi.stack = captureStack(stackSkip + 2)
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
