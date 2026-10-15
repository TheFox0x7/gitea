// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package test

import (
	"context"
	stdslog "log/slog"
	"strings"
	"sync"
	"time"

	"gitea.dev/modules/slog"
)

// LogChecker inspects the log output of a logger, it implements slog.Handler
// and is attached to a named logger.
type LogChecker struct {
	filterMessages []string
	filtered       []bool

	stopMark string
	stopped  bool

	mu sync.Mutex
}

func (lc *LogChecker) Enabled(_ context.Context, _ stdslog.Level) bool {
	return true
}

func (lc *LogChecker) Handle(_ context.Context, r stdslog.Record) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	msg := r.Message
	for i, m := range lc.filterMessages {
		if strings.Contains(msg, m) {
			lc.filtered[i] = true
		}
	}
	if strings.Contains(msg, lc.stopMark) {
		lc.stopped = true
	}
	return nil
}

func (lc *LogChecker) WithAttrs(attrs []stdslog.Attr) stdslog.Handler { return lc }

func (lc *LogChecker) WithGroup(name string) stdslog.Handler { return lc }

func NewLogChecker(namePrefix string) (logChecker *LogChecker, cancel func()) {
	lc := &LogChecker{}
	slog.GetManager().AddHandler(namePrefix, lc)
	return lc, func() { slog.GetManager().RemoveHandler(namePrefix, lc) }
}

// Filter will make the `Check` function to check if these logs are outputted.
func (lc *LogChecker) Filter(msgs ...string) *LogChecker {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.filterMessages = make([]string, len(msgs))
	copy(lc.filterMessages, msgs)
	lc.filtered = make([]bool, len(lc.filterMessages))
	return lc
}

func (lc *LogChecker) StopMark(msg string) *LogChecker {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.stopMark = msg
	lc.stopped = false
	return lc
}

// Check returns the filtered slice and whether the stop mark is reached.
func (lc *LogChecker) Check(d time.Duration) (filtered []bool, stopped bool) {
	stop := time.Now().Add(d)

	for {
		lc.mu.Lock()
		stopped = lc.stopped
		lc.mu.Unlock()

		if time.Now().After(stop) || stopped {
			lc.mu.Lock()
			f := make([]bool, len(lc.filtered))
			copy(f, lc.filtered)
			lc.mu.Unlock()
			return f, stopped
		}
		time.Sleep(10 * time.Millisecond)
	}
}
