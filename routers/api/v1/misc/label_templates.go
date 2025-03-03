// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"net/http"
	"reflect"

	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	"code.gitea.io/gitea/services/convert"
	"github.com/danielgtaylor/huma/v2"
)

// Shows a list of all Label templates
func ListLabelTemplates(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation GET /label/templates miscellaneous listLabelTemplates
	// ---
	// summary: Returns a list of all label templates
	// produces:
	// - application/json
	// responses:
	//   "200":
	//     "$ref": "#/responses/LabelTemplateList"
	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/label/templates",
		Tags:        []string{"miscellaneous"},
		OperationID: "listLabelTemplates",
		Summary:     "Returns a list of all label template",
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Json template",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf([]string{})),
					},
				},
			},
		},
	})
	return func(ctx *context.APIContext) {
		result := make([]string, len(repo_module.LabelTemplateFiles))
		for i := range repo_module.LabelTemplateFiles {
			result[i] = repo_module.LabelTemplateFiles[i].DisplayName
		}

		ctx.JSON(http.StatusOK, result)
	}
}

// Shows all labels in a template
func GetLabelTemplate(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation GET /label/templates/{name} miscellaneous getLabelTemplateInfo
	// ---
	// summary: Returns all labels in a template
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
	//     "$ref": "#/responses/LabelTemplateInfo"
	//   "404":
	//     "$ref": "#/responses/notFound"
	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/label/templates/{name}",
		Tags:        []string{"miscellaneous"},
		OperationID: "getLabelTemplateInfo",
		Summary:     "Returns all labels in a template",
		Parameters: []*huma.Param{
			{Name: "name", In: "Path", Description: "name of the template"},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Json template",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf([]structs.LabelTemplate{})),
					},
				},
			},
			"404": {
				Ref: "#/responses/notFound",
			},
		},
	})
	return func(ctx *context.APIContext) {
		name := util.PathJoinRelX(ctx.PathParam("name"))

		labels, err := repo_module.LoadTemplateLabelsByDisplayName(name)
		if err != nil {
			ctx.APIErrorNotFound()
			return
		}

		ctx.JSON(http.StatusOK, convert.ToLabelTemplateList(labels))
	}
}
