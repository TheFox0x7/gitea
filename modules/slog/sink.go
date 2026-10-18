// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
)

type Format string

const (
	FormatLegacy Format = "legacy" // Gitea's historical text format
	FormatLogfmt Format = "logfmt" // slog.TextHandler
	FormatJSON   Format = "json"   // slog.JSONHandler
)

// maxFailedLineDump bounds the stderr dump of a failed line, so a full stderr
// pipe cannot stall the caller.
const maxFailedLineDump = 32 * 1024

// sinkState is the shared configuration and identity of one sink. It is owned
// by the Manager and referenced by any number of sinkFrontends (one per logger
// tree). The zero values of Level/StacktraceLevel/Format are normalized by
// newSinkState.
type sinkState struct {
	name      string
	sinkType  string
	format    Format
	writerKey string

	level *slog.LevelVar

	prefix          string
	flags           Flags
	colorize        bool
	stacktraceLevel Level
	addSource       bool

	writer io.WriteCloser // the raw writer; frontends wrap it in the pause gate

	release func() // drops this sink's writer reference; the last one closes it
}

// releaseWriter must be called after the sink is no longer reachable from any
// logger tree (a racing in-flight write of an orphaned sink surfaces as a
// write error and lands on stderr).
func (st *sinkState) releaseWriter() {
	if st.release != nil {
		st.release()
	}
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

// sinkFrontend is the per-tree slog.Handler face of a sink. Writes happen
// inline on the caller goroutine through the pause-gated writer; on a write
// error the formatted line is dumped to stderr so no event is silently lost.
type sinkFrontend struct {
	st *sinkState

	pause *pauseGate

	opts *slog.HandlerOptions
	// writer is st.writer gated by the pause gate; inner renders stdlib
	// formats (JSON/logfmt) straight into it
	writer io.Writer
	inner  slog.Handler
	// shapers replay this frontend's With-state onto a fresh handler, used to
	// re-render a failed line into a buffer on the error path
	shapers []func(slog.Handler) slog.Handler
	// legacy With-state: rendered as a key=value suffix (see format_legacy)
	withAttrs []slog.Attr
	groups    []string
}

var _ slog.Handler = (*sinkFrontend)(nil)

func newSinkFrontend(m *Manager, st *sinkState) *sinkFrontend {
	f := &sinkFrontend{
		st:    st,
		pause: &m.pause,
		opts:  &slog.HandlerOptions{AddSource: st.addSource, ReplaceAttr: renameFatalLevel},
	}
	f.writer = &gateWriter{w: st.writer, pause: &m.pause}
	switch st.format {
	case FormatJSON:
		f.inner = slog.NewJSONHandler(f.writer, f.opts)
	case FormatLogfmt:
		f.inner = slog.NewTextHandler(f.writer, f.opts)
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
	var err error
	var line []byte
	if f.st.format == FormatLegacy {
		line = f.st.formatLegacy(&r, f, hi)
		_, err = f.writer.Write(line)
	} else {
		err = f.inner.Handle(context.Background(), r)
	}
	if err != nil {
		f.dumpFailedLine(&r, line, err)
	}
}

// dumpFailedLine preserves a line the sink could not write (Caddy's net-writer
// behavior) by re-rendering it to stderr, bounded.
func (f *sinkFrontend) dumpFailedLine(r *slog.Record, legacyLine []byte, err error) {
	dump := legacyLine
	if dump == nil {
		var buf bytes.Buffer
		var h slog.Handler = slog.NewJSONHandler(&buf, f.opts)
		if f.st.format == FormatLogfmt {
			h = slog.NewTextHandler(&buf, f.opts)
		}
		for _, shape := range f.shapers {
			h = shape(h)
		}
		if e := h.Handle(context.Background(), *r); e == nil {
			dump = buf.Bytes()
		}
	}
	if len(dump) > maxFailedLineDump {
		dump = dump[:maxFailedLineDump]
	}
	_, _ = os.Stderr.Write(dump)
	FallbackErrorf("unable to write log message of %q: %v", f.st.name, err)
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
	nf.shapers = append(slices.Clone(f.shapers), func(h slog.Handler) slog.Handler {
		return h.WithAttrs(attrs)
	})
	nf.withAttrs = append(slices.Clone(f.withAttrs), attrs...)
	if f.inner != nil {
		nf.inner = f.inner.WithAttrs(attrs)
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
	nf.shapers = append(slices.Clone(f.shapers), func(h slog.Handler) slog.Handler {
		return h.WithGroup(name)
	})
	nf.groups = append(slices.Clone(f.groups), name)
	if f.inner != nil {
		nf.inner = f.inner.WithGroup(name)
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
