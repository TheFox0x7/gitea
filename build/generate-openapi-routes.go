// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// generate-openapi-routes projects the API route table (the contract) into
// both OpenAPI 3 and Swagger 2 specs. Both artifacts come from one source —
// the route rows — so they cannot drift.
//
// Run: go run build/generate-openapi-routes.go
// Outputs: templates/swagger/v1-openapi3.generated.json, templates/swagger/v1-swagger.generated.json
//
//go:build ignore

package main

import (
	"fmt"
	"os"

	"gitea.dev/routers/api/v1"
)

func main() {
	spec3, spec2, err := v1.BuildOpenAPISpecs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "building specs: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("templates/swagger/v1-openapi3.generated.json", spec3, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "writing openapi3 spec: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("templates/swagger/v1-swagger.generated.json", spec2, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "writing swagger spec: %v\n", err)
		os.Exit(1)
	}
}
