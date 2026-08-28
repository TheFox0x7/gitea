# Config proposal: logging configuration redesign (breaking)

- Date: 2026-10-10
- Scope: the INI logging configuration (`[log]` section family, `modules/setting/log.go`), targeting the `modules/slog` `Config`/`Apply` model
- Companions: `slog_design.md` (plan), `slog_parity_notes.md` (audit), `slog_new_design.md` (implemented architecture, §3 covers the non-breaking analysis)
- Premise: breaking the configuration is allowed — one migration release, hard errors on old keys

## 1. Problems with the current shape

1. **`MODE` is overloaded.** `logger.<name>.MODE` is a list of config sections; `[log.<mode>] MODE` is a writer-type override. Same key, two meanings, two levels apart (`modules/setting/log.go:117`, `:226`).
2. **Sentinel values.** `logger.x.MODE = ","` means "use `[log] MODE`", `""` means "disabled" — established by `MustString(",")` side effects and `SetValue("")` writes inside `prepareLoggerConfig` (`log.go:55-96`). The config loader mutates the config to normalize syntax.
3. **The `.access` hack.** Sink identity is the section name, so the access logger's writer must be renamed `modeName+".access"` to avoid sharing the default logger's writer — with hidden defaults (`flags=none`, `access.log`) buried in `loadLogModeByName` (`log.go:122-131`).
4. **Sharing is invisible.** `logger.access.MODE = console` silently shares `[log.console]` with the default logger (first-wins). Users cannot see it, and per-logger level differences over a shared section are impossible.
5. **`BUFFER_LEN` is global-only.** It is never read per section; `writerMode.BufferLen` only ever receives the global value (`log.go:243-245`).
6. **Boot swallows errors.** A bad writer section logs `FallbackErrorf` and `continue`s → half-applied logging config, server starts anyway (`log.go:239-252`).
7. **Dead weight.** `EXPRESSION` (dropped with the old framework), the PID-suffix restart hack (`RestartLogsWithPIDSuffix`, its own FIXME admits it breaks rotation/log collectors), and six deprecated aliases (`ACCESS`, `ROUTER`, `XORM`, `ENABLE_ACCESS_LOG`, `DISABLE_ROUTER_LOG`, `ENABLE_XORM_LOG`) kept on life support.

## 2. Design principle

**Sink identity = config section name.** A `[log.<name>]` section defines one sink; the new manager dedupes sinks by exactly this key. Sharing a sink = referencing the same section from multiple loggers — visible in the config, not an emergent behavior. Duplicate sink names become structurally impossible (INI cannot repeat a section), which eliminates the whole class of first-wins surprises for config-driven sinks.

Loggers and sinks are decoupled: a logger section only *references* sinks; all writer detail lives in the sink section. The `.access` special case disappears because access simply references a differently-configured sink.

## 3. Target shape

```ini
[log]
LEVEL = Info            ; default for sinks without their own LEVEL
ROOT_PATH = /var/lib/gitea/log
SINKS = console         ; fallback for loggers without their own section

[log.default]
SINKS = console,main-file

[log.access]
SINKS = access-file     ; sharing = referencing the same section, visible

[log.router]
SINKS = console

[log.xorm]
SINKS = main-file

[log.console]
TYPE = console
FORMAT = legacy         ; legacy | logfmt | json
LEVEL = Info
STREAM = stdout         ; stdout | stderr (replaces STDERR bool)
FLAGS = medfile,shortfuncname,level
COLORIZE = auto         ; auto | true | false
; PREFIX, STACKTRACE_LEVEL as today

[log.access-file]
TYPE = file
FORMAT = legacy
FLAGS = none            ; access defaults become visible config, not code
FILE_NAME = access.log

[log.main-file]
TYPE = file
FORMAT = json
LEVEL = Debug
FILE_NAME = gitea.log
BUFFER_LEN = 10000      ; per sink; global BUFFER_LEN dropped
; LOG_ROTATE, MAX_SIZE_SHIFT, DAILY_ROTATE, MAX_DAYS,
; COMPRESS, COMPRESSION_LEVEL as today

[log.remote]
TYPE = conn
ADDR = logs.example.com:7020
; PROTOCOL, RECONNECT, RECONNECT_ON_MSG as today
```

## 4. Key mapping (old → new)

| Old | New | Notes |
|---|---|---|
| `logger.<name>.MODE = a,b` | `logger.<name>.SINKS = a,b` | renamed; values are sink sections |
| `logger.<name>.MODE = ","` | section absent → `[log] SINKS` | sentinel removed; `[log] MODE` → `[log] SINKS` |
| `logger.<name>.MODE = ""` | `logger.<name>.SINKS =` (empty) | "disabled" becomes explicit |
| `[log.<mode>] MODE = <type>` | `[log.<name>] TYPE = <type>` | required; no implicit type-from-section-name |
| `[log.<mode>] STDERR = true` | `STREAM = stderr` | replaces the bool; `stdout` default |
| — | `FORMAT = legacy\|logfmt\|json` | new; `legacy` default (byte-compat) |
| `[log] BUFFER_LEN` | `[log.<name>] BUFFER_LEN` | moved per sink; global key removed |
| `COLORIZE` bool (default = runtime color detection) | `COLORIZE = auto\|true\|false` | `auto` = current `CanColorStdout/Stderr` behavior, now visible |
| `ACCESS`, `ENABLE_ACCESS_LOG`, `ROUTER`, `DISABLE_ROUTER_LOG`, `XORM`, `ENABLE_XORM_LOG` | removed | hard error with rename hint instead of silent rewrite |
| `[log.<mode>] EXPRESSION` | removed | filter dropped with the old framework |

## 5. Rationale per change

1. **`SINKS` rename + explicit empty value** — removes both sentinels and ends the config-mutation normalization in `prepareLoggerConfig`.
2. **`TYPE` required per sink** — one meaning per key; also the natural place for future sink types once the P0 #1 extension point exists (a custom/test sink would be `TYPE = ...` registered by the embedding test binary).
3. **Access references its own sink section** — deletes the `.access` rename, the code-buried `flags=none`/`access.log` defaults, and structurally prevents the `console-stderr`-style collisions (P1 #4) for config-driven sinks. Defaults move into the shipped/generated default config.
4. **`FORMAT` per sink** — the capability this whole port exists for; `legacy` default preserves output byte-compatibility.
5. **`BUFFER_LEN` per sink** — matches `SinkConfig.BufferLen`; a queue depth is a property of a sink's goroutine, not of the process.
6. **`STREAM` + tri-state `COLORIZE`** — replaces the `STDERR` bool and surfaces what `CanColorStdout`/`CanColorStderr` currently decide invisibly at runtime.
7. **Hard-fail on removed keys** — the config provider can validate at load; old keys produce a startup error naming the replacement, which is strictly better than the current silent rewrite.
8. **Fail fast on invalid sink config** — `Apply` already validates everything before mutating; the port must surface that error at startup instead of `FallbackErrorf` + `continue`.
9. **Drop `RestartLogsWithPIDSuffix` / `filenameSuffix`** — its own FIXME says it breaks log rotation and collectors; with a breaking release it can simply go.
10. **`EXPRESSION` stays dead** — dropped by design. If message filtering is ever needed, a per-sink `FILTER` regex slots into this shape without further breaking changes.

## 6. Deliberately unchanged

- `LEVEL` semantics: per-sink filter, effective logger level = "any sink accepts" (OR). The old model never had a logger-level key either — it was derived as min-of-writers. A future `logger.<name>.LEVEL` gate can be added without breaking anything (it only narrows).
- `STACKTRACE_LEVEL` (legacy format only), `PREFIX`, `FLAGS`, and all file/conn option keys — same names, same meanings, moved nowhere.
- `ROOT_PATH` relative-name resolution for `FILE_NAME`.
- `ACCESS_LOG_TEMPLATE`, `REQUEST_ID_HEADERS`, `ENABLE_SSH_LOG` — not sink configuration.
- Default shipped config behavior: `console` sink, legacy format, `Info` — identical output to today.

## 7. Migration

- One breaking release together with the `modules/slog` swap; old keys hard-error at startup with a pointer to the new key (never silently rewritten).
- Document a mapping table in the release notes (§4 is the draft).
- `app.ini.example` and the config cheat sheet ship the new shape with the access/router/xorm sections explicit — the special cases become documentation, not code paths.

## 8. Prerequisites and order

1. P0 #1 (external sink extension point) — so `TYPE` stays extensible beyond console/file/conn.
2. P1 #4 fix (unique programmatic sink names, e.g. `StderrConfig`) — config-driven collisions die with this proposal, programmatic ones need their own fix.
3. `Dump()` shape fix (P1 #6) — the admin page reads logging state; land before or with the swap.
4. The `modules/setting` port translates this INI shape to `Config{Loggers: ...}` 1:1 — with sharing by section reference, the translation contains no rename hacks and no defaults that the sink layer doesn't know about.
