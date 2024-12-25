// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package tailmsg

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"code.gitea.io/gitea/modules/log"
	"go.opentelemetry.io/otel/sdk/trace"
)

type MsgRecord struct {
	Time    time.Time
	Content string
}

type MsgRecorder interface {
	trace.SpanExporter
	GetRecords() []*MsgRecord
}

type memoryMsgRecorder struct {
	mu    sync.RWMutex
	msgs  []*MsgRecord
	limit int
}

// TODO: use redis for a clustered environment

func (m *memoryMsgRecorder) ExportSpans(_ context.Context, spans []trace.ReadOnlySpan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range spans {
		sb := strings.Builder{}

		SpanToString(t, &sb, 2)
		m.msgs = append(m.msgs, &MsgRecord{
			Time:    time.Now(),
			Content: sb.String(),
		})
		if len(m.msgs) > m.limit {
			m.msgs = m.msgs[len(m.msgs)-m.limit:]
		}

	}
	return nil
}
func SpanToString(t trace.ReadOnlySpan, out *strings.Builder, indent int) {

	out.WriteString(strings.Repeat(" ", indent))
	out.WriteString(t.Name())
	if t.EndTime().IsZero() {
		out.WriteString(" duration: (not ended)")
	} else {
		out.WriteString(fmt.Sprintf(" duration: %.4fs", t.EndTime().Sub(t.StartTime()).Seconds()))
	}
	out.WriteString("\n")
	for _, a := range t.Attributes() {
		out.WriteString(strings.Repeat(" ", indent+2))
		out.WriteString(string(a.Key))
		out.WriteString(": ")
		out.WriteString(a.Value.AsString())
		out.WriteString("\n")
	}
	// for _, c := range t.Parent()..children {
	// 	span := c.internalSpans[t.internalSpanIdx].(*traceBuiltinSpan)
	// 	span.toString(out, indent+2)
	// }
}
func (m *memoryMsgRecorder) Shutdown(_ context.Context) error {
	log.Warn("Shutdown has been called!")
	return nil
}

func (m *memoryMsgRecorder) GetRecords() []*MsgRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret := make([]*MsgRecord, len(m.msgs))
	copy(ret, m.msgs)
	return ret
}

func NewMsgRecorder(limit int) MsgRecorder {
	return &memoryMsgRecorder{
		limit: limit,
	}
}

type Manager struct {
	traceRecorder MsgRecorder
	logRecorder   MsgRecorder
}

func (m *Manager) GetTraceRecorder() MsgRecorder {
	return m.traceRecorder
}

func (m *Manager) GetLogRecorder() MsgRecorder {
	return m.logRecorder
}

var GetManager = sync.OnceValue(func() *Manager {
	return &Manager{
		traceRecorder: NewMsgRecorder(100),
		logRecorder:   NewMsgRecorder(1000),
	}
})
