// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"

	"code.gitea.io/gitea/modules/options"
	repo_module "code.gitea.io/gitea/modules/repository"
	"code.gitea.io/gitea/modules/setting"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/services/context"
	"github.com/danielgtaylor/huma/v2"
)

// LicenseTemplateList
// swagger:response LicenseTemplateList
type LicensesTemplateList []api.LicensesTemplateListEntry

// Returns a list of all License templates
func ListLicenseTemplates(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation GET /licenses miscellaneous listLicenseTemplates
	// ---
	// summary: Returns a list of all license templates
	// produces:
	// - application/json
	// responses:
	//   "200":
	//     "$ref": "#/responses/LicenseTemplateList"
	//

	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/licenses",
		Tags:        []string{"miscellaneous"},
		OperationID: "listLicenseTemplates",
		Summary:     "Returns a list of all license template",
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Json template",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(LicensesTemplateList{})),
					},
				},
			},
		},
	})
	return func(ctx *context.APIContext) {
		response := make([]api.LicensesTemplateListEntry, len(repo_module.Licenses))
		for i, license := range repo_module.Licenses {
			response[i] = api.LicensesTemplateListEntry{
				Key:  license,
				Name: license,
				URL:  fmt.Sprintf("%sapi/v1/licenses/%s", setting.AppURL, url.PathEscape(license)),
			}
		}
		ctx.JSON(http.StatusOK, response)
	}
}

// Returns information about a gitignore template
func GetLicenseTemplateInfo(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/licenses/{name}",
		Tags:        []string{"miscellaneous"},
		OperationID: "getLicenseTemplateInfo",
		Summary:     "Returns information about a license template",
		Parameters: []*huma.Param{
			{Name: "name", In: "path", Description: "name of the license"},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Json template",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(api.LicenseTemplateInfo{})),
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

		text, err := options.License(name)
		if err != nil {
			ctx.APIErrorNotFound()
			return
		}

		response := api.LicenseTemplateInfo{
			Key:  name,
			Name: name,
			URL:  fmt.Sprintf("%sapi/v1/licenses/%s", setting.AppURL, url.PathEscape(name)),
			Body: string(text),
			// This is for combatibilty with the GitHub API. This Text is for some reason added to each License response.
			Implementation: "Create a text file (typically named LICENSE or LICENSE.txt) in the root of your source code and copy the text of the license into the file",
		}

		ctx.JSON(http.StatusOK, response)
	}
}
