// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// DefaultName is the name of the default logger.
const DefaultName = "default"

// Manager owns named loggers. Sinks are owned by the logger trees that
// reference them; the manager only enumerates them (Close/Dump) and stops
// backends when Apply leaves them unreferenced.
type Manager struct {
	mu      sync.Mutex
	loggers map[string]*Logger

	defaultLogger atomic.Pointer[Logger] // fast path for GetLogger(DefaultName)

	pauseMu   sync.RWMutex
	pauseChan chan struct{}
}

// NewManager creates a new manager.
func NewManager() *Manager {
	return &Manager{loggers: map[string]*Logger{}}
}

// GetLogger returns the logger with the given name, creating an empty one
// (which discards all output) if it doesn't exist.
func (m *Manager) GetLogger(name string) *Logger {
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

func (m *Manager) getLoggerLocked(name string) *Logger {
	if l := m.loggers[name]; l != nil {
		return l
	}
	l := newLogger(m, name)
	m.loggers[name] = l
	return l
}

// PauseAll pauses all sinks: each sink goroutine stops writing (and stops
// consuming its queue) until ResumeAll.
func (m *Manager) PauseAll() {
	m.pauseMu.Lock()
	m.pauseChan = make(chan struct{})
	m.pauseMu.Unlock()
}

// ResumeAll resumes all sinks paused by PauseAll.
func (m *Manager) ResumeAll() {
	m.pauseMu.Lock()
	if m.pauseChan != nil {
		close(m.pauseChan)
		m.pauseChan = nil
	}
	m.pauseMu.Unlock()
}

func (m *Manager) getPauseChan() chan struct{} {
	m.pauseMu.RLock()
	defer m.pauseMu.RUnlock()
	return m.pauseChan
}

// Close stops all sinks, flushing their queued events (bounded by timeouts),
// and forgets all loggers. Loggers already held by callers keep working but
// their events are dropped by the stopped backends.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, st := range m.liveSinkStates() {
		st.backend.stop()
	}
	m.loggers = map[string]*Logger{}
	m.defaultLogger.Store(nil)
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
		sinks[st.name] = map[string]any{
			"type":            st.sinkType,
			"format":          st.format,
			"level":           LevelName(st.level.Level()),
			"stacktraceLevel": LevelName(st.stacktraceLevel),
			"colorize":        st.colorize,
			"prefix":          st.prefix,
			"flags":           st.flags.String(),
			"bufferLen":       st.bufferLen,
		}
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

// GetLogger returns a logger from the default manager.
func GetLogger(name string) *Logger {
	return defaultManager.GetLogger(name)
}

// Default returns the default logger ("default") of the default manager.
func Default() *Logger {
	return defaultManager.GetLogger(DefaultName)
}
