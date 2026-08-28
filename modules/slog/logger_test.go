// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"gitea.dev/modules/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRecorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (r *testRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

func (r *testRecorder) Close() error { return nil }

func (r *testRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

// newTestLogger builds a logger with one buffer sink; mutate adjusts the sink
// config before creation. All output is deterministic after m.Close() flushes.
func newTestLogger(t *testing.T, mutate ...func(*SinkConfig)) (*Manager, *Logger, *testRecorder) {
	t.Helper()
	m := NewManager()
	rec := &testRecorder{}
	cfg := SinkConfig{Name: "test", Type: "console", Level: LevelTrace}
	for _, f := range mutate {
		f(&cfg)
	}
	st, err := m.newSinkWithWriter(cfg, nopWriteCloser{rec})
	require.NoError(t, err)
	m.sinks[st.name] = st
	l := m.GetLogger("test")
	l.swapHandler(newMultiHandler([]*sinkFrontend{newSinkFrontend(st)}))
	t.Cleanup(m.Close)
	return m, l, rec
}

func withFlags(flags uint32) func(*SinkConfig) {
	return func(c *SinkConfig) { c.Flags = FlagsFromBits(flags) }
}

func TestLegacyFormatGolden(t *testing.T) {
	newRecord := func() slog.Record {
		return slog.NewRecord(time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC), LevelError, "msg format: arg0 arg1", 0)
	}
	hi := &hiddenInfo{
		caller:     "caller",
		file:       "filename",
		line:       123,
		stacktrace: "stacktrace",
		format:     "msg format: %v %v",
		args:       []any{"arg0", NewColoredValue("arg1", FgBlue)},
	}
	st := &sinkState{
		name:            "t",
		format:          FormatLegacy,
		flags:           FlagsFromBits(0xffffffff),
		stacktraceLevel: LevelDebug,
		prefix:          "[PREFIX] ",
	}

	rec := newRecord()
	assert.Equal(t, `[PREFIX] 2020/01/02 03:04:05.000000 filename:123:caller [E] [no-gopid] msg format: arg0 arg1`+"\n\tstacktrace\n\n",
		string(st.formatLegacy(&rec, &sinkFrontend{st: st}, hi)))

	st.colorize = true
	rec = newRecord()
	assert.Equal(t, "[PREFIX] \x1b[36m2020/01/02 03:04:05.000000 \x1b[0m\x1b[32mfilename:123:\x1b[32mcaller\x1b[0m \x1b[1;31m[E]\x1b[0m [\x1b[93mno-gopid\x1b[0m] msg format: arg0 \x1b[34marg1\x1b[0m\n\tstacktrace\n\n",
		string(st.formatLegacy(&rec, &sinkFrontend{st: st}, hi)))
}

func TestLegacyFormatAttrsSuffix(t *testing.T) {
	st := &sinkState{name: "t", format: FormatLegacy, flags: FlagsFromString("none")}
	fe := &sinkFrontend{st: st, withAttrs: []slog.Attr{slog.String("base", "1")}}
	rec := slog.NewRecord(time.Now(), LevelInfo, "msg", 0)
	rec.Add(slog.String("k", "v"))
	out := string(st.formatLegacy(&rec, fe, nil))
	assert.True(t, strings.HasSuffix(out, "msg base=1 k=v\n"), out)
}

func TestLevelFanout(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	recA, recB := &testRecorder{}, &testRecorder{}
	stA, err := m.newSinkWithWriter(SinkConfig{Name: "a", Type: "console", Level: LevelInfo, Flags: FlagsFromString("none")}, nopWriteCloser{recA})
	require.NoError(t, err)
	stB, err := m.newSinkWithWriter(SinkConfig{Name: "b", Type: "console", Level: LevelDebug, Flags: FlagsFromString("none")}, nopWriteCloser{recB})
	require.NoError(t, err)
	m.sinks["a"], m.sinks["b"] = stA, stB
	l := m.GetLogger("fanout")
	l.swapHandler(newMultiHandler([]*sinkFrontend{newSinkFrontend(stA), newSinkFrontend(stB)}))

	assert.Equal(t, LevelDebug, l.GetLevel())

	l.Trace("trace-level")
	l.Debug("debug-level")
	l.Info("info-level")
	m.Close()

	assert.Equal(t, "info-level\n", recA.String())
	assert.Equal(t, "debug-level\ninfo-level\n", recB.String())
}

func TestPrintfCaller(t *testing.T) {
	_, l, rec := newTestLogger(t, withFlags(Lmedfile|Lshortfuncname))
	genericFunc(l, "123")
	m := l.mgr
	m.Close()
	assert.Regexp(t, `modules/slog/logger_test.go:\w+:genericFunc\(\) from genericFunc: 123`, rec.String())
}

func genericFunc[T any](l *Logger, v T) {
	l.Info("from genericFunc: %v", v)
}

func TestStructuredJSON(t *testing.T) {
	m, l, rec := newTestLogger(t, func(c *SinkConfig) { c.Format = FormatJSON })

	l.Slog().Info("hello", "k", "v")
	l.Warn("num %d", 42)
	m.Close()

	lines := strings.Split(strings.TrimSpace(rec.String()), "\n")
	require.Len(t, lines, 2)

	var structured map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &structured))
	assert.Equal(t, "hello", structured["msg"])
	assert.Equal(t, "INFO", structured["level"])
	assert.Equal(t, "test", structured["logger"])
	assert.Equal(t, "v", structured["k"])

	var printfPath map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &printfPath))
	assert.Equal(t, "num 42", printfPath["msg"])
	assert.Equal(t, "WARN", printfPath["level"])
	assert.NotContains(t, lines[1], "\x00", "hidden attrs must not leak into json")
}

func TestFatalLevelRename(t *testing.T) {
	m, l, rec := newTestLogger(t, func(c *SinkConfig) { c.Format = FormatJSON })
	l.Log(1, LevelFatal, "boom")
	m.Close()
	assert.Contains(t, rec.String(), `"level":"FATAL"`)
}

func TestSharedSink(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	rec := &testRecorder{}
	st, err := m.newSinkWithWriter(SinkConfig{Name: "shared", Type: "console", Level: LevelInfo, Flags: FlagsFromString("none")}, nopWriteCloser{rec})
	require.NoError(t, err)
	m.sinks["shared"] = st

	require.NoError(t, m.Apply(Config{Loggers: []LoggerConfig{
		{Name: "one", Sinks: []SinkConfig{{Name: "shared", Type: "console"}}},
		{Name: "two", Sinks: []SinkConfig{{Name: "shared", Type: "console"}}},
	}}))
	assert.Len(t, m.sinks, 1)

	m.GetLogger("one").Info("from one")
	m.GetLogger("two").Info("from two")
	m.Close()
	assert.Equal(t, "from one\nfrom two\n", rec.String())
}

func TestApplyValidation(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	err := m.Apply(Config{Loggers: []LoggerConfig{{
		Name:  "x",
		Sinks: []SinkConfig{{Name: "bad", Type: "carrier-pigeon"}},
	}}})
	assert.Error(t, err)
	assert.Empty(t, m.sinks)
}

func TestPauseResume(t *testing.T) {
	t.Run("drain while paused", func(t *testing.T) {
		m, l, rec := newTestLogger(t, withFlags(0))
		m.PauseAll()
		l.Info("paused-flush")
		m.Close() // drain must bypass pause
		assert.Contains(t, rec.String(), "paused-flush")
	})

	t.Run("resume writes", func(t *testing.T) {
		m, l, rec := newTestLogger(t, withFlags(0))
		m.PauseAll()
		l.Info("after-resume")
		m.ResumeAll()
		assert.Eventually(t, func() bool {
			return strings.Contains(rec.String(), "after-resume")
		}, 2*time.Second, 10*time.Millisecond)
	})
}

func TestProtectSensitiveInfo(t *testing.T) {
	s, changed := protectSensitiveInfo("mailto:user@example.com")
	assert.False(t, changed)
	assert.Equal(t, "mailto:user@example.com", s)
	s, changed = protectSensitiveInfo("https://example.com")
	assert.False(t, changed)
	assert.Equal(t, "https://example.com", s)
	s, changed = protectSensitiveInfo("https://u:p@example.com/path?k=v#hash")
	assert.True(t, changed)
	assert.Equal(t, "https://_masked_@example.com/path?k=_", s)

	m, l, rec := newTestLogger(t, withFlags(0))
	l.Warn("url %s", "https://u:p@example.com/path?k=v#hash")
	m.Close()
	assert.Contains(t, rec.String(), "https://_masked_@example.com/path?k=_")
}

func TestMixedSinkFanout(t *testing.T) {
	m := NewManager()
	t.Cleanup(m.Close)
	recL, recJ := &testRecorder{}, &testRecorder{}
	stL, err := m.newSinkWithWriter(SinkConfig{Name: "legacy", Type: "console", Level: LevelInfo, Colorize: true, Flags: FlagsFromString("none")}, nopWriteCloser{recL})
	require.NoError(t, err)
	stJ, err := m.newSinkWithWriter(SinkConfig{Name: "json", Type: "console", Level: LevelInfo, Format: FormatJSON}, nopWriteCloser{recJ})
	require.NoError(t, err)
	m.sinks["legacy"], m.sinks["json"] = stL, stJ
	l := m.GetLogger("mixed")
	l.swapHandler(newMultiHandler([]*sinkFrontend{newSinkFrontend(stL), newSinkFrontend(stJ)}))

	l.Info("hello %s", "world")
	m.Close()

	assert.Equal(t, "hello world\n", recL.String()) // colored re-render from raw args
	var structured map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(recJ.String())), &structured))
	assert.Equal(t, "hello world", structured["msg"])
}

func TestSlogReconfigure(t *testing.T) {
	m, l, rec := newTestLogger(t, withFlags(0))
	l.Slog().Info("before")

	rec2 := &testRecorder{}
	st, err := m.newSinkWithWriter(SinkConfig{Name: "test2", Type: "console", Level: LevelInfo, Flags: FlagsFromString("none")}, nopWriteCloser{rec2})
	require.NoError(t, err)
	m.sinks["test2"] = st
	require.NoError(t, m.Apply(Config{Loggers: []LoggerConfig{
		{Name: "test", Sinks: []SinkConfig{{Name: "test2", Type: "console"}}},
	}}))

	l.Slog().Info("after") // a re-fetched Slog() follows the swapped tree
	l.Info("after2")       // the printf path resolves the handler per call
	m.Close()

	assert.Contains(t, rec2.String(), "after")
	assert.Contains(t, rec2.String(), "after2")
	assert.Contains(t, rec.String(), "before")
	assert.NotContains(t, rec2.String(), "before")
}

type testLogString struct {
	Field string
}

func (t testLogString) LogString() string {
	return "log-string"
}

func TestLogStringer(t *testing.T) {
	m, l, rec := newTestLogger(t, func(c *SinkConfig) {
		c.Flags = FlagsFromString("none")
		c.Colorize = true
	})
	l.Info("%s %s %#v %v", testLogString{}, &testLogString{}, testLogString{Field: "detail"}, NewColoredValue(testLogString{}, FgRed))
	m.Close()
	assert.Equal(t, "log-string log-string slog.testLogString{Field:\"detail\"} \x1b[31mlog-string\x1b[0m\n", rec.String())
}

func TestLevelAndFlagsParsing(t *testing.T) {
	lv, ok := LevelFromString("Warning")
	assert.True(t, ok)
	assert.Equal(t, LevelWarn, lv)
	_, ok = LevelFromString("bogus")
	assert.False(t, ok)

	assert.Equal(t, "trace", LevelName(LevelTrace))
	assert.Equal(t, "fatal", LevelName(LevelFatal))

	assert.Equal(t, Lmedfile, FlagsFromString("medfile").Bits())
	assert.Equal(t, LstdFlags, Flags{}.Bits())
	assert.Equal(t, uint32(0), FlagsFromString("none").Bits())
	assert.Equal(t, Lshortfile|Llongfile, FlagsFromString("medfile,longfile").Bits())
}
