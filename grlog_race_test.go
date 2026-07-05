// File: grlog_race_test.go

package grlog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==========================
// Race Condition Tests
// ==========================
// These tests MUST be run with: go test -race
// They test concurrent access patterns to ensure thread-safety

// ==========================
// Basic Concurrent Logging
// ==========================

func TestRace_BasicConcurrentLogging(t *testing.T) {
	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 100
	const messagesPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				logger.Info("concurrent message",
					String("goroutine", fmt.Sprintf("%d", id)),
					Int("message", j),
				)
			}
		}(i)
	}

	wg.Wait()
}

func TestRace_MixedLogLevels(t *testing.T) {
	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 50
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				switch j % 4 {
				case 0:
					logger.Debug("debug", Int("id", id))
				case 1:
					logger.Info("info", Int("id", id))
				case 2:
					logger.Warn("warn", Int("id", id))
				case 3:
					logger.Error("error", Int("id", id))
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestRace_FormattedLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				logger.Infof("goroutine %d message %d", id, j)
				logger.Errorf("error in goroutine %d iteration %d", id, j)
			}
		}(i)
	}

	wg.Wait()
}

// ==========================
// Level Get/Set Race Tests
// ==========================

func TestRace_ConcurrentLevelGetSet(t *testing.T) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Goroutines that set level
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
			for j := 0; j < iterations; j++ {
				logger.SetLevel(levels[j%len(levels)])
			}
		}(i)
	}

	// Goroutines that get level
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = logger.GetLevel()
			}
		}()
	}

	wg.Wait()
}

func TestRace_LoggingWhileChangingLevel(t *testing.T) {
	logger := NewLogger(
		WithLevel(INFO),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine that changes levels
	go func() {
		defer wg.Done()
		levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
		for i := 0; i < 100; i++ {
			logger.SetLevel(levels[i%len(levels)])
			time.Sleep(1 * time.Millisecond)
		}
	}()

	// Goroutines that log
	for i := 0; i < 2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				logger.Debug("debug", Int("id", id))
				logger.Info("info", Int("id", id))
				logger.Warn("warn", Int("id", id))
				logger.Error("error", Int("id", id))
			}
		}(i)
	}

	wg.Wait()
}

// ==========================
// Sink Add/Remove Race Tests
// ==========================

func TestRace_AddSinkDuringLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine that continuously logs
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				logger.Info("message", String("key", "value"))
			}
		}
	}()

	// Goroutine that adds sinks
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			sink := &StdoutSink{
				formatter: PlainFormat(),
				writer:    io.Discard,
			}
			logger.AddSink(sink)
			time.Sleep(2 * time.Millisecond)
		}
		close(done)
	}()

	wg.Wait()
}

func TestRace_RemoveSinkDuringLogging(t *testing.T) {
	// Create multiple sinks
	sinks := make([]LogSink, 10)
	for i := range sinks {
		sinks[i] = &StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}
	}

	logger := NewLogger(WithSink(sinks[0]))
	for _, s := range sinks[1:] {
		logger.AddSink(s)
	}
	defer func() { _ = logger.Close() }()

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine that continuously logs
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				logger.Info("message")
			}
		}
	}()

	// Goroutine that removes sinks
	go func() {
		defer wg.Done()
		for _, s := range sinks[1:] {
			time.Sleep(2 * time.Millisecond)
			logger.RemoveSink(s)
		}
		close(done)
	}()

	wg.Wait()
}

func TestRace_AddAndRemoveSinks(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	var wg sync.WaitGroup
	wg.Add(3)

	done := make(chan struct{})

	// Goroutine that logs
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				logger.Info("message")
			}
		}
	}()

	// Goroutine that adds sinks
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			sink := &StdoutSink{
				formatter: PlainFormat(),
				writer:    io.Discard,
			}
			logger.AddSink(sink)
			time.Sleep(3 * time.Millisecond)
		}
	}()

	// Goroutine that removes sinks
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // Let some sinks accumulate
		sink := &StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}
		for i := 0; i < 20; i++ {
			logger.RemoveSink(sink) // Try to remove (may not exist)
			time.Sleep(3 * time.Millisecond)
		}
		close(done)
	}()

	wg.Wait()
}

// ==========================
// Async Logger Race Tests
// ==========================

func TestRace_AsyncLogging(t *testing.T) {
	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(1000),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 50
	const messages = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messages; j++ {
				logger.Info("async message",
					Int("goroutine", id),
					Int("msg", j),
				)
			}
		}(i)
	}

	wg.Wait()
}

func TestRace_AsyncQueueOverflow(t *testing.T) {
	var counter atomic.Int32

	slowSink := NewCustomSink(func(entry LogEntry) error {
		counter.Add(1)
		time.Sleep(10 * time.Millisecond) // Slow sink
		return nil
	})

	logger := NewLogger(
		WithSink(slowSink),
		WithAsync(10), // Very small buffer
	)

	const goroutines = 10
	const messages = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messages; j++ {
				logger.Info(fmt.Sprintf("msg-%d-%d", id, j))
			}
		}(i)
	}

	wg.Wait()

	// Close logger to drain the async queue
	_ = logger.Close()

	// Verify all messages were eventually processed
	if counter.Load() != goroutines*messages {
		t.Errorf("Expected %d messages, got %d", goroutines*messages, counter.Load())
	}
}

func TestRace_CloseDuringAsyncLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(500),
	)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine that sends many messages
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			logger.Info(fmt.Sprintf("message %d", i))
		}
	}()

	// Goroutine that closes after a short delay
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		_ = logger.Close()
	}()

	wg.Wait()
}

func TestRace_MultipleAsyncLoggers(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex

	sink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := buf.WriteString(entry.Message + "\n")
		return err
	})

	logger1 := NewLogger(WithSink(sink), WithAsync(100))
	logger2 := NewLogger(WithSink(sink), WithAsync(100))
	logger3 := NewLogger(WithSink(sink), WithAsync(100))

	var wg sync.WaitGroup
	wg.Add(3)

	// Three async loggers writing to same sink
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			logger1.Info(fmt.Sprintf("logger1-%d", i))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			logger2.Info(fmt.Sprintf("logger2-%d", i))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			logger3.Info(fmt.Sprintf("logger3-%d", i))
		}
	}()

	wg.Wait()

	_ = logger1.Close()
	_ = logger2.Close()
	_ = logger3.Close()
}

// ==========================
// Context Logger Race Tests
// ==========================

func TestRace_WithContext(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			ctx := context.Background()
			ctx = ContextWithRequestID(ctx, fmt.Sprintf("req-%d", id))
			ctx = ContextWithUserID(ctx, fmt.Sprintf("user-%d", id))

			contextLogger := logger.WithContext(ctx)

			for j := 0; j < 50; j++ {
				contextLogger.Info(fmt.Sprintf("message %d", j))
			}
		}(i)
	}

	wg.Wait()
}

func TestRace_ContextFieldAccess(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithContextFields(
			String("service", "api"),
			String("version", "1.0"),
		),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 30

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				logger.Info("message", Int("count", j))
			}
		}()
	}

	wg.Wait()
}

func TestRace_MultipleContextLoggers(t *testing.T) {
	baseLogger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithContextFields(String("app", "test")),
	)
	defer func() { _ = baseLogger.Close() }()

	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Each goroutine creates its own context logger
			ctx := context.Background()
			ctx = ContextWithRequestID(ctx, fmt.Sprintf("req-%d", id))
			ctxLogger := baseLogger.WithContext(ctx)

			for j := 0; j < 50; j++ {
				ctxLogger.Info("message", Int("num", j))
			}
		}(i)
	}

	wg.Wait()
}

// ==========================
// MultiSink Race Tests
// ==========================

func TestRace_MultiSinkConcurrent(t *testing.T) {
	sink1 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
	sink2 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
	sink3 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}

	multiSink := NewMultiSink(sink1, sink2, sink3)

	logger := NewLogger(WithSink(multiSink))
	defer func() { _ = logger.Close() }()

	const goroutines = 50
	const messages = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messages; j++ {
				logger.Info("multisink message",
					Int("goroutine", id),
					Int("msg", j),
				)
			}
		}(i)
	}

	wg.Wait()
}

func TestRace_CustomSinkConcurrent(t *testing.T) {
	var counter atomic.Int32

	customSink := NewCustomSink(func(entry LogEntry) error {
		counter.Add(1)
		return nil
	})

	logger := NewLogger(WithSink(customSink))
	defer func() { _ = logger.Close() }()

	const goroutines = 30
	const messages = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messages; j++ {
				logger.Info(fmt.Sprintf("msg-%d-%d", id, j))
			}
		}(i)
	}

	wg.Wait()

	expected := int32(goroutines * messages)
	if counter.Load() != expected {
		t.Errorf("Expected %d messages, got %d", expected, counter.Load())
	}
}

// ==========================
// Close Race Tests
// ==========================

func TestRace_CloseWhileLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine that logs
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			logger.Info(fmt.Sprintf("message %d", i))
		}
	}()

	// Goroutine that closes
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		_ = logger.Close()
	}()

	wg.Wait()
}

func TestRace_MultipleCloses(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)

	// Log some messages first
	for i := 0; i < 10; i++ {
		logger.Info(fmt.Sprintf("message %d", i))
	}

	// Try to close from multiple goroutines
	var wg sync.WaitGroup
	wg.Add(10)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			_ = logger.Close()
		}()
	}

	wg.Wait()
}

func TestRace_LogAfterClose(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine that closes
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		_ = logger.Close()
	}()

	// Goroutine that keeps trying to log
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			logger.Info(fmt.Sprintf("message %d", i))
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}

// ==========================
// Stress Tests
// ==========================

func TestRace_HighContentionStress(t *testing.T) {
	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 100
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // 3 operations per goroutine group

	// Goroutines that log
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				logger.Info("message",
					String("g", fmt.Sprintf("%d", id)),
					Int("i", j),
				)
			}
		}(i)
	}

	// Goroutines that change level
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
			for j := 0; j < iterations; j++ {
				logger.SetLevel(levels[j%4])
			}
		}()
	}

	// Goroutines that add/remove sinks
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations/2; j++ {
				sink := &StdoutSink{
					formatter: PlainFormat(),
					writer:    io.Discard,
				}
				logger.AddSink(sink)
				logger.RemoveSink(sink)
			}
		}()
	}

	wg.Wait()
}

func TestRace_MixedSyncAsyncStress(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex

	sink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := buf.WriteString(entry.Message + "\n")
		return err
	})

	syncLogger := NewLogger(WithSink(sink))
	asyncLogger := NewLogger(WithSink(sink), WithAsync(200))

	const goroutines = 20
	const messages = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Sync loggers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messages; j++ {
				syncLogger.Info(fmt.Sprintf("sync-%d-%d", id, j))
			}
		}(i)
	}

	// Async loggers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messages; j++ {
				asyncLogger.Info(fmt.Sprintf("async-%d-%d", id, j))
			}
		}(i)
	}

	wg.Wait()

	_ = syncLogger.Close()
	_ = asyncLogger.Close()
}

func TestRace_FullConcurrencyStorm(t *testing.T) {
	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(&StdoutSink{
			formatter: JSONFormat(),
			writer:    io.Discard,
		}),
		WithAsync(500),
		WithCaller(true),
		WithContextFields(String("app", "test")),
	)

	const workers = 50
	const duration = 100 * time.Millisecond

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Workers that log at all levels
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-stop:
					return
				default:
					switch counter % 4 {
					case 0:
						logger.Debug("debug", Int("w", id), Int("c", counter))
					case 1:
						logger.Info("info", Int("w", id), Int("c", counter))
					case 2:
						logger.Warn("warn", Int("w", id), Int("c", counter))
					case 3:
						logger.Error("error", Int("w", id), Int("c", counter))
					}
					counter++
				}
			}
		}(i)
	}

	// Workers that change level
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
			idx := 0
			for {
				select {
				case <-stop:
					return
				default:
					logger.SetLevel(levels[idx%4])
					idx++
					time.Sleep(5 * time.Millisecond)
				}
			}
		}()
	}

	// Workers that manipulate sinks
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					sink := &StdoutSink{
						formatter: PlainFormat(),
						writer:    io.Discard,
					}
					logger.AddSink(sink)
					time.Sleep(10 * time.Millisecond)
					logger.RemoveSink(sink)
				}
			}
		}()
	}

	// Let it run for the duration
	time.Sleep(duration)
	close(stop)
	wg.Wait()

	_ = logger.Close()
}
