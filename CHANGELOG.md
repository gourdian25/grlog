# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.2] - 2026-07-23

Test coverage pass ahead of the ecosystem-wide flatten/pgx+sqlc/95%-coverage
initiative: no functional/API changes.

### Added

- `grlog_regression_test.go`: ~30 tests closing coverage gaps in the
  `log/slog` adapter (`appendSlogAttr`'s remaining `Kind` branches and group
  handling, `Handle`'s level/closed/sampling/zero-timestamp branches,
  `WithAttrs`/`WithGroup`'s no-op shortcuts, `callerFromPC`'s invalid-PC
  paths), `dispatch`'s `OverflowBlock` policy (both the normal-send and
  shutdown-direct-write branches), the default stderr error handler's
  rate-limiting/suppressed-count reporting, `FileSink.cleanupOldBackups`'s
  age-based path, `Logger.With`/`WithContext`'s no-op shortcuts,
  `Logger.Close`'s sink-close-error propagation, `NewWriterSink`'s nil
  writer/formatter defaults, `formatEntry`'s non-`appendFormatter` fallback,
  `putBuffer`'s oversized-buffer discard, and `CustomSink.Close`'s genuine
  nil-`closeFn` fallback (previously unreachable via `NewCustomSink`, which
  always installs a non-nil default). Raises measured coverage from 88.3%
  to 95%+.

### Documentation

- README: corrected the stale "Statement Coverage" figure (~88%) in the
  Testing section to the freshly-verified 95.0-95.1% (root package, via
  `make coverage-summary` and `make coverage-check`, 2026-07-22). This
  follows the Makefile's `COVERAGE_MIN` already having been raised from 80
  to 95 in a prior commit this session, matching the rest of the ecosystem.

## [0.1.1] - 2026-07-10

Ecosystem-alignment pass ahead of `grauth`: no functional/API changes.

### Changed

- `go.mod`'s `go` directive raised from `1.21` to `1.26.4`, aligning with
  the rest of the gourdian25 ecosystem (still satisfies the `log/slog`
  1.21 floor). `TestGoModVersionConsistency` updated to match.
- README: added a "part of the gourdian25 ecosystem" section (previously
  had none) and corrected the Go version badge/requirement from `1.21+`
  to `1.26.4+`.

### Added

- Makefile: `coverage-check` (80% threshold, matching sibling repos),
  `goreleaser-check`, and a `COVERAGE_MIN` variable — grlog was the only
  repo in the ecosystem missing these targets.

## [0.1.0] - 2026-07-05

Production-readiness overhaul: correctness fixes for shutdown and rotation,
zero-allocation formatting, `log/slog` interoperability, and CI. Contains
**breaking changes** (allowed pre-1.0); see the migration notes below.

### Added

- `log/slog` adapter: `NewSlogHandler(logger)` implements `slog.Handler`, so
  slog-based code routes through grlog sinks (groups become dotted keys,
  `slog.Logger.With` attributes are preserved)
- `Logger.With(fields ...Field)` — derive a logger view with permanent fields
- `FATAL` level with `Fatal`/`Fatalf` (logs, flushes, closes, then exits 1)
  and `OFF` level to disable all output
- `NewWriterSink(w io.Writer, formatter)` — log to any writer (stderr,
  buffers in tests, sockets)
- `NewLeveledSink(sink, minLevel)` — per-destination severity routing
- Async overflow policies via `WithOverflowPolicy`: `OverflowSyncFallback`
  (default), `OverflowBlock`, `OverflowDrop`; drops observable via
  `Logger.Stats().DroppedEntries`
- `WithSampler(every, maxLevel)` — keep 1-in-N entries for noisy low-severity
  levels; skips counted in `Stats().SampledEntries`
- `WithErrorHandler(func(error))` — route sink write failures to metrics or
  alerting; the default stderr reporting is now rate-limited
- `WithCallerSkip(n)` — correct caller info when wrapping the logger
- Typed context helpers `ContextWithRequestID` / `ContextWithTraceID` /
  `ContextWithUserID`, plus `WithContextExtractor` for custom extraction
  (e.g. OpenTelemetry span IDs)
- New field constructors: `Float64`, `Uint64`, `Time`
- `FileSinkConfig.Compress` (gzip rotated backups in the background, written
  atomically) and `FileSinkConfig.MaxAge` (age-based backup cleanup)
- GitHub Actions CI (lint, tests on Go 1.21 + stable, race detector,
  non-blocking benchmarks) and a `.golangci.yml` lint configuration
- Regression test suite (`grlog_regression_test.go`) covering shutdown
  semantics, rotation safety, JSON collisions, and all new features

### Fixed

- **Double-close panic**: closing both a `WithContext` view and its parent
  closed the shared channel twice. `Logger` is now a lightweight view over a
  shared core; `Close` on any view shuts down exactly once
- **`RemoveSink` corruption**: removing a sink through a derived view mutated
  the shared backing array, leaving other views with ghost/duplicated sinks;
  sink updates are now copy-on-write under one lock
- **Lost logs on shutdown**: entries enqueued in the race window between the
  closed-check and `Close` were stranded in the async queue; `Close` now
  performs a final drain before closing sinks
- **Rotation backup overwrites**: backup names had 1-second granularity, so
  rapid rotations silently overwrote earlier backups; names now use
  nanosecond timestamps plus a sequence suffix and are collision-proof
- **Bricked sink after failed rotation**: a failed rename/reopen left the
  sink with a closed file handle and every subsequent write failing; the sink
  now reopens the current file and keeps writing (the triggering entry is
  still written)
- **JSON reserved-key clobbering**: a user field named `message`, `level`,
  `timestamp`, or `caller` silently overwrote entry metadata; colliding
  fields are now emitted under `fields.<key>`
- **JSON marshal-failure data loss**: an unserializable field value used to
  discard the whole entry; values now degrade to their `%v` string form and
  the entry survives
- **Invalid JSON on exotic input**: NaN/Inf floats and invalid UTF-8 strings
  now produce valid JSON
- `context.Value` lookups used raw string keys (collision-prone, flagged by
  staticcheck SA1029); replaced with private typed keys

### Changed (breaking)

- `WithContext` only reads values stored via the typed helper functions; raw
  string keys (`context.WithValue(ctx, "request_id", ...)`) are ignored.
  Migrate to `grlog.ContextWithRequestID(ctx, id)` etc.
- `LogEntry.Context` field removed (was never read or written)
- Caller info is controlled solely by the logger's `WithCaller` option; the
  formatters' `EnableCaller` fields are deprecated and ignored
- JSON output is deterministic: reserved keys first, `CustomFields` in sorted
  order, then entry fields in call order
- `Duration` fields render as human-readable strings in JSON ("45ms") instead
  of nanosecond integers
- `Err(nil)` fields are omitted from output entirely (previously rendered as
  `error=<nil>` / `"error":null`)
- `MultiSink` and `Logger.Close` aggregate errors with `errors.Join`
- Minimum Go version is 1.21 (`log/slog`, `errors.Join`); go.mod previously
  declared an invalid version

### Performance

- Plain formatting: 344 ns / 3 allocs → **78 ns / 0 allocs** (pooled buffers,
  `strconv` fast paths)
- JSON formatting (no fields): 1412 ns / 15 allocs → **82 ns / 0 allocs**
  (direct serialization instead of `map` + `json.Marshal`)
- Sync log without fields: 447 ns / 3 allocs → **125 ns / 0 allocs**
- Filtered-out calls: **2.7 ns / 0 allocs** (guaranteed by benchmark)
- `FileSink` no longer issues a `Stat()` syscall per write (size tracked
  in memory)

## [0.0.1] - 2026-07-05

### Added

- Initial release: leveled structured logging with typed fields, stdout/file/
  multi/custom sinks, plain-text and JSON formatters, async logging, and
  size-based file rotation

[Unreleased]: https://github.com/gourdian25/grlog/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/gourdian25/grlog/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/gourdian25/grlog/compare/v0.0.1...v0.1.0
[0.0.1]: https://github.com/gourdian25/grlog/releases/tag/v0.0.1
