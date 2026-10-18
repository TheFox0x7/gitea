// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func clfRecord(attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC), LevelInfo, "GET /path", 0)
	r.AddAttrs(attrs...)
	return r
}

func TestCLFFormatGolden(t *testing.T) {
	st := &sinkState{name: "t", format: FormatCLF}
	rec := clfRecord(
		slog.String("remote_host", "10.0.0.1"),
		slog.String("identity", "fox"),
		slog.String("method", "GET"),
		slog.String("path", "/path?q=1"),
		slog.String("proto", "HTTP/2.0"),
		slog.Int64("status", 200),
		slog.Int64("size", 8341),
		slog.String("referer", "https://example.com"),
		slog.String("user_agent", "Mozilla/5.0"),
	)
	assert.Equal(t, `10.0.0.1 - fox [02/Jan/2000:03:04:05 +0000] "GET /path?q=1 HTTP/2.0" 200 8341 "https://example.com" "Mozilla/5.0"`+"\n",
		string(st.formatCLF(&rec)))
}

func TestCLFFormatMissingFields(t *testing.T) {
	st := &sinkState{name: "t", format: FormatCLF}
	// no attrs at all: every field falls back to "-", zero size renders "-"
	empty := clfRecord()
	assert.Equal(t, `- - - [02/Jan/2000:03:04:05 +0000] "- - -" 0 - "-" "-"`+"\n",
		string(st.formatCLF(&empty)))

	// zero size is "-" per the CLF spec
	rec := clfRecord(slog.Int64("size", 0), slog.Int64("status", 404))
	assert.Equal(t, `- - - [02/Jan/2000:03:04:05 +0000] "- - -" 404 - "-" "-"`+"\n",
		string(st.formatCLF(&rec)))
}

func TestCLFFormatEscaping(t *testing.T) {
	st := &sinkState{name: "t", format: FormatCLF}
	rec := clfRecord(
		slog.String("identity", `weird"user`),
		slog.String("user_agent", "agent \"quoted\" \\ back\x01slash"),
	)
	assert.Equal(t, `- - weird"user [02/Jan/2000:03:04:05 +0000] "- - -" 0 - "-" "agent \"quoted\" \\ back\x01slash"`+"\n",
		string(st.formatCLF(&rec)))
}
