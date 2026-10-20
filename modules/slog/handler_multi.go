// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"context"
	"log/slog"
	"slices"
)

// child is one node of a logger tree: either a sinkFrontend (printf metadata
// aware) or a plain external slog.Handler (metadata-free).
type child interface {
	slog.Handler
	enabled(level Level) bool
	handle(r slog.Record, hi *hiddenInfo)
	forkWithAttrs(attrs []slog.Attr) child
	forkWithGroup(name string) child
	states() []*sinkState
}

// multiHandler fans a record out to all tree children. Enabled is the OR of
// the children, so a record is built once if any sink wants it; dispatch
// re-checks each child's level so per-sink formatting only happens for sinks
// that pass. The cached hints (needsSource, wantsRawArgs, needsMessage,
// stacktraceLevel, level) are aggregated from the sinks when the tree is built.
type multiHandler struct {
	children []child

	needsSource          bool
	wantsRawArgs         bool
	needsMessage         bool  // some sink renders Record.Message (not colorized legacy)
	stacktraceLevel      Level // minimum StacktraceLevel over all sinks; LevelNone if none
	structuredStackLevel Level // minimum StacktraceLevel over json/logfmt sinks, for the stdlib path
	level                Level // lowest sink level, for GetLevel; LevelNone if no sinks
	buffering            bool  // some child is a bootstrap bufferChild
}

var _ slog.Handler = (*multiHandler)(nil)

func newMultiHandler(frontends []*sinkFrontend) *multiHandler {
	children := make([]child, len(frontends))
	for i, f := range frontends {
		children[i] = f
	}
	return newMultiHandlerChildren(children)
}

func newMultiHandlerChildren(children []child) *multiHandler {
	h := &multiHandler{
		children:             slices.Clone(children),
		stacktraceLevel:      LevelNone,
		structuredStackLevel: LevelNone,
		level:                LevelNone,
	}
	for _, c := range h.children {
		if _, ok := c.(bufferChild); ok {
			h.buffering = true
			// the buffer keeps the rendered message and prefilled source so
			// replayed events are indistinguishable from direct ones
			h.needsMessage = true
			h.needsSource = true
			continue
		}
		states := c.states()
		if len(states) == 0 {
			// plain external handlers render Record.Message and may resolve
			// Record.Source themselves
			h.needsMessage = true
			h.needsSource = true
			continue
		}
		for _, st := range states {
			if st.needsSource() {
				h.needsSource = true
			}
			if st.wantsRawArgs() {
				h.wantsRawArgs = true
			} else {
				h.needsMessage = true
			}
			if st.stacktraceLevel < h.stacktraceLevel {
				h.stacktraceLevel = st.stacktraceLevel
			}
			if (st.format == FormatJSON || st.format == FormatLogfmt) && st.stacktraceLevel < h.structuredStackLevel {
				h.structuredStackLevel = st.stacktraceLevel
			}
			if st.level.Level() < h.level {
				h.level = st.level.Level()
			}
		}
	}
	return h
}

func (h *multiHandler) Enabled(ctx context.Context, level Level) bool {
	for _, c := range h.children {
		if c.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle only serves the slog.Handler interface (the stdlib slog path), whose
// records never carry printf metadata.
func (h *multiHandler) Handle(_ context.Context, r slog.Record) error {
	h.handle(r, nil)
	return nil
}

// handle dispatches printf metadata beside the record, so stdlib-format sinks
// never need to strip anything from it
func (h *multiHandler) handle(r slog.Record, hi *hiddenInfo) {
	for _, f := range h.children {
		if f.enabled(r.Level) {
			f.handle(r, hi)
		}
	}
}

func (h *multiHandler) sinkStates() []*sinkState {
	var sts []*sinkState
	for _, f := range h.children {
		sts = append(sts, f.states()...)
	}
	return sts
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.withAttrs(attrs)
}

func (h *multiHandler) withAttrs(attrs []slog.Attr) *multiHandler {
	nh := *h
	nh.children = slices.Clone(h.children)
	for i, c := range h.children {
		nh.children[i] = c.forkWithAttrs(attrs)
	}
	return &nh
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return h.withGroup(name)
}

func (h *multiHandler) withGroup(name string) *multiHandler {
	if name == "" {
		return h
	}
	nh := *h
	nh.children = slices.Clone(h.children)
	for i, c := range h.children {
		nh.children[i] = c.forkWithGroup(name)
	}
	return &nh
}

// handlerChild adapts a plain external slog.Handler to the child interface.
// It never receives printf metadata and contributes no sinks.
type handlerChild struct {
	h slog.Handler
}

func (c handlerChild) Enabled(ctx context.Context, level Level) bool {
	return c.h.Enabled(ctx, level)
}

func (c handlerChild) enabled(level Level) bool {
	return c.h.Enabled(context.Background(), level)
}

func (c handlerChild) Handle(ctx context.Context, r slog.Record) error {
	return c.h.Handle(ctx, r)
}

func (c handlerChild) handle(r slog.Record, _ *hiddenInfo) {
	_ = c.h.Handle(context.Background(), r)
}

func (c handlerChild) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handlerChild{c.h.WithAttrs(attrs)}
}

func (c handlerChild) WithGroup(name string) slog.Handler {
	return handlerChild{c.h.WithGroup(name)}
}

func (c handlerChild) forkWithAttrs(attrs []slog.Attr) child {
	return handlerChild{c.h.WithAttrs(attrs)}
}

func (c handlerChild) forkWithGroup(name string) child {
	return handlerChild{c.h.WithGroup(name)}
}

func (c handlerChild) states() []*sinkState { return nil }
