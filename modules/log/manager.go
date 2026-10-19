// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"gitea.dev/modules/slog"
)

// LoggerManager manages the loggers, backed by the slog engine's manager.
type LoggerManager struct {
	mgr *slog.Manager
}

const DEFAULT = slog.DefaultName

// NewManager creates a new logger manager
func NewManager() *LoggerManager {
	return &LoggerManager{mgr: slog.NewManager()}
}

var loggerManager = &LoggerManager{mgr: slog.GetManager()}

// GetManager returns the manager of the loggers
func GetManager() *LoggerManager {
	return loggerManager
}

// GetLogger returns a logger with the given name. If the logger doesn't exist, a new empty one will be created.
func (m *LoggerManager) GetLogger(name string) Logger {
	return &logger{l: m.mgr.GetLogger(name)}
}

// Close closes the manager, all loggers and sinks will be closed, the messages are flushed.
func (m *LoggerManager) Close() {
	m.mgr.Close()
}

// DumpLoggers returns a map of all loggers and their sinks, for debugging and display purposes.
func (m *LoggerManager) DumpLoggers() map[string]any {
	dump := m.mgr.Dump()
	sinks, _ := dump["sinks"].(map[string]any)
	loggers, _ := dump["loggers"].(map[string]any)

	out := make(map[string]any, len(loggers))
	for name, ld := range loggers {
		lm := ld.(map[string]any) //nolint:forcetypeassert // the dump maps are constructed by slog.Manager.Dump
		level := m.mgr.GetLogger(name).GetLevel()
		eventWriters := map[string]any{}
		for _, sn := range lm["sinks"].([]string) { //nolint:forcetypeassert // constructed by slog.Manager.Dump
			if sd, ok := sinks[sn]; ok {
				eventWriters[sn] = sd
			}
		}
		out[name] = map[string]any{
			"IsEnabled":    level != slog.LevelNone && level < slog.LevelFatal,
			"EventWriters": eventWriters,
		}
	}
	return out
}
