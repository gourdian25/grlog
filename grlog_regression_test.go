// File: grlog_regression_test.go

package grlog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ==========================
// Section 1: Close semantics of derived views (double-close panic regression)
// ==========================

// Closing both a WithContext view and its parent used to close the shared
// closeChan twice, panicking. Now any view's Close closes the core once.
func TestClose_ParentAndChildView_NoPanic(t *testing.T) {
	logger := NewLogger(
		WithSink(NewWriterSink(io.Discard, PlainFormat())),
		WithAsync(16),
	)

	ctx := ContextWithRequestID(context.Background(), "req-1")
	child := logger.WithContext(ctx)

	child.Info("from child")
	logger.Info("from parent")

	if err := child.Close(); err != nil {
		t.Errorf("child Close failed: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Errorf("parent Close failed: %v", err)
	}
	// A second close on either view must be a no-op
	if err := child.Close(); err != nil {
		t.Errorf("repeated Close failed: %v", err)
	}
}

func TestClose_ConcurrentViews_NoPanic(t *testing.T) {
	logger := NewLogger(
		WithSink(NewWriterSink(io.Discard, PlainFormat())),
		WithAsync(4),
	)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			view := logger.With(Int("goroutine", id))
			view.Info("message")
			_ = view.Close()
		}(i)
	}
	wg.Wait()
	_ = logger.Close()
}

// ==========================
// Section 2: RemoveSink on a derived view (shared-slice corruption regression)
// ==========================

func TestRemoveSink_OnChildView_AffectsCoreConsistently(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	sink1 := NewWriterSink(&buf1, PlainFormat())
	sink2 := NewWriterSink(&buf2, PlainFormat())

	logger := NewLogger(WithSink(sink1), WithSink(sink2), WithCaller(false))
	defer func() { _ = logger.Close() }()

	child := logger.With(String("scope", "child"))

	// Removing via the child must remove exactly one sink, visible everywhere
	child.RemoveSink(sink1)

	logger.Info("after remove")

	if buf1.Len() != 0 {
		t.Errorf("removed sink still received writes: %s", buf1.String())
	}
	if !strings.Contains(buf2.String(), "after remove") {
		t.Errorf("remaining sink missed the write: %s", buf2.String())
	}
	if got := len(logger.core.sinks); got != 1 {
		t.Errorf("expected 1 sink after removal, got %d", got)
	}
}

// ==========================
// Section 3: Shutdown drain (lost-logs regression)
// ==========================

// Every entry accepted by log() before Close must reach the sinks; the async
// queue is drained (including the post-worker race window) before sinks close.
func TestClose_AsyncDrain_NoLostEntries(t *testing.T) {
	var mu sync.Mutex
	count := 0
	sink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	const total = 1000
	logger := NewLogger(WithSink(sink), WithAsync(64), WithCaller(false))
	for i := 0; i < total; i++ {
		logger.Info("entry", Int("i", i))
	}
	_ = logger.Close()

	mu.Lock()
	defer mu.Unlock()
	if count != total {
		t.Errorf("lost log entries on Close: wrote %d, sank %d", total, count)
	}
}

// ==========================
// Section 4: File rotation safety
// ==========================

// Rapid rotations within the same second used to overwrite the previous
// backup (second-granularity names). Backup names are now collision-safe.
func TestFileSink_RapidRotation_NoBackupOverwrite(t *testing.T) {
	dir := t.TempDir()
	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "app",
		Dir:         dir,
		MaxBytes:    64, // rotate on nearly every write
		BackupCount: 100,
	})
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer func() { _ = sink.Close() }()

	const writes = 20
	for i := 0; i < writes; i++ {
		err := sink.Write(LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("x", 80), // exceeds MaxBytes every time
		})
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	backups, _ := filepath.Glob(filepath.Join(dir, "app_*.log"))
	// Every write after the first triggers a rotation; all backups must survive.
	if len(backups) < writes-1 {
		t.Errorf("backups were overwritten: expected >= %d, found %d", writes-1, len(backups))
	}
}

// A failed rotation must not brick the sink: writes keep going to the
// reopened current file.
func TestFileSink_RotationFailure_KeepsWriting(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("directory permissions are not enforced for root")
	}
	dir := t.TempDir()
	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "app",
		Dir:         dir,
		MaxBytes:    64,
		BackupCount: 5,
	})
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Make the directory read-only so the rotation rename fails
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	defer func() { _ = os.Chmod(dir, 0755) }()

	entry := LogEntry{Timestamp: time.Now(), Level: INFO, Message: strings.Repeat("x", 80)}
	if err := sink.Write(entry); err != nil {
		t.Fatalf("first write failed: %v", err)
	}
	// This write exceeds MaxBytes -> rotation fails (rename EACCES), but the
	// entry must still be written and subsequent writes must succeed.
	err = sink.Write(entry)
	if err == nil {
		t.Fatalf("expected rotation error, got nil")
	}
	if !strings.Contains(err.Error(), "rotation failed") {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = os.Chmod(dir, 0755)
	if err := sink.Write(entry); err != nil {
		t.Errorf("sink bricked after failed rotation: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) == 0 {
		t.Errorf("entries were lost during failed rotation")
	}
}

func TestFileSink_Compress(t *testing.T) {
	dir := t.TempDir()
	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "app",
		Dir:         dir,
		MaxBytes:    64,
		BackupCount: 10,
		Compress:    true,
	})
	if err != nil {
		t.Fatalf("NewFileSink failed: %v", err)
	}
	defer func() { _ = sink.Close() }()

	entry := LogEntry{Timestamp: time.Now(), Level: INFO, Message: strings.Repeat("x", 80)}
	for i := 0; i < 3; i++ {
		if err := sink.Write(entry); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	// Compression runs in the background; poll briefly.
	var gzFiles []string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gzFiles, _ = filepath.Glob(filepath.Join(dir, "app_*.log.gz"))
		if len(gzFiles) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(gzFiles) == 0 {
		t.Fatalf("no compressed backups were produced")
	}

	// The archive must be a readable gzip containing the log line
	f, err := os.Open(gzFiles[0])
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("archive is not valid gzip: %v", err)
	}
	content, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if !strings.Contains(string(content), "xxxx") {
		t.Errorf("archive does not contain the log entry")
	}
}

// ==========================
// Section 5: JSON formatter determinism and collisions
// ==========================

func TestJSONFormatter_ReservedKeyCollision(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)

	entry := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     INFO,
		Message:   "real message",
		Fields: []Field{
			String("message", "attacker message"),
			String("level", "FAKE"),
		},
	}

	output := formatter.Format(entry)
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["message"] != "real message" {
		t.Errorf("entry metadata was clobbered by a field: %s", output)
	}
	if result["level"] != "INFO" {
		t.Errorf("level was clobbered by a field: %s", output)
	}
	if result["fields.message"] != "attacker message" {
		t.Errorf("colliding field was not preserved under fields.*: %s", output)
	}
	if result["fields.level"] != "FAKE" {
		t.Errorf("colliding field was not preserved under fields.*: %s", output)
	}
}

func TestJSONFormatter_DeterministicOutput(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.CustomFields = map[string]interface{}{
		"zeta": 1, "alpha": 2, "mid": 3,
	}

	entry := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     INFO,
		Message:   "msg",
		Fields:    []Field{String("b", "1"), String("a", "2")},
	}

	first := string(formatter.Format(entry))
	for i := 0; i < 20; i++ {
		if got := string(formatter.Format(entry)); got != first {
			t.Fatalf("output not deterministic:\n%s\n%s", first, got)
		}
	}

	// Entry field order must be preserved as passed (b before a)
	if strings.Index(first, `"b"`) > strings.Index(first, `"a":"2"`) {
		t.Errorf("entry field order not preserved: %s", first)
	}
}

func TestJSONFormatter_StringEscaping(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "quote\" backslash\\ newline\n tab\t control\x01",
		Fields:    []Field{String("k\"ey", "v\nal")},
	}
	output := formatter.Format(entry)
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("escaped output is invalid JSON: %v\n%s", err, output)
	}
	if result["message"] != "quote\" backslash\\ newline\n tab\t control\x01" {
		t.Errorf("message not round-tripped: %q", result["message"])
	}
	if result["k\"ey"] != "v\nal" {
		t.Errorf("field not round-tripped: %s", output)
	}
}

func TestJSONFormatter_InvalidUTF8(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "bad \xff\xfe utf8",
	}
	output := formatter.Format(entry)
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("invalid-UTF8 message produced invalid JSON: %v\n%s", err, output)
	}
}

// ==========================
// Section 6: Err(nil), typed context keys, extractors
// ==========================

func TestErrNil_SkippedInOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, PlainFormat())), WithCaller(false))
	defer func() { _ = logger.Close() }()

	logger.Info("no error", Err(nil), String("ok", "yes"))

	output := buf.String()
	if strings.Contains(output, "error=") {
		t.Errorf("Err(nil) should be skipped, got: %s", output)
	}
	if !strings.Contains(output, "ok=yes") {
		t.Errorf("other fields must survive: %s", output)
	}

	// JSON path
	buf.Reset()
	jsonLogger := NewLogger(WithSink(NewWriterSink(&buf, JSONFormat())), WithCaller(false))
	defer func() { _ = jsonLogger.Close() }()
	jsonLogger.Info("no error", Err(nil))
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, exists := result["error"]; exists {
		t.Errorf("Err(nil) should be skipped in JSON, got: %s", buf.String())
	}
}

func TestWithContextExtractor(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(
		WithSink(NewWriterSink(&buf, PlainFormat())),
		WithCaller(false),
		WithContextExtractor(func(ctx context.Context) []Field {
			if v, ok := ctx.Value(testCtxKey("tenant")).(string); ok {
				return []Field{String("tenant_id", v)}
			}
			return nil
		}),
	)
	defer func() { _ = logger.Close() }()

	ctx := context.WithValue(context.Background(), testCtxKey("tenant"), "acme")
	logger.WithContext(ctx).Info("hello")

	if !strings.Contains(buf.String(), "tenant_id=acme") {
		t.Errorf("custom extractor fields missing: %s", buf.String())
	}
}

// ==========================
// Section 7: Phase 2 features (overflow policy, error handler, caller skip, With)
// ==========================

func TestOverflowDrop_CountsDropped(t *testing.T) {
	block := make(chan struct{})
	var once sync.Once
	sink := NewCustomSink(func(entry LogEntry) error {
		<-block // stall the worker so the queue fills
		return nil
	})

	logger := NewLogger(
		WithSink(sink),
		WithAsync(2),
		WithOverflowPolicy(OverflowDrop),
		WithCaller(false),
	)

	for i := 0; i < 100; i++ {
		logger.Info("spam")
	}
	stats := logger.Stats()
	once.Do(func() { close(block) })
	_ = logger.Close()

	if stats.DroppedEntries == 0 {
		t.Errorf("expected dropped entries under OverflowDrop, got 0")
	}
}

func TestWithErrorHandler_ReceivesSinkErrors(t *testing.T) {
	var mu sync.Mutex
	var handled []error
	failing := NewCustomSink(func(entry LogEntry) error {
		return errors.New("sink is down")
	})

	logger := NewLogger(
		WithSink(failing),
		WithErrorHandler(func(err error) {
			mu.Lock()
			handled = append(handled, err)
			mu.Unlock()
		}),
	)
	defer func() { _ = logger.Close() }()

	logger.Info("will fail")

	mu.Lock()
	defer mu.Unlock()
	if len(handled) != 1 || !strings.Contains(handled[0].Error(), "sink is down") {
		t.Errorf("error handler not invoked correctly: %v", handled)
	}
}

func TestWithCallerSkip(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(
		WithSink(NewWriterSink(&buf, PlainFormat())),
		WithCaller(true),
		WithCallerSkip(1),
	)
	defer func() { _ = logger.Close() }()

	helper := func() { logger.Info("wrapped") }
	helper()

	// With skip=1 the caller should be this test function, not the helper closure.
	if !strings.Contains(buf.String(), "TestWithCallerSkip") {
		t.Errorf("caller skip not applied: %s", buf.String())
	}
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, PlainFormat())), WithCaller(false))
	defer func() { _ = logger.Close() }()

	dbLog := logger.With(String("component", "db"))
	dbLog.Info("query done", Duration("took", 5*time.Millisecond))

	output := buf.String()
	if !strings.Contains(output, "component=db") || !strings.Contains(output, "took=5ms") {
		t.Errorf("With fields missing: %s", output)
	}

	// The parent view must not carry the child's fields
	buf.Reset()
	logger.Info("plain")
	if strings.Contains(buf.String(), "component=db") {
		t.Errorf("With leaked fields into parent: %s", buf.String())
	}
}

// ==========================
// Section 8: Phase 4 features (levels, leveled sink, sampling, slog)
// ==========================

func TestFatalLevel_AndOff(t *testing.T) {
	if got, err := ParseLogLevel("fatal"); err != nil || got != FATAL {
		t.Errorf("ParseLogLevel(fatal) = %v, %v", got, err)
	}
	if got, err := ParseLogLevel("off"); err != nil || got != OFF {
		t.Errorf("ParseLogLevel(off) = %v, %v", got, err)
	}
	if FATAL.String() != "FATAL" || OFF.String() != "OFF" {
		t.Errorf("level String() wrong: %s, %s", FATAL, OFF)
	}

	// OFF disables everything
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, PlainFormat())), WithLevel(OFF))
	defer func() { _ = logger.Close() }()
	logger.Error("should not appear")
	if buf.Len() != 0 {
		t.Errorf("OFF level still logged: %s", buf.String())
	}
}

func TestFatal_LogsFlushesAndExits(t *testing.T) {
	var buf bytes.Buffer
	exitCode := -1
	origExit := osExit
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = origExit }()

	logger := NewLogger(WithSink(NewWriterSink(&buf, PlainFormat())), WithAsync(8), WithCaller(false))
	logger.Fatal("fatal error", String("cause", "disk full"))

	if exitCode != 1 {
		t.Errorf("Fatal should exit(1), got %d", exitCode)
	}
	if !strings.Contains(buf.String(), "[FATAL]") || !strings.Contains(buf.String(), "cause=disk full") {
		t.Errorf("Fatal entry not flushed before exit: %s", buf.String())
	}
	if !logger.core.closed.Load() {
		t.Errorf("Fatal should close the logger before exiting")
	}
}

func TestLeveledSink_Filters(t *testing.T) {
	var all, errorsOnly bytes.Buffer
	logger := NewLogger(
		WithSink(NewMultiSink(
			NewWriterSink(&all, PlainFormat()),
			NewLeveledSink(NewWriterSink(&errorsOnly, PlainFormat()), ERROR),
		)),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	logger.Info("info msg")
	logger.Error("error msg")

	if !strings.Contains(all.String(), "info msg") || !strings.Contains(all.String(), "error msg") {
		t.Errorf("unleveled sink missed entries: %s", all.String())
	}
	if strings.Contains(errorsOnly.String(), "info msg") {
		t.Errorf("leveled sink leaked INFO: %s", errorsOnly.String())
	}
	if !strings.Contains(errorsOnly.String(), "error msg") {
		t.Errorf("leveled sink missed ERROR: %s", errorsOnly.String())
	}
}

func TestWithSampler(t *testing.T) {
	var mu sync.Mutex
	count := 0
	sink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	logger := NewLogger(WithSink(sink), WithSampler(10, INFO), WithCaller(false))
	defer func() { _ = logger.Close() }()

	for i := 0; i < 100; i++ {
		logger.Info("sampled")
	}
	// ERROR is above maxLevel INFO: never sampled
	for i := 0; i < 10; i++ {
		logger.Error("never sampled")
	}

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 10+10 { // 100/10 sampled INFO + 10 unsampled ERROR
		t.Errorf("expected 20 entries after sampling, got %d", got)
	}
	if s := logger.Stats().SampledEntries; s != 90 {
		t.Errorf("expected 90 sampled-out entries, got %d", s)
	}
}

func TestSlogHandler_Basic(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, JSONFormat())), WithCaller(false))
	defer func() { _ = logger.Close() }()

	sl := slog.New(NewSlogHandler(logger))
	sl = sl.With("service", "api")
	sl.WithGroup("req").Info("handled", "status", 200, "dur", 5*time.Millisecond)

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON from slog path: %v\n%s", err, buf.String())
	}
	if result["message"] != "handled" {
		t.Errorf("message missing: %s", buf.String())
	}
	if result["level"] != "INFO" {
		t.Errorf("level wrong: %s", buf.String())
	}
	if result["service"] != "api" {
		t.Errorf("With attr missing: %s", buf.String())
	}
	if result["req.status"] != float64(200) {
		t.Errorf("group attr missing: %s", buf.String())
	}
	if result["req.dur"] != "5ms" {
		t.Errorf("duration attr wrong: %s", buf.String())
	}
}

func TestSlogHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, PlainFormat())), WithLevel(WARN))
	defer func() { _ = logger.Close() }()

	sl := slog.New(NewSlogHandler(logger))
	if sl.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("debug should be disabled at WARN")
	}
	sl.Info("hidden")
	sl.Warn("visible")

	if strings.Contains(buf.String(), "hidden") {
		t.Errorf("filtered slog record was written: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "visible") {
		t.Errorf("passing slog record missing: %s", buf.String())
	}
}

// ==========================
// Section 9: Field constructors added for production coverage
// ==========================

func TestNewFieldConstructors(t *testing.T) {
	ts := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, JSONFormat())), WithCaller(false))
	defer func() { _ = logger.Close() }()

	logger.Info("typed",
		Float64("ratio", 0.75),
		Uint64("offset", 18446744073709551615),
		Time("at", ts),
	)

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}
	if result["ratio"] != 0.75 {
		t.Errorf("Float64 wrong: %v", result["ratio"])
	}
	// uint64 max exceeds float64 precision; compare the raw JSON text
	if !bytes.Contains(buf.Bytes(), []byte(`"offset":18446744073709551615`)) {
		t.Errorf("Uint64 wrong: %s", buf.String())
	}
	if result["at"] != "2024-06-01T10:00:00Z" {
		t.Errorf("Time wrong: %v", result["at"])
	}
}

func TestJSONFormatter_NaNInf(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "math",
		Fields: []Field{
			Float64("nan", math.NaN()),
			Float64("inf", math.Inf(1)),
		},
	}
	output := formatter.Format(entry)
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("NaN/Inf produced invalid JSON: %v\n%s", err, output)
	}
}

func TestGoModVersionConsistency(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Skip("go.mod not readable")
	}
	if !strings.Contains(string(data), "go 1.21") {
		t.Errorf("go.mod should declare go 1.21 (log/slog requirement), got:\n%s", data)
	}
}
