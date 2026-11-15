// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"fmt"
	"io"
	"time"
)

// SinkTypeDiscard is the built-in sink type that drops every record (the
// sink-level equivalent of slog.DiscardHandler).
const SinkTypeDiscard = "discard"

// ConsoleSinkOptions configures a console sink.
type ConsoleSinkOptions struct {
	Stderr bool
}

// FileSinkOptions configures a file sink. Zero values fall back to the
// rotatingfilewriter defaults (size limit 1<<28, daily rotation, keep 7 days,
// compress).
type FileSinkOptions struct {
	FileName         string
	MaxSize          int64
	LogRotate        bool
	DailyRotate      bool
	MaxDays          int
	Compress         bool
	CompressionLevel int
}

// ConnSinkOptions configures a network sink. DialTimeout defaults to 30s and
// WriteTimeout to 60s (TCP only); zero means "use the default", and a negative
// value disables the deadline. Reconnection is always automatic and
// rate-limited (see connWriter).
type ConnSinkOptions struct {
	Addr         string
	Protocol     string // tcp (default), udp, unix
	DialTimeout  time.Duration
	WriteTimeout time.Duration
}

// SinkConfig configures one sink. Name deduplicates sinks within one Apply
// call, so multiple loggers can share one sink; Type is "console", "file" or
// "conn"; Format defaults to legacy.
// Level defaults to info, StacktraceLevel to none.
// Colorize must be set explicitly (use CanColorStdout /
// CanColorStderr for console sinks).
type SinkConfig struct {
	Name string
	Type string

	Format          Format
	Level           Level
	StacktraceLevel Level
	Flags           Flags
	Prefix          string
	Colorize        bool
	AddSource       bool

	Console *ConsoleSinkOptions
	File    *FileSinkOptions
	Conn    *ConnSinkOptions
}

// LoggerConfig wires a named logger to sinks by config; the same SinkConfig
// Name in multiple loggers refers to one shared sink.
type LoggerConfig struct {
	Name  string
	Sinks []SinkConfig
}

// Config is a full logging configuration.
type Config struct {
	Loggers []LoggerConfig
}

// StderrConfig returns a minimal configuration writing the given logger to
// stderr in legacy format, colored when supported.
func StderrConfig(loggerName string, level Level) Config {
	return Config{Loggers: []LoggerConfig{{
		Name: loggerName,
		Sinks: []SinkConfig{{
			Name:     "console-stderr",
			Type:     "console",
			Level:    level,
			Format:   FormatLegacy,
			Colorize: CanColorStderr,
			Console:  &ConsoleSinkOptions{Stderr: true},
		}},
	}}}
}

// withPreservedHandlers returns newTree with the external handlers (AddHandler)
// of oldTree carried over: they are not sinks and must survive reconfiguration.
func withPreservedHandlers(oldTree, newTree *multiHandler) *multiHandler {
	children := make([]child, 0, len(oldTree.children)+len(newTree.children))
	for _, c := range oldTree.children {
		if _, ok := c.(handlerChild); ok {
			children = append(children, c)
		}
	}
	return newMultiHandlerChildren(append(children, newTree.children...))
}

// Apply validates and installs the configuration on the manager.
func (m *Manager) Apply(cfg Config) error {
	return cfg.provision(m, nil)
}

// apply is the testable variant with injectable sink writers.
func (m *Manager) apply(cfg Config, sinkWriters map[string]io.WriteCloser) error {
	return cfg.provision(m, sinkWriters)
}

// provision validates the configuration and installs it on the manager:
// sinks are built fresh, the listed loggers' trees are swapped atomically,
// and sinks no longer referenced by any logger are released. Loggers not
// listed keep their current configuration (unconfigured ones keep buffering).
func (cfg *Config) provision(m *Manager, sinkWriters map[string]io.WriteCloser) error {
	// validate first, so a bad config doesn't half-apply
	for _, lc := range cfg.Loggers {
		for _, sc := range lc.Sinks {
			if sc.Name == "" {
				return fmt.Errorf("log logger %q has a sink with empty name", lc.Name)
			}
			switch sc.Type {
			case "console", "file", "conn", "discard":
			default:
				return fmt.Errorf("invalid log sink type %q for sink %q", sc.Type, sc.Name)
			}
			if sc.Format != "" && sc.Format != FormatLegacy && sc.Format != FormatLogfmt && sc.Format != FormatJSON && sc.Format != FormatCLF {
				return fmt.Errorf("invalid log sink format %q for sink %q", sc.Format, sc.Name)
			}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// phase 1: build all sinks, nothing is modified yet on error
	built := map[string]*sinkState{}
	trees := make(map[string]*multiHandler, len(cfg.Loggers))
	for _, lc := range cfg.Loggers {
		frontends := make([]*sinkFrontend, 0, len(lc.Sinks))
		for _, sc := range lc.Sinks {
			st := built[sc.Name]
			if st == nil {
				var err error
				if w := sinkWriters[sc.Name]; w != nil {
					st, err = m.newSinkWithWriter(sc, w)
				} else {
					st, err = m.newSink(sc)
				}
				if err != nil {
					for _, s := range built {
						s.releaseWriter()
					}
					return err
				}
				built[sc.Name] = st
			}
			frontends = append(frontends, newSinkFrontend(st))
		}
		trees[lc.Name] = newMultiHandler(frontends)
	}

	// phase 2: swap the trees, then release sinks no tree references anymore.
	// listed loggers leave the bootstrap buffer; the buffer itself replays
	// into the freshly configured trees below. Externally attached handlers
	// (AddHandler) are not sinks: they survive the reconfiguration.
	oldTrees := make([]*multiHandler, 0, len(trees))
	for _, lc := range cfg.Loggers {
		l := m.getLoggerLocked(lc.Name)
		old := l.handler.Load()
		oldTrees = append(oldTrees, old)
		l.configured.Store(true)
		l.swapHandler(withPreservedHandlers(old, trees[lc.Name]))
	}
	live := map[*sinkState]bool{}
	for _, l := range m.loggers {
		for _, st := range l.handler.Load().sinkStates() {
			live[st] = true
		}
	}
	released := map[*sinkState]bool{}
	for _, h := range oldTrees {
		for _, st := range h.sinkStates() {
			if !live[st] && !released[st] {
				released[st] = true
				st.releaseWriter()
			}
		}
	}
	m.flushBufferLocked()
	return nil
}

func (m *Manager) newSink(cfg SinkConfig) (*sinkState, error) {
	if cfg.Type == "discard" {
		// no writer, no pool entry: records are dropped at dispatch
		st, err := newSinkState(cfg)
		if err != nil {
			return nil, err
		}
		st.writer = nopWriteCloser{io.Discard}
		st.release = func() {}
		return st, nil
	}
	op := sinkWriterOpener(cfg)
	if op == nil {
		return nil, fmt.Errorf("invalid log sink type %q for sink %q", cfg.Type, cfg.Name)
	}
	w, release, err := m.writers.acquire(op)
	if err != nil {
		return nil, err
	}
	st, err := m.newSinkWithWriter(cfg, w)
	if err != nil {
		release()
		return nil, err
	}
	st.writerKey = op.WriterKey()
	st.release = release // pooled: the pool closes the writer on last release
	return st, nil
}

// newSinkWithWriter wires an externally provided writer (tests). Closing it
// on release is the caller's contract, matching the pool's close-on-last.
func (m *Manager) newSinkWithWriter(cfg SinkConfig, w io.WriteCloser) (*sinkState, error) {
	st, err := newSinkState(cfg)
	if err != nil {
		return nil, err
	}
	st.writer = w
	st.release = func() { _ = w.Close() }
	return st, nil
}
