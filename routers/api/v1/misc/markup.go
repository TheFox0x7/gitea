// Copyright 2014 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package misc

import (
	"net/http"
	"reflect"

	"code.gitea.io/gitea/modules/markup"
	"code.gitea.io/gitea/modules/markup/markdown"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/util"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/routers/common"
	"code.gitea.io/gitea/services/context"
	"github.com/danielgtaylor/huma/v2"
)

// Markup render markup document to HTML
func Markup(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation POST /markup miscellaneous renderMarkup
	// ---
	// summary: Render a markup document as HTML
	// parameters:
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/MarkupOption"
	// consumes:
	// - application/json
	// produces:
	//     - text/html
	// responses:
	//   "200":
	//     "$ref": "#/responses/MarkupRender"
	//   "422":
	//     "$ref": "#/responses/validationError"

	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/markup",
		Tags:        []string{"miscellaneous"},
		OperationID: "renderMarkup",
		Summary:     "Render a markup document as HTML",

		RequestBody: &huma.RequestBody{
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(api.MarkupOption{})),
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Rendered document",
				Content: map[string]*huma.MediaType{
					"text/html": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
					},
				},
			},
			"422": {
				Ref: "#/components/schemas/validationError",
			},
		},
	})
	return func(ctx *context.APIContext) {
		form := web.GetForm(ctx).(*api.MarkupOption)

		if ctx.HasAPIError() {
			ctx.APIError(http.StatusUnprocessableEntity, ctx.GetErrMsg())
			return
		}

		mode := util.Iif(form.Wiki, "wiki", form.Mode) //nolint:staticcheck
		common.RenderMarkup(ctx.Base, ctx.Repo, mode, form.Text, form.Context, form.FilePath)
	}
}

// Markdown render markdown document to HTML
func Markdown(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation POST /markdown miscellaneous renderMarkdown
	// ---
	// summary: Render a markdown document as HTML
	// parameters:
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/MarkdownOption"
	// consumes:
	// - application/json
	// produces:
	//     - text/html
	// responses:
	//   "200":
	//     "$ref": "#/responses/MarkdownRender"
	//   "422":
	//     "$ref": "#/responses/validationError"

	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/markdown",
		Tags:        []string{"miscellaneous"},
		OperationID: "renderMarkup",
		Summary:     "Render a markdown document as HTML",

		RequestBody: &huma.RequestBody{
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf(api.MarkdownOption{})),
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Rendered document",
				Content: map[string]*huma.MediaType{
					"text/html": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
					},
				},
			},
			"422": {
				Ref: "#/components/schemas/validationError",
			},
		},
	})
	return func(ctx *context.APIContext) {
		form := web.GetForm(ctx).(*api.MarkdownOption)

		if ctx.HasAPIError() {
			ctx.APIError(http.StatusUnprocessableEntity, ctx.GetErrMsg())
			return
		}

		mode := util.Iif(form.Wiki, "wiki", form.Mode) //nolint:staticcheck
		common.RenderMarkup(ctx.Base, ctx.Repo, mode, form.Text, form.Context, "")
	}
}

// MarkdownRaw render raw markdown HTML
func MarkdownRaw(oapi *huma.OpenAPI) func(ctx *context.APIContext) {
	// swagger:operation POST /markdown/raw miscellaneous renderMarkdownRaw
	// ---
	// summary: Render raw markdown as HTML
	// parameters:
	//     - name: body
	//       in: body
	//       description: Request body to render
	//       required: true
	//       schema:
	//         type: string
	// consumes:
	//     - text/plain
	// produces:
	//     - text/html
	// responses:
	//   "200":
	//     "$ref": "#/responses/MarkdownRender"
	//   "422":
	//     "$ref": "#/responses/validationError"
	oapi.AddOperation(&huma.Operation{
		Method:      http.MethodPost,
		Path:        "/markdown/raw",
		Tags:        []string{"miscellaneous"},
		OperationID: "renderMarkdownRaw",
		Summary:     "Render raw markdown as HTML",

		RequestBody: &huma.RequestBody{
			Required:    true,
			Description: "Request body to render",
			Content: map[string]*huma.MediaType{
				"text/plain": {
					Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "Rendered document",
				Content: map[string]*huma.MediaType{
					"text/html": {
						Schema: huma.SchemaFromType(oapi.Components.Schemas, reflect.TypeOf("")),
					},
				},
			},
			"422": {
				Ref: "#/components/schemas/validationError",
			},
		},
	})
	return func(ctx *context.APIContext) {
		defer ctx.Req.Body.Close()
		if err := markdown.RenderRaw(markup.NewRenderContext(ctx), ctx.Req.Body, ctx.Resp); err != nil {
			ctx.APIErrorInternal(err)
			return
		}
	}
}
