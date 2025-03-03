// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"net/http"
	"reflect"

	"code.gitea.io/gitea/modules/setting"
	"code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/services/context"
	"github.com/danielgtaylor/huma/v2"
)

// Version shows the version of the Gitea server
func Version(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation GET /version miscellaneous getVersion
	// ---
	// summary: Returns the version of the Gitea application
	// produces:
	// - application/json
	// responses:
	//   "200":
	//     "$ref": "#/responses/ServerVersion"

	oapi.AddOperation(&huma.Operation{
		Method: http.MethodGet,

		Path:        "/version",
		Tags:        []string{"miscellaneous"},
		OperationID: "getVersion",
		Summary:     "Returns the version of the Gitea application",
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Version",
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(structs.ServerVersion{})),
					},
				},
			},
		},
	})
	return func(ctx *context.APIContext) {
		ctx.JSON(http.StatusOK, &structs.ServerVersion{Version: setting.AppVer})
	}
}
