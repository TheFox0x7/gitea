// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applyConsoleRecorder(m *Manager, name string, rec *testRecorder, level Level) error {
	return m.apply(Config{Loggers: []LoggerConfig{{
		Name:  name,
		Sinks: []SinkConfig{{Name: name + "-sink", Type: "console", Level: level, Flags: FlagsFromString("none")}},
	}}}, map[string]io.WriteCloser{name + "-sink": rec})
}

func TestBootstrapBufferReplay(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	rec := &testRecorder{}

	// before any Apply: events are captured, not printed, not Fatal-echoed
	m.getLogger("default").Info("early one")
	m.getLogger("default").Warn("early two %d", 2)
	assert.Equal(t, "", rec.String())

	// a failed Apply keeps buffering
	err := m.Apply(Config{Loggers: []LoggerConfig{{
		Name:  "default",
		Sinks: []SinkConfig{{Name: "bad", Type: "carrier-pigeon"}},
	}}})
	assert.Error(t, err)

	// the successful Apply replays the buffered events into the real sink
	require.NoError(t, applyConsoleRecorder(m, "default", rec, LevelInfo))
	m.getLogger("default").Info("late")
	m.Close()

	assert.Equal(t, "early one\nearly two 2\nlate\n", rec.String())
	assert.Empty(t, m.buffer.entries)
}

func TestBootstrapBufferUnconfiguredLogger(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	recDefault, recX := &testRecorder{}, &testRecorder{}

	m.getLogger("default").Info("for default")
	m.getLogger("x").Info("for x")

	// only "default" is configured: "x" events stay buffered
	require.NoError(t, applyConsoleRecorder(m, "default", recDefault, LevelInfo))

	// configuring "x" replays its buffered events
	require.NoError(t, applyConsoleRecorder(m, "x", recX, LevelInfo))
	m.Close()
	assert.Equal(t, "for default\n", recDefault.String())
	assert.Equal(t, "for x\n", recX.String())
}

func TestBootstrapBufferLevelGate(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	rec := &testRecorder{}

	m.SetBootstrapLevel(LevelWarn)
	m.getLogger("default").Info("below floor")
	m.getLogger("default").Warn("above floor")
	require.NoError(t, applyConsoleRecorder(m, "default", rec, LevelTrace))
	m.Close()
	assert.Equal(t, "above floor\n", rec.String())
}

func TestBootstrapBufferOverflow(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	m := NewManager()
	t.Cleanup(func() {
		os.Stderr = oldStderr
		m.Close()
	})
	m.buffer.limit = 3
	rec := &testRecorder{}

	for i := range 5 {
		m.getLogger("default").Info("early %d", i)
	}
	require.NoError(t, applyConsoleRecorder(m, "default", rec, LevelInfo))
	m.Close()

	// bounded ring: the oldest entries dropped, the newest survived
	assert.Equal(t, "early 2\nearly 3\nearly 4\n", rec.String())

	require.NoError(t, w.Close())
	bs, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(bs), "2 early log events were dropped")
}

func TestSpillBufferedEvents(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	m := NewManager()
	m.getLogger("default").Info("spilled %s", "event")
	m.getLogger("default").Error("spilled error")
	m.SpillBufferedEvents() // default logger never configured: spill to stderr

	require.NoError(t, w.Close())
	os.Stderr = oldStderr
	m.Close()

	bs, err := io.ReadAll(r)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(bs)), "\n")
	require.Len(t, lines, 2)
	assert.Regexp(t, `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3} \[I\] default: spilled event$`, lines[0])
	assert.Regexp(t, `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{3} \[E\] default: spilled error$`, lines[1])

	// a configured default logger owns its leftovers: nothing spills
	assert.Empty(t, m.buffer.entries)
}

func TestBootstrapBufferTreeAttrs(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	rec := &testRecorder{}

	// tree-level With-state (e.g. a third party using Handler().WithAttrs)
	// must survive buffering and replay
	l := m.getLogger("x")
	forked := l.handler.Load().WithAttrs([]slog.Attr{slog.String("k", "v")})
	require.NoError(t, forked.Handle(context.Background(), slog.NewRecord(time.Now(), LevelInfo, "early", 0)))

	require.NoError(t, m.apply(Config{Loggers: []LoggerConfig{{
		Name:  "x",
		Sinks: []SinkConfig{{Name: "x-sink", Type: "console", Format: FormatJSON}},
	}}}, map[string]io.WriteCloser{"x-sink": rec}))
	m.Close()

	assert.Contains(t, rec.String(), `"k":"v"`)
	assert.Contains(t, rec.String(), `"msg":"early"`)
}
