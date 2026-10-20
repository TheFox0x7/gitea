// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fileSinkCfg(name, file string, maxSize int64) SinkConfig {
	return SinkConfig{Name: name, Type: "file", Flags: FlagsFromString("none"), File: &FileSinkOptions{FileName: file, MaxSize: maxSize}}
}

func TestSharedFileTarget(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	file := filepath.Join(t.TempDir(), "shared.log")

	loggerCfg := func(logger, sink string) Config {
		return Config{Loggers: []LoggerConfig{{Name: logger, Sinks: []SinkConfig{fileSinkCfg(sink, file, 0)}}}}
	}
	require.NoError(t, m.Apply(loggerCfg("one", "one-sink")))
	require.NoError(t, m.Apply(loggerCfg("two", "two-sink"))) // same target, different sink: one shared writer

	assert.Len(t, m.writers.writers, 1)

	m.getLogger("one").Info("from one")
	m.getLogger("two").Info("from two")
	m.Close() // the last released reference closes the writer after both backends drained

	assert.Empty(t, m.writers.writers)
	bs, err := os.ReadFile(file)
	require.NoError(t, err)
	// two independent sink goroutines append to the shared writer: order is not guaranteed
	assert.ElementsMatch(t, []string{"from one", "from two"}, strings.Split(strings.TrimSpace(string(bs)), "\n"))
}

func TestSharedFileTargetDivergentSettings(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	m := NewManager()
	t.Cleanup(func() {
		os.Stderr = oldStderr
		m.Close()
	})
	file := filepath.Join(t.TempDir(), "shared.log")
	require.NoError(t, m.Apply(Config{Loggers: []LoggerConfig{{Name: "one", Sinks: []SinkConfig{fileSinkCfg("s", file, 0)}}}}))
	require.NoError(t, m.Apply(Config{Loggers: []LoggerConfig{{Name: "two", Sinks: []SinkConfig{fileSinkCfg("s", file, 1024*1024)}}}}))

	assert.Len(t, m.writers.writers, 1) // existing writer wins, new settings ignored

	require.NoError(t, w.Close())
	bs, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(bs), "different settings")
}

func TestApplyReusesFileWriter(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	file := filepath.Join(t.TempDir(), "app.log")
	cfg := Config{Loggers: []LoggerConfig{{Name: "default", Sinks: []SinkConfig{fileSinkCfg("s", file, 0)}}}}

	require.NoError(t, m.Apply(cfg))
	w1 := m.writers.writers["file:"+file].w
	require.NoError(t, m.Apply(cfg))
	w2 := m.writers.writers["file:"+file].w
	assert.Same(t, w1, w2) // re-apply keeps the open file instead of closing and reopening it

	m.getLogger("default").Info("kept")
	m.Close()
	bs, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "kept\n", string(bs))
}
