// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"sync"
	"sync/atomic"
)

// DefaultName is the name of the default logger.
const DefaultName = "default"

// Manager owns named loggers and shared sinks. Sinks are deduplicated by name
// and reused across loggers ("first logger wins"): a sink referenced by a
// later logger config keeps the level and format it was first created with.
type Manager struct {
	mu      sync.Mutex
	loggers map[string]*Logger
	sinks   map[string]*sinkState

	defaultLogger atomic.Pointer[Logger] // fast path for GetLogger(DefaultName)

	pauseMu   sync.RWMutex
	pauseChan chan struct{}
}

// NewManager creates a new manager.
func NewManager() *Manager {
	return &Manager{loggers: map[string]*Logger{}, sinks: map[string]*sinkState{}}
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
	for _, st := range m.sinks {
		st.backend.stop()
	}
	m.sinks = map[string]*sinkState{}
	m.loggers = map[string]*Logger{}
	m.defaultLogger.Store(nil)
}

// Dump returns the state of all loggers and sinks, for debugging and display.
func (m *Manager) Dump() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	sinks := map[string]any{}
	for name, st := range m.sinks {
		sinks[name] = map[string]any{
			"type":            st.sinkType,
			"format":          st.format,
			"level":           LevelName(st.level.Level()),
			"stacktraceLevel": LevelName(st.stacktraceLevel),
			"colorize":        st.colorize,
			"prefix":          st.prefix,
		}
	}
	dump := map[string]any{"sinks": sinks}
	loggers := map[string]any{}
	for name, l := range m.loggers {
		loggers[name] = map[string]any{
			"level":   LevelName(l.GetLevel()),
			"enabled": l.LevelEnabled(LevelInfo),
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
