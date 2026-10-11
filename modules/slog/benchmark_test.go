// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"io"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// benchWriter counts writes so benchmarks can wait for the sink goroutines to
// drain instead of flooding the queues (a flood would drop events to stderr).
type benchWriter struct {
	written atomic.Int64
}

func (w *benchWriter) Write(p []byte) (int, error) {
	w.written.Add(1)
	return len(p), nil
}

func (w *benchWriter) Close() error { return nil }

// benchApply wires the given sinks onto the default logger of the default
// manager (the package-level hot path) and returns a wait func for an absolute
// number of logged events.
func benchApply(b *testing.B, sinks []SinkConfig) (wait func(enqueued int64), cleanup func()) {
	b.Helper()
	m := GetManager()
	w := &benchWriter{}
	writers := map[string]io.WriteCloser{}
	for _, sc := range sinks {
		writers[sc.Name] = w
	}
	require.NoError(b, m.apply(Config{Loggers: []LoggerConfig{{Name: DefaultName, Sinks: sinks}}}, writers))
	return func(enqueued int64) {
		for w.written.Load() < enqueued*int64(len(sinks)) {
			runtime.Gosched()
		}
	}, m.Close
}

const benchChunk = 32768 // below BufferLen, so a chunk never overflows

// benchLoop logs the event b.N times in drainable chunks; pass nil to skip
// draining (level-gated benchmarks never enqueue)
func benchLoop(b *testing.B, wait func(int64), log func()) {
	var enqueued int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log()
		enqueued++
		if wait != nil && enqueued%benchChunk == 0 {
			b.StopTimer()
			wait(enqueued)
			b.StartTimer()
		}
	}
	b.StopTimer()
	if wait != nil {
		wait(enqueued)
	}
}

func BenchmarkGateDisabled(b *testing.B) {
	_, cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelWarn, BufferLen: benchChunk * 2,
	}})
	defer cleanup()
	l := Default()
	benchLoop(b, nil, func() {
		l.Info("this event is filtered by the level gate")
	})
}

func BenchmarkPrintfLegacyColorized(b *testing.B) {
	wait, cleanup := benchApply(b, []SinkConfig{{
		// the default Gitea config shape: colorized legacy with std flags
		Name: "bench", Type: "console", Level: LevelInfo, Colorize: true,
		Flags: FlagsFromString("stdflags"), BufferLen: benchChunk * 2,
	}})
	defer cleanup()
	l := Default()
	benchLoop(b, wait, func() {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	})
}

func BenchmarkPrintfLegacyPlain(b *testing.B) {
	wait, cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelInfo,
		Flags: FlagsFromString("stdflags"), BufferLen: benchChunk * 2,
	}})
	defer cleanup()
	l := Default()
	benchLoop(b, wait, func() {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	})
}

func BenchmarkPrintfJSON(b *testing.B) {
	wait, cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelInfo, Format: FormatJSON, BufferLen: benchChunk * 2,
	}})
	defer cleanup()
	l := Default()
	benchLoop(b, wait, func() {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	})
}

func BenchmarkStructuredJSON(b *testing.B) {
	wait, cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelInfo, Format: FormatJSON, BufferLen: benchChunk * 2,
	}})
	defer cleanup()
	l := Default().Slog()
	benchLoop(b, wait, func() {
		l.Info("request handled", "method", "GET", "path", "/api/v1/repos", "status", 200)
	})
}

func BenchmarkFanout3(b *testing.B) {
	wait, cleanup := benchApply(b, []SinkConfig{
		{Name: "console", Type: "console", Level: LevelInfo, Colorize: true, Flags: FlagsFromString("stdflags"), BufferLen: benchChunk * 2},
		{Name: "json", Type: "console", Level: LevelInfo, Format: FormatJSON, BufferLen: benchChunk * 2},
		{Name: "logfmt", Type: "console", Level: LevelInfo, Format: FormatLogfmt, BufferLen: benchChunk * 2},
	})
	defer cleanup()
	l := Default()
	benchLoop(b, wait, func() {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	})
}
