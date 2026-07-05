// File: docs.go

// Package grlog provides a high-performance, production-ready structured logging library for Go.
//
// Overview:
// grlog is a comprehensive logging solution designed for modern cloud-native applications.
// It features a modular architecture with pluggable sinks and formatters, zero-allocation
// field types and formatting, and thread-safe operations suitable for high-concurrency
// environments.
//
// Key Features:
//   - Log levels DEBUG, INFO, WARN, ERROR, FATAL, plus OFF to disable output
//   - Zero-allocation typed field constructors and pooled-buffer formatting
//   - Pluggable architecture with sinks and formatters
//   - Built-in sinks: writer/stdout, file (with rotation, compression, age-based cleanup),
//     multi-sink, leveled sink, custom sinks
//   - Built-in formatters: plain text and deterministic JSON
//   - Asynchronous logging with configurable buffer size and overflow policies
//   - log/slog interoperability via NewSlogHandler
//   - Caller information (file:line:function) with configurable frame skipping
//   - Context-aware logging with collision-proof typed context keys and custom extractors
//   - Sampling for high-volume low-severity logs
//   - Thread-safe operations; views derived via With/WithContext share one core and
//     close exactly once
//
// Getting Started:
//
// Basic example with default configuration:
//
//	package main
//
//	import (
//	    "github.com/gourdian25/grlog"
//	)
//
//	func main() {
//	    // Create logger with default configuration (stdout, plain text, INFO level)
//	    logger := grlog.NewDefaultLogger()
//	    defer logger.Close() // Important: drains async buffers; no entries are lost
//
//	    logger.Info("Application starting")
//	    logger.Debug("Debug information")
//	    logger.Warn("Warning condition detected")
//	    logger.Error("Error occurred", grlog.Err(someError))
//	}
//
// Log Levels:
//
// Log levels control message verbosity. Messages below the configured level are ignored.
// Level order: DEBUG < INFO < WARN < ERROR < FATAL < OFF
//
//	logger.SetLevel(grlog.WARN)      // Only WARN and above will be logged
//	logger.SetLevel(grlog.OFF)       // Disable all logging
//	logger.Fatal("cannot continue")  // Logs, flushes, closes, then os.Exit(1)
//
// Levels can be changed at runtime; reads are atomic and lock-free.
//
// Structured Logging with Typed Fields:
//
//	logger.Info("User authenticated",
//	    grlog.String("user_id", "12345"),
//	    grlog.Int("attempts", 3),
//	    grlog.Bool("success", true),
//	    grlog.Duration("processing_time", 45*time.Millisecond),
//	)
//
// Available field constructors:
//   - String(key, value): string fields
//   - Int(key, value), Int64(key, value), Uint64(key, value): integer fields
//   - Float64(key, value): float fields
//   - Bool(key, value): bool fields
//   - Duration(key, value): time.Duration fields (rendered like "45ms")
//   - Time(key, value): time.Time fields (rendered as RFC3339Nano)
//   - Err(error): error field with key "error"; Err(nil) is skipped entirely
//   - Any(key, value): fallback for any type (uses reflection)
//
// Formatted logging methods (Debugf, Infof, Warnf, Errorf, Fatalf) are available
// for simple printf-style messages.
//
// Architecture:
//
//	Logger → LogEntry → Formatter → LogSink
//
// A Logger is a lightweight view onto shared state. Views derived with With or
// WithContext add permanent fields while sharing sinks, the async queue, and the
// worker goroutine. Calling Close on any view shuts the shared logger down exactly
// once; further Close calls are no-ops.
//
// Sinks (Output Destinations):
//
// 1. WriterSink / StdoutSink: writes to any io.Writer (stdout by default)
//
//	stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())
//	stderrSink := grlog.NewWriterSink(os.Stderr, grlog.JSONFormat())
//
// 2. FileSink: writes to files with automatic size-based rotation
//
//	fileSink, err := grlog.NewFileSink(grlog.FileSinkConfig{
//	    Filename:    "myapp",
//	    Dir:         "logs",
//	    MaxBytes:    10 * 1024 * 1024,   // rotate at 10MB
//	    BackupCount: 5,                  // keep 5 backups
//	    MaxAge:      30 * 24 * time.Hour, // optional: drop backups older than 30 days
//	    Compress:    true,               // optional: gzip rotated backups
//	    Formatter:   grlog.JSONFormat(),
//	})
//
// Rotation is safe: backup names are collision-proof (nanosecond timestamps plus a
// sequence suffix), and if rotation fails the sink reopens the current file and keeps
// writing, so entries are never lost to a rotation error.
//
// 3. MultiSink: fan-out to several sinks; errors are aggregated with errors.Join
//
//	multiSink := grlog.NewMultiSink(stdoutSink, fileSink)
//
// 4. LeveledSink: per-destination severity filtering
//
//	// everything to stdout, only ERROR and above to the file
//	grlog.NewMultiSink(stdoutSink, grlog.NewLeveledSink(fileSink, grlog.ERROR))
//
// 5. CustomSink: implement custom logic (database, message queue, metrics, ...)
//
//	customSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
//	    // custom write logic
//	    return nil
//	})
//
// Formatters (Output Formats):
//
// 1. PlainFormat: human-readable text
//
//	// "2025-12-17 06:45:58.801 [INFO] main.go:42:main Message {key=value}"
//
// 2. JSONFormat: machine-readable JSON with deterministic key order
//
//	// {"timestamp":"...","level":"INFO","message":"Message","key":"value"}
//
// JSON output rules: reserved keys (timestamp, level, message, caller) come first,
// then CustomFields in sorted order, then entry fields in the order passed. A field
// whose key collides with a reserved key is emitted under "fields.<key>" instead of
// overwriting entry metadata. Values that cannot be marshaled degrade to their %v
// string form; the entry is never lost.
//
// Caller info is controlled by the Logger's WithCaller option; formatters render it
// whenever the entry carries it.
//
// Configuration:
//
//	logger := grlog.NewLogger(
//	    grlog.WithLevel(grlog.DEBUG),
//	    grlog.WithSink(stdoutSink),
//	    grlog.WithAsync(1000),                       // 1000-entry buffer
//	    grlog.WithOverflowPolicy(grlog.OverflowDrop), // behavior when buffer is full
//	    grlog.WithCaller(true),                      // include caller info
//	    grlog.WithCallerSkip(1),                     // extra frames for wrappers
//	    grlog.WithErrorHandler(func(err error) {}),  // sink failure callback
//	    grlog.WithSampler(100, grlog.INFO),          // keep 1-in-100 DEBUG/INFO
//	    grlog.WithContextFields(
//	        grlog.String("service", "api"),
//	    ),
//	)
//
// Asynchronous Logging:
//
// WithAsync(n) enables a background worker consuming a buffered queue. When the
// buffer fills, behavior follows the configured OverflowPolicy:
//   - OverflowSyncFallback (default): write synchronously; nothing is lost, entries
//     may appear slightly out of order
//   - OverflowBlock: block the caller until space frees; strict ordering
//   - OverflowDrop: drop the entry; never blocks; drops are counted in
//     logger.Stats().DroppedEntries
//
// Close() stops the worker and drains the queue completely before closing sinks.
//
// Context-Aware Logging:
//
// Context values are stored under private typed keys via helper functions, so they
// cannot collide with other packages' context values:
//
//	ctx := grlog.ContextWithRequestID(ctx, "req-123") // -> request_id
//	ctx = grlog.ContextWithTraceID(ctx, "trace-456")  // -> trace_id
//	ctx = grlog.ContextWithUserID(ctx, "user-789")    // -> user_id
//
//	requestLogger := logger.WithContext(ctx)
//	requestLogger.Info("Processing request") // includes request_id, trace_id, user_id
//
// Custom extraction (e.g. OpenTelemetry span IDs) is supported via
// WithContextExtractor. Permanent fields without a context are added with With:
//
//	dbLog := logger.With(grlog.String("component", "database"))
//
// log/slog Interoperability:
//
// grlog can serve as the backend for the standard library's log/slog:
//
//	slog.SetDefault(slog.New(grlog.NewSlogHandler(logger)))
//	slog.Info("from slog", "status", 200)
//
// Groups become dotted keys ("req.status"); levels map Debug→DEBUG, Info→INFO,
// Warn→WARN, Error→ERROR.
//
// Error Handling:
//
// Sink write failures are reported to stderr by default, rate-limited so a
// persistently failing sink cannot flood it. WithErrorHandler replaces this with a
// custom callback (which must not log through the same logger).
//
// Performance:
//
// Indicative results on Apple M-series (run `make bench` for current numbers):
//   - Typed field construction: ~0.22 ns/op, 0 allocs
//   - Plain formatting: ~78-150 ns/op, 0 allocs (pooled buffers)
//   - JSON formatting (3 fields): ~205 ns/op, 1 alloc
//   - Sync log without fields: ~125 ns/op, 0 allocs
//   - Filtered-out log call: ~2.7 ns/op, 0 allocs
//   - Caller info adds ~500 ns/op (runtime.Caller); disable on hot paths
//
// Best Practices:
//
//  1. Always defer logger.Close() — it drains async buffers.
//  2. Choose appropriate levels: DEBUG/INFO in development, INFO/WARN in production.
//  3. Prefer typed fields over Any() (reflection).
//  4. Size async buffers for your burst profile and pick an OverflowPolicy explicitly.
//  5. Use WithContext/With for request- and component-scoped fields.
//  6. Disable caller info on hot paths.
//  7. Monitor logger.Stats() when using OverflowDrop or sampling.
//
// Testing:
//
// Use NewWriterSink with a bytes.Buffer, or NewCustomSink, to capture output:
//
//	var buf bytes.Buffer
//	logger := grlog.NewLogger(
//	    grlog.WithSink(grlog.NewWriterSink(&buf, grlog.PlainFormat())),
//	    grlog.WithCaller(false),
//	)
//
// License: MIT
// Repository: https://github.com/gourdian25/grlog
package grlog
