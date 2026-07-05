# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

grlog is a zero-dependency (stdlib-only) structured logging library for Go, published as `github.com/gourdian25/grlog`. It is a library — there is nothing to build or run, only lint and test. The entire implementation lives in a single package at the repo root: [grlog.go](grlog.go) holds all code, [docs.go](docs.go) holds package documentation.

## Commands

```sh
make test               # go test ./...
make test-race          # race detector tests (CGO_ENABLED=1 go test -race -v ./...)
make bench              # benchmarks (go test -bench=. -benchmem -run=^$ ./...)
make lint               # golangci-lint run
make lint-fix           # golangci-lint run --fix
make coverage-summary   # per-function coverage
make ci                 # full CI pipeline: lint + test
```

Run a single test with `go test -run TestName ./...` (add `-race` for tests in grlog_race_test.go — they are meaningless without it).

Releases: `make release VERSION=vX.Y.Z` (tags, pushes, runs GoReleaser). VERSION is required.

## Architecture

Everything is in package `grlog`, structured around three abstractions:

- **`Logger`** — the entry point, configured via functional options (`WithLevel`, `WithSink`, `WithAsync`, `WithCaller`, `WithContextFields`). The log level is an `atomic.Int32` so level filtering is lock-free and can be changed at runtime with `SetLevel`. Sinks and context fields are guarded by their own `RWMutex`es since `AddSink`/`RemoveSink`/`WithContext` can race with logging.
- **`LogSink` interface** (`Write(LogEntry) error`, `Close() error`) — output destinations: `StdoutSink`, `FileSink` (size-based rotation with backup cleanup), `MultiSink` (fan-out), `CustomSink` (user callbacks).
- **`Formatter` interface** (`Format(LogEntry) []byte`) — `PlainFormatter` and `JSONFormatter`; sinks own their formatter.

**Fields, not maps**: structured data uses the typed `Field` struct (`String()`, `Int()`, `Bool()`, `Duration()`, `Err()`, `Any()`, …) with a `FieldType` tag so formatting avoids reflection and field construction is zero-allocation. Preserving the zero-allocation property is a core design goal — check `make bench` results when touching the hot path (`log`, `Field` constructors, formatters).

**Async mode**: `WithAsync(bufferSize)` starts a single worker goroutine consuming a buffered channel. If the buffer is full, `log` falls back to writing synchronously (never drops or blocks). `Close()` signals the worker, which drains remaining entries before exiting — tests and examples must `defer logger.Close()` or async entries may be lost.

## Testing conventions

Three test files split by concern: [grlog_test.go](grlog_test.go) (unit/behavior), [grlog_race_test.go](grlog_race_test.go) (concurrency — always run with `-race`), [grlog_bench_test.go](grlog_bench_test.go) (benchmarks, many asserting 0 allocs/op). Concurrency-sensitive changes need coverage in the race file.

## Repo conventions

- Every source file starts with a `// File: <path>` header, maintained by the `bark` tool ([.bark.toml](.bark.toml), `bark.txt`, `.barks/` backups). Keep the header when editing; add one to new files.
- Exported symbols carry exhaustive doc comments (Arguments/Returns/Use case sections) — match that style for new exported API.
