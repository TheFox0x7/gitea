# ADE: API route table as the OpenAPI contract

Date: 2026-10-20
Status: Proposed

## Context

The API surface was documented in `swagger:operation` comment blocks and
`swagger:response` wrapper structs, converted by go-swagger into a Swagger 2.0
spec, then by a second tool into OpenAPI 3.0. Three artifacts, none structurally
tied to the router: a served route could change while its documentation
stayed stale, and the only drift check was a generated-file diff.

## Decision

1. **A route registration IS the contract.** Every endpoint is one `Route`
   row — method, pattern, summary, operation id, parameters, request body,
   responses — in `routers/api/v1/openapi_routes_gen.go`, keyed to the handler
   it documents. Both specs project from the rows; there is no second
   artifact and no comment-parsing pipeline.
2. **No package-level mutable state.** Rows are returned by a plain function
   (`openAPIRoutes()`); the only stateful object is `OpenAPIRouter`
   (`modules/openapi`), created per document and owning the projected
   document and cached bytes.
3. **The engine is kin-openapi.** Schemas reflect from the Go DTOs
   (`modules/structs`) via openapi3gen with a `SchemaCustomizer`; recursive
   types (e.g. `api.Repository.Parent`) come out as `$ref` cycles. The v2
   projection uses openapi2conv from the same v3 document.
4. **Validation is enforced, not optional.** `Mount` fails fast on a row
   missing a summary, operation id, tags or responses, on duplicate
   method+pattern, and on path parameters that disagree with the pattern.
   `TestOpenAPIRouteTable` and `TestAPIContractMatchesServedRoutes` (route
   rows ↔ served chi routes) are the drift net.
5. **Examples follow the DTO ADR.** Types document themselves via the
   `ExampleProvider` interface; the reflector normalizes typed instances
   through `encoding/json` before attaching, so kin's validator sees
   JSON-shaped values. `doc`, `enum` and `default` struct tags shape schemas.
6. **Fail-fast over spec-diff.** `make generate-swagger` regenerates both
   `v1-swagger.generated.json` and `v1-openapi3.generated.json` from the rows
   (`build/generate-openapi-routes.go`); `swagger-check`/`openapi3-check`
   remain generated-file diffs, and `swagger-validate` runs the projection
   tests.

## Consequences

- The go-swagger toolchain, `routers/api/v1/swagger` wrapper structs and the
  2.0→3.0 converter are gone; one projection replaces two pipelines.
- The `swagger:operation` comments on handlers are inert documentation now;
  they can be removed incrementally, domain by domain.
- The generated rows file is reviewable in one place; hand edits are expected
  as the API matures (the generator is `contrib/openapi-port/gen_routes_doc.py`,
  kept for the initial bulk generation).
- The served spec endpoints (`/api/swagger.v1.json`, `/api/openapi3.v1.json`)
  keep the AppVer/AppSubURL placeholder substitution at serve time.
