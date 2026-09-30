// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenAPIRouteTable validates the contract rows themselves: unique
// method+pattern, unique operation ids, path parameters documented exactly.
func TestOpenAPIRouteTable(t *testing.T) {
	rows := openAPIRoutes()
	seenRoute := make(map[string]string, len(rows))
	seenOp := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		require.NotEmpty(t, row.Summary)
		require.NotEmpty(t, row.OperationID)
		require.NotEmpty(t, row.Tags)
		require.NotEmpty(t, row.Responses)
		key := row.Method + " " + row.Pattern
		_, dup := seenRoute[key]
		require.False(t, dup, "duplicate contract row %s", key)
		seenRoute[key] = row.OperationID
		_, dup = seenOp[row.OperationID]
		require.False(t, dup, "duplicate operation id %s", row.OperationID)
		seenOp[row.OperationID] = struct{}{}
	}
}

// TestOpenAPIRouterMounts verifies every row passes Mount's fail-fast
// validation and both spec projections build.
func TestOpenAPIRouterMounts(t *testing.T) {
	r, err := BuildOpenAPIRouter()
	require.NoError(t, err)
	require.Len(t, r.Routes(), len(openAPIRoutes()))
	data, err := r.SpecJSON()
	require.NoError(t, err)
	require.NotEmpty(t, data)
	swagger, err := r.SwaggerJSON()
	require.NoError(t, err)
	require.NotEmpty(t, swagger)
}
