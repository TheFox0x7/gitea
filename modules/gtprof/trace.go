// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gtprof

import (
	"context"
	"fmt"
	"sync"
	"time"

	"code.gitea.io/gitea/modules/util"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"
)

type contextKey struct {
	name string
}

var ContextKeySpan = &contextKey{"span"}

type traceStarter interface {
	start(ctx context.Context, traceSpan *TraceSpan, internalSpanIdx int) (context.Context, traceSpanInternal)
}

type traceSpanInternal interface {
	end()
}

type TraceSpan struct {
	noop.Span
	// immutable
	parent        *TraceSpan
	internalSpans []traceSpanInternal

	// mutable, must be protected by mutex
	mu         sync.RWMutex
	name       string
	startTime  time.Time
	endTime    time.Time
	attributes []TraceAttribute
	children   []*TraceSpan
}

// IsRecording implements trace.Span.
func (s *TraceSpan) IsRecording() bool {
	return true
}

// SetAttributes implements trace.Span.
func (s *TraceSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range kv {
		s.attributes = append(s.attributes, TraceAttribute{
			Key:   string(kv[i].Key),
			Value: TraceValue{v: kv[i].Value.AsString()}})
	}
}

// SetName implements trace.Span.
func (s *TraceSpan) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.name = name
}

type TraceAttribute struct {
	Key   string
	Value TraceValue
}

type TraceValue struct {
	v any
}

func (t *TraceValue) AsString() string {
	return fmt.Sprint(t.v)
}

func (t *TraceValue) AsInt64() int64 {
	v, _ := util.ToInt64(t.v)
	return v
}

func (t *TraceValue) AsFloat64() float64 {
	v, _ := util.ToFloat64(t.v)
	return v
}

var globalTraceStarters []traceStarter

type InternalTracer struct {
	embedded.Tracer

	starters []traceStarter
}

func (t InternalTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	starters := t.starters
	if starters == nil {
		starters = globalTraceStarters
	}
	ts := &TraceSpan{name: spanName, startTime: time.Now()}
	existingCtxSpan := GetContextSpan(ctx)
	if existingCtxSpan != nil {
		existingCtxSpan.mu.Lock()
		existingCtxSpan.children = append(existingCtxSpan.children, ts)
		existingCtxSpan.mu.Unlock()
		ts.parent = existingCtxSpan
	}
	for internalSpanIdx, tsp := range starters {
		var internalSpan traceSpanInternal
		ctx, internalSpan = tsp.start(ctx, ts, internalSpanIdx)
		ts.internalSpans = append(ts.internalSpans, internalSpan)
	}
	ctx = context.WithValue(ctx, ContextKeySpan, ts)
	return ctx, ts
}

func (s *TraceSpan) End(options ...trace.SpanEndOption) {
	s.mu.Lock()
	s.endTime = time.Now()
	s.mu.Unlock()

	for _, tsp := range s.internalSpans {
		tsp.end()
	}
}

func GetContextSpan(ctx context.Context) *TraceSpan {
	ts, _ := ctx.Value(ContextKeySpan).(*TraceSpan)
	return ts
}

// InternalTracerProvider is a custom provider which handles internal tracers
type InternalTracerProvider struct{ embedded.TracerProvider }

// Tracer implements trace.Tracer for internal Tracing
func (InternalTracerProvider) Tracer(_ string, _ ...trace.TracerOption) trace.Tracer {
	return InternalTracer{}
}

func init() {
	otel.SetTracerProvider(InternalTracerProvider{})
}
