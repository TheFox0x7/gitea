// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"context"
	"log/slog"
	"slices"
	"sync"
)

const defaultBufferLimit = 10000

// bufferedRecord is one event captured before its logger was configured.
type bufferedRecord struct {
	loggerName string
	rec        slog.Record
	hi         *hiddenInfo
	attrs      []slog.Attr
	groups     []string
}

// recordBuffer holds pre-configuration events, bounded: once full the oldest
// entries are dropped and counted, and the count is reported on flush.
type recordBuffer struct {
	mu      sync.Mutex
	entries []bufferedRecord
	dropped int
	limit   int // zero means defaultBufferLimit
}

func (b *recordBuffer) add(e bufferedRecord) {
	b.mu.Lock()
	defer b.mu.Unlock()
	limit := b.limit
	if limit == 0 {
		limit = defaultBufferLimit
	}
	if len(b.entries) >= limit {
		b.entries = append(b.entries[:0], b.entries[1:]...)
		b.dropped++
	}
	b.entries = append(b.entries, e)
}

func (b *recordBuffer) takeAll() ([]bufferedRecord, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entries, dropped := b.entries, b.dropped
	b.entries, b.dropped = nil, 0
	return entries, dropped
}

func (b *recordBuffer) restore(entries []bufferedRecord) {
	if len(entries) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, entries...)
}

// bufferChild is the initial child of every logger tree: until the logger is
// configured by an Apply, it captures events into the manager's buffer, so
// early startup logs replay into the real sinks instead of being lost. The
// buffer is not a sink: it never counts as "accepted" for the Fatal echo.
type bufferChild struct {
	l      *logger
	attrs  []slog.Attr
	groups []string
}

var _ child = bufferChild{}

func (c bufferChild) Enabled(_ context.Context, level Level) bool { return c.enabled(level) }

func (c bufferChild) enabled(level Level) bool {
	return !c.l.configured.Load() && level >= Level(c.l.mgr.bootstrapLevel.Load())
}

func (c bufferChild) Handle(_ context.Context, r slog.Record) error {
	c.handle(r, nil)
	return nil
}

func (c bufferChild) handle(r slog.Record, hi *hiddenInfo) {
	if !c.enabled(r.Level) {
		return
	}
	// attrs/groups are tree-level With-state (a handler-derived tree fork);
	// stdlib With-state rides the record instead, so this is empty there
	c.l.mgr.buffer.add(bufferedRecord{
		loggerName: c.l.name,
		rec:        r.Clone(),
		hi:         hi,
		attrs:      c.attrs,
		groups:     c.groups,
	})
}

func (c bufferChild) WithAttrs(attrs []slog.Attr) slog.Handler { return c.cloneWithAttrs(attrs) }

func (c bufferChild) cloneWithAttrs(attrs []slog.Attr) bufferChild {
	n := c
	n.attrs = append(slices.Clone(c.attrs), attrs...)
	return n
}

func (c bufferChild) WithGroup(name string) slog.Handler { return c.cloneWithGroup(name) }

func (c bufferChild) cloneWithGroup(name string) bufferChild {
	if name == "" {
		return c
	}
	n := c
	n.groups = append(slices.Clone(c.groups), name)
	return n
}

func (c bufferChild) forkWithAttrs(attrs []slog.Attr) child { return c.cloneWithAttrs(attrs) }

func (c bufferChild) forkWithGroup(name string) child { return c.cloneWithGroup(name) }

func (c bufferChild) states() []*sinkState { return nil }
