# Config pipeline proof-of-concept

Standalone, dependency-free demo of the proposed configuration system,
exercised on the `[database]` section only. Not wired into the build or the
runtime in any way.

Run with:

```
go run ./contrib/config-poc
```

## Stages

| File | Stage |
|---|---|
| `registry.go` | Single source of truth per option: section/key, type, default (incl. dynamic `DefFn`), enum, conditional `DependsOn`, deprecation metadata, example-emission control |
| `pipeline.go` | Format-neutral `Document`, `Loader` (ini/toml/yaml), merge semantics, `Source` interface (env overlay, future vault), validation, struct hydration |
| `main.go` | Demo pipeline run + generators: `app.example.ini` fragment and JSON Schema fragment rendered from the registry |

## Demonstrates

1. Multiple config formats (ini, toml, yaml) auto-detected, all producing the
   same canonical `Document` tree.
2. Scattered files merged in order, later files winning per key.
3. Env vars (`GITEA__SECTION__KEY`) as a formal `Source` applied after files.
4. Validation against the registry: unknown keys, enum violations, type errors
   (the future `gitea config check`).
5. Typed struct hydration with no `MustXxx` write-back and no package globals.
6. `app.example.ini` and JSON Schema generated from the same registry, with
   `HiddenFromWrite` keys (e.g. `PASSWD`) never written by Gitea itself.

## Known simplifications

- Minimal parsers; real implementation wraps `gopkg.in/ini.v1`,
  `BurntSushi/toml` and `go.yaml.in/yaml`.
- `DependsOn` (conditional defaults like `SSL_MODE` per `DB_TYPE`) is declared
  but not yet consumed during hydration.
- The `app.example.ini` preamble prose and curated section ordering are not
  modeled yet.
