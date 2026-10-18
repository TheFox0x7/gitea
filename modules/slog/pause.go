// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"io"
	"sync"
)

// pauseGate gates sink writes: while paused, callers block inside the write
// until Resume. Idempotent — nested Pause calls are no-ops.
type pauseGate struct {
	mu   sync.RWMutex
	held bool
}

// Pause blocks all sink writes until Resume.
func (g *pauseGate) Pause() {
	if g.held {
		return
	}
	g.mu.Lock()
	g.held = true
}

// Resume unblocks sink writes. Safe to call without a prior Pause.
func (g *pauseGate) Resume() {
	if g.held {
		g.held = false
		g.mu.Unlock()
	}
}

// gateWriter is the write end of a sink: it acquires the pause gate around
// each write, so a paused manager stalls writers instead of dropping.
type gateWriter struct {
	w     io.Writer
	pause *pauseGate
}

func (gw *gateWriter) Write(p []byte) (int, error) {
	gw.pause.mu.RLock()
	n, err := gw.w.Write(p)
	gw.pause.mu.RUnlock()
	return n, err
}
