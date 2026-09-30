// Stage 2: Loaders produce a canonical Document (nested map) from any format.
// Stage 3: merge + env overlay + validation against the registry.
package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Document is the format-neutral config tree: section -> key -> value.
type Document map[string]map[string]string

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
	for line := range splitLines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec = strings.Trim(line, "[]")
			if doc[sec] == nil {
				doc[sec] = map[string]string{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid ini line: %q", line)
		}
		if doc[sec] == nil {
			doc[sec] = map[string]string{}
		}
		doc[sec][strings.TrimSpace(k)] = stripInlineComment(strings.TrimSpace(v))
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
	for line := range splitLines(string(data)) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			sec = strings.TrimSuffix(trimmed, ":")
			doc[sec] = map[string]string{}
			continue
		}
		if strings.HasPrefix(line, "  ") && sec != "" {
			k, v, ok := strings.Cut(trimmed, ":")
			if !ok {
				return nil, fmt.Errorf("invalid yaml line: %q", trimmed)
			}
			doc[sec][strings.TrimSpace(k)] = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"`))
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

func (d Document) MergeFrom(other Document) {
	for sec, kvs := range other {
		if d[sec] == nil {
			d[sec] = map[string]string{}
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
		sec = strings.ToLower(sec)
		if doc[sec] == nil {
			doc[sec] = map[string]string{}
		}
		doc[sec][key] = v
	}
	return nil
}

// --- validation against the registry (goal 2, feeds `gitea config check`) ---

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
			if len(o.Enum) > 0 {
				found := false
				for _, e := range o.Enum {
					if fmt.Sprint(e) == v {
						found = true
					}
				}
				if !found {
					problems = append(problems, Problem{sec, k, fmt.Sprintf("invalid value %q, must be one of %v", v, o.Enum)})
				}
			}
			switch o.Type {
			case TInt:
				if _, err := strconv.Atoi(strings.TrimSpace(v)); err != nil {
					problems = append(problems, Problem{sec, k, fmt.Sprintf("invalid integer %q", v)})
				}
			case TBool:
				if _, err := strconv.ParseBool(strings.TrimSpace(v)); err != nil {
					problems = append(problems, Problem{sec, k, fmt.Sprintf("invalid boolean %q", v)})
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
}

func (d *Database) Hydrate(doc Document, reg []Opt) error {
	sec := doc["database"]
	if sec == nil {
		sec = map[string]string{}
	}
	for _, o := range reg {
		raw, has := sec[o.Key]
		if !has {
			if o.DefFn != nil {
				raw, has = fmt.Sprint(o.DefFn()), true
			} else if o.Def != nil {
				raw, has = fmt.Sprint(o.Def), true
			}
		}
		if !has {
			continue
		}
		switch {
		case o.Key == "DB_TYPE":
			d.Type = raw
		case o.Key == "HOST":
			d.Host = raw
		case o.Key == "NAME":
			d.Name = raw
		case o.Key == "USER":
			d.User = raw
		case o.Key == "PASSWD":
			d.Passwd = raw
		case o.Key == "SSL_MODE":
			d.SSLMode = raw
		case o.Key == "PATH":
			d.Path = raw
		case o.Key == "SQLITE_TIMEOUT":
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			if n < 5000 { // preserves today's clamp behavior from loadDBSetting
				n = 20000
			}
			d.SQLiteBusyTimeout = n
		case o.Key == "ITERATE_BUFFER_SIZE":
			d.IterateBufferSize, _ = strconv.Atoi(strings.TrimSpace(raw))
		case o.Key == "LOG_SQL":
			d.LogSQL, _ = strconv.ParseBool(strings.TrimSpace(raw))
		}
	}
	return nil
}

// --- helpers ---

func splitLines(s string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for _, l := range strings.Split(s, "\n") {
			if !yield(l) {
				return
			}
		}
	}
}

func stripInlineComment(v string) string {
	if i := strings.Index(v, " ;"); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}
