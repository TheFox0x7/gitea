// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

// DefaultName is the name of the default logger.
const DefaultName = "default"

// Manager coordinates named loggers. It is intentionally thin: the logger
// registry, the default-logger fast path and the shared writer
// pool live here; sink lifecycles are provisioned by Config (see
// Config.provision) and tracked via the logger trees.
type Manager struct {
	mu      sync.Mutex
	loggers map[string]*logger

	defaultLogger atomic.Pointer[logger] // fast path for GetLogger(DefaultName)

	writers        writerPool
	bootstrapLevel atomic.Int32 // minimum level captured before a logger is configured
	buffer         recordBuffer
}

// NewManager creates a new manager.
func NewManager() *Manager {
	m := &Manager{loggers: map[string]*logger{}}
	m.bootstrapLevel.Store(int32(LevelTrace)) // capture everything pre-configuration by default
	return m
}

// SetBootstrapLevel sets the minimum level captured for loggers that have no
// configuration yet (the CLI maps --quiet/--debug/--verbose onto it). Captured
// events replay into the real sinks on Apply.
func (m *Manager) SetBootstrapLevel(level Level) {
	m.bootstrapLevel.Store(int32(level))
}

// BootstrapLevel returns the capture floor set by SetBootstrapLevel.
func (m *Manager) BootstrapLevel() Level {
	return Level(m.bootstrapLevel.Load())
}

// GetLogger returns the named logger as a stdlib *slog.Logger, creating one
// (buffering until configured) if it doesn't exist. The returned logger is
// stable for the process lifetime: it follows reconfigurations (see
// dynamicHandler), so it can be held and passed to downstream libraries.
func (m *Manager) GetLogger(name string) *slog.Logger {
	return m.getLogger(name).stdLogger()
}

// PrintfLogger returns the transitional printf view of a named logger. It is
// used only by the modules/log shim until the migration deletes both.
func (m *Manager) PrintfLogger(name string) *PrintfLogger {
	return &PrintfLogger{l: m.getLogger(name)}
}

func (m *Manager) getLogger(name string) *logger {
	if name == DefaultName {
		// the default logger is on every package-level log call: check the
		// atomic cache before taking the mutex
		if l := m.defaultLogger.Load(); l != nil {
			return l
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.getLoggerLocked(name)
	if name == DefaultName {
		m.defaultLogger.Store(l)
	}
	return l
}

func (m *Manager) getLoggerLocked(name string) *logger {
	if l := m.loggers[name]; l != nil {
		return l
	}
	l := newLogger(m, name)
	m.loggers[name] = l
	return l
}

// AddHandler attaches a plain external slog.Handler to a named logger. It
// receives records without printf metadata and contributes no sinks.
func (m *Manager) AddHandler(loggerName string, h slog.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.getLoggerLocked(loggerName)
	old := l.handler.Load()
	children := make([]child, 0, len(old.children)+1)
	children = append(children, old.children...)
	children = append(children, handlerChild{h})
	l.swapHandler(newMultiHandlerChildren(children))
}

// RemoveHandler detaches a previously added external slog.Handler.
func (m *Manager) RemoveHandler(loggerName string, h slog.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := m.getLoggerLocked(loggerName)
	old := l.handler.Load()
	children := make([]child, 0, len(old.children))
	for _, c := range old.children {
		if hc, ok := c.(handlerChild); !ok || hc.h != h {
			children = append(children, c)
		}
	}
	l.swapHandler(newMultiHandlerChildren(children))
}

// Close releases all writers and forgets all loggers. Loggers already held by
// callers keep working, but writes to released writers fail and land on stderr.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, st := range m.liveSinkStates() {
		st.releaseWriter()
	}
	m.loggers = map[string]*logger{}
	m.defaultLogger.Store(nil)
	m.buffer = recordBuffer{}
}

// flushBufferLocked replays buffered events into the loggers that now have a
// real configuration; events of still-unconfigured loggers stay buffered for a
// later Apply. Callers hold m.mu.
func (m *Manager) flushBufferLocked() {
	entries, dropped := m.buffer.takeAll()
	var requeue []bufferedRecord
	for _, e := range entries {
		l, ok := m.loggers[e.loggerName]
		if !ok || l.handler.Load().buffering {
			requeue = append(requeue, e)
			continue
		}
		h := l.handler.Load()
		for _, g := range e.groups {
			h = h.withGroup(g)
		}
		if len(e.attrs) > 0 {
			h = h.withAttrs(e.attrs)
		}
		h.handle(e.rec, e.hi)
	}
	m.buffer.restore(requeue)
	if dropped > 0 {
		FallbackErrorf("%d early log events were dropped before the logging configuration was applied", dropped)
	}
}

// SpillBufferedEvents writes events still held for unconfigured loggers to
// stderr (best-effort). The CLI entry calls it once a command finishes, so
// pre-configuration events are not lost when no Apply ever happened.
func (m *Manager) SpillBufferedEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if def := m.loggers[DefaultName]; def != nil && def.configured.Load() {
		return // the real sinks own these events' loggers; leftovers stay buffered
	}
	entries, dropped := m.buffer.takeAll()
	for _, e := range entries {
		level := LevelName(e.rec.Level)
		FallbackErrorf("%s [%s] %s: %s", e.rec.Time.Format("2006/01/02 15:04:05.000"),
			strings.ToUpper(level[:1]), e.loggerName, e.rec.Message)
	}
	if dropped > 0 {
		FallbackErrorf("%d early log events were dropped before any logging configuration was applied", dropped)
	}
}

// liveSinkStates returns the deduplicated sink states of all logger trees.
// Callers must hold m.mu.
func (m *Manager) liveSinkStates() []*sinkState {
	var sts []*sinkState
	seen := map[*sinkState]bool{}
	for _, l := range m.loggers {
		for _, st := range l.handler.Load().sinkStates() {
			if !seen[st] {
				seen[st] = true
				sts = append(sts, st)
			}
		}
	}
	return sts
}

// Dump returns the state of all loggers and sinks, for debugging and display.
func (m *Manager) Dump() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	sinks := map[string]any{}
	for _, st := range m.liveSinkStates() {
		entry := map[string]any{
			"type":            st.sinkType,
			"format":          st.format,
			"level":           LevelName(st.level.Level()),
			"stacktraceLevel": LevelName(st.stacktraceLevel),
			"colorize":        st.colorize,
			"prefix":          st.prefix,
			"flags":           st.flags.String(),
			"addSource":       st.addSource,
		}
		if st.writerKey != "" {
			entry["writer"] = st.writerKey // shared when another sink targets the same key
		}
		sinks[st.name] = entry
	}
	dump := map[string]any{"sinks": sinks}
	loggers := map[string]any{}
	for name, l := range m.loggers {
		sinkNames := make([]string, 0, 2)
		for _, st := range l.handler.Load().sinkStates() {
			sinkNames = append(sinkNames, st.name)
		}
		loggers[name] = map[string]any{
			"level":   LevelName(l.GetLevel()),
			"enabled": l.LevelEnabled(LevelInfo),
			"sinks":   sinkNames,
		}
	}
	dump["loggers"] = loggers
	return dump
}

var defaultManager = NewManager()

// GetManager returns the process-wide default manager.
func GetManager() *Manager {
	return defaultManager
}

// GetLogger returns the named logger (a stdlib *slog.Logger) from the default
// manager.
func GetLogger(name string) *slog.Logger {
	return defaultManager.GetLogger(name)
}

// Default returns the default logger ("default") of the default manager.
func Default() *slog.Logger {
	return defaultManager.GetLogger(DefaultName)
}

// GetPrintfLogger returns the transitional printf view used by the modules/log
// shim. Deprecated: use GetLogger.
func GetPrintfLogger(name string) *PrintfLogger {
	return defaultManager.PrintfLogger(name)
}

// DefaultPrintfLogger returns the transitional printf view of the default
// logger. Deprecated: use Default.
func DefaultPrintfLogger() *PrintfLogger {
	return defaultManager.PrintfLogger(DefaultName)
}
