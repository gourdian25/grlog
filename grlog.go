// File: grlog.go

package grlog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LogLevel represents the severity level of a log message.
// It is implemented as an integer type for efficient comparison and atomic operations.
type LogLevel int32

const (
	DEBUG LogLevel = iota // DEBUG level for detailed debugging information
	INFO                  // INFO level for general operational information
	WARN                  // WARN level for warning conditions that are not errors
	ERROR                 // ERROR level for error conditions that require attention
)

// String returns the string representation of the log level.
// Returns:
//   - string: The uppercase name of the log level (e.g., "DEBUG", "INFO")
//   - For unknown levels, returns "UNKNOWN"
//
// Use case: When you need to display or serialize the log level as text.
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel converts a string to a LogLevel.
// Arguments:
//   - s: string - The string to parse (case-insensitive)
//
// Returns:
//   - LogLevel: The parsed log level
//   - error: Error if the string is not a valid log level
//
// Supported values: "DEBUG", "INFO", "WARN", "WARNING", "ERROR"
// Use case: When reading log level configuration from environment variables or config files.
func ParseLogLevel(s string) (LogLevel, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return DEBUG, nil
	case "INFO":
		return INFO, nil
	case "WARN", "WARNING":
		return WARN, nil
	case "ERROR":
		return ERROR, nil
	default:
		return DEBUG, fmt.Errorf("invalid log level: %s", s)
	}
}

// LogEntry represents a single log event with all associated metadata.
// This is the canonical structure passed to all sinks for processing.
type LogEntry struct {
	Timestamp  time.Time       // When the log event occurred
	Level      LogLevel        // Severity level of the log
	Message    string          // Primary log message
	Fields     []Field         // Structured key-value pairs for context
	CallerInfo string          // Information about where the log was called from
	Context    context.Context // Optional context for request-scoped data
}

// Field represents a typed key-value pair for structured logging.
// This replaces map[string]interface{} for better performance, type safety, and zero-allocation for primitives.
type Field struct {
	Key   string      // The field name/key
	Value interface{} // The field value
	Type  FieldType   // The type of the value for efficient formatting
}

// FieldType indicates the type of the field value for optimal serialization.
type FieldType int

const (
	StringType   FieldType = iota // Value is a string
	IntType                       // Value is an int
	Int64Type                     // Value is an int64
	BoolType                      // Value is a bool
	DurationType                  // Value is a time.Duration
	ErrorType                     // Value is an error (stored as string)
	AnyType                       // Value is any interface{} type (fallback)
)

// String creates a string field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: string - The string value
//
// Returns:
//   - Field: A typed string field
//
// Use case: Adding string metadata like "user_id", "request_path", etc.
func String(key, val string) Field {
	return Field{Key: key, Value: val, Type: StringType}
}

// Int creates an integer field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: int - The integer value
//
// Returns:
//   - Field: A typed int field
//
// Use case: Adding integer metadata like "status_code", "attempt", "count", etc.
func Int(key string, val int) Field {
	return Field{Key: key, Value: val, Type: IntType}
}

// Int64 creates an int64 field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: int64 - The int64 value
//
// Returns:
//   - Field: A typed int64 field
//
// Use case: Adding large integer metadata like "file_size", "duration_ns", "timestamp_ms", etc.
func Int64(key string, val int64) Field {
	return Field{Key: key, Value: val, Type: Int64Type}
}

// Bool creates a boolean field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: bool - The boolean value
//
// Returns:
//   - Field: A typed bool field
//
// Use case: Adding boolean flags like "cached", "authenticated", "retryable", etc.
func Bool(key string, val bool) Field {
	return Field{Key: key, Value: val, Type: BoolType}
}

// Duration creates a time.Duration field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: time.Duration - The duration value
//
// Returns:
//   - Field: A typed duration field
//
// Use case: Adding timing metadata like "latency", "timeout", "processing_time", etc.
func Duration(key string, val time.Duration) Field {
	return Field{Key: key, Value: val, Type: DurationType}
}

// Err creates an error field that safely handles nil errors.
// Arguments:
//   - err: error - The error to log (can be nil)
//
// Returns:
//   - Field: A typed error field with key "error"
//
// Use case: Logging errors with consistent field naming. If err is nil, stores nil value.
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Value: nil, Type: ErrorType}
	}
	return Field{Key: "error", Value: err.Error(), Type: ErrorType}
}

// Any creates a field for any value type (fallback).
// Arguments:
//   - key: string - The field name
//   - val: interface{} - Any Go value
//
// Returns:
//   - Field: An untyped field that uses reflection for formatting
//
// Use case: When you need to log values of unknown or complex types. Less efficient than typed fields.
func Any(key string, val interface{}) Field {
	return Field{Key: key, Value: val, Type: AnyType}
}

// LogSink is the core interface for all log destinations.
// Implementations define where logs go (stdout, file, database, network, etc.).
type LogSink interface {
	// Write processes a single log entry.
	// Arguments:
	//   - entry: LogEntry - The complete log event to write
	// Returns:
	//   - error: Any error that occurred during writing
	// Use case: All sinks must implement this to receive log entries.
	Write(entry LogEntry) error

	// Close gracefully shuts down the sink and releases resources.
	// Returns:
	//   - error: Any error that occurred during closure
	// Use case: Clean shutdown during application termination or sink replacement.
	Close() error
}

// Formatter converts a LogEntry into bytes for writing.
// Used by sinks that need text/JSON output (stdout, file).
type Formatter interface {
	// Format transforms a log entry into a byte sequence.
	// Arguments:
	//   - entry: LogEntry - The log entry to format
	// Returns:
	//   - []byte: The formatted log data, typically ending with newline
	// Use case: Serialization for human-readable or machine-parsable output.
	Format(entry LogEntry) []byte
}

// PlainFormatter formats logs as human-readable plain text.
// Ideal for development environments and console output.
type PlainFormatter struct {
	TimestampFormat string // Go time format string for timestamps
	EnableCaller    bool   // Whether to include caller information
}

// PlainFormat creates a default plain text formatter.
// Returns:
//   - Formatter: A PlainFormatter with sensible defaults
//
// Defaults:
//   - TimestampFormat: "2006-01-02 15:04:05.000" (millisecond precision)
//   - EnableCaller: true
//
// Use case: Quick setup for development logging with human-readable output.
func PlainFormat() Formatter {
	return &PlainFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000",
		EnableCaller:    true,
	}
}

// Format implements the Formatter interface for plain text output.
// Arguments:
//   - entry: LogEntry - The log entry to format
//
// Returns:
//   - []byte: Formatted text line ending with newline
//
// Format: "TIMESTAMP [LEVEL] CALLER: MESSAGE {key1=value1, key2=value2}"
// Use case: When logs need to be easily readable by humans in terminals or log files.
func (f *PlainFormatter) Format(entry LogEntry) []byte {
	var buf strings.Builder
	buf.Grow(256)

	// Timestamp
	buf.WriteString(entry.Timestamp.Format(f.TimestampFormat))
	buf.WriteString(" [")
	buf.WriteString(entry.Level.String())
	buf.WriteString("] ")

	// Caller info
	if f.EnableCaller && entry.CallerInfo != "" {
		buf.WriteString(entry.CallerInfo)
		buf.WriteString(": ")
	}

	// Message
	buf.WriteString(entry.Message)

	// Fields
	if len(entry.Fields) > 0 {
		buf.WriteString(" {")
		for i, field := range entry.Fields {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(field.Key)
			buf.WriteString("=")
			buf.WriteString(formatFieldValue(field))
		}
		buf.WriteString("}")
	}

	buf.WriteString("\n")
	return []byte(buf.String())
}

// JSONFormatter formats logs as JSON for machine consumption.
// Ideal for production environments and log aggregation systems.
type JSONFormatter struct {
	TimestampFormat string                 // Go time format string for timestamps
	EnableCaller    bool                   // Whether to include caller information
	PrettyPrint     bool                   // Whether to format JSON with indentation
	CustomFields    map[string]interface{} // Static fields added to every log entry
}

// JSONFormat creates a default JSON formatter.
// Returns:
//   - Formatter: A JSONFormatter with sensible defaults
//
// Defaults:
//   - TimestampFormat: time.RFC3339Nano (ISO 8601 with nanoseconds)
//   - EnableCaller: true
//   - PrettyPrint: false (compact JSON)
//   - CustomFields: empty
//
// Use case: Production logging where logs will be parsed by machines (ELK stack, Splunk, etc.).
func JSONFormat() Formatter {
	return &JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
		EnableCaller:    true,
		PrettyPrint:     false,
	}
}

// Format implements the Formatter interface for JSON output.
// Arguments:
//   - entry: LogEntry - The log entry to format
//
// Returns:
//   - []byte: JSON object as bytes, ending with newline
//
// Structure: {"timestamp": "...", "level": "...", "message": "...", "caller": "...", "field1": value1, ...}
// Use case: When logs need to be parsed by log aggregation systems or external tools.
func (f *JSONFormatter) Format(entry LogEntry) []byte {
	m := make(map[string]interface{}, len(entry.Fields)+4+len(f.CustomFields))

	m["timestamp"] = entry.Timestamp.Format(f.TimestampFormat)
	m["level"] = entry.Level.String()
	m["message"] = entry.Message

	if f.EnableCaller && entry.CallerInfo != "" {
		m["caller"] = entry.CallerInfo
	}

	// Custom fields first
	for k, v := range f.CustomFields {
		m[k] = v
	}

	// Entry fields
	for _, field := range entry.Fields {
		m[field.Key] = field.Value
	}

	var data []byte
	var err error

	if f.PrettyPrint {
		data, err = json.MarshalIndent(m, "", "  ")
	} else {
		data, err = json.Marshal(m)
	}

	if err != nil {
		// Fallback on marshal error
		return []byte(fmt.Sprintf(`{"error":"marshal failed: %v"}`+"\n", err))
	}

	result := make([]byte, len(data)+1)
	copy(result, data)
	result[len(data)] = '\n'
	return result
}

// formatFieldValue converts a typed field to its string representation.
// Arguments:
//   - field: Field - The typed field to format
//
// Returns:
//   - string: String representation of the field value
//
// Use case: Internal utility for plain text formatting of typed fields.
func formatFieldValue(field Field) string {
	switch field.Type {
	case StringType:
		return field.Value.(string)
	case IntType:
		return fmt.Sprintf("%d", field.Value.(int))
	case Int64Type:
		return fmt.Sprintf("%d", field.Value.(int64))
	case BoolType:
		return fmt.Sprintf("%t", field.Value.(bool))
	case DurationType:
		return field.Value.(time.Duration).String()
	case ErrorType:
		if field.Value == nil {
			return "<nil>"
		}
		return field.Value.(string)
	case AnyType:
		return fmt.Sprintf("%v", field.Value)
	default:
		return fmt.Sprintf("%v", field.Value)
	}
}

// StdoutSink writes logs to stdout (cloud-native default).
// Suitable for containerized environments where stdout is captured by the orchestrator.
type StdoutSink struct {
	mu        sync.Mutex // Protects concurrent writes to stdout
	formatter Formatter  // Formatter for log entries
	writer    io.Writer  // Output writer (default: os.Stdout)
}

// NewStdoutSink creates a sink that writes to stdout.
// Arguments:
//   - formatter: Formatter - The formatter to use (nil defaults to PlainFormat)
//
// Returns:
//   - LogSink: A StdoutSink instance
//
// Use case: Default logging in cloud-native applications where container stdout is collected.
func NewStdoutSink(formatter Formatter) LogSink {
	if formatter == nil {
		formatter = PlainFormat()
	}
	return &StdoutSink{
		formatter: formatter,
		writer:    os.Stdout,
	}
}

// Write formats and writes a log entry to stdout.
// Arguments:
//   - entry: LogEntry - The log entry to write
//
// Returns:
//   - error: Any write error from the underlying writer
//
// Use case: Synchronous logging to console with thread-safe writes.
func (s *StdoutSink) Write(entry LogEntry) error {
	formatted := s.formatter.Format(entry)

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.writer.Write(formatted)
	return err
}

// Close is a no-op for stdout as it doesn't need resource cleanup.
// Returns:
//   - error: Always nil
//
// Use case: Satisfying the LogSink interface without side effects.
func (s *StdoutSink) Close() error {
	return nil
}

// FileSink writes logs to a file with rotation support.
// Suitable for applications running on VMs or bare metal.
type FileSink struct {
	mu          sync.Mutex // Protects file operations
	file        *os.File   // Current log file handle
	formatter   Formatter  // Formatter for log entries
	maxBytes    int64      // Maximum file size before rotation
	backupCount int        // Number of backup files to keep
	baseDir     string     // Directory containing log files
	baseFile    string     // Base filename without extension
}

// FileSinkConfig holds configuration for file-based logging.
type FileSinkConfig struct {
	Filename    string    // Base filename without extension (e.g., "app")
	Dir         string    // Directory for logs (default: "logs")
	MaxBytes    int64     // Max file size before rotation (default: 10MB)
	BackupCount int       // Number of backups to keep (default: 5)
	Formatter   Formatter // Log formatter (default: PlainFormat)
}

// NewFileSink creates a file sink with rotation capabilities.
// Arguments:
//   - config: FileSinkConfig - Configuration for file logging
//
// Returns:
//   - LogSink: A FileSink instance
//   - error: Any error during file/directory creation
//
// Defaults if not specified:
//   - Dir: "logs"
//   - MaxBytes: 10MB (10 * 1024 * 1024)
//   - BackupCount: 5
//   - Formatter: PlainFormat()
//
// Use case: Applications that need persistent log storage with automatic rotation.
func NewFileSink(config FileSinkConfig) (LogSink, error) {
	if config.Dir == "" {
		config.Dir = "logs"
	}
	if config.Filename == "" {
		config.Filename = "app"
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = 10 * 1024 * 1024 // 10MB
	}
	if config.BackupCount <= 0 {
		config.BackupCount = 5
	}
	if config.Formatter == nil {
		config.Formatter = PlainFormat()
	}

	// Create log directory
	if err := os.MkdirAll(config.Dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	logPath := filepath.Join(config.Dir, config.Filename+".log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &FileSink{
		file:        file,
		formatter:   config.Formatter,
		maxBytes:    config.MaxBytes,
		backupCount: config.BackupCount,
		baseDir:     config.Dir,
		baseFile:    config.Filename,
	}, nil
}

// Write formats and writes a log entry to the current log file.
// Arguments:
//   - entry: LogEntry - The log entry to write
//
// Returns:
//   - error: Any write error or rotation failure
//
// Automatically rotates the file if it exceeds maxBytes.
// Use case: Writing logs to disk with size-based rotation.
func (s *FileSink) Write(entry LogEntry) error {
	formatted := s.formatter.Format(entry)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if rotation needed
	if s.file != nil {
		info, err := s.file.Stat()
		if err == nil && info.Size() >= s.maxBytes {
			if err := s.rotate(); err != nil {
				return fmt.Errorf("rotation failed: %w", err)
			}
		}
	}

	if s.file == nil {
		return fmt.Errorf("file is closed")
	}

	_, err := s.file.Write(formatted)
	return err
}

// rotate performs log file rotation.
// Returns:
//   - error: Any error during rotation process
//
// Steps:
//  1. Close current file
//  2. Rename with timestamp suffix
//  3. Open new file
//  4. Cleanup old backups
//
// Use case: Internal method called when current log file reaches size limit.
func (s *FileSink) rotate() error {
	if s.file == nil {
		return fmt.Errorf("no file to rotate")
	}

	// Close current file
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	// Backup current file
	currentPath := filepath.Join(s.baseDir, s.baseFile+".log")
	backupPath := filepath.Join(s.baseDir, fmt.Sprintf("%s_%s.log",
		s.baseFile, time.Now().Format("20060102_150405")))

	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	// Open new file
	file, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new file: %w", err)
	}
	s.file = file

	// Cleanup old backups
	s.cleanupOldBackups()

	return nil
}

// cleanupOldBackups removes old backup files beyond backupCount.
// Use case: Maintaining disk space by removing oldest log files.
func (s *FileSink) cleanupOldBackups() {
	pattern := filepath.Join(s.baseDir, s.baseFile+"_*.log")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) <= s.backupCount {
		return
	}

	// Sort by modification time
	sort.Slice(files, func(i, j int) bool {
		info1, _ := os.Stat(files[i])
		info2, _ := os.Stat(files[j])
		if info1 == nil || info2 == nil {
			return false
		}
		return info1.ModTime().Before(info2.ModTime())
	})

	// Remove oldest files
	for _, f := range files[:len(files)-s.backupCount] {
		_ = os.Remove(f)
	}
}

// Close closes the current log file and releases resources.
// Returns:
//   - error: Any error from closing the file
//
// Use case: Graceful shutdown to ensure all logs are flushed to disk.
func (s *FileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// MultiSink writes to multiple sinks simultaneously.
// Useful for sending logs to multiple destinations (e.g., stdout and file).
type MultiSink struct {
	sinks []LogSink // List of sinks to write to
}

// NewMultiSink creates a sink that writes to multiple sinks.
// Arguments:
//   - sinks: ...LogSink - One or more sinks to write to
//
// Returns:
//   - LogSink: A MultiSink instance
//
// Use case: When logs need to go to multiple destinations simultaneously.
func NewMultiSink(sinks ...LogSink) LogSink {
	return &MultiSink{sinks: sinks}
}

// Write writes the log entry to all configured sinks.
// Arguments:
//   - entry: LogEntry - The log entry to write
//
// Returns:
//   - error: Combined errors from all sinks (if any)
//
// Note: Continues writing to all sinks even if some fail.
// Use case: Fan-out logging to multiple destinations with error aggregation.
func (m *MultiSink) Write(entry LogEntry) error {
	var errs []error
	for _, sink := range m.sinks {
		if err := sink.Write(entry); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multi-sink errors: %v", errs)
	}
	return nil
}

// Close closes all underlying sinks.
// Returns:
//   - error: Combined errors from closing all sinks (if any)
//
// Note: Attempts to close all sinks even if some fail.
// Use case: Graceful shutdown of all logging destinations.
func (m *MultiSink) Close() error {
	var errs []error
	for _, sink := range m.sinks {
		if err := sink.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multi-sink close errors: %v", errs)
	}
	return nil
}

// CustomSink allows creation of sinks with custom write logic.
// Useful for integrating with databases, message queues, or external services.
type CustomSink struct {
	writeFn func(LogEntry) error // Custom write function
	closeFn func() error         // Custom close function
}

// NewCustomSink creates a custom sink with only a write function.
// Arguments:
//   - writeFn: func(LogEntry) error - Function that processes log entries
//
// Returns:
//   - LogSink: A CustomSink instance with no-op close
//
// Use case: Simple custom sinks that don't need cleanup.
func NewCustomSink(writeFn func(LogEntry) error) LogSink {
	return &CustomSink{
		writeFn: writeFn,
		closeFn: func() error { return nil },
	}
}

// NewCustomSinkWithClose creates a custom sink with both write and close functions.
// Arguments:
//   - writeFn: func(LogEntry) error - Function that processes log entries
//   - closeFn: func() error - Function that cleans up resources
//
// Returns:
//   - LogSink: A CustomSink instance with custom close
//
// Use case: Custom sinks that need resource cleanup (database connections, network sockets, etc.).
func NewCustomSinkWithClose(writeFn func(LogEntry) error, closeFn func() error) LogSink {
	return &CustomSink{
		writeFn: writeFn,
		closeFn: closeFn,
	}
}

// Write delegates to the custom write function.
// Arguments:
//   - entry: LogEntry - The log entry to process
//
// Returns:
//   - error: Error from the custom write function
//
// Use case: Custom log processing logic without implementing full LogSink interface.
func (s *CustomSink) Write(entry LogEntry) error {
	return s.writeFn(entry)
}

// Close delegates to the custom close function.
// Returns:
//   - error: Error from the custom close function
//
// Use case: Custom resource cleanup for the sink.
func (s *CustomSink) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

// Logger is the main logging interface.
// It creates LogEntry objects and dispatches them to configured sinks.
type Logger struct {
	level          atomic.Int32   // Minimum log level (atomic for lock-free reads)
	sinks          []LogSink      // List of output sinks
	sinksMu        sync.RWMutex   // Protects sinks slice modification
	async          bool           // Whether logging is asynchronous
	queue          chan LogEntry  // Buffer for async logging
	closeChan      chan struct{}  // Signal channel for shutdown
	wg             sync.WaitGroup // Wait group for async worker
	closed         atomic.Bool    // Whether logger is closed (atomic)
	enableCaller   bool           // Whether to include caller information
	contextFields  []Field        // Fields added to every log entry
	contextFieldMu sync.RWMutex   // Protects contextFields slice
}

// LoggerOption is a functional option for configuring a Logger.
type LoggerOption func(*Logger)

// WithLevel sets the minimum log level for the logger.
// Arguments:
//   - level: LogLevel - The minimum level to log
//
// Returns:
//   - LoggerOption: Configuration function
//
// Use case: Dynamically changing log verbosity based on environment or configuration.
func WithLevel(level LogLevel) LoggerOption {
	return func(l *Logger) {
		l.level.Store(int32(level))
	}
}

// WithSink adds a sink to the logger.
// Arguments:
//   - sink: LogSink - The sink to add
//
// Returns:
//   - LoggerOption: Configuration function
//
// Use case: Adding additional log destinations to a logger instance.
func WithSink(sink LogSink) LoggerOption {
	return func(l *Logger) {
		if sink != nil {
			l.sinks = append(l.sinks, sink)
		}
	}
}

// WithAsync enables asynchronous logging with a buffered channel.
// Arguments:
//   - bufferSize: int - Size of the log entry buffer
//
// Returns:
//   - LoggerOption: Configuration function
//
// Notes:
//   - bufferSize must be > 0 to enable async logging
//   - If buffer is full, logs fall back to synchronous writing
//
// Use case: High-performance logging where log writing shouldn't block application code.
func WithAsync(bufferSize int) LoggerOption {
	return func(l *Logger) {
		if bufferSize > 0 {
			l.async = true
			l.queue = make(chan LogEntry, bufferSize)
		}
	}
}

// WithCaller enables or disables caller information in logs.
// Arguments:
//   - enabled: bool - Whether to include caller info
//
// Returns:
//   - LoggerOption: Configuration function
//
// Use case: Controlling whether to include file:line:function info in logs.
func WithCaller(enabled bool) LoggerOption {
	return func(l *Logger) {
		l.enableCaller = enabled
	}
}

// WithContextFields adds fields that will be included in every log entry.
// Arguments:
//   - fields: ...Field - Fields to add to all logs
//
// Returns:
//   - LoggerOption: Configuration function
//
// Use case: Adding application-wide context like service name, version, or environment.
func WithContextFields(fields ...Field) LoggerOption {
	return func(l *Logger) {
		l.contextFields = append(l.contextFields, fields...)
	}
}

// NewLogger creates a new Logger with the given options.
// Arguments:
//   - opts: ...LoggerOption - Configuration options
//
// Returns:
//   - *Logger: A configured logger instance
//
// Default behavior if no options provided:
//   - Level: DEBUG (default atomic value)
//   - Sinks: StdoutSink with PlainFormat()
//   - Caller: Enabled
//   - Async: Disabled
//
// Use case: Creating a customized logger for specific application needs.
func NewLogger(opts ...LoggerOption) *Logger {
	logger := &Logger{
		sinks:        []LogSink{},
		enableCaller: true,
		closeChan:    make(chan struct{}),
	}

	// Apply options
	for _, opt := range opts {
		opt(logger)
	}

	// Default sink if none provided
	if len(logger.sinks) == 0 {
		logger.sinks = append(logger.sinks, NewStdoutSink(PlainFormat()))
	}

	// Start async worker if enabled
	if logger.async && logger.queue != nil {
		logger.wg.Add(1)
		go logger.asyncWorker()
	}

	return logger
}

// NewDefaultLogger creates a logger with sensible production defaults.
// Returns:
//   - *Logger: A logger configured for typical production use
//
// Default configuration:
//   - Level: INFO
//   - Sink: StdoutSink with PlainFormat()
//   - Caller: Enabled
//   - Async: Disabled (synchronous)
//
// Use case: Quick setup for applications that need standard logging.
func NewDefaultLogger() *Logger {
	return NewLogger(
		WithLevel(INFO),
		WithSink(NewStdoutSink(PlainFormat())),
		WithCaller(true),
	)
}

// asyncWorker processes log entries from the async queue.
// Use case: Background goroutine that handles log writing without blocking callers.
func (l *Logger) asyncWorker() {
	defer l.wg.Done()

	for {
		select {
		case entry := <-l.queue:
			l.writeSinks(entry)
		case <-l.closeChan:
			// Drain remaining entries
			for {
				select {
				case entry := <-l.queue:
					l.writeSinks(entry)
				default:
					return
				}
			}
		}
	}
}

// writeSinks writes a log entry to all configured sinks.
// Arguments:
//   - entry: LogEntry - The log entry to write
//
// Use case: Internal method that handles the actual sink writing logic.
func (l *Logger) writeSinks(entry LogEntry) {
	l.sinksMu.RLock()
	defer l.sinksMu.RUnlock()

	for _, sink := range l.sinks {
		if err := sink.Write(entry); err != nil {
			// Best effort - log to stderr on failure
			fmt.Fprintf(os.Stderr, "logger: sink write failed: %v\n", err)
		}
	}
}

// GetLevel returns the current minimum log level.
// Returns:
//   - LogLevel: The current log level setting
//
// Use case: Checking the current log level for conditional logging logic.
func (l *Logger) GetLevel() LogLevel {
	return LogLevel(l.level.Load())
}

// SetLevel changes the minimum log level.
// Arguments:
//   - level: LogLevel - The new minimum log level
//
// Use case: Dynamically adjusting log verbosity (e.g., via admin API or SIGHUP).
func (l *Logger) SetLevel(level LogLevel) {
	l.level.Store(int32(level))
}

// AddSink adds a new sink to the logger.
// Arguments:
//   - sink: LogSink - The sink to add
//
// Use case: Adding log destinations at runtime (e.g., adding file logging after startup).
func (l *Logger) AddSink(sink LogSink) {
	if sink == nil {
		return
	}

	l.sinksMu.Lock()
	defer l.sinksMu.Unlock()

	l.sinks = append(l.sinks, sink)
}

// RemoveSink removes a sink from the logger.
// Arguments:
//   - sink: LogSink - The sink to remove
//
// Use case: Removing log destinations at runtime (e.g., disabling file logging).
func (l *Logger) RemoveSink(sink LogSink) {
	l.sinksMu.Lock()
	defer l.sinksMu.Unlock()

	for i, s := range l.sinks {
		if s == sink {
			l.sinks = append(l.sinks[:i], l.sinks[i+1:]...)
			return
		}
	}
}

// WithContext returns a new logger that includes context-derived fields.
// Arguments:
//   - ctx: context.Context - The context to extract values from
//
// Returns:
//   - *Logger: A new logger instance with context fields
//
// Extracts common context values:
//   - "request_id": string - For request tracing
//   - "trace_id": string - For distributed tracing
//   - "user_id": string - For user identification
//
// Use case: Adding request-scoped metadata to all logs within a request handler.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// Create a copy of the logger with context
	newLogger := &Logger{
		sinks:        l.sinks,
		async:        l.async,
		queue:        l.queue,
		closeChan:    l.closeChan,
		enableCaller: l.enableCaller,
	}
	newLogger.level.Store(l.level.Load())

	// Extract common context values
	contextFields := make([]Field, 0, len(l.contextFields)+3)
	contextFields = append(contextFields, l.contextFields...)

	// Try both string keys and contextKey type for backwards compatibility
	// Try string keys first
	if requestID, ok := ctx.Value("request_id").(string); ok {
		contextFields = append(contextFields, String("request_id", requestID))
	}
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		contextFields = append(contextFields, String("trace_id", traceID))
	}
	if userID, ok := ctx.Value("user_id").(string); ok {
		contextFields = append(contextFields, String("user_id", userID))
	}

	newLogger.contextFields = contextFields
	return newLogger
}

// Close shuts down the logger and closes all sinks.
// Returns:
//   - error: Combined errors from closing all sinks
//
// Steps:
//  1. Stop async worker and drain queue
//  2. Close all sinks
//  3. Aggregate any errors
//
// Use case: Graceful shutdown to ensure all logs are flushed and resources are released.
func (l *Logger) Close() error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Stop async worker
	if l.async {
		close(l.closeChan)
		l.wg.Wait()
	}

	// Close all sinks
	l.sinksMu.Lock()
	defer l.sinksMu.Unlock()

	var errs []error
	for _, sink := range l.sinks {
		if err := sink.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sink close errors: %v", errs)
	}
	return nil
}

// log is the core logging method that creates and dispatches log entries.
// Arguments:
//   - level: LogLevel - The severity level
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// Steps:
//  1. Check if level meets minimum threshold
//  2. Build LogEntry with all metadata
//  3. Dispatch to sinks (async or sync)
//
// Use case: Internal method called by all public logging methods.
func (l *Logger) log(level LogLevel, msg string, fields ...Field) {
	if level < l.GetLevel() {
		return
	}

	if l.closed.Load() {
		return
	}

	// Build entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   msg,
		Fields:    make([]Field, 0, len(fields)+len(l.contextFields)),
	}

	// Add context fields first
	l.contextFieldMu.RLock()
	entry.Fields = append(entry.Fields, l.contextFields...)
	l.contextFieldMu.RUnlock()

	// Add provided fields
	entry.Fields = append(entry.Fields, fields...)

	// Get caller info if enabled
	if l.enableCaller {
		entry.CallerInfo = getCallerInfo(3) // Skip log -> Debug/Info/etc -> actual caller
	}

	// Dispatch
	if l.async && l.queue != nil {
		select {
		case l.queue <- entry:
		default:
			// Queue full - log synchronously to avoid loss
			l.writeSinks(entry)
		}
	} else {
		l.writeSinks(entry)
	}
}

// getCallerInfo extracts caller information from the call stack.
// Arguments:
//   - skip: int - Number of stack frames to skip
//
// Returns:
//   - string: Formatted caller info as "file:line:function"
//
// Use case: Adding source location information to logs for debugging.
func getCallerInfo(skip int) string {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return fmt.Sprintf("%s:%d", filepath.Base(file), line)
	}

	// Extract function name
	fnName := fn.Name()
	if lastSlash := strings.LastIndex(fnName, "/"); lastSlash >= 0 {
		fnName = fnName[lastSlash+1:]
	}
	if lastDot := strings.LastIndex(fnName, "."); lastDot >= 0 {
		fnName = fnName[lastDot+1:]
	}

	return fmt.Sprintf("%s:%d:%s", filepath.Base(file), line, fnName)
}

// Debug logs a message at DEBUG level.
// Arguments:
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// Use case: Detailed debugging information that is typically disabled in production.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(DEBUG, msg, fields...)
}

// Info logs a message at INFO level.
// Arguments:
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// Use case: General operational information about application state.
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(INFO, msg, fields...)
}

// Warn logs a message at WARN level.
// Arguments:
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// Use case: Warning conditions that are not errors but may require attention.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(WARN, msg, fields...)
}

// Error logs a message at ERROR level.
// Arguments:
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// Use case: Error conditions that require investigation or intervention.
func (l *Logger) Error(msg string, fields ...Field) {
	l.log(ERROR, msg, fields...)
}

// Debugf logs a formatted message at DEBUG level.
// Arguments:
//   - format: string - Format string (fmt.Printf style)
//   - args: ...interface{} - Arguments for the format string
//
// Use case: Debug logging with formatted strings when structured fields are not needed.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DEBUG, fmt.Sprintf(format, args...))
}

// Infof logs a formatted message at INFO level.
// Arguments:
//   - format: string - Format string (fmt.Printf style)
//   - args: ...interface{} - Arguments for the format string
//
// Use case: Info logging with formatted strings for simple messages.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(INFO, fmt.Sprintf(format, args...))
}

// Warnf logs a formatted message at WARN level.
// Arguments:
//   - format: string - Format string (fmt.Printf style)
//   - args: ...interface{} - Arguments for the format string
//
// Use case: Warning logging with formatted strings.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WARN, fmt.Sprintf(format, args...))
}

// Errorf logs a formatted message at ERROR level.
// Arguments:
//   - format: string - Format string (fmt.Printf style)
//   - args: ...interface{} - Arguments for the format string
//
// Use case: Error logging with formatted strings for simple error messages.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ERROR, fmt.Sprintf(format, args...))
}
