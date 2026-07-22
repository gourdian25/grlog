// File: examples/basic/main.go

// Command basic demonstrates grlog's zero-configuration path: a default
// logger writing plain-text output to stdout, structured typed fields, and
// the printf-style helpers.
package main

import (
	"errors"
	"time"

	"github.com/gourdian25/grlog"
)

func main() {
	logger := grlog.NewDefaultLogger()
	defer func() { _ = logger.Close() }() // drains any buffered output before the process exits

	logger.Info("service starting", grlog.String("version", "1.0.0"))

	logger.Debug("this is filtered out at the default INFO level")

	logger.Info("user authenticated",
		grlog.String("user_id", "u_12345"),
		grlog.Int("attempts", 1),
		grlog.Bool("mfa_used", true),
		grlog.Duration("latency", 42*time.Millisecond),
	)

	logger.Warn("cache miss rate high", grlog.Float64("ratio", 0.42))

	if err := doWork(); err != nil {
		// grlog.Err is nil-safe, so it can be passed unconditionally; here
		// it is guaranteed non-nil.
		logger.Error("background job failed", grlog.Err(err))
	}

	logger.Infof("processed %d items in %s", 128, 12*time.Millisecond)

	logger.Info("service ready")
}

func doWork() error {
	return errors.New("connection to upstream timed out")
}
