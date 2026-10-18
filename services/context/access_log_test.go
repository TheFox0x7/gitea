// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"bytes"
	stdslog "log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"gitea.dev/modules/json"
	"gitea.dev/modules/setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAccessLoggerResponseWriterMock struct{}

func (t testAccessLoggerResponseWriterMock) Header() http.Header {
	return nil
}

func (t testAccessLoggerResponseWriterMock) Before(f func(ResponseWriter)) {}

func (t testAccessLoggerResponseWriterMock) WriteHeader(statusCode int) {}

func (t testAccessLoggerResponseWriterMock) Write(bytes []byte) (int, error) {
	return 0, nil
}

func (t testAccessLoggerResponseWriterMock) Flush() {}

func (t testAccessLoggerResponseWriterMock) WrittenStatus() int {
	return http.StatusOK
}

func (t testAccessLoggerResponseWriterMock) WrittenSize() int {
	return 123123
}

type failingResponseWriterMock struct {
	testAccessLoggerResponseWriterMock
}

func (t failingResponseWriterMock) WrittenStatus() int {
	return http.StatusInternalServerError
}

func newTestAccessRecorder(t *testing.T) (*accessLogRecorder, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	recorder := &accessLogRecorder{logger: stdslog.New(stdslog.NewJSONHandler(buf, nil))}
	return recorder, buf
}

func testAccessRequest() *http.Request {
	req := &http.Request{
		RemoteAddr: "remote-addr",
		Method:     http.MethodGet,
		Proto:      "https",
		URL:        &url.URL{Path: "/path"},
	}
	req.Header = http.Header{}
	req.Header.Add("Referer", "referer")
	req.Header.Add("User-Agent", "user-agent")
	return req
}

func decodeAccessRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec))
	return rec
}

func TestAccessLogger(t *testing.T) {
	recorder, buf := newTestAccessRecorder(t)
	recorder.record(time.Now(), &testAccessLoggerResponseWriterMock{}, testAccessRequest())

	rec := decodeAccessRecord(t, buf)
	assert.Equal(t, "GET /path", rec["msg"])
	assert.Equal(t, "GET", rec["method"])
	assert.Equal(t, "/path", rec["path"])
	assert.Equal(t, "https", rec["proto"])
	assert.InDelta(t, float64(http.StatusOK), rec["status"], 0)
	assert.InDelta(t, float64(123123), rec["size"], 0)
	assert.Equal(t, "-", rec["identity"])
	assert.Equal(t, "referer", rec["referer"])
	assert.Equal(t, "user-agent", rec["user_agent"])
	assert.NotEmpty(t, rec["remote_host"])
	assert.Contains(t, rec, "duration_ms")
	assert.NotContains(t, rec, "request_id")
}

func TestAccessLoggerRequestID(t *testing.T) {
	oldHeaders := setting.Log.RequestIDHeaders
	setting.Log.RequestIDHeaders = []string{"X-Request-Id"}
	t.Cleanup(func() { setting.Log.RequestIDHeaders = oldHeaders })

	recorder, buf := newTestAccessRecorder(t)
	recorder.needRequestID = true
	req := testAccessRequest()
	req.Header.Set("X-Request-Id", "req-123")
	recorder.record(time.Now(), &testAccessLoggerResponseWriterMock{}, req)
	assert.Equal(t, "req-123", decodeAccessRecord(t, buf)["request_id"])

	assert.False(t, isSafeRequestID("\x00"))
	assert.True(t, isSafeRequestID("a b-c"))
}

func TestAccessLoggerServerErrorLevel(t *testing.T) {
	recorder, buf := newTestAccessRecorder(t)
	recorder.record(time.Now(), &failingResponseWriterMock{}, testAccessRequest())
	rec := decodeAccessRecord(t, buf)
	assert.Equal(t, "ERROR", rec["level"])
	assert.InDelta(t, float64(http.StatusInternalServerError), rec["status"], 0)
}
