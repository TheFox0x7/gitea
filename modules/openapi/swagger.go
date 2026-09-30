// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package openapi

import (
	"encoding/json"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
)

// SwaggerJSON projects the same route rows into a Swagger 2.0 document via
// openapi2conv, for clients that still require v2. Both specs come from one
// artifact: the mounted rows.
func (r *OpenAPIRouter) SwaggerJSON() ([]byte, error) {
	doc, err := r.BuildDoc()
	if err != nil {
		return nil, err
	}
	v2, err := openapi2conv.FromV3(doc)
	if err != nil {
		return nil, err
	}
	if r.basePath != "" {
		v2.BasePath = r.basePath
	}
	return json.Marshal(v2)
}
