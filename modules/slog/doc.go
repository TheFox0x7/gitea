// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package slog provides Gitea's logging on top of log/slog: named loggers fan
// out records to sinks (console/file/conn), each sink owns exactly one format
// (legacy text, logfmt, json) and one dynamic level.
//
// Lifecycle: before a logger is configured, its events are captured in a
// bounded bootstrap buffer and replayed into the real sinks once an Apply
// configures it (see buffer.go). The CLI sets the capture floor from
// --quiet/--debug, and spills the buffer to stderr when a command finishes
// without ever applying a logging configuration. A Fatal event is echoed to
// stderr when no sink accepted it, so early failures are never lost. Writes
// are synchronous: a paused manager stalls writers until Resume, and a
// failing sink surfaces its line on stderr rather than dropping it.
//
// Architecture (see slog_design.md):
//
//	Logger (facade) → multiHandler (OR-Enabled fan-out) → sink frontends →
//	inline, pause-gated writes to the shared writer (Caddy's model: no queues,
//	no sink goroutines, no drops).
//
// Writers are opened through a usage-counted pool keyed by target identity
// (file path, address, stream): sinks with the same target share one writer,
// so rotation and connection state can never diverge; the writer closes when
// the last sink referencing it is stopped (see writer.go). Unconfigured
// loggers hold a bootstrap buffer child instead of sinks (see buffer.go).
//
// The printf-style API is deprecated and kept only for gradual migration from
// modules/log; new code should use Logger.Slog() for structured logging.
package slog
