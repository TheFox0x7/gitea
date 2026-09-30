// PoC of the proposed config pipeline, demonstrated on the [database] section.
// Stage 1: the registry — the single source of truth that replaces
// hand-written app.example.ini docs + scattered loadDBSetting() defaults.
package main

import "time"

// OptType is the value type used for schema generation and validation.
type OptType string

const (
	TString   OptType = "string"
	TBool     OptType = "boolean"
	TInt      OptType = "integer"
	TDuration OptType = "duration" // Go time.Duration string, e.g. "3s"
)

// Opt describes one config key in the registry.
type Opt struct {
	Section string // "database"
	Key     string // "DB_TYPE"
	Type    OptType

	Def       any     // default value (nil = no default / required)
	Enum      []any   // allowed values, e.g. DB_TYPE
	DefFn     func() any // dynamic default, e.g. PATH depending on AppDataPath
	DependsOn map[string]any // conditional defaults/validity, e.g. SSL_MODE per DB_TYPE

	Deprecated  string // non-empty: replacement key name
	RemovedIn   string // version where it stops working

	Desc string // one-liner for app.example.ini + schema description
	ExampleComment string // extra prose emitted in the example file

	// Emission control for the example generator:
	// The real app.example.ini shows most keys commented-out per-DB-flavor.
	// The registry therefore tags keys with example groups instead of
	// duplicating the whole section four times by hand.
	ExampleGroups   []string // e.g. ["mysql","postgres"], nil = always shown
	HiddenFromWrite bool     // shown as comment in example, but never written by Gitea itself
}

// Registry for the database section, seeded from the real loadDBSetting()
// (modules/setting/database.go) and app.example.ini.
var dbRegistry = []Opt{
	{Section: "database", Key: "DB_TYPE", Type: TString,
		Enum: []any{"mysql", "postgres", "mssql", "sqlite3"},
		Desc: "Database to use. Either \"mysql\", \"postgres\", \"mssql\" or \"sqlite3\".",
		ExampleGroups: []string{"mysql", "postgres", "mssql", "sqlite3"}},

	{Section: "database", Key: "HOST", Type: TString,
		Desc:           "Database host. Can use a socket path e.g. /var/run/mysqld/mysqld.sock.",
		ExampleGroups:  []string{"mysql", "postgres", "mssql"},
		ExampleComment: "can use socket e.g. /var/run/mysqld/mysqld.sock"},

	{Section: "database", Key: "NAME", Type: TString, Def: "gitea",
		Desc: "Database name.", ExampleGroups: []string{"mysql", "postgres", "mssql"}},

	{Section: "database", Key: "USER", Type: TString,
		Desc: "Database user.", ExampleGroups: []string{"mysql", "postgres", "mssql"}},

	// The whole POINT of goal 6: PASSWD is documented, validated, but the
	// example generator marks it Hidden from any generated file; recommended
	// sources are env (GITEA__DATABASE__PASSWD) or GITEA__DATABASE__PASSWD__FILE.
	{Section: "database", Key: "PASSWD", Type: TString,
		Desc: "Database password. Use `your password` for quoting special characters.",
		ExampleComment: "Use PASSWD = `your password` for quoting if you use special characters in the password.",
		ExampleGroups: []string{"mysql", "postgres", "mssql"},
		HiddenFromWrite: true},

	{Section: "database", Key: "SCHEMA", Type: TString,
		Desc: "Postgres schema to use.", ExampleGroups: []string{"postgres"}},

	{Section: "database", Key: "SSL_MODE", Type: TString, Def: "disable",
		// conditional defaults + enums — formalized DependsOn instead of prose
		DependsOn: map[string]any{"DB_TYPE": "postgres"},
		Enum:      []any{"disable", "require", "verify-full"},
		Desc:      "Postgres SSL mode: \"disable\" (default), \"require\", or \"verify-full\".",
		ExampleGroups: []string{"postgres"}},

	{Section: "database", Key: "CHARSET_COLLATION", Type: TString, Def: "",
		Desc: "Empty as default, Gitea will try to find a case-sensitive collation. Don't change it unless you clearly know what you need.",
		ExampleGroups: []string{"mysql", "mssql"}},

	{Section: "database", Key: "PATH", Type: TString,
		DefFn: func() any { return "data/gitea.db" }, // real one: filepath.Join(AppDataPath, "gitea.db")
		Desc:          "SQLite database file path. Defaults to data/gitea.db.",
		ExampleGroups: []string{"sqlite3"}},

	{Section: "database", Key: "SQLITE_TIMEOUT", Type: TInt, Def: 20000,
		Desc: "SQLite query timeout in milliseconds; values below 5000 are clamped to 20000 (mattn driver quirk).",
		ExampleGroups: []string{"sqlite3"}},

	{Section: "database", Key: "SQLITE_JOURNAL_MODE", Type: TString, Def: "",
		Desc:          "Defaults to sqlite default (often DELETE), can enable WAL mode.",
		ExampleGroups: []string{"sqlite3"}},

	// cross-cutting keys — today they live at the bottom of the section
	{Section: "database", Key: "ITERATE_BUFFER_SIZE", Type: TInt, Def: 50,
		Desc: "Buffer size for iterating the database."},
	{Section: "database", Key: "LOG_SQL", Type: TBool, Def: false,
		Desc: "Show the database-generated SQL in logs."},
	{Section: "database", Key: "MAX_IDLE_CONNS", Type: TInt, Def: 2,
		Desc: "Max idle connections."},
	{Section: "database", Key: "MAX_OPEN_CONNS", Type: TInt, Def: 0,
		Desc: "Max open connections (0 = unlimited)."},
	{Section: "database", Key: "CONN_MAX_LIFETIME", Type: TDuration,
		DefFn: func() any { return 3 * time.Second }, // simplified: real default depends on DB_TYPE
		Desc: "Connection max lifetime (e.g. 3s)."},
	{Section: "database", Key: "DB_RETRIES", Type: TInt, Def: 10,
		Desc: "Number of connection retries at startup."},
	{Section: "database", Key: "DB_RETRY_BACKOFF", Type: TDuration,
		DefFn: func() any { return 3 * time.Second },
		Desc: "Backoff between connection retries."},
	{Section: "database", Key: "SLOW_QUERY_THRESHOLD", Type: TDuration,
		DefFn: func() any { return 5 * time.Second },
		Desc: "Threshold above which queries are logged as slow."},
}
