// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// benchApply wires the given sinks onto the default logger of the default
// manager (the package-level hot path).
func benchApply(b *testing.B, sinks []SinkConfig) func() {
	b.Helper()
	m := GetManager()
	writers := map[string]io.WriteCloser{}
	for _, sc := range sinks {
		writers[sc.Name] = nopWriteCloser{io.Discard}
	}
	require.NoError(b, m.apply(Config{Loggers: []LoggerConfig{{Name: DefaultName, Sinks: sinks}}}, writers))
	return m.Close
}

func BenchmarkGateDisabled(b *testing.B) {
	cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelWarn,
	}})
	defer cleanup()
	l := GetManager().getLogger(DefaultName)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("this event is filtered by the level gate")
	}
}

func BenchmarkPrintfLegacyColorized(b *testing.B) {
	cleanup := benchApply(b, []SinkConfig{{
		// the default Gitea config shape: colorized legacy with std flags
		Name: "bench", Type: "console", Level: LevelInfo, Colorize: true,
		Flags: FlagsFromString("stdflags"),
	}})
	defer cleanup()
	l := GetManager().getLogger(DefaultName)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	}
}

func BenchmarkPrintfLegacyPlain(b *testing.B) {
	cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelInfo,
		Flags: FlagsFromString("stdflags"),
	}})
	defer cleanup()
	l := GetManager().getLogger(DefaultName)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	}
}

func BenchmarkPrintfJSON(b *testing.B) {
	cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelInfo, Format: FormatJSON,
	}})
	defer cleanup()
	l := GetManager().getLogger(DefaultName)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	}
}

func BenchmarkStructuredJSON(b *testing.B) {
	cleanup := benchApply(b, []SinkConfig{{
		Name: "bench", Type: "console", Level: LevelInfo, Format: FormatJSON,
	}})
	defer cleanup()
	l := Default()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request handled", "method", "GET", "path", "/api/v1/repos", "status", 200)
	}
}

func BenchmarkFanout3(b *testing.B) {
	cleanup := benchApply(b, []SinkConfig{
		{Name: "console", Type: "console", Level: LevelInfo, Colorize: true, Flags: FlagsFromString("stdflags")},
		{Name: "json", Type: "console", Level: LevelInfo, Format: FormatJSON},
		{Name: "logfmt", Type: "console", Level: LevelInfo, Format: FormatLogfmt},
	})
	defer cleanup()
	l := GetManager().getLogger(DefaultName)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	}
}

// BenchmarkLegacyFile writes to a real file: the syscall cost is paid inline
// by the caller (the sync engine has no queue to absorb it).
func BenchmarkLegacyFile(b *testing.B) {
	m := NewManager()
	f, err := os.Create(filepath.Join(b.TempDir(), "bench.log"))
	require.NoError(b, err)
	st, err := m.newSinkWithWriter(SinkConfig{Name: "bench", Type: "file", Level: LevelInfo, Flags: FlagsFromString("stdflags")}, f)
	require.NoError(b, err)
	l := m.getLogger("bench")
	l.swapHandler(newMultiHandler([]*sinkFrontend{newSinkFrontend(st)}))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request %s took %d ms for user %d", "GET /api/v1/repos", 12, 97)
	}
	b.StopTimer()
	m.Close()
}
