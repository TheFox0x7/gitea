# Parity and performance review: `modules/slog` vs `modules/log`

- Date: 2026-10-10
- Scope: `modules/slog` (branch `slog`, commit `efa20e3d4d`), audited against `modules/log` consumers and `slog_design.md`
- Companion: `slog_new_design.md` (implemented architecture, event flows, config-migration analysis)
- The per-writer `Expression` regex filter is intentionally excluded (dropped per design §1 non-goals)

## 1. Parity matrix

| Feature | Status | Notes |
|---|---|---|
| Legacy text output byte-compat | OK | Ported + golden tests incl. colors/gopid/stacktrace/prefix/flags. Fixes two old bugs (stacktrace level aggregation, double-`ResumeAll` panic) — intentional non-bug-compat |
| Arbitrary caller skip (XORM skip=8) | OK | `Log(skip,...)` → `runtime.Callers(skip+2)`; skip semantics identical to `LoggerImpl.Log` (`runtime.Caller(skip+1)`) |
| Per-sink levels, fan-out, shared sinks | OK | First-wins dedup matches old `GetSharedWriter`/`NewSharedWriter` registry incl. the `.access` writer-name suffix special case |
| PauseAll/ResumeAll, flush-on-close | OK | Drain is bounded (5s deadline) vs old unbounded queue drain — strictly better; drain bypasses pause (old drained via queue close too) |
| File-open-failure → stderr fallback | OK | Same trade-off and behavior as `NewEventWriterFile` |
| Conn writer (reconnect/reconnect-on-msg) | OK | Plus dial/write deadlines and keepalive — the improvement design §4.1 requested |
| printf API surface (587 importers) | OK | Same skip conventions (`global.Log` → `Default().Log(skip+1)`), matching `logger_global.go` |
| `LogStringer` / `ColoredValue` / sensitive-URL masking | OK printf / GAP structured | `protectSensitiveInfo` runs only in `logPrintf`; `Slog().Info("u", url)` leaks credentials into json/logfmt |
| Router prefilled caller (`HTTPRequest`) | GAP | Mechanism exists (`hiddenInfo.caller` → `sourceParts`) but no public API to set it; `modules/web/routing/logger.go:26` is the sole consumer |
| Access log (template → buffer → log) | OK | Pre-rendered buffer via `Log(1, level, "%s", buf)` works unchanged |
| Reconfigure semantics (`RestartLogsWithPIDSuffix`, re-init) | OK | Old `initLoggerByName` also reuses existing shared writers by name; new `Apply` matches |
| Stacktrace below `StacktraceLevel` | OK | Legacy-format only in new; old had no other format, so no regression (documented in `SinkConfig`) |
| No-writer loggers | OK (silent) | Old `[no logger writer]` fallback was unreachable (level gate = NONE); design §7.5 decision should still be documented |
| `Event.Time` prefill | Dropped | Zero non-test consumers; acceptable |
| Slog()/structured logging, json/logfmt formats | New | Per design; FATAL renamed via `ReplaceAttr`; TRACE renders `DEBUG-4` (naming wart) |

## 2. Gaps, prioritized

### P0 — block the phase-1 swap ("no importer changes")

1. **No external sink extension point.** `newSinkWriter` hard-codes console/file/conn.
   `modules/testlogger` registers a `"test"` writer type used by every Go test in the repo;
   `RegisterEventWriter` is its only hook. Need a public escape hatch (e.g.
   `SinkConfig` with an `io.WriteCloser`, or public `NewSinkWithWriter`). Design §7.6 planned this port.
2. **Adapters not ported** (~30 LOC total, all with live callers). Port verdicts in `slog_new_design.md` §4:
   - `LoggerToWriter` — golog bridge: `modules/setting/log.go:200`, `cmd/doctor.go:103,175` → replace with
     `slog.NewLogLogger(Slog().Handler(), LevelInfo)`. Strictly better: PC resolves to the real golog caller
     (old resolved inside `loggerToWriter.Write`). Caveats: golog's trailing `\n` survives into json/logfmt
     `msg`; no URL masking on this path (negligible — only fires when the whole line parses as an http URL)
   - `BaseLoggerToGeneralLogger` — `services/doctor/doctor.go:86-87`, `modules/process` → mostly delete:
     the new `*Logger` already has the general printf surface; keep a ~10-line shim for `Base`-implementors
     (`doctorCheckLogger`); the `*Event` parameter disappears from its signature
   - `PrintfLogger` → drop, no non-test callers; `ErrorWithSkip` (`services/context/api.go:132,158`,
     `services/context/private.go:60`, `modules/setting/setting.go:262`) → 2-line fn over `Log`
   - `IsLoggerEnabled` (`modules/setting/log.go:265,269`) → `GetLevel() != LevelNone`
     (old = `level < FATAL && writers > 0`, `LoggerImpl.IsEnabled`; equivalent for realistic configs)
   - `Level.ColorAttributes()` (`modules/web/routing/logger.go:15-20`) → export `levelColorAttrs`
   - `rotatingfilewriter.ErrorPrintf` / `process.TraceCallback` wiring (`modules/log/init.go:25,27`) →
     re-point at `modules/slog` equivalents, no adapter type needed
3. **Prefilled-caller public API missing** (router logger). The internal mechanism
   now exists (`hiddenInfo.prefillSource` in `logPrintf`, shared by all legacy sinks);
   only the public setter is missing.

### P1 — behavioral parity traps

4. **`StderrConfig` sink-name collision.** It hard-codes sink name `"console-stderr"` for any
   logger. Gitea calls `SetupStderrLogger(DEFAULT, "console-stderr", level)` and then
   `InitSQLLoggersForCli` → `SetupStderrLogger("xorm", "console-stderr", level)` — old creates
   two independent non-shared writers; new first-wins reuse gives the xorm logger the default
   logger's sink level. Config-driven sinks were name-deduped in old code; `SetupStderrLogger`
   writers were not. Port needs per-logger sink names.
5. **Dynamic levels unreachable.** Per-sink `slog.LevelVar` exists but has no setter API —
   the design's "dynamic atomic levels" gain (§1, §4.4) is dead code as-is.
6. **`Dump()` shape changed** vs `DumpLoggers()`/`DumpWriters()` consumed by
   `routers/web/admin/config.go:101` and `modules/setting/log_test.go` golden dumps.
7. **Config defaults drift.** Old global `BUFFER_LEN` default is 10000 (`loadLogGlobalFrom`),
   new per-sink default is 1000. `config.go`'s `FileSinkOptions` comment claims zero values fall
   back to rotatingfilewriter defaults ("size 1<<28, daily, 7 days, compress") — false:
   `rotatingfilewriter.Open` normalizes nothing, zero `Rotate` disables rotation entirely. The
   setting port must set BufferLen and every file option (`LOG_ROTATE`, `MAX_SIZE_SHIFT`,
   `DAILY_ROTATE`, `MAX_DAYS`, `COMPRESS`, `COMPRESSION_LEVEL`) explicitly.
8. **Access-log special case has no new home.** Old: writer name gets `".access"`, flags default
   `none`, file default `access.log` (`modules/setting/log.go:122-131`). New `SinkConfig.Name` has
   no such rule — a naive mode-name-as-sink-name port makes access share the default logger's
   sinks and inherit their flags/level. Port must namespace names and set flags/filename explicitly.

### P2 — polish

- Masking on the `Slog()` path (or explicit docs).
- TRACE naming in json/logfmt (`DEBUG-4`).
- `slog.SetDefault` reverse bridge (design §3 last row) not wired.
- Document the silent no-writer decision (design §7.5).
- ~~gofmt: `modules/slog/logger_test.go` (trailing whitespace)~~ — fixed 2026-10-10.
- `doc.go` references `slog_design.md` — now present at repo root.
- Fix `config.go`'s `FileSinkOptions` doc comment: zero values are *not* rotatingfilewriter defaults (see P1 #7).

## 3. Internal correctness observations

- **FIXED 2026-10-10**: `Slog()` cache race and stale-after-reconfigure — `swapHandler`
  now stores a rebuilt `*slog.Logger` as a pair with the handler (never nils the
  cache), so `CAS(nil, …)` can only win before any swap. Held `*slog.Logger` values
  still stay bound to their tree (documented on `Slog()`); re-fetch after reconfigure.
- **FIXED 2026-10-10**: printf metadata (format/args/stacktrace/prefilled source)
  travels beside the record as `*hiddenInfo` via internal `multiHandler.handle`;
  hidden record attrs, the `\x00` key and the stdlib-sink strip pass are gone. This
  also removes the dead child-error branch (`sinkFrontend.Handle` never errors).
- **Effectively dead error branch**: `multiHandler.Handle` logs child handler errors, but
  `sinkFrontend.Handle` never returns one (enqueue is non-blocking). Harmless. *(removed
  by the fix above)*

## 4. Performance

### 4.1 Real issue: manager mutex on every package-level log call

**FIXED 2026-10-10**: `GetLogger(DefaultName)` now checks an
`atomic.Pointer[Logger]` cache before locking (cleared on `Close`), matching the
old `modules/log` fast path.

`Info/Warn/... → Log → Default() → defaultManager.GetLogger("default")` takes
`m.mu.Lock()` on **every** call (`modules/slog/manager.go:32-36`, `modules/slog/global.go`).
Old code has an `atomic.Pointer` fast path for exactly this
(`modules/log/manager.go:31-35`). This serializes all logging goroutines through one mutex —
a regression on the hottest path in the codebase (589 importing files). Fix: cache the
default logger in an `atomic.Pointer[Logger]` and check it before locking, as old code does.

### 4.2 Small regressions (micro)

- **FIXED 2026-10-10**: per-legacy-sink source resolution — `logPrintf` now resolves
  once via `hiddenInfo.prefillSource` when `needsSource`; `sourceParts` consumes the
  prefill. (Also kept: lazy PC capture only when a sink needs source.)
- **FIXED 2026-10-10**: mixed colorized-legacy + stdlib trees — the strip pass is gone
  entirely (metadata travels beside the record, never in it); only the deferred-format
  `Clone()` remains.
- **FIXED 2026-10-10**: colorized-only trees (the default config) skip the discarded
  uncolored pre-render via the `needsMessage` tree hint; `formatLegacy` renders from
  the raw args.
- **FIXED 2026-10-10**: `gofmt` trailing whitespace in `logger_test.go`; `make lint-go`
  is clean for the package.
- **Level gate.** Old: one `atomic.Int32` compare. New: atomic handler load + per-child
  interface dispatch reading `LevelVar` atomics. A few ns; not actionable.

### 4.3 Wins vs old

- **Lazy PC capture.** Old ran `runtime.Caller` + `FuncForPC` on *every* event regardless of
  flags; new captures PC only when a sink needs source. Access logger (flags `none`) skips it
  on every request; `flags=none` sinks skip it entirely.
- **Zero-arg fast path.** New skips the args clone/mask loop for `len(args)==0`; old always
  copied.
- **Deferred json/logfmt formatting** moves reflection+encoding off the caller goroutine.
- **Cheaper overload drops.** Old JSON-marshaled the entire event before logging the drop;
  new reports sink name only.
- **Reconfigure = atomic pointer swap** vs old stop/start writer goroutines under mutex.

### 4.4 Parity areas (no change)

Queue/goroutine model (bounded chan, default 1000, drop-on-overflow + stderr), one goroutine
per sink, eager legacy formatting on the caller goroutine (required for `ColoredValue`),
pause via RLock'd chan read in the sink goroutine, per-event `time.Now()`, 1024-byte format
buffer, close-timeout shape (2s stop / sequential over sinks).

### 4.5 Missing: benchmarks

Design §9 phase 1 requires benchmarks (disabled fast path, single legacy console, json eager
vs deferred, 3-sink fan-out). None exist. Add them next to `modules/log`'s if any, and
re-run after fixing 4.1.
