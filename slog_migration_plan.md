# Migration plan: `modules/log` → `modules/slog` via compatibility shim

- Date: 2026-10-11
- Scope: whole-rebase migration of the 587 importing files, `modules/log`, `modules/setting/log.go` and the special consumers
- Companions: `slog_design.md` (plan), `slog_parity_notes.md` (audit), `slog_new_design.md` (implemented architecture + config analysis), `slog_config_proposal.md` (breaking config)
- Premise: executed last — after the new system passes parity, performance and stability testing on its own

## 1. Strategy

Turn `modules/log` into a thin compatibility shim over `modules/slog`, delete the bespoke
internals, then migrate call sites incrementally:

1. Mark every public `modules/log` function as deprecated, pointing to `modules/slog`.
2. Remove the `log` module apart from those functions.
3. Add `slog` loggers where the removal demands it.

Why this shape works:

- **Step 2 *is* the full-scale parity test.** The moment the internals die, all 587
  importers and their tests run through the shim onto the new engine with zero call-site
  churn — compile-plus-test parity at the scale no grep-based audit can reach.
- **Two facades, one system.** The shim forwards to the same manager; there is no double
  emission and no state to keep in sync.
- **The deprecation marks are the worklist.** Migrating a file is mechanical
  (`log.Info` → `slog.Info`), greppable, and CI stays green throughout.
- **Performance is neutral**: one extra forwarding frame on the hot path, inlinable, and
  deleted by the final mass migration.

## 2. Prerequisites (block step 2, not step 3)

The shim forwards into a system that must already be configurable. Before step 2:

1. **INI → `slog.Config` port** (`modules/setting/log.go`): `initLoggerByName`'s writer
   pipeline translated to `Apply`, `SetupStderrLogger` → `StderrConfig`, the
   `PrepareConsoleLoggerLevel` hook → one `Apply` call, golog bridge →
   `slog.NewLogLogger` (see `slog_new_design.md` §4). Without it the server logs to
   nothing after step 2.
2. **Sink-type registry (P0 #1)**: `modules/testlogger` registers the `"test"` writer
   used by every Go test; it needs a public seam (e.g.
   `slog.RegisterSinkType(name string, open func(SinkConfig) (io.WriteCloser, error))`,
   which subsumes the internal `apply(cfg, sinkWriters)` test hook).
3. **Prefilled-caller API (P0 #3)**: `routing/logger.go` needs
   `hiddenInfo.caller`/`prefillSource` exposure before step 2 deletes `Event.Caller` —
   otherwise route patterns silently disappear from logs.
4. **Admin `Dump` adaptation (P1 #6)**: `routers/web/admin/config.go:101` consumes
   `DumpLoggers()`; the page must survive step 2.

## 3. Step 1 — the shim

`modules/log` keeps its package surface; bodies forward to `modules/slog`. Types are
**aliases** so both facades share one implementation with no boundary conversion:

```go
type ColoredValue  = slog.ColoredValue
type Flags         = slog.Flags
type LogStringer   = slog.LogStringer
type Level         = slog.Level // see 3.1
```

### 3.1 The `Level` exception

Old `Level` is iota-based (`UNDEFINED=0 … NONE=7`); slog's are `TRACE=-8 … NONE=16`.
The shim aliases `Level` and **redefines the constants to the new numeric values** —
without this, old `TRACE(1)` silently means "between Info and Warn". Sized fallout:

- `Level.String()` — 2 sites (`modules/setting/log.go:38,133`); slog renders `INFO`, not
  `info`. Migrate to `LevelName` in step 1 itself.
- `Level.ColorAttributes()` — only `modules/web/routing/logger.go:15-20`; that file
  migrates wholesale in step 3 (`slog.LevelColors`).
- `UNDEFINED` comparison — only `modules/web/routing/logger.go:83`; same file. New code
  expresses "unset" via config defaults / empty sinks, not a sentinel level.

### 3.2 Shim inventory (survives step 2)

| Surface | Shim body |
|---|---|
| `Trace/Debug/Info/Warn/Error/Critical` (pkg + `Logger` iface) | forward to `slog.Default()` / shim logger |
| `Fatal` | `slog.Fatal` (echoes stderr when unaccepted — early fatals safe, see `doc.go`) |
| `Log(skip, *Event, format, v…)` | `Log(skip+1, event.Level, …)`; `Event` becomes a stub struct `{Level}` |
| `ErrorWithSkip`, `IsLoggerEnabled`, `GetLevel`, `IsTrace/IsDebug`, `FallbackErrorf` | one-line forwards |
| `GetLogger(name) Logger` | `baseToLogger`-style shim (~40 LOC, misc.go pattern) over `slog.GetLogger` |
| `SetupStderrLogger` | `Apply(StderrConfig(...))` |
| `LoggerToWriter` | `slog.NewLogLogger` (better PC attribution) |
| `NewColoredValue`, `Stack`, `CanColorStdout/Stderr`, color consts | aliases / forwards |
| `Base`/`Logger` interfaces | shim types over `*slog.Logger` facade |

All marked `// Deprecated: use modules/slog[… instead]`.

## 4. Step 2 — delete the internals

Everything not in §3.2 goes: `logger_impl.go`, `event_writer*.go`, `event_format.go`,
`manager.go`, `init.go` (wiring moves to slog/setting), `misc.go` (superseded by
`slog_new_design.md` §4), and the bespoke `Level`/`Event` machinery. Roughly 2–3k LOC.

At this point the whole repo is live on the slog engine through the shim — the
parity/performance/stability gate from the premise runs here, before any call-site work.

## 5. Step 3 — migrate consumers

Order: special consumers first (they unblock shim deletion), then the printf mass.

| Consumer | Migration |
|---|---|
| `modules/testlogger`, `modules/test/logchecker` | sink-type registry / injected writers; first, so tests exercise the public seam |
| `modules/setting/log.go` | lands as prerequisite §2.1 |
| `modules/web/routing/logger.go` | `slog` prefilled-caller API + `LevelColors`; drops `Event`/`UNDEFINED` |
| `routers/web/admin/config.go` | new `Dump` shape (P1 #6) |
| `services/doctor` | `*slog.Logger` facade directly (general-logger shim deleted) |
| `models/db/log.go` (XORM skip=8) | `slog.GetLogger("xorm").Log(skip+1, …)` — skip arithmetic verified identical |
| `cmd/helper.go`, `cmd/doctor.go` | `slog.NewLogLogger` golog bridge |
| printf mass (587 files) | mechanical rename per file; deprecation marks guide |

When the mass is done: delete `modules/log` entirely, remove the deprecated marks, and
the deprecated printf API on `modules/slog` itself becomes the only (intentionally
retained) legacy surface.

## 6. End state

- One logging system: `modules/slog`; `modules/log` gone.
- Structured logging available everywhere via `Logger.Slog()`; printf paths reduced to
  the deliberate legacy facade.
- `modules/slog` benchmarks (design §9 phase 1) re-run post-migration to confirm the
  forwarding layer's removal.

## 7. Risks

| Risk | Mitigation |
|---|---|
| `Level` value change breaks numeric comparisons | constants redefined in step 1; the 2 `.String()` sites migrated then |
| Writer-level API users missed in step 2 | consumer list is closed (§5); `go build ./...` catches all |
| Early `log.Fatal` sites lose messages during shim period | shim `Fatal` → `slog.Fatal` echoes stderr pre-`Apply` (doc.go contract) |
| Config regressions at step 2 | INI port keeps `setting/log_test.go` golden coverage, ported to the new `Dump` shape |
| Deprecation lint noise | desired worklist; golangci config does not fail CI on SA1019 |
