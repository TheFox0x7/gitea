// Copyright 2024 TheFox0x7. All rights reserved.
// SPDX-License-Identifier: EUPL-1.2

//go:build !opentelemetry

package opentelemetry

import (
	"context"

	"go.opentelemetry.io/otel/sdk/resource"
)

// Not implemented yet
func setupTraceProvider(ctx context.Context, r *resource.Resource) (func(context.Context) error, error) {
	return func(c context.Context) error { return nil }, nil
}
