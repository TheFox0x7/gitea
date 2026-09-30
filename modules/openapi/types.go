// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package openapi

// Route is one row of the API contract: method, pattern and documentation in
// a single literal. A route registration IS the contract; the spec is a
// projection of the rows (ADR 0001).
type Route struct {
	OperationID string
	Method      string
	Pattern     string
	Summary     string
	Tags        []string
	Description string
	Deprecated  bool
	Params      []Param
	Body        *RequestBody
	Responses   map[int]Response
	Security    []string
}

// Param documents one path/query/header/cookie parameter.
type Param struct {
	Name        string
	In          string // path, query, header or cookie
	Type        string // string, integer, boolean, array
	Format      string // e.g. int64
	Description string
	Required    bool
	Enum        []string
	Default     any
	Deprecated  bool
}

// RequestBody documents the operation's request payload.
type RequestBody struct {
	Description string
	Required    bool
	Schema      any // Go struct type or value, reflected into a component
	File        string // non-empty for a file upload field (multipart/binary)
}

// Response documents one HTTP response.
type Response struct {
	Description string
	Schema      any  // Go struct type or value, reflected into a component
	File       bool // binary payload
}

// SecurityScheme describes one named security scheme.
type SecurityScheme struct {
	Type        string // basic, apiKey
	Name        string
	In          string // query or header
	Description string
}
