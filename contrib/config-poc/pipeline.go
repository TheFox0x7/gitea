// Stage 2: Loaders produce a canonical Document (nested map) from any format.
// Stage 3: merge + env overlay + validation against the registry.
//
// Every value carries provenance: whether it was set by the user (and from
// which source) or fell back to the registry default. This distinction is a
// hard requirement of the design:
//   - generated config files may only contain user-set values (goal 6)
//   - validation reports user mistakes, not built-in defaults
//   - it mirrors config.Option.HasValue in the existing codebase
package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Origin identifies where a value came from.
type Origin string

const (
	OriginDefault Origin = "default" // not set by the user anywhere
	OriginFile    Origin = "file"    // set in a config file (which one, in Detail)
	OriginEnv     Origin = "env"     // set via GITEA__* environment variable
	OriginVault   Origin = "vault"   // future source, same treatment as env
)

// Value is one config value plus its provenance.
type Value struct {
	Raw    string
	Origin Origin
	Detail string // file name / env var name; empty for defaults
}

// SetByUser reports whether the user explicitly set this value.
func (v Value) SetByUser() bool { return v.Origin != OriginDefault }

// Document is the format-neutral config tree: section -> key -> value.
type Document map[string]map[string]Value

func (d Document) Set(section, key, raw string, origin Origin, detail string) {
	if d[section] == nil {
		d[section] = map[string]Value{}
	}
	d[section][key] = Value{Raw: raw, Origin: origin, Detail: detail}
}

func (d Document) Get(section, key string) Value {
	return d[section][key] // zero Value => not set anywhere, treated as default
}

// Loader loads one file (or source) into a Document.
type Loader interface {
	Name() string
	Load(data []byte) (Document, error)
}

// --- INI loader (read-only; today's gopkg.in/ini.v1 becomes an implementation detail) ---

type INILoader struct{}

func (INILoader) Name() string { return "ini" }

func (INILoader) Load(data []byte) (Document, error) {
	doc := Document{}
	sec := "DEFAULT"
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec = strings.Trim(line, "[]")
			if doc[sec] == nil {
				doc[sec] = map[string]Value{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid ini line: %q", line)
		}
		doc.Set(sec, strings.TrimSpace(k), stripInlineComment(strings.TrimSpace(v)), OriginFile, "")
	}
	return doc, nil
}

// --- TOML loader (BurntSushi/toml in real life; here a minimal reader) ---

type TOMLLoader struct{}

func (TOMLLoader) Name() string { return "toml" }

func (TOMLLoader) Load(data []byte) (Document, error) {
	return INILoader{}.Load(data) // same subset for the PoC: [sec] + k = v
}

// --- YAML loader (go.yaml.in/yaml in real life; minimal subset here) ---

type YAMLLoader struct{}

func (YAMLLoader) Name() string { return "yaml" }

func (YAMLLoader) Load(data []byte) (Document, error) {
	doc := Document{}
	sec := ""
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			sec = strings.TrimSuffix(trimmed, ":")
			doc[sec] = map[string]Value{}
			continue
		}
		if strings.HasPrefix(line, " ") && sec != "" {
			k, v, ok := strings.Cut(trimmed, ":")
			if !ok {
				return nil, fmt.Errorf("invalid yaml line: %q", trimmed)
			}
			doc.Set(sec, strings.TrimSpace(k), strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"`)), OriginFile, "")
		}
	}
	return doc, nil
}

// LoaderFor is the format auto-detection hook (goal 1).
func LoaderFor(ext string) (Loader, error) {
	switch strings.ToLower(ext) {
	case ".ini":
		return INILoader{}, nil
	case ".toml":
		return TOMLLoader{}, nil
	case ".yaml", ".yml":
		return YAMLLoader{}, nil
	}
	return nil, fmt.Errorf("unsupported config format: %q", ext)
}

// --- merge: later files win, sections merge key-wise (goal 3) ---
// The winning value keeps its own provenance; the loser's is discarded, so
// "which file set this" stays answerable after any number of merges.

func (d Document) MergeFrom(other Document) {
	for sec, kvs := range other {
		if d[sec] == nil {
			d[sec] = map[string]Value{}
		}
		for k, v := range kvs {
			d[sec][k] = v
		}
	}
}

// --- env overlay (today's EnvironmentToConfig, formalized as a Source; goal 5) ---

// Source is the abstraction for goal 5: file, env, or a future vault are all
// just Sources applied in a defined precedence order onto the Document.
type Source interface {
	Name() string
	Apply(doc Document) error
}

type EnvSource struct {
	Environ []string // os.Environ() in real life
}

func (e EnvSource) Name() string { return "env" }

// GITEA__SECTION__KEY=..., with _0X2E_ escaping for dots, __FILE suffix for
// secret files — exactly the existing contract from config_env.go.
func (e EnvSource) Apply(doc Document) error {
	for _, kv := range e.Environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(k, "GITEA__") {
			continue
		}
		body := strings.TrimPrefix(k, "GITEA__")
		body = strings.ReplaceAll(body, "_0X2E_", ".") // simplified escaping
		sec, key, ok := strings.Cut(body, "__")
		if !ok {
			continue
		}
		doc.Set(strings.ToLower(sec), key, v, OriginEnv, k)
	}
	return nil
}

// --- validation against the registry (goal 2, feeds `gitea config check`) ---
// Only user-set values are validated: defaults come from the registry and are
// trusted, a user typo is not.

type Problem struct {
	Section, Key, Message string
}

func Validate(doc Document, reg []Opt) (problems []Problem) {
	regKeys := map[string]map[string]bool{}
	for _, o := range reg {
		if regKeys[o.Section] == nil {
			regKeys[o.Section] = map[string]bool{}
		}
		regKeys[o.Section][o.Key] = true
	}
	// unknown keys — the typo killer ("SERVE_RVV_PATH" style mistakes fail silently today)
	for sec, kvs := range doc {
		for k := range kvs {
			if !regKeys[sec][k] {
				problems = append(problems, Problem{sec, k, "unknown config option"})
			}
		}
	}
	byKey := map[string]Opt{}
	for _, o := range reg {
		byKey[o.Section+"."+o.Key] = o
	}
	for sec, kvs := range doc {
		for k, v := range kvs {
			o, ok := byKey[sec+"."+k]
			if !ok {
				continue
			}
			if o.Deprecated != "" {
				problems = append(problems, Problem{sec, k, "deprecated, use " + o.Deprecated + " instead (removal: " + o.RemovalIn + ")"})
			}
			if !v.SetByUser() {
				continue
			}
			if len(o.Enum) > 0 {
				found := false
				for _, e := range o.Enum {
					if fmt.Sprint(e) == v.Raw {
						found = true
					}
				}
				if !found {
					problems = append(problems, Problem{sec, k, fmt.Sprintf("invalid value %q, must be one of %v", v.Raw, o.Enum)})
				}
			}
			switch o.Type {
			case TInt:
				if _, err := strconv.Atoi(strings.TrimSpace(v.Raw)); err != nil {
					problems = append(problems, Problem{sec, k, fmt.Sprintf("invalid integer %q", v.Raw)})
				}
			case TBool:
				if _, err := strconv.ParseBool(strings.TrimSpace(v.Raw)); err != nil {
					problems = append(problems, Problem{sec, k, fmt.Sprintf("invalid boolean %q", v.Raw)})
				}
			}
		}
	}
	sort.Slice(problems, func(i, j int) bool {
		return problems[i].Section+problems[i].Key < problems[j].Section+problems[j].Key
	})
	return problems
}

// --- struct hydration: replaces loadDBSetting's MustXxx chain (side-goal prep) ---

type Database struct {
	Type              string
	Host              string
	Name              string
	User              string
	Passwd            string
	SSLMode           string
	Path              string
	SQLiteBusyTimeout int
	IterateBufferSize int
	LogSQL            bool

	// provenance per hydrated field, key = option name (e.g. "HOST")
	// SetByUser("HOST") answers "did the user set this or is it a default?"
	// — the query the real system must be able to answer everywhere.
	origin map[string]Value
}

func (d *Database) Hydrate(doc Document, reg []Opt) error {
	d.origin = map[string]Value{}
	sec := doc["database"]
	for _, o := range reg {
		v, has := sec[o.Key]
		if !has {
			if o.DefFn != nil {
				v = Value{Raw: fmt.Sprint(o.DefFn()), Origin: OriginDefault}
			} else if o.Def != nil {
				v = Value{Raw: fmt.Sprint(o.Def), Origin: OriginDefault}
			} else {
				continue
			}
		}
		d.origin[o.Key] = v
		switch o.Key {
		case "DB_TYPE":
			d.Type = v.Raw
		case "HOST":
			d.Host = v.Raw
		case "NAME":
			d.Name = v.Raw
		case "USER":
			d.User = v.Raw
		case "PASSWD":
			d.Passwd = v.Raw
		case "SSL_MODE":
			d.SSLMode = v.Raw
		case "PATH":
			d.Path = v.Raw
		case "SQLITE_TIMEOUT":
			n, _ := strconv.Atoi(strings.TrimSpace(v.Raw))
			if n < 5000 { // preserves today's clamp behavior from loadDBSetting
				n = 20000
			}
			d.SQLiteBusyTimeout = n
		case "ITERATE_BUFFER_SIZE":
			d.IterateBufferSize, _ = strconv.Atoi(strings.TrimSpace(v.Raw))
		case "LOG_SQL":
			d.LogSQL, _ = strconv.ParseBool(strings.TrimSpace(v.Raw))
		}
	}
	return nil
}

// Provenance reports the full origin of one hydrated option.
func (d *Database) Provenance(key string) Value { return d.origin[key] }

// SetByUser reports whether the user explicitly set this option.
func (d *Database) SetByUser(key string) bool {
	v, ok := d.origin[key]
	return ok && v.SetByUser()
}

// UserSetKeys lists all options the user explicitly set.
func (d *Database) UserSetKeys() []string {
	var keys []string
	for k, v := range d.origin {
		if v.SetByUser() {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// --- helpers ---

func stripInlineComment(v string) string {
	if i := strings.Index(v, " ;"); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}
