// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package slog provides Gitea's logging on top of log/slog: named loggers fan
// out records to sinks (console/file/conn), each sink owns exactly one format
// (legacy text, logfmt, json) and one dynamic level.
//
// Lifecycle: before the configuration is applied, every logger is empty and
// silently discards events, so early errors are returned to the CLI runner
// (printed to stderr) instead of logged. A provisional stderr logger can be
// applied before config load for console output (see setting's console logger
// setup), and the real configuration is applied once loaded. A Fatal event is
// echoed to stderr if no sink accepted it, so early failures are never lost.
//
// Architecture (see slog_design.md):
//
//	Logger (facade) → multiHandler (OR-Enabled fan-out) → sink frontends →
//	refcounted shared backends (bounded queue + one goroutine + io.Writer).
//
// The printf-style API is deprecated and kept only for gradual migration from
// modules/log; new code should use Logger.Slog() for structured logging.
package slog
