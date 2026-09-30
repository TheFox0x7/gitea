// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package v1

import (
	"fmt"

	oapi "gitea.dev/modules/openapi"
	"gitea.dev/modules/setting"
)

// OpenAPIRoutes returns the API's documentation rows (the contract).
func OpenAPIRoutes() []oapi.Route {
	return openAPIRoutes()
}

// BuildOpenAPIRouter mounts the API's documentation rows into a fresh
// OpenAPIRouter. The rows are the contract (ADR 0001): both the OpenAPI 3
// and Swagger 2 documents are projections of them, so they cannot drift.
func BuildOpenAPIRouter() (*oapi.OpenAPIRouter, error) {
	// the version and sub-url placeholders are substituted at serve time (routers/web/swagger_json.go)
	appVer := setting.AppVer
	if appVer == "" {
		appVer = "0.0.0+GITEA-API-APP-VERSION"
	}
	r := oapi.NewOpenAPIRouter("Gitea API", appVer, "/GITEA-API-APP-SUBURL/api/v1")
	for name, scheme := range securitySchemes {
		r.AddSecurityScheme(name, scheme)
	}
	if err := r.Mount(openAPIRoutes()); err != nil {
		return nil, fmt.Errorf("invalid route table: %w", err)
	}
	return r, nil
}

// BuildOpenAPISpecs projects the mounted rows into both specs.
func BuildOpenAPISpecs() (openapi3 []byte, swagger2 []byte, err error) {
	r, err := BuildOpenAPIRouter()
	if err != nil {
		return nil, nil, err
	}
	openapi3, err = r.SpecJSON()
	if err != nil {
		return nil, nil, err
	}
	swagger2, err = r.SwaggerJSON()
	if err != nil {
		return nil, nil, err
	}
	return openapi3, swagger2, nil
}

var securitySchemes = map[string]oapi.SecurityScheme{
	"BasicAuth":              {Type: "basic", Description: "Basic authentication"},
	"Token":                  {Type: "apiKey", Name: "token", In: "query", Description: "This authentication option is deprecated for removal in Gitea 1.23. Please use AuthorizationHeaderToken instead."},
	"AccessToken":            {Type: "apiKey", Name: "access_token", In: "query", Description: "This authentication option is deprecated for removal in Gitea 1.23. Please use AuthorizationHeaderToken instead."},
	"AuthorizationHeaderToken": {Type: "apiKey", Name: "Authorization", In: "header", Description: "API tokens must be prepended with \"token\" followed by a space."},
	"SudoParam":              {Type: "apiKey", Name: "sudo", In: "query", Description: "Sudo API request as the user provided as the key. Admin privileges are required."},
	"SudoHeader":             {Type: "apiKey", Name: "Sudo", In: "header", Description: "Sudo API request as the user provided as the key. Admin privileges are required."},
	"TOTPHeader":             {Type: "apiKey", Name: "X-GITEA-OTP", In: "header", Description: "Must be used in combination with BasicAuth if two-factor authentication is enabled."},
}
