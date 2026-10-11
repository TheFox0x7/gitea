// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"context"
	"log/slog"
	"slices"
)

// multiHandler fans a record out to all sink frontends. Enabled is the OR of
// the children, so a record is built once if any sink wants it; dispatch
// re-checks each child's level so per-sink formatting only happens for sinks
// that pass. The cached hints (needsSource, wantsRawArgs, needsMessage,
// stacktraceLevel, level) are aggregated from the sinks when the tree is built.
type multiHandler struct {
	children []*sinkFrontend

	needsSource     bool
	wantsRawArgs    bool
	needsMessage    bool  // some sink renders Record.Message (not colorized legacy)
	stacktraceLevel Level // minimum legacy StacktraceLevel; LevelNone if none
	level           Level // lowest sink level, for GetLevel; LevelNone if no sinks
}

var _ slog.Handler = (*multiHandler)(nil)

func newMultiHandler(frontends []*sinkFrontend) *multiHandler {
	h := &multiHandler{
		children:        make([]*sinkFrontend, len(frontends)),
		stacktraceLevel: LevelNone,
		level:           LevelNone,
	}
	for i, f := range frontends {
		h.children[i] = f
		if f.st.needsSource() {
			h.needsSource = true
		}
		if f.st.wantsRawArgs() {
			h.wantsRawArgs = true
		} else {
			h.needsMessage = true
		}
		if f.st.format == FormatLegacy && f.st.stacktraceLevel < h.stacktraceLevel {
			h.stacktraceLevel = f.st.stacktraceLevel
		}
		if f.st.level.Level() < h.level {
			h.level = f.st.level.Level()
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
	sts := make([]*sinkState, len(h.children))
	for i, f := range h.children {
		sts[i] = f.st
	}
	return sts
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := *h
	nh.children = slices.Clone(h.children)
	for i, c := range h.children {
		nh.children[i] = c.cloneWithAttrs(attrs)
	}
	return &nh
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := *h
	nh.children = slices.Clone(h.children)
	for i, c := range h.children {
		nh.children[i] = c.cloneWithGroup(name)
	}
	return &nh
}
