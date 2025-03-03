// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"net/http"
	"reflect"

	"code.gitea.io/gitea/modules/options"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/structs"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	"github.com/danielgtaylor/huma/v2"
)

// GitignoreTemplateInfo
// swagger:response GitignoreTemplateInfo
type swaggerResponseGitignoreTemplateInfo api.GitignoreTemplateInfo

// GitignoreTemplateList
// swagger:response GitignoreTemplateList
type swaggerResponseGitignoreTemplateList []string

// Shows a list of all Gitignore templates
func ListGitignoresTemplates(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	oapi.AddOperation(&huma.Operation{
		OperationID: "listGitignoresTemplates",
		Method:      http.MethodGet,
		Path:        "/gitignore/templates",
		Tags:        []string{"miscellaneous"},
		Summary:     "Returns a list of all gitignore templates",
		Responses: map[string]*huma.Response{
			"200": {
				Description: "List of gitignore templates",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(swaggerResponseGitignoreTemplateList{})),
					},
				},
			},
		},
	})
	return func(ctx *context.APIContext) {
		ctx.JSON(http.StatusOK, repo_module.Gitignores)
	}
}

// SHows information about a gitignore template
func GetGitignoreTemplateInfo(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation GET /gitignore/templates/{name} miscellaneous getGitignoreTemplateInfo
	// ---
	// summary: Returns information about a gitignore template
	// produces:
	// - application/json
	// parameters:
	// - name: name
	//   in: path
	//   description: name of the template
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/GitignoreTemplateInfo"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//

	oapi.AddOperation(&huma.Operation{
		OperationID: "getGitignoreTemplateInfo",
		Method:      http.MethodGet,
		Path:        "/gitignore/templates/{name}",
		Tags:        []string{"miscellaneous"},
		Summary:     "Returns information about a gitignore template",

		Parameters: []*huma.Param{
			{
				Name:        "name",
				In:          "path",
				Description: "name of the template",
				// Schema:      huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
				// Required:    true,
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Information about a gitignore template",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(swaggerResponseGitignoreTemplateInfo{})),
					},
				},
			},
			"404": {
				Ref: "#/components/schemas/notFound",
			},
		},
	})

	return func(ctx *context.APIContext) {
		name := util.PathJoinRelX(ctx.PathParam("name"))

		text, err := options.Gitignore(name)
		if err != nil {
			ctx.APIErrorNotFound()
			return
		}

		ctx.JSON(http.StatusOK, &structs.GitignoreTemplateInfo{Name: name, Source: string(text)})
	}
}
