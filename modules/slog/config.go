// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"fmt"
	"io"
	"os"
	"time"

	"gitea.dev/modules/util/rotatingfilewriter"
)

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
// value disables the deadline.
type ConnSinkOptions struct {
	Addr           string
	Protocol       string // tcp (default), udp, unix
	Reconnect      bool
	ReconnectOnMsg bool
	DialTimeout    time.Duration
	WriteTimeout   time.Duration
}

// SinkConfig configures one sink. Name deduplicates sinks within one Apply
// call, so multiple loggers can share one sink; Type is "console", "file" or
// "conn"; Format defaults to legacy.
// Level defaults to info, StacktraceLevel (legacy format only) to none,
// BufferLen to 1000. Colorize must be set explicitly (use CanColorStdout /
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
	BufferLen       int

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

// Apply builds the loggers from cfg: sinks are created fresh on every call
// (deduplicated by name within the call, so multiple loggers can share one
// sink), the listed loggers' trees are swapped atomically, and sinks no longer
// referenced by any logger are stopped. Loggers not listed keep their current
// configuration.
func (m *Manager) Apply(cfg Config) error {
	return m.apply(cfg, nil)
}

func (m *Manager) apply(cfg Config, sinkWriters map[string]io.WriteCloser) error {
	// validate first, so a bad config doesn't half-apply
	for _, lc := range cfg.Loggers {
		for _, sc := range lc.Sinks {
			if sc.Name == "" {
				return fmt.Errorf("log logger %q has a sink with empty name", lc.Name)
			}
			switch sc.Type {
			case "console", "file", "conn":
			default:
				return fmt.Errorf("invalid log sink type %q for sink %q", sc.Type, sc.Name)
			}
			if sc.Format != "" && sc.Format != FormatLegacy && sc.Format != FormatLogfmt && sc.Format != FormatJSON {
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
						s.backend.stop()
					}
					return err
				}
				built[sc.Name] = st
			}
			frontends = append(frontends, newSinkFrontend(st))
		}
		trees[lc.Name] = newMultiHandler(frontends)
	}

	// phase 2: swap the trees, then stop backends no tree references anymore
	oldTrees := make([]*multiHandler, 0, len(trees))
	for _, lc := range cfg.Loggers {
		l := m.getLoggerLocked(lc.Name)
		oldTrees = append(oldTrees, l.handler.Load())
		l.swapHandler(trees[lc.Name])
	}
	live := map[*sinkState]bool{}
	for _, l := range m.loggers {
		for _, st := range l.handler.Load().sinkStates() {
			live[st] = true
		}
	}
	stopped := map[*sinkState]bool{}
	for _, h := range oldTrees {
		for _, st := range h.sinkStates() {
			if !live[st] && !stopped[st] {
				stopped[st] = true
				st.backend.stop()
			}
		}
	}
	return nil
}

func (m *Manager) newSink(cfg SinkConfig) (*sinkState, error) {
	w, err := m.newSinkWriter(cfg)
	if err != nil {
		return nil, err
	}
	return m.newSinkWithWriter(cfg, w)
}

func (m *Manager) newSinkWithWriter(cfg SinkConfig, w io.WriteCloser) (*sinkState, error) {
	st, err := newSinkState(cfg)
	if err != nil {
		return nil, err
	}
	st.backend = newSinkBackend(st.name, w, st.bufferLen, m.getPauseChan)
	return st, nil
}

func (m *Manager) newSinkWriter(cfg SinkConfig) (io.WriteCloser, error) {
	switch cfg.Type {
	case "console":
		opt := cfg.Console
		if opt == nil {
			opt = &ConsoleSinkOptions{}
		}
		if opt.Stderr {
			return nopWriteCloser{os.Stderr}, nil
		}
		return nopWriteCloser{os.Stdout}, nil
	case "file":
		opt := cfg.File
		if opt == nil {
			opt = &FileSinkOptions{}
		}
		w, err := rotatingfilewriter.Open(opt.FileName, &rotatingfilewriter.Options{
			Rotate:           opt.LogRotate,
			MaximumSize:      opt.MaxSize,
			RotateDaily:      opt.DailyRotate,
			KeepDays:         opt.MaxDays,
			Compress:         opt.Compress,
			CompressionLevel: opt.CompressionLevel,
		})
		if err != nil {
			// same trade-off as modules/log: a broken file sink degrades to
			// stderr rather than disabling logging entirely
			FallbackErrorf("unable to open log file %q: %v", opt.FileName, err)
			return nopWriteCloser{os.Stderr}, nil
		}
		return w, nil
	case "conn":
		opt := cfg.Conn
		if opt == nil {
			opt = &ConnSinkOptions{}
		}
		protocol := opt.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		dialTimeout, writeTimeout := opt.DialTimeout, opt.WriteTimeout
		if dialTimeout == 0 {
			dialTimeout = 30 * time.Second
		}
		if writeTimeout == 0 {
			writeTimeout = 60 * time.Second
		}
		return &connWriter{
			protocol:       protocol,
			addr:           opt.Addr,
			reconnect:      opt.Reconnect,
			reconnectOnMsg: opt.ReconnectOnMsg,
			dialTimeout:    dialTimeout,
			writeTimeout:   writeTimeout,
		}, nil
	default:
		return nil, fmt.Errorf("invalid log sink type %q", cfg.Type)
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
