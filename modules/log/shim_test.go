// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package log

import (
	"context"
	stdslog "log/slog"
	"strings"
	"sync"
	"testing"

	"gitea.dev/modules/slog"

	"github.com/stretchr/testify/assert"
)

type captureHandler struct {
	mu      sync.Mutex
	sources []stdslog.Source
}

func (h *captureHandler) Enabled(_ context.Context, _ stdslog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r stdslog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if src := r.Source(); src != nil {
		h.sources = append(h.sources, *src)
	}
	return nil
}

func (h *captureHandler) WithAttrs(attrs []stdslog.Attr) stdslog.Handler { return h }

func (h *captureHandler) WithGroup(name string) stdslog.Handler { return h }

func (h *captureHandler) last() stdslog.Source {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sources[len(h.sources)-1]
}

//go:noinline
func shimViaInfo() { Info("via-info") }

//go:noinline
func shimViaWarn() { GetLogger(DEFAULT).Warn("via-warn") }

//go:noinline
func shimViaLog() { Log(1, INFO, "via-log") }

//go:noinline
func shimViaLoggerLog() { GetLogger(DEFAULT).Log(1, &Event{Level: INFO}, "via-logger-log") }

// TestCallerAttribution pins the skip arithmetic of the compatibility shim:
// logged caller must be the caller of the shim function, not the shim itself.
func TestCallerAttribution(t *testing.T) {
	m := slog.GetManager() // the shim's engine
	t.Cleanup(m.Close)
	h := &captureHandler{}
	m.AddHandler("default", h)

	shimViaInfo()
	assert.True(t, strings.HasSuffix(h.last().File, "shim_test.go"), "Info caller: %v", h.last())

	shimViaWarn()
	assert.True(t, strings.HasSuffix(h.last().File, "shim_test.go"), "Warn caller: %v", h.last())

	shimViaLog()
	assert.True(t, strings.HasSuffix(h.last().File, "shim_test.go"), "Log caller: %v", h.last())

	shimViaLoggerLog()
	assert.True(t, strings.HasSuffix(h.last().File, "shim_test.go"), "Logger.Log caller: %v", h.last())
}
