// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package openapi

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3gen"
)

type schemaRegistry struct {
	schemas openapi3.Schemas
	gen     *openapi3gen.Generator
}

// reflectSchema reflects a Go type or value into a schema ref. Named struct
// types come out as component $refs (recursive types as $ref cycles);
// ExampleProvider values are normalized through encoding/json so kin's
// validator only sees JSON-shaped values (ADR 0002 addendum).
func (r *OpenAPIRouter) reflectSchema(v any) (*openapi3.SchemaRef, error) {
	if v == nil {
		return &openapi3.SchemaRef{Value: openapi3.NewSchema()}, nil
	}
	if r.schemas.gen == nil {
		r.schemas.gen = openapi3gen.NewGenerator(
			openapi3gen.SchemaCustomizer(customizeSchema),
			openapi3gen.CreateComponentSchemas(openapi3gen.ExportComponentSchemasOptions{
				ExportComponentSchemas: true,
				ExportTopLevelSchema:   true,
			}),
		)
	}
	return r.schemas.gen.NewSchemaRefForValue(v, r.schemas.schemas)
}

// customizeSchema maps the doc struct tags onto schema fields (doc, enum,
// default) and attaches ExampleProvider examples, normalized.
func customizeSchema(name string, t reflect.Type, tag reflect.StructTag, schema *openapi3.Schema) error {
	if desc, ok := tag.Lookup("doc"); ok {
		schema.Description = desc
	}
	if en, ok := tag.Lookup("enum"); ok {
		var values []any
		for _, v := range strings.Split(en, ",") {
			values = append(values, v)
		}
		schema.Enum = values
	}
	if def, ok := tag.Lookup("default"); ok {
		schema.Default = def
	}
	if ep, ok := reflect.PointerTo(t).Interface().(ExampleProvider); ok {
		schema.Example = exampleValue(ep.Example())
	}
	return nil
}

// exampleValue roundtrips a typed instance through encoding/json so kin's
// validator sees a JSON-shaped value, not a struct instance.
func exampleValue(v any) any {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
