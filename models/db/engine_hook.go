// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"time"

	"code.gitea.io/gitea/modules/log"
	"go.opentelemetry.io/otel"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"

	"xorm.io/xorm/contexts"
)

var tracer = otel.Tracer("code.gitea.io/gitea/models/db")

type EngineHook struct {
	Threshold time.Duration
	Logger    log.Logger
}

var _ contexts.Hook = (*EngineHook)(nil)

func (*EngineHook) BeforeProcess(c *contexts.ContextHook) (context.Context, error) {
	ctx, _ := tracer.Start(c.Ctx, "database_query", trace.WithAttributes(semconv.DBQueryText(c.SQL)))

	return ctx, nil
}

func (h *EngineHook) AfterProcess(c *contexts.ContextHook) error {
	span := trace.SpanFromContext(c.Ctx)
	if c.Err != nil {
		span.RecordError(c.Err)
	}
	span.End()
	if c.ExecuteTime >= h.Threshold {
		// 8 is the amount of skips passed to runtime.Caller, so that in the log the correct function
		// is being displayed (the function that ultimately wants to execute the query in the code)
		// instead of the function of the slow query hook being called.
		h.Logger.Log(8, log.WARN, "[Slow SQL Query] %s %v - %v", c.SQL, c.Args, c.ExecuteTime)
	}
	return nil
}
