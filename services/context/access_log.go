// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"context"
	stdslog "log/slog"
	"net/http"
	"time"
	"unicode"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/httplib"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/slog"
	"gitea.dev/modules/web/middleware"
)

// requestIDMaxLength: a UUID/hex/trace id never exceeds 40 characters.
const requestIDMaxLength = 40

func isSafeRequestID(id string) bool {
	for _, r := range id {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func parseRequestIDFromRequestHeader(req *http.Request) string {
	requestID := "-"
	for _, key := range setting.Log.RequestIDHeaders {
		if req.Header.Get(key) != "" {
			requestID = req.Header.Get(key)
			break
		}
	}
	if !isSafeRequestID(requestID) {
		return "-"
	}
	if len(requestID) > requestIDMaxLength {
		requestID = requestID[:requestIDMaxLength] + "..."
	}
	return requestID
}

// accessLogRecorder emits one structured access record per request; the sinks
// decide the shape (FORMAT = clf renders an Apache combined log line, json
// emits the fields as-is). The old editable template is gone.
type accessLogRecorder struct {
	logger        *stdslog.Logger
	needRequestID bool
}

func (lr *accessLogRecorder) record(start time.Time, respWriter ResponseWriter, req *http.Request) {
	identity := "-"
	data := middleware.GetContextData(req.Context())
	if signedUser, ok := data[middleware.ContextDataKeySignedUser].(*user_model.User); ok {
		identity = signedUser.Name
	}

	attrs := make([]stdslog.Attr, 0, 10)
	attrs = append(attrs,
		stdslog.String("remote_host", httplib.RemoteHost(req)),
		stdslog.String("identity", identity),
		stdslog.String("method", req.Method),
		stdslog.String("path", req.URL.RequestURI()),
		stdslog.String("proto", req.Proto),
		stdslog.Int64("status", int64(respWriter.WrittenStatus())),
		stdslog.Int64("size", int64(respWriter.WrittenSize())),
		stdslog.Float64("duration_ms", float64(time.Since(start).Nanoseconds())/1e6),
		stdslog.String("referer", req.Referer()),
		stdslog.String("user_agent", req.UserAgent()),
	)
	if lr.needRequestID {
		attrs = append(attrs, stdslog.String("request_id", parseRequestIDFromRequestHeader(req)))
	}

	// server errors are logged at ERROR so alerting and error-only sinks can
	// pick them out (Caddy's access logger does the same)
	level := stdslog.LevelInfo
	if respWriter.WrittenStatus() >= 500 {
		level = stdslog.LevelError
	}
	lr.logger.LogAttrs(context.Background(), level, req.Method+" "+req.URL.RequestURI(), attrs...)
}

func newAccessLogRecorder() *accessLogRecorder {
	return &accessLogRecorder{
		logger:        slog.GetLogger("access").Slog(),
		needRequestID: len(setting.Log.RequestIDHeaders) > 0,
	}
}

// AccessLogger returns a middleware to log access logger
func AccessLogger() func(http.Handler) http.Handler {
	recorder := newAccessLogRecorder()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			respWriter := WrapResponseWriter(w)
			next.ServeHTTP(respWriter, req)
			recorder.record(start, respWriter, req)
		})
	}
}
