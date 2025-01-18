// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"time"

	"code.gitea.io/gitea/modules/gtprof"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"xorm.io/xorm/contexts"
)

type EngineHook struct {
	Threshold time.Duration
	Logger    log.Logger
}

var tracer = otel.Tracer("code.gitea.io/gitea/models/db")
var _ contexts.Hook = (*EngineHook)(nil)

func (*EngineHook) BeforeProcess(c *contexts.ContextHook) (context.Context, error) {
	ctx, _ := tracer.Start(c.Ctx, gtprof.TraceSpanDatabase)
	return ctx, nil
}

func (h *EngineHook) AfterProcess(c *contexts.ContextHook) error {
	span := gtprof.GetContextSpan(c.Ctx)
	if span != nil {
		span.SetAttributes(attribute.String(gtprof.TraceAttrDbSQL, c.SQL))
		span.End()
	} else {
		setting.PanicInDevOrTesting("span in database engine hook is nil")
	}
	if c.ExecuteTime >= h.Threshold {
		// 8 is the amount of skips passed to runtime.Caller, so that in the log the correct function
		// is being displayed (the function that ultimately wants to execute the query in the code)
		// instead of the function of the slow query hook being called.
		h.Logger.Log(8, log.WARN, "[Slow SQL Query] %s %v - %v", c.SQL, c.Args, c.ExecuteTime)
	}
	return nil
}
