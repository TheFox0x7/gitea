// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"context"
	"log/slog"
	"runtime"
	"time"
)

// gologBridge dispatches stdlib log output to the slog engine, capturing the
// caller of the golog call — an io.Writer form of slog.NewLogLogger.
type gologBridge struct {
	handler slog.Handler
	level   slog.Level
}

func (b gologBridge) Write(p []byte) (int, error) {
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // Callers, Write, golog Output → golog caller
	r := slog.NewRecord(time.Now(), b.level, string(p), pcs[0])
	_ = b.handler.Handle(context.Background(), r)
	return len(p), nil
}
