// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package slog

import (
	"log/slog"
	"strconv"
)

// formatCLF renders an Apache combined log format line from structured
// request fields on the record (remote_host, identity, method, path, proto,
// status, size, referer, user_agent — as emitted by the access logger).
// Missing fields render as "-", size 0 renders as "-" per the CLF spec.
// The line timestamp is the record's time, i.e. request completion
// (nginx-style), not request start.
func (st *sinkState) formatCLF(r *slog.Record) []byte {
	host, identity, method, path, proto, referer, userAgent := "-", "-", "-", "-", "-", "-", "-"
	var status int64
	var size int64 = -1
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "remote_host":
			host = a.Value.String()
		case "identity":
			identity = a.Value.String()
		case "method":
			method = a.Value.String()
		case "path":
			path = a.Value.String()
		case "proto":
			proto = a.Value.String()
		case "status":
			status = a.Value.Int64()
		case "size":
			size = a.Value.Int64()
		case "referer":
			referer = a.Value.String()
		case "user_agent":
			userAgent = a.Value.String()
		}
		return true
	})

	b := make([]byte, 0, 192)
	b = append(b, host...)
	b = append(b, " - "...)
	b = append(b, identity...)
	b = append(b, " ["...)
	b = r.Time.AppendFormat(b, "02/Jan/2006:15:04:05 -0700")
	b = append(b, "] "...)
	b = appendCLFQuoted(b, method+" "+path+" "+proto)
	b = append(b, ' ')
	b = strconv.AppendInt(b, status, 10)
	b = append(b, ' ')
	if size > 0 {
		b = strconv.AppendInt(b, size, 10)
	} else {
		b = append(b, '-')
	}
	b = append(b, ' ')
	b = appendCLFQuoted(b, referer)
	b = append(b, ' ')
	b = appendCLFQuoted(b, userAgent)
	return append(b, '\n')
}

// appendCLFQuoted appends s as a CLF quoted string, escaping quotes,
// backslashes and non-printables per Apache's mod_log_config.
func appendCLFQuoted(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b = append(b, '\\', c)
		case c < 0x20 || c == 0x7f:
			b = append(b, '\\', 'x')
			const hex = "0123456789abcdef"
			b = append(b, hex[c>>4], hex[c&0xf])
		default:
			b = append(b, c)
		}
	}
	return append(b, '"')
}
