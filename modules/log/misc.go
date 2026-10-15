// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"io"
)

// LoggerToWriter wraps a log function to an io.Writer
func LoggerToWriter(logf func(format string, args ...any)) io.Writer {
	return &loggerToWriter{logf: logf}
}

type loggerToWriter struct {
	logf func(format string, args ...any)
}

func (p *loggerToWriter) Write(bs []byte) (int, error) {
	p.logf("%s", string(bs))
	return len(bs), nil
}

type baseToLogger struct {
	base BaseLogger
}

// BaseLoggerToGeneralLogger wraps a BaseLogger (which only has Log() function) to a Logger (which has Info() function)
// Deprecated: depend on the slog facade directly.
func BaseLoggerToGeneralLogger(b BaseLogger) Logger {
	l := &baseToLogger{base: b}
	return l
}

var _ Logger = (*baseToLogger)(nil)

func (s *baseToLogger) Log(skip int, event *Event, format string, v ...any) {
	// codeql[disable-next-line=go/clear-text-logging]
	s.base.Log(skip+1, event, format, v...)
}

func (s *baseToLogger) GetLevel() Level {
	return s.base.GetLevel()
}

func (s *baseToLogger) LevelEnabled(level Level) bool {
	return s.base.GetLevel() <= level
}

// IsEnabled reports whether any sink would accept an event
func (s *baseToLogger) IsEnabled() bool {
	level := s.base.GetLevel()
	return level != NONE && level < FATAL
}

func (s *baseToLogger) Trace(format string, v ...any) {
	s.base.Log(1, &Event{Level: TRACE}, format, v...)
}

func (s *baseToLogger) Debug(format string, v ...any) {
	s.base.Log(1, &Event{Level: DEBUG}, format, v...)
}

func (s *baseToLogger) Info(format string, v ...any) {
	s.base.Log(1, &Event{Level: INFO}, format, v...)
}

func (s *baseToLogger) Warn(format string, v ...any) {
	s.base.Log(1, &Event{Level: WARN}, format, v...)
}

func (s *baseToLogger) Error(format string, v ...any) {
	s.base.Log(1, &Event{Level: ERROR}, format, v...)
}

func (s *baseToLogger) Critical(format string, v ...any) {
	s.base.Log(1, &Event{Level: CRITICAL}, format, v...)
}
