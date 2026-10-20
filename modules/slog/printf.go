// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

// PrintfLogger is the transitional printf API. It exists only for the
// modules/log shim and the migration period; new code uses the stdlib logger
// returned by GetLogger. It is deleted together with modules/log.
type PrintfLogger struct {
	l *logger
}

func (p *PrintfLogger) GetLevel() Level {
	return p.l.GetLevel()
}

func (p *PrintfLogger) LevelEnabled(level Level) bool {
	return p.l.LevelEnabled(level)
}

// LogPC logs with an explicitly captured PC. See logger.LogPC.
func (p *PrintfLogger) LogPC(pc uintptr, level Level, stackSkip int, format string, args ...any) {
	p.l.LogPC(pc, level, stackSkip, format, args...)
}

// LogWithCallerPC logs with a prefilled caller name and an explicitly
// captured PC. See logger.LogWithCallerPC.
func (p *PrintfLogger) LogWithCallerPC(pc uintptr, level Level, caller string, stackSkip int, format string, args ...any) {
	p.l.LogWithCallerPC(pc, level, caller, stackSkip, format, args...)
}
