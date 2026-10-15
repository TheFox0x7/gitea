// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"fmt"
	"io"
	"sync"
)

var (
	sinkTypeMu      sync.RWMutex
	registeredSinks = map[string]func(SinkConfig) (io.WriteCloser, error){}
)

// RegisterSinkType adds a custom sink writer type (e.g. a test sink writing to
// the testing framework). It must be called before Apply. Note: this is a
// package-level registry for now; a no-globals design would make it a Manager
// option instead (see slog_new_design.md §5.2).
func RegisterSinkType(name string, open func(SinkConfig) (io.WriteCloser, error)) {
	if name == "" || open == nil {
		panic("sink type name and open function must not be empty")
	}
	sinkTypeMu.Lock()
	defer sinkTypeMu.Unlock()
	registeredSinks[name] = open
}

func hasSinkType(name string) bool {
	sinkTypeMu.RLock()
	defer sinkTypeMu.RUnlock()
	_, ok := registeredSinks[name]
	return ok
}

func openRegisteredSink(cfg SinkConfig) (io.WriteCloser, error) {
	sinkTypeMu.RLock()
	open, ok := registeredSinks[cfg.Type]
	sinkTypeMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown sink type %q", cfg.Type)
	}
	return open(cfg)
}
