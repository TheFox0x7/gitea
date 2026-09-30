// Stage 4: generators — app.example.ini (and schema) rendered from the registry.
// Stage 5: main() walks the whole pipeline on the database section.
package main

import (
	"fmt"
	"sort"
	"strings"
)

// genExampleINI renders the registry as an app.example.ini fragment,
// reproducing today's "grouped, mostly-commented" style from ExampleGroups.
func genExampleINI(reg []Opt) string {
	var b strings.Builder
	b.WriteString("; Generated from the config registry. DO NOT EDIT.\n")
	b.WriteString("[database]\n")
	for _, g := range []string{"mysql", "postgres", "mssql", "sqlite3", ""} {
		switch g {
		case "mysql":
			b.WriteString("\n;;\n;; MySQL Configuration\n;;\n")
		case "postgres":
			b.WriteString("\n;;\n;; Postgres Configuration\n;;\n")
		case "mssql":
			b.WriteString("\n;;\n;; MSSQL Configuration\n;;\n")
		case "sqlite3":
			b.WriteString("\n;;\n;; SQLite Configuration\n;;\n")
		case "":
			b.WriteString("\n;;\n;; Other settings\n;;\n")
		}
		for _, o := range reg {
			if g == "" {
				if len(o.ExampleGroups) > 0 {
					continue
				}
			} else if !contains(o.ExampleGroups, g) {
				continue
			}
			def := ""
			if o.Def != nil {
				def = fmt.Sprint(o.Def)
			} else if o.DefFn != nil {
				def = fmt.Sprint(o.DefFn())
			}
			if o.HiddenFromWrite {
				fmt.Fprintf(&b, ";%s = ; see description (recommended: set via env)\n", o.Key)
				if o.Desc != "" {
					fmt.Fprintf(&b, "; %s\n", o.Desc)
				}
				continue
			}
			if def == "" && o.Key != "DB_TYPE" {
				fmt.Fprintf(&b, ";%s =\n", o.Key)
			} else if o.Key == "DB_TYPE" {
				fmt.Fprintf(&b, "DB_TYPE = %s\n", def) // today's file has it uncommented for mysql
			} else {
				fmt.Fprintf(&b, ";%s = %s\n", o.Key, def)
			}
			if o.ExampleComment != "" {
				fmt.Fprintf(&b, "; %s\n", o.ExampleComment)
			}
		}
	}
	return b.String()
}

// genJSONSchema emits a fragment of the JSON Schema artifact (goal 2),
// usable by editors and `gitea config check`.
func genJSONSchema(reg []Opt) string {
	var b strings.Builder
	b.WriteString("{\n  \"$schema\": \"https://json-schema.org/draft/2020-12/schema\",\n")
	b.WriteString("  \"type\": \"object\",\n  \"properties\": {\n    \"database\": {\n      \"type\": \"object\",\n      \"properties\": {\n")
	keys := make([]string, 0)
	for _, o := range reg {
		keys = append(keys, o.Key)
	}
	sort.Strings(keys)
	for i, k := range keys {
		o := findOpt(reg, k)
		fmt.Fprintf(&b, "        %q: {\"type\": %q, \"description\": %q}", k, schemaType(o.Type), o.Desc)
		if len(o.Enum) > 0 {
			vals := make([]string, len(o.Enum))
			for j, e := range o.Enum {
				vals[j] = fmt.Sprintf("%q", fmt.Sprint(e))
			}
			fmt.Fprintf(&b, ", \"enum\": [%s]", strings.Join(vals, ", "))
		}
		if o.Deprecated != "" {
			fmt.Fprintf(&b, ", \"deprecated\": true, \"x-replacement\": %q, \"x-removed-in\": %q", o.Deprecated, o.RemovalIn)
		}
		if i < len(keys)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("      }\n    }\n  }\n}")
	return b.String()
}

func schemaType(t OptType) string {
	switch t {
	case TInt, TDuration:
		return "integer" // durations serialized as seconds in schema terms
	case TBool:
		return "boolean"
	}
	return "string"
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func findOpt(reg []Opt, key string) Opt {
	for _, o := range reg {
		if o.Key == key {
			return o
		}
	}
	return Opt{}
}

func main() {
	// ----- goal 3: scattered files, mixed formats, merged in order -----
	files := []struct {
		name, ext, content string
	}{
		{"base.ini", ".ini", `
[database]
DB_TYPE = postgres
HOST = 127.0.0.1:5432
NAME = gitea
USER = gitea
LOG_SQL = true
`},
		{"override.toml", ".toml", `
[database]
HOST = db.internal:5432 ; override just the host
CHARSET_COLLATION = ; typo'd comment style, ignored
`},
		{"extra.yaml", ".yaml", `
database:
  MAX_OPEN_CONNS: 100
  SLOW_QUERY_THRESHOLD: 10s
`},
	}

	doc := Document{}
	for _, f := range files {
		loader, err := LoaderFor(f.ext)
		if err != nil {
			panic(err)
		}
		d, err := loader.Load([]byte(f.content))
		if err != nil {
			panic(err)
		}
		fmt.Printf("loaded %s via %s loader: %v\n", f.name, loader.Name(), d["database"])
		doc.MergeFrom(d)
	}

	// ----- goal 5: env as a formal Source, applied after files -----
	err := EnvSource{Environ: []string{
		"GITEA__DATABASE__PASSWD=secret-from-env",
		"GITEA__DATABASE__DB_RETRIES=25",
	}}.Apply(doc)
	if err != nil {
		panic(err)
	}

	// ----- goal 2: validate against the registry -----
	problems := Validate(doc, dbRegistry)
	fmt.Println("\n== validation problems ==")
	for _, p := range problems {
		fmt.Printf("  [%s].%s: %s\n", p.Section, p.Key, p.Message)
	}
	if len(problems) == 0 {
		fmt.Println("  (none)")
	}

	// ----- hydrate the typed struct (no globals, no MustXxx pollution) -----
	db := &Database{}
	_ = db.Hydrate(doc, dbRegistry)
	fmt.Printf("\n== hydrated Database ==\n%+v\n", *db)

	// ----- goal: generated artifacts -----
	fmt.Println("\n== generated app.example.ini fragment ==")
	fmt.Println(genExampleINI(dbRegistry))
	fmt.Println("== generated JSON Schema fragment ==")
	fmt.Println(genJSONSchema(dbRegistry))
}
