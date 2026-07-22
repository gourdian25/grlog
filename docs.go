// File: docs.go

// Package grlog is a zero-dependency, structured logging library for Go.
//
// It gives applications leveled, typed-field logging with pluggable output
// destinations and formats, an asynchronous mode for latency-sensitive call
// sites, and a [log/slog] adapter so slog-based code (or dependencies) can
// route through the same sinks. The entire implementation uses only the
// standard library.
//
// # Design goals
//
// grlog optimizes for two things that are often in tension: a small,
// discoverable API, and a genuinely zero-allocation hot path. Structured
// data is passed as typed [Field] values instead of map[string]interface{},
// so field construction and formatting avoid reflection entirely for the
// common types (string, int, bool, duration, ...). Output destinations
// ([LogSink]) and output formats ([Formatter]) are separate, composable
// interfaces, so adding a destination (a database, a message queue, a test
// buffer) never requires reimplementing text or JSON formatting.
//
// grlog does not attempt to be a full observability platform: it has no
// built-in metrics, tracing, or log shipping. It focuses on producing
// correctly-formatted, structured log entries and getting them to one or
// more destinations without losing them or blocking the caller more than
// configured to.
//
// # Getting started
//
// The zero-config path uses stdout and plain-text formatting:
//
//	logger := grlog.NewDefaultLogger()
//	defer logger.Close() // drains async buffers; safe even when async is off
//
//	logger.Info("server started", grlog.Int("port", 8080))
//	logger.Error("request failed", grlog.Err(err), grlog.String("path", "/users"))
//
// [NewLogger] gives full control via functional options ([WithLevel],
// [WithSink], [WithAsync], [WithCaller], and others documented on each
// option).
//
// # Levels
//
// [LogLevel] values, in increasing severity, are [DEBUG], [INFO], [WARN],
// [ERROR], and [FATAL], plus [OFF] to disable output entirely. A Logger
// drops any entry below its configured level before doing any other work
// ([Logger.SetLevel] uses an atomic store, so changing the level at
// runtime — from a signal handler or an admin endpoint — is lock-free and
// safe from any goroutine). [Logger.Fatal] logs, closes the logger to
// flush buffered output, and then calls os.Exit(1); it is the only level
// that terminates the process.
//
// # Fields
//
// Typed constructors — [String], [Int], [Int64], [Uint64], [Float64],
// [Bool], [Duration], [Time], [Err], and the reflection-based fallback
// [Any] — build a [Field] without boxing into map[string]interface{}.
// [Err] is nil-safe: a nil error produces a [Field] that every built-in
// [Formatter] skips entirely, so `grlog.Err(err)` can be passed
// unconditionally at every call site.
//
// # Sinks and formatters
//
// A [LogSink] is a write destination; a [Formatter] turns a [LogEntry] into
// bytes. The built-in sinks are [WriterSink] (any io.Writer; [StdoutSink]
// is an alias), [FileSink] (size-based rotation, optional gzip compression
// and age-based cleanup of backups), [MultiSink] (fan-out to several
// sinks, errors aggregated with errors.Join), [LeveledSink] (wraps a sink
// to drop entries below a per-destination minimum level — e.g. send
// everything to stdout but only ERROR-and-above to a paging system), and
// [CustomSink] (arbitrary write/close functions, for destinations grlog
// doesn't ship — a database, a message queue, a metrics counter). The
// built-in formatters are [PlainFormatter] (human-readable text) and
// [JSONFormatter] (deterministic key order: reserved keys, then sorted
// CustomFields, then entry fields in call order; a field whose key
// collides with a reserved key is emitted under "fields.<key>" rather than
// overwriting entry metadata). Sinks and formatters compose freely — any
// sink can use any formatter.
//
// # Asynchronous logging
//
// [WithAsync] starts a single background worker consuming a buffered
// channel, so [Logger.Info] and friends return without waiting for a sink
// write. [WithOverflowPolicy] governs what happens when the buffer is
// full: [OverflowSyncFallback] (the default) writes the entry synchronously
// so nothing is ever lost; [OverflowBlock] blocks the caller until space
// frees, preserving strict ordering; [OverflowDrop] discards the entry
// without blocking, counted in [Logger.Stats]'s DroppedEntries. [WithSampler]
// separately reduces volume for noisy low-severity levels, independent of
// async — skipped entries are counted in SampledEntries. [Logger.Close]
// always drains any buffered entries before closing sinks, whether or not
// async is enabled.
//
// # Context and derived loggers
//
// A [Logger] returned by [NewLogger] is a lightweight view over shared
// state (sinks, the async queue, the worker goroutine). [Logger.With]
// derives a view that adds permanent fields to every entry it logs;
// [Logger.WithContext] derives a view from a context.Context, surfacing
// values stored via [ContextWithRequestID], [ContextWithTraceID], and
// [ContextWithUserID] (collision-proof typed keys, not raw strings), plus
// anything returned by extractors registered with [WithContextExtractor].
// Both return new views without touching the shared state, so a base
// logger and any number of derived views can be used concurrently. Calling
// [Logger.Close] on any view closes the shared logger exactly once;
// further calls on any view are no-ops.
//
// # log/slog interoperability
//
// [NewSlogHandler] adapts a Logger to slog.Handler, so slog-based code
// (including third-party dependencies that log via slog) can be routed
// through grlog's sinks and formatters:
//
//	slog.SetDefault(slog.New(grlog.NewSlogHandler(logger)))
//
// Levels map Debug→DEBUG, Info→INFO, Warn→WARN, Error→ERROR. slog groups
// become dotted key prefixes ("req.status"); attributes added via
// slog.Logger.With are preserved across the handler's WithAttrs/WithGroup.
//
// # Concurrency and thread safety
//
// A Logger and every view derived from it are safe for concurrent use.
// The level is an atomic int32; sinks are protected by a RWMutex, and
// [Logger.AddSink]/[Logger.RemoveSink] copy-on-write the sink slice so
// concurrent readers never observe a partially mutated list; [Logger.Close]
// uses a compare-and-swap so it runs its shutdown sequence exactly once no
// matter how many views or goroutines call it. The one guarantee grlog
// does not provide automatically: a handler passed to [WithErrorHandler]
// must not log through the same logger it was registered on, since a
// persistently failing sink would otherwise recurse.
//
// # Performance
//
// Typed field construction and both built-in formatters avoid allocation
// on their common paths (formatters reuse pooled buffers internally); a
// filtered-out log call — one below the configured level — costs a single
// atomic load and returns. These properties are asserted, not just
// claimed: see the zero-allocs-per-op assertions in the module's benchmark
// suite. Run `go test -bench=. -benchmem -run=^$` for current numbers on
// your hardware; enabling [WithCaller] adds a runtime.Caller lookup
// (worth avoiding on the hottest paths).
//
// # Limitations
//
// grlog does not redact or scope sensitive field values — callers are
// responsible for not logging secrets, tokens, or raw PII. The plain-text
// formatter does not escape newlines in messages or field values (prefer
// [JSONFormatter] when output is consumed by another system, since its
// escaping is complete). There are no built-in network, database, or
// message-queue sinks; reach those through [CustomSink] or by implementing
// [LogSink] directly. There is no built-in facility for redacting or
// masking specific fields.
package grlog
