// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"fmt"
	"net/http"
	"reflect"

	asymkey_service "code.gitea.io/gitea/services/asymkey"
	"code.gitea.io/gitea/services/context"
	"github.com/danielgtaylor/huma/v2"
)

// SigningKey returns the public key of the default signing key if it exists
func SigningKey(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation GET /signing-key.gpg miscellaneous getSigningKey
	// ---
	// summary: Get default signing-key.gpg
	// produces:
	//     - text/plain
	// responses:
	//   "200":
	//     description: "GPG armored public key"
	//     schema:
	//       type: string

	// swagger:operation GET /repos/{owner}/{repo}/signing-key.gpg repository repoSigningKey
	// ---
	// summary: Get signing-key.gpg for given repository
	// produces:
	//     - text/plain
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     description: "GPG armored public key"
	//     schema:
	//       type: string

	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/signing-key.gpg",
		Tags:        []string{"repository"},
		OperationID: "getSigningKey",
		Summary:     "Returns instance signing key",
		Responses: map[string]*huma.Response{
			"200": {
				Description: "GPG armored public key",
				Content: map[string]*huma.MediaType{
					"text/plain": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
					},
				},
			},
		},
	})
	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/repos/{owner}/{repo}/signing-key.gpg",
		Tags:        []string{"miscellaneous"},
		OperationID: "getSigningKey",
		Parameters: []*huma.Param{
			{Name: "owner", In: "path", Description: "Owner of the repo"},
			{Name: "repo", In: "path", Description: "name of the repo"},
		},
		Summary: "Returns instance signing key",
		Responses: map[string]*huma.Response{
			"200": {
				Description: "GPG armored public key",
				Content: map[string]*huma.MediaType{
					"text/plain": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
					},
				},
			},
		},
	})
	return func(ctx *context.APIContext) {
		path := ""
		if ctx.Repo != nil && ctx.Repo.Repository != nil {
			path = ctx.Repo.Repository.RepoPath()
		}

		content, err := asymkey_service.PublicSigningKey(ctx, path)
		if err != nil {
			ctx.APIErrorInternal(err)
			return
		}
		_, err = ctx.Write([]byte(content))
		if err != nil {
			ctx.APIErrorInternal(fmt.Errorf("Error writing key content %w", err))
		}
	}
}
