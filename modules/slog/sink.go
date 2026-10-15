// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

type Format string

const (
	FormatLegacy Format = "legacy" // Gitea's historical text format
	FormatLogfmt Format = "logfmt" // slog.TextHandler
	FormatJSON   Format = "json"   // slog.JSONHandler
)

// sinkState is the shared configuration and identity of one sink. It is owned
// by the Manager and referenced by any number of sinkFrontends (one per logger
// tree). The zero values of Level/StacktraceLevel/Format/BufferLen are
// normalized by newSinkState.
type sinkState struct {
	name     string
	sinkType string
	format   Format

	level *slog.LevelVar

	prefix          string
	flags           Flags
	colorize        bool
	stacktraceLevel Level
	addSource       bool
	bufferLen       int

	backend *sinkBackend
}

func newSinkState(cfg SinkConfig) (*sinkState, error) {
	format := cfg.Format
	if format == "" {
		format = FormatLegacy
	}
	if format != FormatLegacy && format != FormatLogfmt && format != FormatJSON {
		return nil, fmt.Errorf("invalid format %q for log sink %q", format, cfg.Name)
	}
	stacktraceLevel := cfg.StacktraceLevel
	if stacktraceLevel == 0 {
		stacktraceLevel = LevelNone
	}
	bufferLen := cfg.BufferLen
	if bufferLen == 0 {
		bufferLen = 1000
	}
	level := new(slog.LevelVar)
	level.Set(cfg.Level) // zero Level == LevelInfo
	return &sinkState{
		name:            cfg.Name,
		sinkType:        cfg.Type,
		format:          format,
		level:           level,
		prefix:          cfg.Prefix,
		flags:           cfg.Flags,
		colorize:        cfg.Colorize,
		stacktraceLevel: stacktraceLevel,
		addSource:       cfg.AddSource,
		bufferLen:       bufferLen,
		backend:         nil, // set by newSink
	}, nil
}

func (st *sinkState) needsSource() bool {
	if st.format == FormatLegacy {
		return st.flags.Bits()&(Llongfile|Lshortfile|Lfuncname|Lshortfuncname) != 0
	}
	return st.addSource
}

// wantsRawArgs reports whether this sink re-renders printf args (colorized
// legacy), requiring the raw format+args on the record.
func (st *sinkState) wantsRawArgs() bool {
	return st.format == FormatLegacy && st.colorize
}

// sinkItem is one queued unit: either fully formatted bytes (legacy, rendered
// on the caller goroutine) or a cloned record deferred to the sink goroutine.
type sinkItem struct {
	buf  []byte
	rec  *slog.Record
	fmtH slog.Handler // stdlib-format handler that renders rec
}

// sinkBackend owns the queue, the writer goroutine and the io.Writer. It is
// never closed while a racing enqueue can be in flight: stop is signaled via
// the done channel and enqueues select on it, so the worst case is a dropped
// event, never a send-on-closed-channel panic.
type sinkBackend struct {
	name     string
	queue    chan sinkItem
	done     chan struct{}
	exited   chan struct{}
	writer   io.WriteCloser
	pause    func() chan struct{}
	stopOnce sync.Once
}

func newSinkBackend(name string, writer io.WriteCloser, bufferLen int, pause func() chan struct{}) *sinkBackend {
	b := &sinkBackend{
		name:   name,
		queue:  make(chan sinkItem, bufferLen),
		done:   make(chan struct{}),
		exited: make(chan struct{}),
		writer: writer,
		pause:  pause,
	}
	go b.run()
	return b
}

func (b *sinkBackend) enqueue(it sinkItem) {
	select {
	case <-b.done:
	case b.queue <- it:
	default:
		FallbackErrorf("log sink %q queue is full, event dropped", b.name)
	}
}

func (b *sinkBackend) run() {
	defer close(b.exited)
	defer b.writer.Close()
	for {
		select {
		case <-b.done:
			b.drain()
			return
		case it := <-b.queue:
			b.waitPaused()
			b.write(it)
		}
	}
}

// drain writes out what is already queued, best-effort with a deadline so a
// wedged writer cannot stall shutdown indefinitely.
func (b *sinkBackend) drain() {
	deadline := time.After(5 * time.Second)
	for {
		select {
		case it := <-b.queue:
			b.write(it)
		default:
			return
		}
		select {
		case <-deadline:
			FallbackErrorf("log sink %q unable to drain in time, remaining events dropped", b.name)
			return
		default:
		}
	}
}

func (b *sinkBackend) write(it sinkItem) {
	var err error
	if it.rec != nil {
		err = it.fmtH.Handle(context.Background(), *it.rec)
	} else {
		_, err = b.writer.Write(it.buf)
	}
	if err != nil {
		FallbackErrorf("unable to write log message of %q: %v", b.name, err)
	}
}

func (b *sinkBackend) waitPaused() {
	if ch := b.pause(); ch != nil {
		select {
		case <-ch:
		case <-b.done:
		}
	}
}

func (b *sinkBackend) stop() {
	b.stopOnce.Do(func() { close(b.done) })
	select {
	case <-b.exited:
	case <-time.After(2 * time.Second):
		FallbackErrorf("unable to stop log sink %q in time, skip", b.name)
	}
}

// sinkFrontend is the per-tree slog.Handler face of a sink. It is cheap to
// clone so WithAttrs/WithGroup can return new handlers without touching the
// shared backend — slog requires handlers to not mutate under With*.
type sinkFrontend struct {
	st *sinkState

	inner     slog.Handler // stdlib formats: JSON/Text handler bound to the backend writer
	withAttrs []slog.Attr  // legacy format: With-state rendered as key=value suffix
	groups    []string     // legacy format: group names, flattened into the suffix
}

var _ slog.Handler = (*sinkFrontend)(nil)

func newSinkFrontend(st *sinkState) *sinkFrontend {
	f := &sinkFrontend{st: st}
	opts := &slog.HandlerOptions{
		AddSource:   st.addSource,
		ReplaceAttr: renameFatalLevel,
	}
	switch st.format {
	case FormatJSON:
		f.inner = slog.NewJSONHandler(st.backend.writer, opts)
	case FormatLogfmt:
		f.inner = slog.NewTextHandler(st.backend.writer, opts)
	}
	return f
}

// renameFatalLevel renames the stock "ERROR+4" rendering of the custom FATAL
// level in stdlib formats.
func renameFatalLevel(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.LevelKey {
		if lv, ok := a.Value.Any().(slog.Level); ok && lv == LevelFatal {
			a.Value = slog.StringValue(strings.ToUpper(LevelName(LevelFatal)))
		}
	}
	return a
}

func (f *sinkFrontend) Enabled(_ context.Context, level Level) bool {
	return f.enabled(level)
}

func (f *sinkFrontend) enabled(level Level) bool {
	return level >= f.st.level.Level()
}

// handle is the internal dispatch; hi travels beside the record and never
// becomes a record attr
func (f *sinkFrontend) handle(r slog.Record, hi *hiddenInfo) {
	if f.st.format == FormatLegacy {
		// legacy formats eagerly on the caller goroutine: colorized
		// re-rendering must not cross goroutines
		f.st.backend.enqueue(sinkItem{buf: f.st.formatLegacy(&r, f, hi)})
		return
	}
	rc := r.Clone() // formatting is deferred to the sink goroutine
	f.st.backend.enqueue(sinkItem{rec: &rc, fmtH: f.inner})
}

// Handle only serves the slog.Handler interface (the stdlib slog path)
func (f *sinkFrontend) Handle(_ context.Context, r slog.Record) error {
	f.handle(r, nil)
	return nil
}

func (f *sinkFrontend) WithAttrs(attrs []slog.Attr) slog.Handler {
	return f.cloneWithAttrs(attrs)
}

func (f *sinkFrontend) cloneWithAttrs(attrs []slog.Attr) *sinkFrontend {
	nf := *f
	if f.inner != nil {
		nf.inner = f.inner.WithAttrs(attrs)
	} else {
		nf.withAttrs = append(slices.Clone(f.withAttrs), attrs...)
	}
	return &nf
}

func (f *sinkFrontend) WithGroup(name string) slog.Handler {
	return f.cloneWithGroup(name)
}

func (f *sinkFrontend) cloneWithGroup(name string) *sinkFrontend {
	if name == "" {
		return f
	}
	nf := *f
	if f.inner != nil {
		nf.inner = f.inner.WithGroup(name)
	} else {
		nf.groups = append(slices.Clone(f.groups), name)
	}
	return &nf
}

func (f *sinkFrontend) forkWithAttrs(attrs []slog.Attr) child {
	return f.cloneWithAttrs(attrs)
}

func (f *sinkFrontend) forkWithGroup(name string) child {
	return f.cloneWithGroup(name)
}

func (f *sinkFrontend) states() []*sinkState {
	return []*sinkState{f.st}
}
