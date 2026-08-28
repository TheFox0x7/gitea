// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package slog provides Gitea's logging on top of log/slog: named loggers fan
// out records to sinks (console/file/conn), each sink owns exactly one format
// (legacy text, logfmt, json) and one dynamic level.
//
// Architecture (see slog_design.md):
//
//	Logger (facade) → multiHandler (OR-Enabled fan-out) → sink frontends →
//	refcounted shared backends (bounded queue + one goroutine + io.Writer).
//
// The printf-style API is deprecated and kept only for gradual migration from
// modules/log; new code should use Logger.Slog() for structured logging.
package slog
