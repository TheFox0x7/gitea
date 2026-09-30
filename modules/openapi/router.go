// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package openapi

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// ExampleProvider is implemented by API types that can produce a JSON example
// of themselves for the OpenAPI document (ADR: DTO handling).
type ExampleProvider interface {
	Example() any
}

// OpenAPIRouter owns one OpenAPI document and its cached spec bytes. Created
// per server; two routers in one process are isolated, and there is no
// package-level state.
type OpenAPIRouter struct {
	title           string
	version         string
	basePath        string
	routes          []Route
	routeKeys       map[string]struct{} // "METHOD pattern"
	securitySchemes map[string]SecurityScheme
	schemas         *schemaRegistry
	specCache       []byte
}

// NewOpenAPIRouter creates a router owning one OpenAPI document.
func NewOpenAPIRouter(title, version, basePath string) *OpenAPIRouter {
	return &OpenAPIRouter{
		title:      title,
		version:    version,
		basePath:   basePath,
		routeKeys:  map[string]struct{}{},
		schemas:    &schemaRegistry{schemas: openapi3.Schemas{}},
	}
}

// AddSecurityScheme registers a named scheme referenced by routes' Security.
func (r *OpenAPIRouter) AddSecurityScheme(name string, scheme SecurityScheme) {
	if r.securitySchemes == nil {
		r.securitySchemes = map[string]SecurityScheme{}
	}
	r.securitySchemes[name] = scheme
}

// Mount registers route rows into the document. It fails fast on any invalid
// row: missing summary, operation id, or responses, a duplicate
// method+pattern, an unknown security scheme, or a path parameter mismatch
// with the pattern. The route rows are the contract; the spec is their
// projection.
func (r *OpenAPIRouter) Mount(routes []Route) error {
	for i, route := range routes {
		if err := r.mount(route); err != nil {
			return fmt.Errorf("route %d %s %s: %w", i, route.Method, route.Pattern, err)
		}
	}
	return nil
}

func (r *OpenAPIRouter) mount(route Route) error {
	if route.Summary == "" {
		return fmt.Errorf("missing summary")
	}
	if route.OperationID == "" {
		return fmt.Errorf("missing operation id")
	}
	if len(route.Tags) == 0 {
		return fmt.Errorf("no tags")
	}
	if len(route.Responses) == 0 {
		return fmt.Errorf("no responses")
	}
	key := strings.ToUpper(route.Method) + " " + route.Pattern
	if _, dup := r.routeKeys[key]; dup {
		return fmt.Errorf("duplicate method+pattern")
	}
	for _, scheme := range route.Security {
		if _, ok := r.securitySchemes[scheme]; !ok {
			return fmt.Errorf("unknown security scheme %q", scheme)
		}
	}
	pattern := route.Pattern
	for _, p := range route.Params {
		if p.In == "path" && !strings.Contains(pattern, "{"+p.Name+"}") {
			return fmt.Errorf("path parameter %q absent from pattern", p.Name)
		}
	}
	for name := range patternParams(pattern) {
		found := false
		for _, p := range route.Params {
			if p.In == "path" && p.Name == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("pattern parameter %q not documented", name)
		}
	}
	r.routeKeys[key] = struct{}{}
	r.routes = append(r.routes, route)
	r.specCache = nil
	return nil
}

func patternParams(pattern string) map[string]struct{} {
	out := map[string]struct{}{}
	for len(pattern) > 0 {
		start := strings.IndexByte(pattern, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(pattern[start:], '}')
		if end < 0 {
			break
		}
		out[pattern[start+1:start+end]] = struct{}{}
		pattern = pattern[start+end:]
	}
	return out
}

// Routes returns the mounted rows (the contract) in registration order.
func (r *OpenAPIRouter) Routes() []Route {
	return r.routes
}

// SpecJSON builds the OpenAPI 3 document lazily and caches the bytes.
func (r *OpenAPIRouter) SpecJSON() ([]byte, error) {
	if r.specCache != nil {
		return r.specCache, nil
	}
	doc, err := r.BuildDoc()
	if err != nil {
		return nil, err
	}
	data, err := doc.MarshalJSON()
	if err != nil {
		return nil, err
	}
	r.specCache = data
	return data, nil
}

// BuildDoc projects the mounted rows into an OpenAPI 3.0 document.
func (r *OpenAPIRouter) BuildDoc() (*openapi3.T, error) {
	doc := &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:   r.title,
			Version: r.version,
		},
		Paths:      openapi3.NewPaths(),
		Components: &openapi3.Components{Schemas: openapi3.Schemas{}, SecuritySchemes: openapi3.SecuritySchemes{}},
	}
	if r.basePath != "" {
		doc.AddServer(&openapi3.Server{URL: r.basePath})
	}
	for name, scheme := range r.securitySchemes {
		ss := &openapi3.SecurityScheme{Type: scheme.Type, Description: scheme.Description}
		if scheme.Type == "apiKey" {
			ss.Name = scheme.Name
			ss.In = scheme.In
		}
		doc.Components.SecuritySchemes[name] = &openapi3.SecuritySchemeRef{Value: ss}
	}
	for _, route := range r.routes {
		op, err := r.buildOperation(route)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", route.Method, route.Pattern, err)
		}
		doc.AddOperation(route.Pattern, strings.ToLower(route.Method), op)
	}
	doc.Components.Schemas = r.schemas.schemas
	return doc, nil
}

func (r *OpenAPIRouter) buildOperation(route Route) (*openapi3.Operation, error) {
	op := &openapi3.Operation{
		OperationID: route.OperationID,
		Summary:     route.Summary,
		Description: route.Description,
		Tags:        route.Tags,
		Deprecated:  route.Deprecated,
		Responses:   openapi3.NewResponses(),
	}
	for _, p := range route.Params {
		op.Parameters = append(op.Parameters, &openapi3.ParameterRef{Value: &openapi3.Parameter{
			Name:        p.Name,
			In:          p.In,
			Description: p.Description,
			Required:    p.Required,
			Deprecated:  p.Deprecated,
			Schema:      &openapi3.SchemaRef{Value: paramSchema(p)},
		}})
	}
	if route.Body != nil {
		schemaRef, err := r.reflectSchema(route.Body.Schema)
		if err != nil {
			return nil, fmt.Errorf("request body schema: %w", err)
		}
		op.RequestBody = &openapi3.RequestBodyRef{Value: &openapi3.RequestBody{
			Description: route.Body.Description,
			Required:    route.Body.Required,
			Content:     openapi3.Content{"application/json": &openapi3.MediaType{Schema: schemaRef}},
		}}
	}
	codes := make([]int, 0, len(route.Responses))
	for code := range route.Responses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	for _, code := range codes {
		resp := route.Responses[code]
		response := openapi3.NewResponse().WithDescription(resp.Description)
		if resp.Schema != nil {
			schemaRef, err := r.reflectSchema(resp.Schema)
			if err != nil {
				return nil, fmt.Errorf("response %d schema: %w", code, err)
			}
			response.Content = openapi3.Content{"application/json": &openapi3.MediaType{Schema: schemaRef}}
		}
		op.Responses.Set(fmt.Sprintf("%d", code), &openapi3.ResponseRef{Value: response})
	}
	if len(route.Security) > 0 {
		requirement := openapi3.SecurityRequirement{}
		for _, scheme := range route.Security {
			requirement[scheme] = []string{}
		}
		op.Security = &openapi3.SecurityRequirements{requirement}
	}
	return op, nil
}

func paramSchema(p Param) *openapi3.Schema {
	s := &openapi3.Schema{}
	switch p.Type {
	case "integer":
		s.Type = &openapi3.Types{openapi3.TypeInteger}
		if p.Format == "int64" {
			s.Format = "int64"
		}
	case "boolean":
		s.Type = &openapi3.Types{openapi3.TypeBoolean}
	case "array":
		s.Type = &openapi3.Types{openapi3.TypeArray}
		s.Items = &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{openapi3.TypeString}}}
	default:
		s.Type = &openapi3.Types{openapi3.TypeString}
	}
	if len(p.Enum) > 0 {
		enum := make([]any, len(p.Enum))
		for i, v := range p.Enum {
			enum[i] = v
		}
		s.Enum = enum
	}
	if p.Default != nil {
		s.Default = p.Default
	}
	return s
}

// ServeSpec serves the OpenAPI 3 document.
func (r *OpenAPIRouter) ServeSpec(resp http.ResponseWriter, req *http.Request) {
	data, err := r.SpecJSON()
	if err != nil {
		http.Error(resp, "unable to build OpenAPI document: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp.Header().Set("Content-Type", "application/json")
	_, _ = resp.Write(data)
}
