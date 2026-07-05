// File: grlog_bench_test.go

package grlog

import (
	"context"
	"io"
	"testing"
	"time"
)

// ==========================
// Baseline Benchmarks
// ==========================

// BenchmarkBaseline_NoLogging measures the cost of NOT logging (for comparison)
func BenchmarkBaseline_NoLogging(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Do nothing - baseline measurement
		_ = i
	}
}

// ==========================
// Field Construction Benchmarks
// ==========================

func BenchmarkFields_String(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = String("key", "value")
	}
}

func BenchmarkFields_Int(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Int("count", 42)
	}
}

func BenchmarkFields_Int64(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Int64("bignum", 9223372036854775807)
	}
}

func BenchmarkFields_Bool(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Bool("flag", true)
	}
}

func BenchmarkFields_Duration(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Duration("latency", 100*time.Millisecond)
	}
}

func BenchmarkFields_Mixed5Fields(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fields := []Field{
			String("method", "GET"),
			String("path", "/api/users"),
			Int("status", 200),
			Duration("latency", 50*time.Millisecond),
			Bool("cached", false),
		}
		_ = fields
	}
}

func BenchmarkFields_Mixed10Fields(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fields := []Field{
			String("method", "POST"),
			String("path", "/api/orders"),
			String("user_id", "user123"),
			String("request_id", "req-abc-123"),
			Int("status", 201),
			Int("body_size", 1024),
			Int64("user_balance", 50000),
			Duration("latency", 120*time.Millisecond),
			Bool("authenticated", true),
			Bool("success", true),
		}
		_ = fields
	}
}

// ==========================
// Formatter Benchmarks
// ==========================

func BenchmarkFormatter_Plain_NoFields(b *testing.B) {
	formatter := PlainFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "user logged in",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkFormatter_Plain_3Fields(b *testing.B) {
	formatter := PlainFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "request completed",
		Fields: []Field{
			String("method", "GET"),
			Int("status", 200),
			Duration("latency", 50*time.Millisecond),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkFormatter_Plain_10Fields(b *testing.B) {
	formatter := PlainFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "order processed",
		Fields: []Field{
			String("order_id", "ord123"),
			String("user_id", "user456"),
			String("payment_method", "credit_card"),
			Int("items", 5),
			Int("quantity", 12),
			Int64("amount_cents", 4999),
			Duration("processing_time", 250*time.Millisecond),
			Bool("express_shipping", true),
			Bool("gift_wrap", false),
			Bool("success", true),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkFormatter_JSON_NoFields(b *testing.B) {
	formatter := JSONFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "user logged in",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkFormatter_JSON_3Fields(b *testing.B) {
	formatter := JSONFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "request completed",
		Fields: []Field{
			String("method", "GET"),
			Int("status", 200),
			Duration("latency", 50*time.Millisecond),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkFormatter_JSON_10Fields(b *testing.B) {
	formatter := JSONFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "order processed",
		Fields: []Field{
			String("order_id", "ord123"),
			String("user_id", "user456"),
			Int("status", 200),
			Int64("amount", 4999),
			Bool("success", true),
			String("method", "POST"),
			Duration("latency", 150*time.Millisecond),
			String("region", "us-west"),
			Int("retry_count", 0),
			Bool("cached", false),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkFormatter_JSON_PrettyPrint(b *testing.B) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.PrettyPrint = true
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "debug output",
		Fields: []Field{
			String("key1", "value1"),
			String("key2", "value2"),
			Int("count", 42),
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

// ==========================
// Logger Benchmarks - Sync vs Async
// ==========================

func BenchmarkLogger_Sync_NoFields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("request processed")
	}
}

func BenchmarkLogger_Sync_3Fields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("request processed",
			String("method", "GET"),
			Int("status", 200),
			Duration("latency", 50*time.Millisecond),
		)
	}
}

func BenchmarkLogger_Sync_10Fields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("order placed",
			String("order_id", "ord123"),
			String("user_id", "user456"),
			String("payment_method", "card"),
			Int("items", 5),
			Int("status", 201),
			Int64("amount", 9999),
			Duration("processing", 200*time.Millisecond),
			Bool("express", true),
			Bool("gift", false),
			Bool("success", true),
		)
	}
}

func BenchmarkLogger_Async_NoFields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(10000),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("request processed")
	}
}

func BenchmarkLogger_Async_3Fields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(10000),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("request processed",
			String("method", "GET"),
			Int("status", 200),
			Duration("latency", 50*time.Millisecond),
		)
	}
}

func BenchmarkLogger_Async_10Fields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(10000),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("order placed",
			String("order_id", "ord123"),
			String("user_id", "user456"),
			String("payment_method", "card"),
			Int("items", 5),
			Int("status", 201),
			Int64("amount", 9999),
			Duration("processing", 200*time.Millisecond),
			Bool("express", true),
			Bool("gift", false),
			Bool("success", true),
		)
	}
}

// ==========================
// Level Filtering Benchmarks
// ==========================

func BenchmarkLogger_FilteredOut_Debug(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO), // DEBUG messages will be filtered
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Debug("debug message", String("key", "value"))
	}
}

func BenchmarkLogger_FilteredOut_Info(b *testing.B) {
	logger := NewLogger(
		WithLevel(ERROR), // INFO messages will be filtered
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("info message", String("key", "value"))
	}
}

// ==========================
// Caller Info Benchmarks
// ==========================

func BenchmarkLogger_WithCaller(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(true), // Enabled
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("message with caller")
	}
}

func BenchmarkLogger_WithoutCaller(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false), // Disabled
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("message without caller")
	}
}

// ==========================
// Context Logger Benchmarks
// ==========================

func BenchmarkLogger_WithContext_NoExtraction(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	ctx := context.Background() // Empty context
	contextLogger := logger.WithContext(ctx)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		contextLogger.Info("request")
	}
}

func BenchmarkLogger_WithContext_WithValues(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	ctx := context.Background()
	ctx = ContextWithRequestID(ctx, "req-123")
	ctx = ContextWithTraceID(ctx, "trace-456")
	ctx = ContextWithUserID(ctx, "user-789")
	contextLogger := logger.WithContext(ctx)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		contextLogger.Info("request")
	}
}

func BenchmarkLogger_WithContextFields(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
		WithContextFields(
			String("service", "api"),
			String("version", "1.0.0"),
			String("environment", "production"),
		),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("request", String("path", "/api/users"))
	}
}

// ==========================
// Formatted Logging Benchmarks
// ==========================

func BenchmarkLogger_Infof(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Infof("user %s logged in from %s", "alice", "192.168.1.1")
	}
}

func BenchmarkLogger_Info_vs_Infof(b *testing.B) {
	b.Run("Info_with_fields", func(b *testing.B) {
		logger := NewLogger(
			WithLevel(INFO),
			WithSink(&StdoutSink{
				formatter: PlainFormat(),
				writer:    io.Discard,
			}),
			WithCaller(false),
		)
		defer func() { _ = logger.Close() }()

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			logger.Info("user logged in",
				String("user", "alice"),
				String("ip", "192.168.1.1"),
			)
		}
	})

	b.Run("Infof_formatted", func(b *testing.B) {
		logger := NewLogger(
			WithLevel(INFO),
			WithSink(&StdoutSink{
				formatter: PlainFormat(),
				writer:    io.Discard,
			}),
			WithCaller(false),
		)
		defer func() { _ = logger.Close() }()

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			logger.Infof("user %s logged in from %s", "alice", "192.168.1.1")
		}
	})
}

// ==========================
// Multi-Sink Benchmarks
// ==========================

func BenchmarkLogger_SingleSink(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("message")
	}
}

func BenchmarkLogger_MultiSink_3Sinks(b *testing.B) {
	sink1 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
	sink2 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
	sink3 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}

	logger := NewLogger(
		WithLevel(INFO),
		WithSink(NewMultiSink(sink1, sink2, sink3)),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("message")
	}
}

// ==========================
// Parallel Benchmarks
// ==========================

func BenchmarkLogger_Parallel_Sync(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("concurrent message",
				String("thread", "worker"),
				Int("count", 1),
			)
		}
	})
}

func BenchmarkLogger_Parallel_Async(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(10000),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("concurrent message",
				String("thread", "worker"),
				Int("count", 1),
			)
		}
	})
}

// ==========================
// Real-World Scenario Benchmarks
// ==========================

func BenchmarkRealWorld_HTTPRequestLog(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: JSONFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("http request",
			String("method", "POST"),
			String("path", "/api/v1/users"),
			String("remote_addr", "192.168.1.100"),
			String("user_agent", "Mozilla/5.0"),
			Int("status", 201),
			Int("response_size", 1024),
			Duration("latency", 45*time.Millisecond),
			Bool("authenticated", true),
		)
	}
}

func BenchmarkRealWorld_ErrorLog(b *testing.B) {
	logger := NewLogger(
		WithLevel(ERROR),
		WithSink(&StdoutSink{
			formatter: JSONFormat(),
			writer:    io.Discard,
		}),
		WithCaller(true),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Error("database connection failed",
			String("database", "postgres"),
			String("host", "db.example.com"),
			Int("port", 5432),
			String("error", "connection timeout"),
			Int("retry_attempt", 3),
			Duration("timeout", 30*time.Second),
		)
	}
}

func BenchmarkRealWorld_StructuredBusinessEvent(b *testing.B) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: JSONFormat(),
			writer:    io.Discard,
		}),
		WithCaller(false),
		WithContextFields(
			String("service", "order-service"),
			String("version", "2.3.1"),
		),
	)
	defer func() { _ = logger.Close() }()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logger.Info("order_created",
			String("order_id", "ORD-2024-001234"),
			String("customer_id", "CUST-456789"),
			String("payment_method", "credit_card"),
			String("shipping_method", "express"),
			Int("item_count", 3),
			Int64("total_amount_cents", 149999),
			Int64("tax_amount_cents", 12000),
			Duration("checkout_duration", 125*time.Millisecond),
			Bool("gift_wrap", true),
			Bool("express_shipping", true),
			String("promo_code", "SAVE20"),
		)
	}
}
