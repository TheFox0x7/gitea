// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
)

var unknown = []byte("???")

// captureStack records the PCs of the frames above the caller of captureStack:
// skip 0 starts at captureStack's caller.
func captureStack(skip int) []uintptr {
	pcs := make([]uintptr, 64) // runtime.Callers never grows the slice; 64 frames is ample
	return pcs[:runtime.Callers(skip+1, pcs)]
}

// Stack skips back the provided number of frames and returns a stack trace
// with source code. A skip of 0 returns the stack trace for the calling
// function, not including this call.
func Stack(skip int) string {
	return formatLegacyStack(captureStack(skip + 1)) // +1: skip the Stack frame itself
}

// formatLegacyStack renders frames in the historical text format:
//
//	file:line (0xpc)\n\tfunc: source\n
func formatLegacyStack(pcs []uintptr) string {
	if len(pcs) == 0 {
		return ""
	}
	buf := new(bytes.Buffer)

	// Store the last file we opened as it's probable that the preceding stack
	// frame will be in the same file
	var lines [][]byte
	var lastFilename string
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()

		_, _ = fmt.Fprintf(buf, "%s:%d (0x%x)\n", frame.File, frame.Line, frame.PC)
		if frame.File != lastFilename {
			data, err := os.ReadFile(frame.File)
			if err != nil {
				if !more {
					break
				}
				continue
			}
			lines = bytes.Split(data, []byte{'\n'})
			lastFilename = frame.File
		}
		_, _ = fmt.Fprintf(buf, "\t%s: %s\n", shortFunctionName(frame.Function), source(lines, frame.Line))
		if !more {
			break
		}
	}
	return buf.String()
}

// formatStackFrames renders frames in the conventional structured-log layout
// (zap's shape, what stack parsers expect): no PCs, no source reads, and the
// trailing runtime frame dropped.
//
//	func\n\tfile:line\nfunc2\n\tfile2:line2
func formatStackFrames(pcs []uintptr) string {
	var b strings.Builder
	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if !more {
			break // the last frame is runtime.main or runtime.goexit: noise
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(frame.Function)
		b.WriteString("\n\t")
		b.WriteString(frame.File)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(frame.Line))
	}
	return b.String()
}

// stackHandler injects a "stacktrace" attr for the Slog() path: the stdlib has
// no stack mechanism, and Handle runs on the caller goroutine, so the captured
// stack is still the caller's.
type stackHandler struct {
	inner slog.Handler
	level Level
}

var _ slog.Handler = (*stackHandler)(nil)

func (h *stackHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *stackHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= h.level {
		r.AddAttrs(slog.String("stacktrace", formatStackFrames(captureStack(3))))
	}
	return h.inner.Handle(ctx, r)
}

func (h *stackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &stackHandler{inner: h.inner.WithAttrs(attrs), level: h.level}
}

func (h *stackHandler) WithGroup(name string) slog.Handler {
	return &stackHandler{inner: h.inner.WithGroup(name), level: h.level}
}

// shortFunctionName strips the package path and keeps the last two elements,
// e.g. "gitea.dev/modules/slog.(*Logger).Info" -> "(*Logger).Info".
func shortFunctionName(name string) string {
	if lastslash := strings.LastIndexByte(name, '/'); lastslash >= 0 {
		name = name[lastslash+1:]
	}
	if period := strings.IndexByte(name, '.'); period >= 0 {
		name = name[period+1:]
	}
	return strings.ReplaceAll(name, "·", ".")
}

func source(lines [][]byte, n int) []byte {
	n-- // in stack traces, lines are 1-indexed but our array is 0-indexed
	if n < 0 || n >= len(lines) {
		return unknown
	}
	return bytes.TrimSpace(lines[n])
}
