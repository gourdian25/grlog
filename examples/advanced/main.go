// File: examples/advanced/main.go

// Command advanced demonstrates a production-shaped grlog setup: async
// logging with an explicit overflow policy, a multi-sink fan-out (stdout
// JSON plus a rotating file), context-scoped and component-scoped derived
// loggers, sampling, a custom error handler, runtime stats, and a
// graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gourdian25/grlog"
)

func main() {
	fileSink, err := grlog.NewFileSink(grlog.FileSinkConfig{
		Filename:    "advanced-example",
		Dir:         "logs",
		MaxBytes:    5 * 1024 * 1024, // rotate at 5MB
		BackupCount: 3,
		Compress:    true,
		Formatter:   grlog.JSONFormat(),
	})
	if err != nil {
		// Fall back to stdout rather than failing to start.
		fileSink = grlog.NewStdoutSink(grlog.JSONFormat())
	}

	stdoutSink := grlog.NewStdoutSink(grlog.JSONFormat())

	logger := grlog.NewLogger(
		grlog.WithLevel(grlog.INFO),
		grlog.WithSink(grlog.NewMultiSink(stdoutSink, fileSink)),
		grlog.WithAsync(10000),
		grlog.WithOverflowPolicy(grlog.OverflowSyncFallback),
		grlog.WithCaller(true),
		grlog.WithSampler(50, grlog.INFO), // keep 1-in-50 DEBUG/INFO under load
		grlog.WithContextFields(
			grlog.String("service", "orders-api"),
			grlog.String("environment", "production"),
		),
		grlog.WithErrorHandler(func(err error) {
			// Must not log through this same logger (risk of recursion) -
			// route to a metrics counter or a second, independent logger.
			_, _ = os.Stderr.WriteString("sink write failed: " + err.Error() + "\n")
		}),
	)
	defer func() { _ = logger.Close() }() // drains the async queue before sinks close

	logger.Info("service starting")

	// A permanent component-scoped view: every entry through dbLog carries
	// component=database in addition to the base context fields.
	dbLog := logger.With(grlog.String("component", "database"))
	dbLog.Info("connection pool ready", grlog.Int("pool_size", 20))

	// A request-scoped view built from typed, collision-proof context keys.
	ctx := grlog.ContextWithRequestID(context.Background(), "req-8f21")
	ctx = grlog.ContextWithTraceID(ctx, "trace-40aa")
	ctx = grlog.ContextWithUserID(ctx, "user-772")
	reqLog := logger.WithContext(ctx)
	reqLog.Info("request handled",
		grlog.String("method", "POST"),
		grlog.String("path", "/orders"),
		grlog.Int("status", 201),
		grlog.Duration("latency", 18*time.Millisecond),
	)

	// Dynamic sink management: attach a destination after startup.
	auditSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
		// e.g. forward entry.Level >= grlog.ERROR to an alerting system
		return nil
	})
	logger.AddSink(auditSink)
	defer logger.RemoveSink(auditSink)

	if stats := logger.Stats(); stats.DroppedEntries > 0 || stats.SampledEntries > 0 {
		logger.Warn("logger is shedding load",
			grlog.Uint64("dropped", stats.DroppedEntries),
			grlog.Uint64("sampled", stats.SampledEntries),
		)
	}

	logger.Info("service ready")

	// Block until an operator or orchestrator asks the process to stop,
	// then shut down so buffered async entries are flushed.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
		logger.Info("shutdown signal received")
	case <-time.After(100 * time.Millisecond):
		// This example exits quickly instead of blocking forever; a real
		// service would omit this case and wait on the signal indefinitely.
		logger.Info("example run complete")
	}
}
