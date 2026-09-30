// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"testing"

	apiv1 "gitea.dev/routers/api/v1"

	"github.com/stretchr/testify/assert"
)

// TestAPIContractMatchesServedRoutes proves the documentation rows (the API
// contract) and the routes actually served describe the same surface: no
// documented-but-unregistered operation and no registered-but-undocumented
// one. This is the drift net replacing the old generated-spec diff
// (ADR 0001 decision 6).
func TestAPIContractMatchesServedRoutes(t *testing.T) {
	m := apiv1.Routes()
	served := make(map[string]struct{})
	for _, route := range m.Routes() {
		served[route.Method+" "+route.Pattern] = struct{}{}
	}
	rows := apiv1.OpenAPIRoutes()
	assert.NotEmpty(t, rows)
	for _, row := range rows {
		key := row.Method + " " + row.Pattern
		_, ok := served[key]
		assert.True(t, ok, "contract row %s (%s) is not served", key, row.OperationID)
	}
}
