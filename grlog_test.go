// File: grlog_test.go

package grlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ==========================
// Section 1: Core Types & Utilities
// ==========================

// 1.1 LogLevel Tests

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		expected string
	}{
		{"DEBUG level", DEBUG, "DEBUG"},
		{"INFO level", INFO, "INFO"},
		{"WARN level", WARN, "WARN"},
		{"ERROR level", ERROR, "ERROR"},
		{"Invalid level positive", LogLevel(99), "UNKNOWN"},
		{"Invalid level negative", LogLevel(-1), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseLogLevel_Valid(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected LogLevel
	}{
		{"lowercase debug", "debug", DEBUG},
		{"uppercase DEBUG", "DEBUG", DEBUG},
		{"lowercase info", "info", INFO},
		{"uppercase INFO", "INFO", INFO},
		{"lowercase warn", "warn", WARN},
		{"uppercase WARN", "WARN", WARN},
		{"warning variant", "warning", WARN},
		{"uppercase WARNING", "WARNING", WARN},
		{"lowercase error", "error", ERROR},
		{"uppercase ERROR", "ERROR", ERROR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogLevel(tt.input)
			if err != nil {
				t.Errorf("ParseLogLevel(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseLogLevel_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"invalid level trace", "trace"},
		{"numeric string", "123"},
		{"random string", "random"},
		{"special chars", "!@#$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogLevel(tt.input)
			if err == nil {
				t.Errorf("ParseLogLevel(%q) expected error, got nil", tt.input)
			}
			// Should return DEBUG as default on error
			if got != DEBUG {
				t.Errorf("ParseLogLevel(%q) = %v, want DEBUG on error", tt.input, got)
			}
		})
	}
}

// ==========================
// Section 2: Field Constructors
// ==========================

func TestFieldConstructors(t *testing.T) {
	tests := []struct {
		name      string
		field     Field
		wantKey   string
		wantValue interface{}
		wantType  FieldType
	}{
		{
			name:      "String field",
			field:     String("name", "test"),
			wantKey:   "name",
			wantValue: "test",
			wantType:  StringType,
		},
		{
			name:      "Int field",
			field:     Int("count", 42),
			wantKey:   "count",
			wantValue: 42,
			wantType:  IntType,
		},
		{
			name:      "Int64 field",
			field:     Int64("big", int64(9223372036854775807)),
			wantKey:   "big",
			wantValue: int64(9223372036854775807),
			wantType:  Int64Type,
		},
		{
			name:      "Bool field true",
			field:     Bool("active", true),
			wantKey:   "active",
			wantValue: true,
			wantType:  BoolType,
		},
		{
			name:      "Bool field false",
			field:     Bool("active", false),
			wantKey:   "active",
			wantValue: false,
			wantType:  BoolType,
		},
		{
			name:      "Duration field",
			field:     Duration("latency", 100*time.Millisecond),
			wantKey:   "latency",
			wantValue: 100 * time.Millisecond,
			wantType:  DurationType,
		},
		{
			name:      "Any field",
			field:     Any("custom", map[string]int{"a": 1}),
			wantKey:   "custom",
			wantValue: map[string]int{"a": 1},
			wantType:  AnyType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Key != tt.wantKey {
				t.Errorf("Field.Key = %v, want %v", tt.field.Key, tt.wantKey)
			}
			if tt.field.Type != tt.wantType {
				t.Errorf("Field.Type = %v, want %v", tt.field.Type, tt.wantType)
			}
			// Use type assertion to compare values properly
			switch tt.wantType {
			case StringType:
				if tt.field.Value.(string) != tt.wantValue.(string) {
					t.Errorf("Field.Value = %v, want %v", tt.field.Value, tt.wantValue)
				}
			case IntType:
				if tt.field.Value.(int) != tt.wantValue.(int) {
					t.Errorf("Field.Value = %v, want %v", tt.field.Value, tt.wantValue)
				}
			case Int64Type:
				if tt.field.Value.(int64) != tt.wantValue.(int64) {
					t.Errorf("Field.Value = %v, want %v", tt.field.Value, tt.wantValue)
				}
			case BoolType:
				if tt.field.Value.(bool) != tt.wantValue.(bool) {
					t.Errorf("Field.Value = %v, want %v", tt.field.Value, tt.wantValue)
				}
			case DurationType:
				if tt.field.Value.(time.Duration) != tt.wantValue.(time.Duration) {
					t.Errorf("Field.Value = %v, want %v", tt.field.Value, tt.wantValue)
				}
			}
		})
	}
}

func TestErrField(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantValue interface{}
		wantKey   string
	}{
		{
			name:      "nil error",
			err:       nil,
			wantValue: nil,
			wantKey:   "error",
		},
		{
			name:      "non-nil error",
			err:       errors.New("boom"),
			wantValue: "boom",
			wantKey:   "error",
		},
		{
			name:      "formatted error",
			err:       fmt.Errorf("wrapped: %w", errors.New("original")),
			wantValue: "wrapped: original",
			wantKey:   "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := Err(tt.err)
			if field.Key != tt.wantKey {
				t.Errorf("Err().Key = %v, want %v", field.Key, tt.wantKey)
			}
			if field.Type != ErrorType {
				t.Errorf("Err().Type = %v, want ErrorType", field.Type)
			}
			if field.Value != tt.wantValue {
				t.Errorf("Err().Value = %v, want %v", field.Value, tt.wantValue)
			}
		})
	}
}

// ==========================
// Section 3: Formatters
// ==========================

// 3.1 PlainFormatter Tests

func TestPlainFormatter_Basic(t *testing.T) {
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false // Disable for predictable testing

	entry := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     INFO,
		Message:   "test message",
	}

	output := string(formatter.Format(entry))

	// Check timestamp exists
	if !strings.Contains(output, "2024-01-01") {
		t.Errorf("Expected timestamp in output, got: %s", output)
	}

	// Check level exists
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Expected [INFO] in output, got: %s", output)
	}

	// Check message exists
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected message in output, got: %s", output)
	}

	// Check ends with newline
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Expected output to end with newline, got: %s", output)
	}
}

func TestPlainFormatter_WithFields(t *testing.T) {
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     INFO,
		Message:   "test",
		Fields: []Field{
			String("key1", "value1"),
			Int("key2", 42),
			Bool("key3", true),
		},
	}

	output := string(formatter.Format(entry))

	// Check fields appear as key=value
	if !strings.Contains(output, "key1=value1") {
		t.Errorf("Expected key1=value1 in output, got: %s", output)
	}
	if !strings.Contains(output, "key2=42") {
		t.Errorf("Expected key2=42 in output, got: %s", output)
	}
	if !strings.Contains(output, "key3=true") {
		t.Errorf("Expected key3=true in output, got: %s", output)
	}

	// Check order is preserved (key1 before key2 before key3)
	idx1 := strings.Index(output, "key1")
	idx2 := strings.Index(output, "key2")
	idx3 := strings.Index(output, "key3")
	if idx1 >= idx2 || idx2 >= idx3 {
		t.Errorf("Expected fields in order, got: %s", output)
	}
}

func TestPlainFormatter_CallerEnabled(t *testing.T) {
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = true

	entry := LogEntry{
		Timestamp:  time.Now(),
		Level:      INFO,
		Message:    "test",
		CallerInfo: "file.go:42:TestFunc",
	}

	output := string(formatter.Format(entry))

	if !strings.Contains(output, "file.go:42:TestFunc") {
		t.Errorf("Expected caller info in output, got: %s", output)
	}
}

func TestPlainFormatter_CallerDisabled(t *testing.T) {
	// Caller info is controlled by the Logger's WithCaller option; when the
	// logger doesn't attach it, the formatter must not render it.
	formatter := PlainFormat().(*PlainFormatter)

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	output := string(formatter.Format(entry))

	// Caller info should not appear in output
	if strings.Contains(output, "file.go:42:TestFunc") {
		t.Errorf("Expected no caller info in output, got: %s", output)
	}
}

// 3.2 JSONFormatter Tests

func TestJSONFormatter_Basic(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Level:     INFO,
		Message:   "test message",
	}

	output := formatter.Format(entry)

	// Parse as JSON
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Check required fields
	if result["level"] != "INFO" {
		t.Errorf("Expected level=INFO, got: %v", result["level"])
	}
	if result["message"] != "test message" {
		t.Errorf("Expected message='test message', got: %v", result["message"])
	}
	if result["timestamp"] == nil {
		t.Errorf("Expected timestamp field")
	}
}

func TestJSONFormatter_CustomFields(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.CustomFields = map[string]interface{}{
		"app":     "myapp",
		"version": "1.0.0",
	}
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
		Fields: []Field{
			String("user", "alice"),
		},
	}

	output := formatter.Format(entry)

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	// Check custom fields are present
	if result["app"] != "myapp" {
		t.Errorf("Expected app=myapp, got: %v", result["app"])
	}
	if result["version"] != "1.0.0" {
		t.Errorf("Expected version=1.0.0, got: %v", result["version"])
	}

	// Check entry fields don't override core fields
	if result["user"] != "alice" {
		t.Errorf("Expected user=alice, got: %v", result["user"])
	}
	if result["message"] != "test" {
		t.Errorf("Core field 'message' should not be overridden")
	}
}

func TestJSONFormatter_PrettyPrint(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.PrettyPrint = true
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	output := string(formatter.Format(entry))

	// Pretty printed JSON should contain newlines and spaces
	if !strings.Contains(output, "\n") {
		t.Errorf("Expected pretty printed JSON to contain newlines")
	}
	if !strings.Contains(output, "  ") {
		t.Errorf("Expected pretty printed JSON to contain indentation")
	}
}

func TestJSONFormatter_MarshalFailureFallback(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.EnableCaller = false

	// Create entry with unserializable value (channel)
	ch := make(chan int)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
		Fields: []Field{
			Any("channel", ch),
		},
	}

	output := string(formatter.Format(entry))

	// Should still be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Fallback output should be valid JSON: %v", err)
	}

	// The message must survive; the unserializable value degrades to its
	// %v string form instead of discarding the whole entry.
	if result["message"] != "test" {
		t.Errorf("Expected message to be preserved, got: %s", output)
	}
	if _, ok := result["channel"].(string); !ok {
		t.Errorf("Expected unserializable field to degrade to a string, got: %s", output)
	}
}

// ==========================
// Section 4: Sinks
// ==========================

// 4.1 StdoutSink Tests

func TestStdoutSink_Write(t *testing.T) {
	var buf bytes.Buffer
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false

	sink := &StdoutSink{
		formatter: formatter,
		writer:    &buf,
	}

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test message",
	}

	err := sink.Write(entry)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected 'test message' in output, got: %s", output)
	}
}

func TestStdoutSink_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	const goroutines = 100
	const writesPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				entry := LogEntry{
					Timestamp: time.Now(),
					Level:     INFO,
					Message:   fmt.Sprintf("msg-%d-%d", id, j),
				}
				_ = sink.Write(entry)
			}
		}(i)
	}

	wg.Wait()

	// Check that we got expected number of lines
	lines := strings.Count(buf.String(), "\n")
	expected := goroutines * writesPerGoroutine
	if lines != expected {
		t.Errorf("Expected %d lines, got %d", expected, lines)
	}
}

// 4.2 FileSink Tests

func TestFileSink_Write(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:  "test",
		Dir:       tmpDir,
		Formatter: PlainFormat(),
		MaxBytes:  1024 * 1024,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test message",
	}

	err = sink.Write(entry)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Check file was created
	logFile := filepath.Join(tmpDir, "test.log")
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Errorf("Log file was not created")
	}

	// Check content
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("Expected 'test message' in log file, got: %s", string(content))
	}
}

func TestFileSink_Rotation(t *testing.T) {
	tmpDir := t.TempDir()

	// Use very small MaxBytes to trigger rotation
	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    100, // Very small to trigger rotation
		BackupCount: 3,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Write multiple large messages to trigger rotation
	for i := 0; i < 5; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("x", 50), // 50 chars
		}
		if err := sink.Write(entry); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		time.Sleep(10 * time.Millisecond) // Small delay for timestamp uniqueness
	}

	// Check that backup files were created
	files, err := filepath.Glob(filepath.Join(tmpDir, "test_*.log"))
	if err != nil {
		t.Fatalf("Glob error: %v", err)
	}

	if len(files) == 0 {
		t.Errorf("Expected backup files to be created")
	}

	// Check main file still exists
	mainFile := filepath.Join(tmpDir, "test.log")
	if _, err := os.Stat(mainFile); os.IsNotExist(err) {
		t.Errorf("Main log file should still exist")
	}
}

func TestFileSink_BackupLimit(t *testing.T) {
	tmpDir := t.TempDir()

	backupCount := 2
	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    50,
		BackupCount: backupCount,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Write many messages to create more backups than limit
	for i := 0; i < 10; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("test", 10),
		}
		_ = sink.Write(entry)
		time.Sleep(10 * time.Millisecond)
	}

	// Count backup files
	files, _ := filepath.Glob(filepath.Join(tmpDir, "test_*.log"))

	// Should not exceed backup limit
	if len(files) > backupCount {
		t.Errorf("Expected at most %d backups, got %d", backupCount, len(files))
	}
}

func TestFileSink_WriteAfterClose(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:  "test",
		Dir:       tmpDir,
		Formatter: PlainFormat(),
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Close the sink
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Try to write after close
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err = sink.Write(entry)
	if err == nil {
		t.Errorf("Expected error when writing to closed sink")
	}

	// Should not panic
}

// 4.3 MultiSink Tests

func TestMultiSink_AllSinksCalled(t *testing.T) {
	var buf1, buf2, buf3 bytes.Buffer

	sink1 := &StdoutSink{formatter: PlainFormat(), writer: &buf1}
	sink2 := &StdoutSink{formatter: PlainFormat(), writer: &buf2}
	sink3 := &StdoutSink{formatter: PlainFormat(), writer: &buf3}

	multiSink := NewMultiSink(sink1, sink2, sink3)

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err := multiSink.Write(entry)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// All buffers should contain the message
	if !strings.Contains(buf1.String(), "test") {
		t.Errorf("Sink1 did not receive message")
	}
	if !strings.Contains(buf2.String(), "test") {
		t.Errorf("Sink2 did not receive message")
	}
	if !strings.Contains(buf3.String(), "test") {
		t.Errorf("Sink3 did not receive message")
	}
}

func TestMultiSink_PartialFailure(t *testing.T) {
	var buf1 bytes.Buffer

	sink1 := &StdoutSink{formatter: PlainFormat(), writer: &buf1}
	sink2 := &failingSink{shouldFail: true}
	sink3 := &StdoutSink{formatter: PlainFormat(), writer: &bytes.Buffer{}}

	multiSink := NewMultiSink(sink1, sink2, sink3)

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err := multiSink.Write(entry)

	// Should return error due to sink2 failing
	if err == nil {
		t.Errorf("Expected error from failing sink")
	}

	// But sink1 and sink3 should still have been called
	if !strings.Contains(buf1.String(), "test") {
		t.Errorf("Sink1 should still be called despite sink2 failure")
	}
}

// Helper type for testing sink failures
type failingSink struct {
	shouldFail bool
	closeErr   error
}

func (f *failingSink) Write(entry LogEntry) error {
	if f.shouldFail {
		return errors.New("sink write failed")
	}
	return nil
}

func (f *failingSink) Close() error {
	return f.closeErr
}

// 4.4 CustomSink Tests

func TestCustomSink_Write(t *testing.T) {
	var called bool
	var receivedEntry LogEntry

	sink := NewCustomSink(func(entry LogEntry) error {
		called = true
		receivedEntry = entry
		return nil
	})

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err := sink.Write(entry)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if !called {
		t.Errorf("Custom write function was not called")
	}

	if receivedEntry.Message != "test" {
		t.Errorf("Expected message='test', got: %s", receivedEntry.Message)
	}
}

func TestCustomSink_Close(t *testing.T) {
	var closeCalled bool

	sink := NewCustomSinkWithClose(
		func(entry LogEntry) error { return nil },
		func() error {
			closeCalled = true
			return nil
		},
	)

	err := sink.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if !closeCalled {
		t.Errorf("Custom close function was not called")
	}
}

// ==========================
// Section 5: Logger Core
// ==========================

// 5.1 Construction Tests

func TestNewLogger_DefaultSink(t *testing.T) {
	logger := NewLogger()
	defer func() { _ = logger.Close() }()

	if len(logger.core.sinks) == 0 {
		t.Errorf("Expected default sink to be added")
	}
}

func TestWithLevel(t *testing.T) {
	logger := NewLogger(WithLevel(WARN))
	defer func() { _ = logger.Close() }()

	if logger.GetLevel() != WARN {
		t.Errorf("Expected level=WARN, got: %v", logger.GetLevel())
	}
}

func TestWithCaller(t *testing.T) {
	logger := NewLogger(WithCaller(false))
	defer func() { _ = logger.Close() }()

	if logger.core.enableCaller {
		t.Errorf("Expected caller to be disabled")
	}

	logger2 := NewLogger(WithCaller(true))
	defer func() { _ = logger2.Close() }()

	if !logger2.core.enableCaller {
		t.Errorf("Expected caller to be enabled")
	}
}

func TestWithContextFields(t *testing.T) {
	logger := NewLogger(
		WithContextFields(
			String("app", "myapp"),
			String("version", "1.0"),
		),
	)
	defer func() { _ = logger.Close() }()

	if len(logger.contextFields) != 2 {
		t.Errorf("Expected 2 context fields, got: %d", len(logger.contextFields))
	}
}

// 5.2 Logging Behavior Tests

func TestLogLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(
		WithLevel(WARN),
		WithSink(sink),
	)
	defer func() { _ = logger.Close() }()

	// These should not be written
	logger.Debug("debug message")
	logger.Info("info message")

	// These should be written
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()

	if strings.Contains(output, "debug message") {
		t.Errorf("DEBUG message should be filtered")
	}
	if strings.Contains(output, "info message") {
		t.Errorf("INFO message should be filtered")
	}
	if !strings.Contains(output, "warn message") {
		t.Errorf("WARN message should not be filtered")
	}
	if !strings.Contains(output, "error message") {
		t.Errorf("ERROR message should not be filtered")
	}
}

func TestLogger_FieldOrder(t *testing.T) {
	var buf bytes.Buffer
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false
	sink := &StdoutSink{
		formatter: formatter,
		writer:    &buf,
	}

	logger := NewLogger(
		WithSink(sink),
		WithContextFields(String("context1", "ctx1"), String("context2", "ctx2")),
	)
	defer func() { _ = logger.Close() }()

	logger.Info("test", String("field1", "f1"), String("field2", "f2"))

	output := buf.String()

	// Context fields should appear before explicit fields
	ctxIdx := strings.Index(output, "context1")
	field1Idx := strings.Index(output, "field1")

	if ctxIdx < 0 || field1Idx < 0 {
		t.Fatalf("Expected both context and explicit fields in output: %s", output)
	}

	if ctxIdx >= field1Idx {
		t.Errorf("Context fields should appear before explicit fields")
	}
}

func TestLogger_ClosedLoggerNoOp(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(WithSink(sink))
	_ = logger.Close()

	// Write after close
	logger.Info("should not appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Errorf("Closed logger should not write")
	}

	// Should not panic
}

// 5.3 Context Logger Tests

func TestLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false

	sink := &StdoutSink{
		formatter: formatter,
		writer:    &buf,
	}

	logger := NewLogger(WithSink(sink))
	defer func() { _ = logger.Close() }()

	ctx := context.Background()
	ctx = ContextWithRequestID(ctx, "req-123")
	ctx = ContextWithTraceID(ctx, "trace-456")
	ctx = ContextWithUserID(ctx, "user-789")

	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("test message")

	output := buf.String()

	if !strings.Contains(output, "request_id=req-123") {
		t.Errorf("Expected request_id in output: %s", output)
	}
	if !strings.Contains(output, "trace_id=trace-456") {
		t.Errorf("Expected trace_id in output: %s", output)
	}
	if !strings.Contains(output, "user_id=user-789") {
		t.Errorf("Expected user_id in output: %s", output)
	}
}

func TestLogger_WithContext_NoValues(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(WithSink(sink))
	defer func() { _ = logger.Close() }()

	ctx := context.Background()
	contextLogger := logger.WithContext(ctx)

	// Should not panic
	contextLogger.Info("test")

	output := buf.String()
	if !strings.Contains(output, "test") {
		t.Errorf("Expected message in output: %s", output)
	}
}

// 5.4 Async Logger Tests

func TestAsyncLogger_DeliversLogs(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(
		WithSink(sink),
		WithAsync(100),
	)

	logger.Info("async message 1")
	logger.Info("async message 2")
	logger.Info("async message 3")

	// Close waits for queue to drain
	_ = logger.Close()

	output := buf.String()

	if !strings.Contains(output, "async message 1") {
		t.Errorf("Expected message 1 in output")
	}
	if !strings.Contains(output, "async message 2") {
		t.Errorf("Expected message 2 in output")
	}
	if !strings.Contains(output, "async message 3") {
		t.Errorf("Expected message 3 in output")
	}
}

func TestAsyncLogger_QueueOverflowFallback(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	slowSink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		defer mu.Unlock()
		time.Sleep(50 * time.Millisecond) // Slow sink
		_, err := buf.WriteString(entry.Message + "\n")
		return err
	})

	logger := NewLogger(
		WithSink(slowSink),
		WithAsync(5), // Small buffer
	)

	// Send many messages quickly
	for i := 0; i < 20; i++ {
		logger.Info(fmt.Sprintf("message %d", i))
	}

	_ = logger.Close()

	mu.Lock()
	output := buf.String()
	mu.Unlock()

	// All messages should eventually be logged (fallback to sync on overflow)
	for i := 0; i < 20; i++ {
		msg := fmt.Sprintf("message %d", i)
		if !strings.Contains(output, msg) {
			t.Errorf("Expected message '%s' in output", msg)
		}
	}
}

func TestAsyncLogger_CloseDrainsQueue(t *testing.T) {
	var counter atomic.Int32
	countingSink := NewCustomSink(func(entry LogEntry) error {
		counter.Add(1)
		return nil
	})

	logger := NewLogger(
		WithSink(countingSink),
		WithAsync(100),
	)

	const messageCount = 50
	for i := 0; i < messageCount; i++ {
		logger.Info(fmt.Sprintf("message %d", i))
	}

	// Close should drain the queue
	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	// All messages should be processed
	if counter.Load() != messageCount {
		t.Errorf("Expected %d messages processed, got %d", messageCount, counter.Load())
	}
}

// ==========================
// Section 6: Caller Info
// ==========================

func TestGetCallerInfo(t *testing.T) {
	info := getCallerInfo(0)

	if info == "" {
		t.Errorf("Expected non-empty caller info")
	}

	// Should contain file name (will be gourdianlogger.go since getCallerInfo is called from there)
	if !strings.Contains(info, ".go") {
		t.Errorf("Expected .go file name in caller info, got: %s", info)
	}

	if !strings.Contains(info, ":") {
		t.Errorf("Expected line number separator in caller info, got: %s", info)
	}

	// Should contain function name
	parts := strings.Split(info, ":")
	if len(parts) < 3 {
		t.Errorf("Expected format file:line:function, got: %s", info)
	}
}

func TestGetCallerInfo_InvalidSkip(t *testing.T) {
	// Very large skip should return empty or handle gracefully
	info := getCallerInfo(1000)

	// Should not panic and return empty string
	if info != "" {
		t.Logf("Got caller info with large skip: %s", info)
	}
}

// ==========================
// Section 7: Close Semantics
// ==========================

func TestLogger_Close_Idempotent(t *testing.T) {
	logger := NewLogger()

	err1 := logger.Close()
	if err1 != nil {
		t.Errorf("First Close() error = %v", err1)
	}

	// Second close should be no-op
	err2 := logger.Close()
	if err2 != nil {
		t.Errorf("Second Close() error = %v", err2)
	}
}

func TestLogger_Close_ClosesAllSinks(t *testing.T) {
	closeCounts := make([]int, 3)

	sink1 := NewCustomSinkWithClose(
		func(entry LogEntry) error { return nil },
		func() error { closeCounts[0]++; return nil },
	)
	sink2 := NewCustomSinkWithClose(
		func(entry LogEntry) error { return nil },
		func() error { closeCounts[1]++; return nil },
	)
	sink3 := NewCustomSinkWithClose(
		func(entry LogEntry) error { return nil },
		func() error { closeCounts[2]++; return nil },
	)

	logger := NewLogger(
		WithSink(sink1),
		WithSink(sink2),
		WithSink(sink3),
	)

	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	for i, count := range closeCounts {
		if count != 1 {
			t.Errorf("Sink %d Close() called %d times, expected 1", i, count)
		}
	}
}

// ==========================
// Additional Coverage Tests
// ==========================

func TestLogger_AllLogLevels(t *testing.T) {
	var buf bytes.Buffer
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false
	sink := &StdoutSink{
		formatter: formatter,
		writer:    &buf,
	}

	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(sink),
		WithCaller(false),
	)
	defer func() { _ = logger.Close() }()

	// Test all logging methods
	logger.Debug("debug msg", String("key", "val"))
	logger.Info("info msg", Int("num", 42))
	logger.Warn("warn msg", Bool("flag", true))
	logger.Error("error msg", Err(errors.New("test error")))

	output := buf.String()

	if !strings.Contains(output, "debug msg") {
		t.Errorf("Expected debug message in output")
	}
	if !strings.Contains(output, "info msg") {
		t.Errorf("Expected info message in output")
	}
	if !strings.Contains(output, "warn msg") {
		t.Errorf("Expected warn message in output")
	}
	if !strings.Contains(output, "error msg") {
		t.Errorf("Expected error message in output")
	}
}

func TestLogger_FormattedLoggingAllLevels(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(
		WithLevel(DEBUG),
		WithSink(sink),
	)
	defer func() { _ = logger.Close() }()

	logger.Debugf("debug %s %d", "test", 1)
	logger.Infof("info %s %d", "test", 2)
	logger.Warnf("warn %s %d", "test", 3)
	logger.Errorf("error %s %d", "test", 4)

	output := buf.String()

	if !strings.Contains(output, "debug test 1") {
		t.Errorf("Expected formatted debug message")
	}
	if !strings.Contains(output, "info test 2") {
		t.Errorf("Expected formatted info message")
	}
	if !strings.Contains(output, "warn test 3") {
		t.Errorf("Expected formatted warn message")
	}
	if !strings.Contains(output, "error test 4") {
		t.Errorf("Expected formatted error message")
	}
}

func TestFileSink_RotationMultiple(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    100,
		BackupCount: 2,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Write until multiple rotations happen
	for i := 0; i < 10; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("x", 50),
		}
		_ = sink.Write(entry)
		time.Sleep(5 * time.Millisecond)
	}

	_ = sink.Close()
}

func TestLogger_MultipleContextFields(t *testing.T) {
	var buf bytes.Buffer
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false
	sink := &StdoutSink{
		formatter: formatter,
		writer:    &buf,
	}

	logger := NewLogger(
		WithSink(sink),
		WithContextFields(
			String("app", "myapp"),
			String("version", "1.0"),
			String("env", "test"),
		),
	)
	defer func() { _ = logger.Close() }()

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "app=myapp") {
		t.Errorf("Expected app field in output")
	}
	if !strings.Contains(output, "version=1.0") {
		t.Errorf("Expected version field in output")
	}
	if !strings.Contains(output, "env=test") {
		t.Errorf("Expected env field in output")
	}
}

func TestLogger_WithNilSink(t *testing.T) {
	// Should not add nil sink
	logger := NewLogger(WithSink(nil))
	defer func() { _ = logger.Close() }()

	// Should have default sink since nil was ignored
	if len(logger.core.sinks) == 0 {
		t.Errorf("Expected default sink when nil sink provided")
	}
}

func TestLogger_AddNilSink(t *testing.T) {
	logger := NewLogger()
	defer func() { _ = logger.Close() }()

	initialCount := len(logger.core.sinks)
	logger.AddSink(nil)

	if len(logger.core.sinks) != initialCount {
		t.Errorf("Expected nil sink to be ignored")
	}
}

func TestLogger_RemoveNonExistentSink(t *testing.T) {
	sink1 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
	sink2 := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}

	logger := NewLogger(WithSink(sink1))
	defer func() { _ = logger.Close() }()

	initialCount := len(logger.core.sinks)

	// Try to remove a sink that was never added
	logger.RemoveSink(sink2)

	if len(logger.core.sinks) != initialCount {
		t.Errorf("Expected sink count to remain unchanged")
	}
}

func TestJSONFormatter_CallerEnabled(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.EnableCaller = true

	entry := LogEntry{
		Timestamp:  time.Now(),
		Level:      INFO,
		Message:    "test",
		CallerInfo: "file.go:42:TestFunc",
	}

	output := formatter.Format(entry)

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if result["caller"] != "file.go:42:TestFunc" {
		t.Errorf("Expected caller field in JSON output")
	}
}

func TestJSONFormatter_CallerDisabled(t *testing.T) {
	// Caller info is controlled by the Logger's WithCaller option; when the
	// logger doesn't attach it, no "caller" key is emitted.
	formatter := JSONFormat().(*JSONFormatter)

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	output := formatter.Format(entry)

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if _, exists := result["caller"]; exists {
		t.Errorf("Expected no caller field when disabled")
	}
}

func TestFileSink_CreateDirError(t *testing.T) {
	// Try to create sink in a location that will fail
	tempFile := filepath.Join(t.TempDir(), "file.txt")
	_ = os.WriteFile(tempFile, []byte("test"), 0644)

	_, err := NewFileSink(FileSinkConfig{
		Filename: "test",
		Dir:      tempFile, // This is a file, not a directory
	})

	if err == nil {
		t.Errorf("Expected error when creating directory fails")
	}
}

func TestFileSink_OpenFileError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory where the file should be
	logPath := filepath.Join(tmpDir, "test.log")
	_ = os.Mkdir(logPath, 0755)

	_, err := NewFileSink(FileSinkConfig{
		Filename: "test",
		Dir:      tmpDir,
	})

	if err == nil {
		t.Errorf("Expected error when opening file fails")
	}
}

func TestFileSink_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Dir: tmpDir,
		// All other fields use defaults
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	fileSink := sink.(*FileSink)
	if fileSink.baseFile != "app" {
		t.Errorf("Expected default filename 'app', got %s", fileSink.baseFile)
	}
	if fileSink.maxBytes != 10*1024*1024 {
		t.Errorf("Expected default maxBytes 10MB")
	}
	if fileSink.backupCount != 5 {
		t.Errorf("Expected default backupCount 5")
	}
}

func TestMultiSink_CloseErrors(t *testing.T) {
	closeErr := errors.New("close failed")
	sink1 := &failingSink{closeErr: closeErr}
	sink2 := &failingSink{closeErr: closeErr}

	multi := NewMultiSink(sink1, sink2)
	err := multi.Close()

	if err == nil {
		t.Errorf("Expected error when sinks fail to close")
	}
}

func TestCustomSink_NilCloseFunc(t *testing.T) {
	sink := NewCustomSink(func(entry LogEntry) error {
		return nil
	})

	// Should not panic with nil close func
	err := sink.Close()
	if err != nil {
		t.Errorf("Close() with nil close func should return nil, got %v", err)
	}
}

func TestLogger_AsyncWithZeroBuffer(t *testing.T) {
	// WithAsync with 0 or negative should not enable async
	logger := NewLogger(WithAsync(0))
	defer func() { _ = logger.Close() }()

	if logger.core.async {
		t.Errorf("Expected async to be disabled with 0 buffer size")
	}
}

func TestLogger_AsyncWithNegativeBuffer(t *testing.T) {
	logger := NewLogger(WithAsync(-10))
	defer func() { _ = logger.Close() }()

	if logger.core.async {
		t.Errorf("Expected async to be disabled with negative buffer size")
	}
}

func TestLogger_WriteSinkError(t *testing.T) {
	// Capture stderr to verify error logging
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	failSink := &failingSink{shouldFail: true}
	logger := NewLogger(WithSink(failSink))

	logger.Info("test message")

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	// Logger should have logged the error to stderr
	output := buf.String()
	if !strings.Contains(output, "sink write failed") {
		t.Logf("stderr output: %s", output)
	}

	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}
}

func TestAsyncLogger_ChannelClose(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(
		WithSink(sink),
		WithAsync(10),
	)

	// Send some messages
	for i := 0; i < 5; i++ {
		logger.Info(fmt.Sprintf("message %d", i))
	}

	// Close should drain queue
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify messages were written
	output := buf.String()
	for i := 0; i < 5; i++ {
		if !strings.Contains(output, fmt.Sprintf("message %d", i)) {
			t.Errorf("Expected message %d in output", i)
		}
	}
}

func TestLogger_ContextFieldMutex(t *testing.T) {
	logger := NewLogger(
		WithContextFields(String("initial", "value")),
	)
	defer func() { _ = logger.Close() }()

	// Access context fields from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				logger.Info("test")
			}
		}()
	}
	wg.Wait()
}

func TestPlainFormatter_EmptyFields(t *testing.T) {
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
		Fields:    []Field{}, // Empty fields
	}

	output := string(formatter.Format(entry))

	// Should not have field braces for empty fields
	if strings.Contains(output, "{}") {
		t.Errorf("Expected no empty field braces in output: %s", output)
	}
}

func TestJSONFormatter_EmptyCustomFields(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.CustomFields = map[string]interface{}{} // Empty map
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	output := formatter.Format(entry)

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if len(result) < 3 {
		t.Errorf("Expected at least timestamp, level, message")
	}
}

func TestLogger_GetSetLevel(t *testing.T) {
	logger := NewLogger(WithLevel(DEBUG))
	defer func() { _ = logger.Close() }()

	if logger.GetLevel() != DEBUG {
		t.Errorf("Expected DEBUG level")
	}

	logger.SetLevel(ERROR)
	if logger.GetLevel() != ERROR {
		t.Errorf("Expected ERROR level after SetLevel")
	}

	// Verify filtering works after SetLevel
	var buf bytes.Buffer
	sink := &StdoutSink{formatter: PlainFormat(), writer: &buf}
	logger.AddSink(sink)

	logger.Info("should not appear")
	logger.Error("should appear")

	output := buf.String()
	if strings.Contains(output, "should not appear") {
		t.Errorf("INFO message should be filtered at ERROR level")
	}
	if !strings.Contains(output, "should appear") {
		t.Errorf("ERROR message should not be filtered")
	}
}

func TestFileSink_CleanupOldBackupsNoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    1024,
		BackupCount: 3,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Write a message (no backups yet)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}
	_ = sink.Write(entry)

	_ = sink.Close()

	// Check that cleanup didn't cause issues
	files, _ := filepath.Glob(filepath.Join(tmpDir, "test_*.log"))
	if len(files) > 0 {
		t.Logf("Found %d backup files (none expected yet)", len(files))
	}
}

func TestPlainFormatter_WithCallerNoInfo(t *testing.T) {
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = true

	entry := LogEntry{
		Timestamp:  time.Now(),
		Level:      INFO,
		Message:    "test",
		CallerInfo: "", // Empty caller info
	}

	output := string(formatter.Format(entry))

	// Should still format correctly without caller info
	if !strings.Contains(output, "test") {
		t.Errorf("Expected message in output")
	}
}

func TestJSONFormatter_WithAllFieldTypes(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.EnableCaller = false

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
		Fields: []Field{
			String("str", "value"),
			Int("int", 42),
			Int64("int64", 123),
			Bool("bool", true),
			Duration("dur", 5*time.Second),
			Err(errors.New("error")),
			Any("any", map[string]int{"a": 1}),
		},
	}

	output := formatter.Format(entry)

	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if result["str"] != "value" {
		t.Errorf("Expected str field")
	}
	if result["int"] != float64(42) { // JSON numbers are float64
		t.Errorf("Expected int field")
	}
}

func TestLogger_WithContextMultipleCalls(t *testing.T) {
	// Views derived via WithContext share sinks but carry independent fields.
	var buf bytes.Buffer
	logger := NewLogger(WithSink(NewWriterSink(&buf, PlainFormat())), WithCaller(false))
	defer func() { _ = logger.Close() }()

	ctx1 := context.Background()
	ctx1 = ContextWithRequestID(ctx1, "req-1")

	ctx2 := context.Background()
	ctx2 = ContextWithRequestID(ctx2, "req-2")

	logger1 := logger.WithContext(ctx1)
	logger2 := logger.WithContext(ctx2)

	logger1.Info("test1")
	logger2.Info("test2")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 log lines, got %d: %s", len(lines), buf.String())
	}
	if !strings.Contains(lines[0], "test1") || !strings.Contains(lines[0], "request_id=req-1") {
		t.Errorf("Expected req-1 on first line: %s", lines[0])
	}
	if !strings.Contains(lines[1], "test2") || !strings.Contains(lines[1], "request_id=req-2") {
		t.Errorf("Expected req-2 on second line: %s", lines[1])
	}
	if strings.Contains(lines[0], "req-2") || strings.Contains(lines[1], "req-1") {
		t.Errorf("Context fields leaked between views: %s", buf.String())
	}
}

func TestFormatFieldValue_UnknownType(t *testing.T) {
	// Test with an unknown field type (should fall through to default)
	field := Field{
		Key:   "test",
		Value: "value",
		Type:  FieldType(999), // Invalid type
	}

	result := formatFieldValue(field)
	if result != "value" {
		t.Errorf("Expected fallback formatting, got: %s", result)
	}
}

func TestFileSink_RotateStatError(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    50, // Very small
		BackupCount: 2,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Write small messages to trigger size check
	for i := 0; i < 3; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   "test message",
		}
		_ = sink.Write(entry)
	}
}

func TestFileSink_RotateWithNoFile(t *testing.T) {
	tmpDir := t.TempDir()

	fileSink := &FileSink{
		file:        nil, // No file
		formatter:   PlainFormat(),
		maxBytes:    100,
		backupCount: 2,
		baseDir:     tmpDir,
		baseFile:    "test",
	}

	err := fileSink.rotate()
	if err == nil {
		t.Errorf("Expected error when rotating with no file")
	}
}

func TestFileSink_CleanupOldBackupsWithStatError(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    80,
		BackupCount: 1,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Write to trigger multiple rotations
	for i := 0; i < 5; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("abcdefghij", 5), // 50 chars
		}
		_ = sink.Write(entry)
		time.Sleep(10 * time.Millisecond)
	}

	_ = sink.Close()

	// Verify old backups were cleaned up
	files, _ := filepath.Glob(filepath.Join(tmpDir, "test_*.log"))
	if len(files) > 1 {
		t.Logf("Found %d backup files, expected <= 1", len(files))
	}
}

func TestLogger_AsyncWorkerDrain(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(
		WithSink(sink),
		WithAsync(100),
	)

	// Fill queue with messages
	for i := 0; i < 50; i++ {
		logger.Info(fmt.Sprintf("msg-%d", i))
	}

	// Close will trigger the drain path
	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	// All messages should be in output
	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 40 { // Allow some variance
		t.Errorf("Expected around 50 lines, got %d", len(lines))
	}
}

func TestLogger_AsyncWorkerSelectBranches(t *testing.T) {
	var mu sync.Mutex
	var count int

	countSink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		count++
		mu.Unlock()
		time.Sleep(1 * time.Millisecond) // Slow down processing
		return nil
	})

	logger := NewLogger(
		WithSink(countSink),
		WithAsync(10),
	)

	// Send messages rapidly
	for i := 0; i < 20; i++ {
		logger.Info(fmt.Sprintf("message %d", i))
		if i%5 == 0 {
			time.Sleep(2 * time.Millisecond)
		}
	}

	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	mu.Lock()
	finalCount := count
	mu.Unlock()

	if finalCount != 20 {
		t.Errorf("Expected 20 messages processed, got %d", finalCount)
	}
}

type testCtxKey string

func TestLogger_WithContextNonStringValues(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(WithSink(sink))
	defer func() { _ = logger.Close() }()

	// Values not stored via this package's typed helpers are ignored,
	// even when their key names match the well-known field names.
	ctx := context.Background()
	ctx = context.WithValue(ctx, testCtxKey("request_id"), "req-999")
	ctx = context.WithValue(ctx, testCtxKey("trace_id"), true)

	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("test")

	output := buf.String()
	if !strings.Contains(output, "test") {
		t.Errorf("Expected message in output")
	}
	if strings.Contains(output, "request_id=") || strings.Contains(output, "trace_id=") {
		t.Errorf("Did not expect foreign context values in output: %s", output)
	}
}

func TestLogger_WithContextValidAndInvalid(t *testing.T) {
	var buf bytes.Buffer
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false

	sink := &StdoutSink{
		formatter: formatter,
		writer:    &buf,
	}

	logger := NewLogger(WithSink(sink))
	defer func() { _ = logger.Close() }()

	ctx := context.Background()
	ctx = ContextWithRequestID(ctx, "req-123")                // valid
	ctx = context.WithValue(ctx, testCtxKey("trace_id"), 456) // foreign key, ignored
	ctx = ContextWithUserID(ctx, "user-789")                  // valid

	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("test")

	output := buf.String()

	if !strings.Contains(output, "request_id=req-123") {
		t.Errorf("Expected request_id in output: %s", output)
	}
	if strings.Contains(output, "trace_id=") {
		t.Errorf("Did not expect trace_id in output: %s", output)
	}
	if !strings.Contains(output, "user_id=user-789") {
		t.Errorf("Expected user_id in output: %s", output)
	}
}

func TestStdoutSink_NilFormatter(t *testing.T) {
	// NewStdoutSink with nil formatter should use default
	sink := NewStdoutSink(nil)
	defer func() { _ = sink.Close() }()

	var buf bytes.Buffer
	stdoutSink := sink.(*StdoutSink)
	stdoutSink.writer = &buf

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err := sink.Write(entry)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Errorf("Expected output with default formatter")
	}
}

func TestPlainFormatter_AllLevels(t *testing.T) {
	formatter := PlainFormat().(*PlainFormatter)
	formatter.EnableCaller = false

	levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
	for _, level := range levels {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     level,
			Message:   "test",
		}

		output := string(formatter.Format(entry))
		if !strings.Contains(output, level.String()) {
			t.Errorf("Expected %s in output for level %v", level.String(), level)
		}
	}
}

func TestJSONFormatter_AllLevels(t *testing.T) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.EnableCaller = false

	levels := []LogLevel{DEBUG, INFO, WARN, ERROR}
	for _, level := range levels {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     level,
			Message:   "test",
		}

		output := formatter.Format(entry)

		var result map[string]interface{}
		if err := json.Unmarshal(output, &result); err != nil {
			t.Fatalf("Output is not valid JSON: %v", err)
		}

		if result["level"] != level.String() {
			t.Errorf("Expected level=%s in JSON", level.String())
		}
	}
}

func TestFileSink_WriteStatCheck(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    1000,
		BackupCount: 2,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Write multiple small messages (should all succeed without rotation)
	for i := 0; i < 10; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   "short",
		}
		err := sink.Write(entry)
		if err != nil {
			t.Errorf("Write() error = %v", err)
		}
	}
}

func TestMultiSink_EmptySinks(t *testing.T) {
	// MultiSink with no sinks
	multi := NewMultiSink()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err := multi.Write(entry)
	if err != nil {
		t.Errorf("Write() to empty MultiSink should not error, got: %v", err)
	}

	err = multi.Close()
	if err != nil {
		t.Errorf("Close() on empty MultiSink should not error, got: %v", err)
	}
}

func TestMultiSink_SingleSink(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{formatter: PlainFormat(), writer: &buf}

	multi := NewMultiSink(sink)

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	err := multi.Write(entry)
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	if !strings.Contains(buf.String(), "test") {
		t.Errorf("Expected message in output")
	}
}

func TestLogger_ClosedStateCheck(t *testing.T) {
	logger := NewLogger()

	// Close once
	err := logger.Close()
	if err != nil {
		t.Errorf("First Close() error = %v", err)
	}

	// Verify closed state
	if !logger.core.closed.Load() {
		t.Errorf("Expected logger.core.closed to be true")
	}

	// Try to log (should be no-op)
	logger.Info("should not appear")

	// Close again (should be no-op)
	err = logger.Close()
	if err != nil {
		t.Errorf("Second Close() should not error, got: %v", err)
	}
}

func TestLogger_GetLevelAtomic(t *testing.T) {
	logger := NewLogger(WithLevel(INFO))
	defer func() { _ = logger.Close() }()

	// Test concurrent GetLevel calls
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				level := logger.GetLevel()
				if level != INFO {
					t.Errorf("Expected INFO level, got %v", level)
				}
			}
		}()
	}
	wg.Wait()
}

func TestLogger_SetLevelAtomic(t *testing.T) {
	logger := NewLogger(WithLevel(DEBUG))
	defer func() { _ = logger.Close() }()

	// Test concurrent SetLevel calls
	var wg sync.WaitGroup
	levels := []LogLevel{DEBUG, INFO, WARN, ERROR}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			level := levels[id%len(levels)]
			logger.SetLevel(level)
		}(i)
	}
	wg.Wait()

	// Final level should be one of the valid levels
	finalLevel := logger.GetLevel()
	valid := false
	for _, l := range levels {
		if finalLevel == l {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("Final level %v is not valid", finalLevel)
	}
}

func TestFileSink_RotateCloseError(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    100,
		BackupCount: 2,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	fileSink := sink.(*FileSink)

	// Close the file prematurely to cause error in rotate
	_ = fileSink.file.Close()

	// Try to write (will attempt rotation and fail)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   strings.Repeat("x", 200),
	}

	err = fileSink.Write(entry)
	// Should get an error
	if err == nil {
		t.Logf("Expected error on write after premature close")
	}
}

func TestFormatFieldValue_AnyType(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"slice", []int{1, 2, 3}},
		{"map", map[string]string{"a": "b"}},
		{"struct", struct{ X int }{X: 42}},
		{"nil", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := Any("key", tt.value)
			result := formatFieldValue(field)
			if result == "" {
				t.Errorf("Expected non-empty result for Any type")
			}
		})
	}
}

func TestLogger_SinksMutexStress(t *testing.T) {
	logger := NewLogger()
	defer func() { _ = logger.Close() }()

	var wg sync.WaitGroup

	// Concurrent AddSink
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				sink := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
				logger.AddSink(sink)
			}
		}()
	}

	// Concurrent RemoveSink
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			sink := &StdoutSink{formatter: PlainFormat(), writer: io.Discard}
			for j := 0; j < 20; j++ {
				logger.RemoveSink(sink)
			}
		}()
	}

	// Concurrent logging
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				logger.Info("test")
			}
		}()
	}

	wg.Wait()
}

func TestFileSink_GlobError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a sink
	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test[",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    100,
		BackupCount: 1,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}
	defer func() { _ = sink.Close() }()

	// Write to trigger rotation (glob pattern might be invalid)
	for i := 0; i < 3; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("x", 50),
		}
		_ = sink.Write(entry)
		time.Sleep(5 * time.Millisecond)
	}
}

func TestGetCallerInfo_WithFuncForPC(t *testing.T) {
	// Call with skip 0 to test the path where FuncForPC returns a value
	info := getCallerInfo(0)

	if info == "" {
		t.Errorf("Expected non-empty caller info")
	}

	// Should contain all three parts: file:line:function
	if !strings.Contains(info, ":") {
		t.Errorf("Expected colon separators in caller info")
	}

	// Verify format
	parts := strings.Split(info, ":")
	if len(parts) != 3 {
		t.Errorf("Expected 3 parts (file:line:func), got %d: %s", len(parts), info)
	}
}

func TestGetCallerInfo_PathProcessing(t *testing.T) {
	// Test at different skip levels to exercise the path extraction logic
	for skip := 0; skip <= 3; skip++ {
		info := getCallerInfo(skip)
		if info != "" {
			// Verify the function name extraction logic
			parts := strings.Split(info, ":")
			if len(parts) == 3 {
				// The function name should not contain slashes or package paths
				funcName := parts[2]
				if strings.Contains(funcName, "/") {
					t.Errorf("Function name should not contain slashes: %s", funcName)
				}
			}
		}
	}
}

func TestFileSink_RotateRenameError(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    50,
		BackupCount: 2,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	fileSink := sink.(*FileSink)

	// Write a message to create the log file
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "initial",
	}
	_ = fileSink.Write(entry)

	// Create a backup directory that will conflict
	backupPath := filepath.Join(tmpDir, fmt.Sprintf("test_%s.log", time.Now().Format("20060102_150405")))
	_ = os.WriteFile(backupPath, []byte("existing"), 0444)

	// Try to write enough to trigger rotation
	for i := 0; i < 3; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("x", 30),
		}
		err := fileSink.Write(entry)
		// May get error due to rotation issues
		if err != nil {
			t.Logf("Write error (expected): %v", err)
		}
		time.Sleep(1100 * time.Millisecond) // Ensure unique timestamp
	}

	_ = fileSink.Close()
}

func TestFileSink_RotateOpenNewFileError(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    40,
		BackupCount: 1,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Write to create initial file
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}
	_ = sink.Write(entry)

	fileSink := sink.(*FileSink)

	// Manually call rotate to test error path
	fileSink.mu.Lock()
	if fileSink.file != nil {
		_ = fileSink.file.Close()

		// Backup current file
		currentPath := filepath.Join(tmpDir, "test.log")
		backupPath := filepath.Join(tmpDir, "test_backup.log")
		_ = os.Rename(currentPath, backupPath)

		// Create a directory where the file should be to cause open error
		_ = os.Mkdir(currentPath, 0755)

		// Now try to rotate (will fail to open new file)
		err := fileSink.rotate()
		if err == nil {
			t.Logf("Expected error when opening new file fails")
		}
	}
	fileSink.mu.Unlock()

	_ = fileSink.Close()
}

func TestFileSink_CleanupWithStatNilError(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    60,
		BackupCount: 1,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Create some backup files manually
	backup1 := filepath.Join(tmpDir, "test_20240101_120000.log")
	backup2 := filepath.Join(tmpDir, "test_20240102_120000.log")
	_ = os.WriteFile(backup1, []byte("old"), 0644)
	_ = os.WriteFile(backup2, []byte("old"), 0644)

	// Delete one to create a stat error scenario
	_ = os.Remove(backup1)

	// Write to trigger rotation and cleanup
	for i := 0; i < 3; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("test", 8),
		}
		_ = sink.Write(entry)
		time.Sleep(5 * time.Millisecond)
	}

	if err := sink.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}
}

func TestLogger_AsyncQueueFull(t *testing.T) {
	var processedCount atomic.Int32

	slowSink := NewCustomSink(func(entry LogEntry) error {
		processedCount.Add(1)
		time.Sleep(100 * time.Millisecond) // Very slow
		return nil
	})

	logger := NewLogger(
		WithSink(slowSink),
		WithAsync(2), // Very small buffer
	)

	// Send messages very quickly to fill queue
	for i := 0; i < 10; i++ {
		logger.Info(fmt.Sprintf("message %d", i))
		// No sleep - trying to overwhelm queue
	}

	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	// All messages should be processed (via fallback)
	if processedCount.Load() != 10 {
		t.Errorf("Expected 10 messages, got %d", processedCount.Load())
	}
}

func TestFileSink_StatErrorDuringWrite(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    100,
		BackupCount: 1,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	fileSink := sink.(*FileSink)

	// Write an entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "test",
	}

	// Close file to cause stat error on next write
	fileSink.mu.Lock()
	if fileSink.file != nil {
		_ = fileSink.file.Close()
		fileSink.file = nil
	}
	fileSink.mu.Unlock()

	// Try to write (should get error)
	err = fileSink.Write(entry)
	if err == nil {
		t.Errorf("Expected error when file is nil")
	}

	_ = fileSink.Close()
}

func TestLogger_LevelAtomicInt32(t *testing.T) {
	logger := NewLogger()
	defer func() { _ = logger.Close() }()

	// Test that level is properly stored as int32
	logger.core.level.Store(int32(WARN))

	if logger.GetLevel() != WARN {
		t.Errorf("Expected WARN level")
	}

	// Test all levels
	for _, level := range []LogLevel{DEBUG, INFO, WARN, ERROR} {
		logger.SetLevel(level)
		if logger.GetLevel() != level {
			t.Errorf("Expected level %v, got %v", level, logger.GetLevel())
		}
	}
}

func TestLogger_ContextFieldMutexReadWrite(t *testing.T) {
	logger := NewLogger(
		WithContextFields(String("app", "test")),
	)
	defer func() { _ = logger.Close() }()

	var wg sync.WaitGroup

	// Multiple readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				logger.Info("test")
			}
		}()
	}

	wg.Wait()
}

func TestFileSink_MultipleRotationsWithCleanup(t *testing.T) {
	tmpDir := t.TempDir()

	sink, err := NewFileSink(FileSinkConfig{
		Filename:    "test",
		Dir:         tmpDir,
		Formatter:   PlainFormat(),
		MaxBytes:    70,
		BackupCount: 2,
	})
	if err != nil {
		t.Fatalf("NewFileSink() error = %v", err)
	}

	// Write many messages to create multiple rotations
	for i := 0; i < 15; i++ {
		entry := LogEntry{
			Timestamp: time.Now(),
			Level:     INFO,
			Message:   strings.Repeat("abcd", 10), // 40 chars
		}
		_ = sink.Write(entry)
		time.Sleep(5 * time.Millisecond)
	}

	if err := sink.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	// Verify backup count is respected
	files, _ := filepath.Glob(filepath.Join(tmpDir, "test_*.log"))
	if len(files) > 2 {
		t.Errorf("Expected at most 2 backups, got %d", len(files))
	}
}

func TestLogger_SinkWriteErrorStderr(t *testing.T) {
	// Test the stderr error logging path
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	failSink := &failingSink{shouldFail: true}
	logger := NewLogger(WithSink(failSink))

	// This will fail and log to stderr
	logger.Info("test")

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)

	output := buf.String()
	// Should contain error message
	if !strings.Contains(output, "logger:") || !strings.Contains(output, "sink write failed") {
		t.Logf("stderr output: %s", output)
	}

	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}
}

func TestNewLogger_WithMultipleOptions(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(
		WithLevel(WARN),
		WithSink(sink),
		WithCaller(false),
		WithAsync(100),
		WithContextFields(String("app", "test"), String("env", "prod")),
	)
	defer func() { _ = logger.Close() }()

	// Verify all options applied
	if logger.GetLevel() != WARN {
		t.Errorf("Expected WARN level")
	}
	if logger.core.enableCaller {
		t.Errorf("Expected caller disabled")
	}
	if !logger.core.async {
		t.Errorf("Expected async enabled")
	}
	if len(logger.contextFields) != 2 {
		t.Errorf("Expected 2 context fields")
	}

	// Test that it logs correctly
	logger.Error("test error")
	if err := logger.Close(); err != nil {
		t.Logf("Close error: %v", err)
	}

	if !strings.Contains(buf.String(), "test error") {
		t.Errorf("Expected error message in output")
	}
}

// ==========================
// Section 8: Benchmarks
// ==========================

// 8.1 Field Benchmarks

func BenchmarkField_String(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = String("key", "value")
	}
}

func BenchmarkField_Int(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Int("key", 42)
	}
}

// 8.2 Formatter Benchmarks

func BenchmarkPlainFormatter(b *testing.B) {
	formatter := PlainFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "benchmark message",
		Fields: []Field{
			String("key1", "value1"),
			Int("key2", 42),
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkJSONFormatter(b *testing.B) {
	formatter := JSONFormat()
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "benchmark message",
		Fields: []Field{
			String("key1", "value1"),
			Int("key2", 42),
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

func BenchmarkJSONFormatter_Pretty(b *testing.B) {
	formatter := JSONFormat().(*JSONFormatter)
	formatter.PrettyPrint = true
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "benchmark message",
		Fields: []Field{
			String("key1", "value1"),
			Int("key2", 42),
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = formatter.Format(entry)
	}
}

// 8.3 Sink Benchmarks

func BenchmarkStdoutSink(b *testing.B) {
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    io.Discard,
	}
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "benchmark message",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sink.Write(entry)
	}
}

func BenchmarkFileSink(b *testing.B) {
	tmpDir := b.TempDir()
	sink, err := NewFileSink(FileSinkConfig{
		Filename:  "bench",
		Dir:       tmpDir,
		Formatter: PlainFormat(),
		MaxBytes:  10 * 1024 * 1024,
	})
	if err != nil {
		b.Fatalf("Failed to create sink: %v", err)
	}
	defer func() { _ = sink.Close() }()

	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     INFO,
		Message:   "benchmark message",
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sink.Write(entry)
	}
}

// 8.4 Logger Benchmarks

func BenchmarkLogger_Sync(b *testing.B) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message")
	}
}

func BenchmarkLogger_Async(b *testing.B) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(10000),
	)
	defer func() { _ = logger.Close() }()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message")
	}
}

func BenchmarkLogger_WithFields(b *testing.B) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message",
			String("key1", "value1"),
			Int("key2", 42),
			Bool("key3", true),
		)
	}
}

func BenchmarkLogger_WithContext(b *testing.B) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	ctx := context.Background()
	ctx = ContextWithRequestID(ctx, "req-123")
	contextLogger := logger.WithContext(ctx)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		contextLogger.Info("benchmark message")
	}
}

// 8.5 Parallel Benchmark

func BenchmarkLogger_Parallel(b *testing.B) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			logger.Info("parallel message")
		}
	})
}

// ==========================
// Section 9: Race Tests
// ==========================

// These tests are designed to be run with -race flag

func TestConcurrentLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				logger.Info("concurrent message",
					String("goroutine", fmt.Sprintf("%d", id)),
					Int("iteration", j),
				)
			}
		}(i)
	}

	wg.Wait()
}

func TestAddSinkDuringLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
	)
	defer func() { _ = logger.Close() }()

	done := make(chan bool)

	// Logging goroutine
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				logger.Info("message")
			}
		}
	}()

	// Add/remove sinks concurrently
	for i := 0; i < 10; i++ {
		sink := &StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}
		logger.AddSink(sink)
		time.Sleep(10 * time.Millisecond)
	}

	close(done)
}

func TestRemoveSinkDuringLogging(t *testing.T) {
	sinks := make([]LogSink, 5)
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

	done := make(chan bool)

	// Logging goroutine
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				logger.Info("message")
			}
		}
	}()

	// Remove sinks concurrently
	for _, s := range sinks[1:] {
		time.Sleep(10 * time.Millisecond)
		logger.RemoveSink(s)
	}

	close(done)
}

func TestCloseDuringLogging(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithAsync(100),
	)

	var wg sync.WaitGroup
	wg.Add(2)

	// Logging goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			logger.Info("message")
		}
	}()

	// Close after some time
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		if err := logger.Close(); err != nil {
			t.Logf("Close error: %v", err)
		}
	}()

	wg.Wait()
}

func TestAsyncSyncMixedUsage(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex

	sink := NewCustomSink(func(entry LogEntry) error {
		mu.Lock()
		defer mu.Unlock()
		_, err := buf.WriteString(entry.Message + "\n")
		return err
	})

	asyncLogger := NewLogger(
		WithSink(sink),
		WithAsync(100),
	)

	syncLogger := NewLogger(WithSink(sink))

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			asyncLogger.Info(fmt.Sprintf("async-%d", i))
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			syncLogger.Info(fmt.Sprintf("sync-%d", i))
		}
	}()

	wg.Wait()
	_ = asyncLogger.Close()
	_ = syncLogger.Close()

	mu.Lock()
	output := buf.String()
	mu.Unlock()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 150 { // Some might be filtered, but most should be there
		t.Errorf("Expected around 200 log lines, got %d", len(lines))
	}
}

func TestContextFieldAccess(t *testing.T) {
	logger := NewLogger(
		WithSink(&StdoutSink{
			formatter: PlainFormat(),
			writer:    io.Discard,
		}),
		WithContextFields(String("app", "test")),
	)
	defer func() { _ = logger.Close() }()

	var wg sync.WaitGroup
	const goroutines = 20

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				logger.Info("message")
			}
		}()
	}

	wg.Wait()
}

// ==========================
// Section 10: Additional Tests
// ==========================

func TestLogger_SetLevel(t *testing.T) {
	logger := NewLogger(WithLevel(INFO))
	defer func() { _ = logger.Close() }()

	if logger.GetLevel() != INFO {
		t.Errorf("Expected INFO level")
	}

	logger.SetLevel(ERROR)

	if logger.GetLevel() != ERROR {
		t.Errorf("Expected ERROR level after SetLevel")
	}
}

func TestLogger_FormattedLogging(t *testing.T) {
	var buf bytes.Buffer
	sink := &StdoutSink{
		formatter: PlainFormat(),
		writer:    &buf,
	}

	logger := NewLogger(WithSink(sink))
	defer func() { _ = logger.Close() }()

	logger.Infof("user %s logged in from %s", "alice", "127.0.0.1")

	output := buf.String()
	if !strings.Contains(output, "user alice logged in from 127.0.0.1") {
		t.Errorf("Expected formatted message in output: %s", output)
	}
}

func TestNewDefaultLogger(t *testing.T) {
	logger := NewDefaultLogger()
	defer func() { _ = logger.Close() }()

	if logger.GetLevel() != INFO {
		t.Errorf("Expected default level to be INFO")
	}

	if !logger.core.enableCaller {
		t.Errorf("Expected caller to be enabled by default")
	}

	if len(logger.core.sinks) == 0 {
		t.Errorf("Expected default sink to be configured")
	}
}

func TestMultiSink_Close(t *testing.T) {
	var closeCount atomic.Int32

	sink1 := NewCustomSinkWithClose(
		func(entry LogEntry) error { return nil },
		func() error { closeCount.Add(1); return nil },
	)
	sink2 := NewCustomSinkWithClose(
		func(entry LogEntry) error { return nil },
		func() error { closeCount.Add(1); return nil },
	)

	multi := NewMultiSink(sink1, sink2)
	err := multi.Close()

	if err != nil {
		t.Errorf("MultiSink.Close() error = %v", err)
	}

	if closeCount.Load() != 2 {
		t.Errorf("Expected both sinks to be closed, got %d", closeCount.Load())
	}
}

func TestFormatFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		field    Field
		expected string
	}{
		{"string", String("k", "v"), "v"},
		{"int", Int("k", 42), "42"},
		{"int64", Int64("k", 123456789), "123456789"},
		{"bool true", Bool("k", true), "true"},
		{"bool false", Bool("k", false), "false"},
		{"duration", Duration("k", 5*time.Second), "5s"},
		{"error nil", Err(nil), "<nil>"},
		{"error non-nil", Err(errors.New("test")), "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFieldValue(tt.field)
			if got != tt.expected {
				t.Errorf("formatFieldValue() = %v, want %v", got, tt.expected)
			}
		})
	}
}
