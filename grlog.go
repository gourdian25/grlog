// File: grlog.go

package grlog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// LogLevel represents the severity level of a log message.
// It is implemented as an integer type for efficient comparison and atomic operations.
type LogLevel int32

const (
	DEBUG LogLevel = iota // DEBUG level for detailed debugging information
	INFO                  // INFO level for general operational information
	WARN                  // WARN level for warning conditions that are not errors
	ERROR                 // ERROR level for error conditions that require attention
	FATAL                 // FATAL level for unrecoverable errors; Fatal() exits the process
	OFF                   // OFF disables all logging when used with SetLevel/WithLevel
)

// numLevels is the number of levels that can appear on a LogEntry (DEBUG..FATAL).
const numLevels = int(FATAL) + 1

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
	case FATAL:
		return "FATAL"
	case OFF:
		return "OFF"
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
// Supported values: "DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL", "OFF"
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
	case "FATAL":
		return FATAL, nil
	case "OFF":
		return OFF, nil
	default:
		return DEBUG, fmt.Errorf("invalid log level: %s", s)
	}
}

// LogEntry represents a single log event with all associated metadata.
// This is the canonical structure passed to all sinks for processing.
type LogEntry struct {
	Timestamp  time.Time // When the log event occurred
	Level      LogLevel  // Severity level of the log
	Message    string    // Primary log message
	Fields     []Field   // Structured key-value pairs for context
	CallerInfo string    // Information about where the log was called from
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
	Float64Type                   // Value is a float64
	Uint64Type                    // Value is a uint64
	TimeType                      // Value is a time.Time
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

// Uint64 creates a uint64 field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: uint64 - The uint64 value
//
// Returns:
//   - Field: A typed uint64 field
//
// Use case: Adding unsigned metadata like "offset", "sequence", "bytes_total", etc.
func Uint64(key string, val uint64) Field {
	return Field{Key: key, Value: val, Type: Uint64Type}
}

// Float64 creates a float64 field with zero allocation.
// Arguments:
//   - key: string - The field name
//   - val: float64 - The float value
//
// Returns:
//   - Field: A typed float64 field
//
// Use case: Adding numeric metadata like "ratio", "temperature", "duration_seconds", etc.
func Float64(key string, val float64) Field {
	return Field{Key: key, Value: val, Type: Float64Type}
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

// Time creates a time.Time field.
// Arguments:
//   - key: string - The field name
//   - val: time.Time - The time value (formatted as RFC3339Nano)
//
// Returns:
//   - Field: A typed time field
//
// Use case: Adding timestamps like "expires_at", "created_at", "deadline", etc.
func Time(key string, val time.Time) Field {
	return Field{Key: key, Value: val, Type: TimeType}
}

// Err creates an error field that safely handles nil errors.
// Arguments:
//   - err: error - The error to log (can be nil)
//
// Returns:
//   - Field: A typed error field with key "error"
//
// Note: Fields created from a nil error are skipped entirely by the built-in
// formatters, so `grlog.Err(err)` is always safe to pass unconditionally.
// Use case: Logging errors with consistent field naming.
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

// contextKey is an unexported type for context keys defined by this package.
// Using a distinct type prevents collisions with context keys defined elsewhere.
type contextKey int

const (
	requestIDKey contextKey = iota
	traceIDKey
	userIDKey
)

// ContextWithRequestID returns a context carrying a request ID that
// Logger.WithContext will surface as a "request_id" field.
// Arguments:
//   - ctx: context.Context - The parent context
//   - requestID: string - The request identifier
//
// Returns:
//   - context.Context: A derived context carrying the request ID
//
// Use case: HTTP middleware storing a per-request ID for downstream log correlation.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// ContextWithTraceID returns a context carrying a trace ID that
// Logger.WithContext will surface as a "trace_id" field.
// Arguments:
//   - ctx: context.Context - The parent context
//   - traceID: string - The distributed trace identifier
//
// Returns:
//   - context.Context: A derived context carrying the trace ID
//
// Use case: Propagating distributed tracing identifiers into log output.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// ContextWithUserID returns a context carrying a user ID that
// Logger.WithContext will surface as a "user_id" field.
// Arguments:
//   - ctx: context.Context - The parent context
//   - userID: string - The user identifier
//
// Returns:
//   - context.Context: A derived context carrying the user ID
//
// Use case: Associating authenticated user identity with request-scoped logs.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// ContextExtractor extracts structured fields from a context.
// Registered via WithContextExtractor and invoked by Logger.WithContext.
type ContextExtractor func(ctx context.Context) []Field

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

// appendFormatter is an optional interface formatters can implement to format
// into a caller-provided buffer, avoiding per-entry allocations. The built-in
// sinks use it together with an internal buffer pool.
type appendFormatter interface {
	AppendFormat(dst []byte, entry LogEntry) []byte
}

// bufPool recycles formatting buffers across log writes.
var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 0, 1024)
		return &b
	},
}

func getBuffer() *[]byte {
	return bufPool.Get().(*[]byte)
}

func putBuffer(b *[]byte) {
	// Don't hoard unusually large buffers; let them be collected.
	if cap(*b) > 1<<16 {
		return
	}
	*b = (*b)[:0]
	bufPool.Put(b)
}

// formatEntry formats an entry into dst, using the zero-allocation
// AppendFormat path when the formatter supports it.
func formatEntry(f Formatter, dst []byte, entry LogEntry) []byte {
	if af, ok := f.(appendFormatter); ok {
		return af.AppendFormat(dst[:0], entry)
	}
	return append(dst[:0], f.Format(entry)...)
}

// PlainFormatter formats logs as human-readable plain text.
// Ideal for development environments and console output.
type PlainFormatter struct {
	TimestampFormat string // Go time format string for timestamps

	// Deprecated: caller info is controlled by the Logger's WithCaller option;
	// the formatter renders caller info whenever the entry carries it.
	EnableCaller bool
}

// PlainFormat creates a default plain text formatter.
// Returns:
//   - Formatter: A PlainFormatter with sensible defaults
//
// Defaults:
//   - TimestampFormat: "2006-01-02 15:04:05.000" (millisecond precision)
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
	return f.AppendFormat(make([]byte, 0, 256), entry)
}

// AppendFormat formats the entry into dst and returns the extended buffer.
// Arguments:
//   - dst: []byte - Destination buffer (may be nil)
//   - entry: LogEntry - The log entry to format
//
// Returns:
//   - []byte: dst with the formatted line appended
//
// Use case: Allocation-free formatting with pooled buffers.
func (f *PlainFormatter) AppendFormat(dst []byte, entry LogEntry) []byte {
	format := f.TimestampFormat
	if format == "" {
		format = "2006-01-02 15:04:05.000"
	}
	dst = entry.Timestamp.AppendFormat(dst, format)
	dst = append(dst, " ["...)
	dst = append(dst, entry.Level.String()...)
	dst = append(dst, "] "...)

	// Caller info (rendered whenever present; see Logger's WithCaller option)
	if entry.CallerInfo != "" {
		dst = append(dst, entry.CallerInfo...)
		dst = append(dst, ": "...)
	}

	// Message
	dst = append(dst, entry.Message...)

	// Fields
	if len(entry.Fields) > 0 {
		wroteAny := false
		for _, field := range entry.Fields {
			// Skip nil-error fields produced by Err(nil)
			if field.Type == ErrorType && field.Value == nil {
				continue
			}
			if wroteAny {
				dst = append(dst, ", "...)
			} else {
				dst = append(dst, " {"...)
			}
			wroteAny = true
			dst = append(dst, field.Key...)
			dst = append(dst, '=')
			dst = appendFieldValue(dst, field)
		}
		if wroteAny {
			dst = append(dst, '}')
		}
	}

	dst = append(dst, '\n')
	return dst
}

// JSONFormatter formats logs as JSON for machine consumption.
// Ideal for production environments and log aggregation systems.
//
// Output is deterministic: reserved keys ("timestamp", "level", "message",
// "caller") come first, then CustomFields in sorted key order, then entry
// fields in the order they were passed. An entry field whose key collides
// with a reserved key is emitted under "fields.<key>" instead of silently
// overwriting the entry metadata.
type JSONFormatter struct {
	TimestampFormat string                 // Go time format string for timestamps
	PrettyPrint     bool                   // Whether to format JSON with indentation
	CustomFields    map[string]interface{} // Static fields added to every log entry

	// Deprecated: caller info is controlled by the Logger's WithCaller option;
	// the formatter renders caller info whenever the entry carries it.
	EnableCaller bool
}

// JSONFormat creates a default JSON formatter.
// Returns:
//   - Formatter: A JSONFormatter with sensible defaults
//
// Defaults:
//   - TimestampFormat: time.RFC3339Nano (ISO 8601 with nanoseconds)
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
	return f.AppendFormat(make([]byte, 0, 256), entry)
}

// AppendFormat formats the entry as JSON into dst and returns the extended buffer.
// Arguments:
//   - dst: []byte - Destination buffer (may be nil)
//   - entry: LogEntry - The log entry to format
//
// Returns:
//   - []byte: dst with the JSON line appended
//
// Use case: Allocation-conscious JSON formatting with pooled buffers.
func (f *JSONFormatter) AppendFormat(dst []byte, entry LogEntry) []byte {
	if !f.PrettyPrint {
		return f.appendCompact(dst, entry)
	}

	compact := f.appendCompact(nil, entry)
	var out bytes.Buffer
	// Strip the trailing newline before indenting, then re-add it.
	if err := json.Indent(&out, compact[:len(compact)-1], "", "  "); err != nil {
		return append(dst, compact...)
	}
	out.WriteByte('\n')
	return append(dst, out.Bytes()...)
}

// isReservedJSONKey reports whether key would collide with entry metadata.
func isReservedJSONKey(key string, hasCaller bool) bool {
	switch key {
	case "timestamp", "level", "message":
		return true
	case "caller":
		return hasCaller
	}
	return false
}

// appendCompact writes the entry as a single compact JSON object plus newline.
func (f *JSONFormatter) appendCompact(dst []byte, entry LogEntry) []byte {
	format := f.TimestampFormat
	if format == "" {
		format = time.RFC3339Nano
	}
	hasCaller := entry.CallerInfo != ""

	dst = append(dst, `{"timestamp":`...)
	dst = append(dst, '"')
	dst = entry.Timestamp.AppendFormat(dst, format)
	dst = append(dst, '"')
	dst = append(dst, `,"level":`...)
	dst = appendJSONString(dst, entry.Level.String())
	dst = append(dst, `,"message":`...)
	dst = appendJSONString(dst, entry.Message)

	if hasCaller {
		dst = append(dst, `,"caller":`...)
		dst = appendJSONString(dst, entry.CallerInfo)
	}

	// Custom fields in sorted key order. A custom field is skipped when an
	// entry field with the same key exists (entry fields take precedence,
	// matching pre-existing behavior).
	if len(f.CustomFields) > 0 {
		keys := make([]string, 0, len(f.CustomFields))
		for k := range f.CustomFields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if entryHasFieldKey(entry.Fields, k) {
				continue
			}
			dst = append(dst, ',')
			if isReservedJSONKey(k, hasCaller) {
				dst = appendJSONString(dst, "fields."+k)
			} else {
				dst = appendJSONString(dst, k)
			}
			dst = append(dst, ':')
			dst = appendJSONAny(dst, f.CustomFields[k])
		}
	}

	// Entry fields in the order they were passed.
	for _, field := range entry.Fields {
		// Skip nil-error fields produced by Err(nil)
		if field.Type == ErrorType && field.Value == nil {
			continue
		}
		dst = append(dst, ',')
		if isReservedJSONKey(field.Key, hasCaller) {
			dst = appendJSONString(dst, "fields."+field.Key)
		} else {
			dst = appendJSONString(dst, field.Key)
		}
		dst = append(dst, ':')
		dst = appendJSONFieldValue(dst, field)
	}

	dst = append(dst, '}', '\n')
	return dst
}

// entryHasFieldKey reports whether any entry field uses the given key.
func entryHasFieldKey(fields []Field, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

const hexDigits = "0123456789abcdef"

// appendJSONString appends s as a JSON-escaped, quoted string.
func appendJSONString(dst []byte, s string) []byte {
	// Invalid UTF-8 would produce invalid JSON with the fast path below;
	// delegate those rare strings to encoding/json (which sanitizes them).
	if !utf8.ValidString(s) {
		if b, err := json.Marshal(s); err == nil {
			return append(dst, b...)
		}
	}

	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			dst = append(dst, '\\', '"')
		case c == '\\':
			dst = append(dst, '\\', '\\')
		case c == '\n':
			dst = append(dst, '\\', 'n')
		case c == '\r':
			dst = append(dst, '\\', 'r')
		case c == '\t':
			dst = append(dst, '\\', 't')
		case c < 0x20:
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		default:
			dst = append(dst, c)
		}
	}
	return append(dst, '"')
}

// appendJSONAny appends an arbitrary value as JSON, degrading to its %v
// string form if it cannot be marshaled (the entry is never lost).
func appendJSONAny(dst []byte, v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return appendJSONString(dst, fmt.Sprintf("%v", v))
	}
	return append(dst, b...)
}

// appendJSONFieldValue appends a typed field value as JSON.
func appendJSONFieldValue(dst []byte, field Field) []byte {
	switch field.Type {
	case StringType:
		if v, ok := field.Value.(string); ok {
			return appendJSONString(dst, v)
		}
	case IntType:
		if v, ok := field.Value.(int); ok {
			return strconv.AppendInt(dst, int64(v), 10)
		}
	case Int64Type:
		if v, ok := field.Value.(int64); ok {
			return strconv.AppendInt(dst, v, 10)
		}
	case Uint64Type:
		if v, ok := field.Value.(uint64); ok {
			return strconv.AppendUint(dst, v, 10)
		}
	case Float64Type:
		if v, ok := field.Value.(float64); ok {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				// NaN/Inf are not valid JSON numbers
				return appendJSONString(dst, strconv.FormatFloat(v, 'g', -1, 64))
			}
			return strconv.AppendFloat(dst, v, 'g', -1, 64)
		}
	case BoolType:
		if v, ok := field.Value.(bool); ok {
			return strconv.AppendBool(dst, v)
		}
	case DurationType:
		if v, ok := field.Value.(time.Duration); ok {
			return appendJSONString(dst, v.String())
		}
	case TimeType:
		if v, ok := field.Value.(time.Time); ok {
			dst = append(dst, '"')
			dst = v.AppendFormat(dst, time.RFC3339Nano)
			return append(dst, '"')
		}
	case ErrorType:
		if field.Value == nil {
			return append(dst, "null"...)
		}
		if v, ok := field.Value.(string); ok {
			return appendJSONString(dst, v)
		}
	}
	return appendJSONAny(dst, field.Value)
}

// appendFieldValue appends a typed field's plain-text representation.
func appendFieldValue(dst []byte, field Field) []byte {
	switch field.Type {
	case StringType:
		if v, ok := field.Value.(string); ok {
			return append(dst, v...)
		}
	case IntType:
		if v, ok := field.Value.(int); ok {
			return strconv.AppendInt(dst, int64(v), 10)
		}
	case Int64Type:
		if v, ok := field.Value.(int64); ok {
			return strconv.AppendInt(dst, v, 10)
		}
	case Uint64Type:
		if v, ok := field.Value.(uint64); ok {
			return strconv.AppendUint(dst, v, 10)
		}
	case Float64Type:
		if v, ok := field.Value.(float64); ok {
			return strconv.AppendFloat(dst, v, 'g', -1, 64)
		}
	case BoolType:
		if v, ok := field.Value.(bool); ok {
			return strconv.AppendBool(dst, v)
		}
	case DurationType:
		if v, ok := field.Value.(time.Duration); ok {
			return append(dst, v.String()...)
		}
	case TimeType:
		if v, ok := field.Value.(time.Time); ok {
			return v.AppendFormat(dst, time.RFC3339Nano)
		}
	case ErrorType:
		if field.Value == nil {
			return append(dst, "<nil>"...)
		}
		if v, ok := field.Value.(string); ok {
			return append(dst, v...)
		}
	}
	return append(dst, fmt.Sprintf("%v", field.Value)...)
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
	return string(appendFieldValue(nil, field))
}

// WriterSink writes formatted log entries to an arbitrary io.Writer.
// StdoutSink is an alias of this type for backwards compatibility.
type WriterSink struct {
	mu        sync.Mutex // Protects concurrent writes to the writer
	formatter Formatter  // Formatter for log entries
	writer    io.Writer  // Output writer
}

// StdoutSink writes logs to stdout (cloud-native default).
// Suitable for containerized environments where stdout is captured by the orchestrator.
type StdoutSink = WriterSink

// NewWriterSink creates a sink that writes to any io.Writer.
// Arguments:
//   - w: io.Writer - Destination writer (nil defaults to os.Stdout)
//   - formatter: Formatter - The formatter to use (nil defaults to PlainFormat)
//
// Returns:
//   - LogSink: A WriterSink instance
//
// Use case: Directing logs at stderr, an in-memory buffer in tests, a network
// connection, or any other io.Writer.
func NewWriterSink(w io.Writer, formatter Formatter) LogSink {
	if w == nil {
		w = os.Stdout
	}
	if formatter == nil {
		formatter = PlainFormat()
	}
	return &WriterSink{
		formatter: formatter,
		writer:    w,
	}
}

// NewStdoutSink creates a sink that writes to stdout.
// Arguments:
//   - formatter: Formatter - The formatter to use (nil defaults to PlainFormat)
//
// Returns:
//   - LogSink: A sink writing to os.Stdout
//
// Use case: Default logging in cloud-native applications where container stdout is collected.
func NewStdoutSink(formatter Formatter) LogSink {
	return NewWriterSink(os.Stdout, formatter)
}

// Write formats and writes a log entry to the underlying writer.
// Arguments:
//   - entry: LogEntry - The log entry to write
//
// Returns:
//   - error: Any write error from the underlying writer
//
// Use case: Synchronous logging to a writer with thread-safe writes.
func (s *WriterSink) Write(entry LogEntry) error {
	buf := getBuffer()
	defer putBuffer(buf)
	*buf = formatEntry(s.formatter, *buf, entry)

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.writer.Write(*buf)
	return err
}

// Close is a no-op; the sink does not own the underlying writer.
// Returns:
//   - error: Always nil
//
// Use case: Satisfying the LogSink interface without side effects.
func (s *WriterSink) Close() error {
	return nil
}

// FileSink writes logs to a file with rotation support.
// Suitable for applications running on VMs or bare metal.
type FileSink struct {
	mu          sync.Mutex    // Protects file operations
	file        *os.File      // Current log file handle
	size        int64         // Current file size, tracked to avoid a Stat per write
	formatter   Formatter     // Formatter for log entries
	maxBytes    int64         // Maximum file size before rotation
	backupCount int           // Number of backup files to keep
	maxAge      time.Duration // Maximum age of backups (0 = no age limit)
	compress    bool          // Whether to gzip rotated backups
	baseDir     string        // Directory containing log files
	baseFile    string        // Base filename without extension
}

// FileSinkConfig holds configuration for file-based logging.
type FileSinkConfig struct {
	Filename    string        // Base filename without extension (e.g., "app")
	Dir         string        // Directory for logs (default: "logs")
	MaxBytes    int64         // Max file size before rotation (default: 10MB)
	BackupCount int           // Number of backups to keep (default: 5)
	MaxAge      time.Duration // Remove backups older than this (default: 0, disabled)
	Compress    bool          // Gzip rotated backups in the background (default: false)
	Formatter   Formatter     // Log formatter (default: PlainFormat)
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

	var size int64
	if info, err := file.Stat(); err == nil {
		size = info.Size()
	}

	return &FileSink{
		file:        file,
		size:        size,
		formatter:   config.Formatter,
		maxBytes:    config.MaxBytes,
		backupCount: config.BackupCount,
		maxAge:      config.MaxAge,
		compress:    config.Compress,
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
// Automatically rotates the file if it exceeds maxBytes. If rotation fails,
// the entry is still written to the current file so no logs are lost, and the
// rotation error is returned.
// Use case: Writing logs to disk with size-based rotation.
func (s *FileSink) Write(entry LogEntry) error {
	buf := getBuffer()
	defer putBuffer(buf)
	*buf = formatEntry(s.formatter, *buf, entry)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if rotation needed
	var rotateErr error
	if s.file != nil && s.size >= s.maxBytes {
		rotateErr = s.rotate()
	}

	if s.file == nil {
		if rotateErr != nil {
			return fmt.Errorf("file unavailable after failed rotation: %w", rotateErr)
		}
		return fmt.Errorf("file is closed")
	}

	n, err := s.file.Write(*buf)
	s.size += int64(n)
	if err != nil {
		return err
	}
	if rotateErr != nil {
		return fmt.Errorf("rotation failed (entry written to current file): %w", rotateErr)
	}
	return nil
}

// rotate performs log file rotation. Must be called with s.mu held.
// Returns:
//   - error: Any error during rotation process
//
// Steps:
//  1. Close current file
//  2. Rename with a collision-safe timestamp suffix
//  3. Open new file
//  4. Cleanup old backups (compressing the new backup first if configured)
//
// On failure the sink reopens the original file in append mode so logging
// continues; rotation is retried on a later write.
// Use case: Internal method called when current log file reaches size limit.
func (s *FileSink) rotate() error {
	if s.file == nil {
		return errors.New("no file to rotate")
	}

	// Close current file
	closeErr := s.file.Close()
	s.file = nil
	if closeErr != nil {
		// Handle state is unknown; recover by reopening so logging continues.
		reopenErr := s.reopen()
		return errors.Join(fmt.Errorf("failed to close file: %w", closeErr), reopenErr)
	}

	// Backup current file under a name that cannot clobber a previous backup
	currentPath := filepath.Join(s.baseDir, s.baseFile+".log")
	backupPath := s.nextBackupPath()

	if err := os.Rename(currentPath, backupPath); err != nil {
		reopenErr := s.reopen()
		return errors.Join(fmt.Errorf("failed to rename file: %w", err), reopenErr)
	}

	// Open new file
	if err := s.reopen(); err != nil {
		return fmt.Errorf("failed to open new file: %w", err)
	}

	// Cleanup old backups (after background compression when enabled, so the
	// fresh backup is counted under its final name)
	if s.compress {
		go func() {
			compressBackup(backupPath)
			s.cleanupOldBackups()
		}()
	} else {
		s.cleanupOldBackups()
	}

	return nil
}

// reopen opens the primary log file in append mode and resets size tracking.
// Must be called with s.mu held.
func (s *FileSink) reopen() error {
	file, err := os.OpenFile(filepath.Join(s.baseDir, s.baseFile+".log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	s.file = file
	s.size = 0
	if info, err := file.Stat(); err == nil {
		s.size = info.Size()
	}
	return nil
}

// nextBackupPath returns a backup filename that does not exist yet.
// Nanosecond timestamps plus a sequence suffix make collisions impossible
// even when rotations happen within the same instant.
func (s *FileSink) nextBackupPath() string {
	stamp := time.Now().Format("20060102_150405.000000000")
	path := filepath.Join(s.baseDir, fmt.Sprintf("%s_%s.log", s.baseFile, stamp))
	for seq := 1; ; seq++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = filepath.Join(s.baseDir, fmt.Sprintf("%s_%s.%d.log", s.baseFile, stamp, seq))
	}
}

// compressBackup gzips a rotated backup file and removes the original.
// The archive is written under a temporary name and renamed into place only
// when complete, so readers never observe a partial .gz file. Best effort:
// on any failure the original file is kept.
func compressBackup(path string) {
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = in.Close() }()

	tmpPath := path + ".gz.tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return
	}

	gw := gzip.NewWriter(out)
	_, copyErr := io.Copy(gw, in)
	gzErr := gw.Close()
	outErr := out.Close()

	if copyErr != nil || gzErr != nil || outErr != nil {
		_ = os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, path+".gz"); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	_ = os.Remove(path)
}

// cleanupOldBackups removes backups beyond backupCount and older than maxAge.
// Use case: Maintaining disk space by removing oldest log files.
func (s *FileSink) cleanupOldBackups() {
	var files []string
	for _, pattern := range []string{s.baseFile + "_*.log", s.baseFile + "_*.log.gz"} {
		matches, err := filepath.Glob(filepath.Join(s.baseDir, pattern))
		if err == nil {
			files = append(files, matches...)
		}
	}

	// Age-based cleanup
	if s.maxAge > 0 {
		cutoff := time.Now().Add(-s.maxAge)
		kept := files[:0]
		for _, f := range files {
			if info, err := os.Stat(f); err == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(f)
				continue
			}
			kept = append(kept, f)
		}
		files = kept
	}

	// Count-based cleanup
	if len(files) <= s.backupCount {
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
//   - error: Combined errors from all sinks (joined via errors.Join)
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
		return fmt.Errorf("multi-sink errors: %w", errors.Join(errs...))
	}
	return nil
}

// Close closes all underlying sinks.
// Returns:
//   - error: Combined errors from closing all sinks (joined via errors.Join)
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
		return fmt.Errorf("multi-sink close errors: %w", errors.Join(errs...))
	}
	return nil
}

// LeveledSink wraps another sink and drops entries below a minimum level.
// Useful for routing only high-severity logs to expensive destinations.
type LeveledSink struct {
	sink     LogSink  // The wrapped sink
	minLevel LogLevel // Minimum level this sink accepts
}

// NewLeveledSink wraps a sink with per-sink level filtering.
// Arguments:
//   - sink: LogSink - The sink to wrap
//   - minLevel: LogLevel - Entries below this level are silently dropped
//
// Returns:
//   - LogSink: A LeveledSink instance
//
// Use case: Send everything to stdout but only ERROR and above to a file:
//
//	NewMultiSink(stdoutSink, NewLeveledSink(fileSink, ERROR))
func NewLeveledSink(sink LogSink, minLevel LogLevel) LogSink {
	return &LeveledSink{sink: sink, minLevel: minLevel}
}

// Write forwards the entry to the wrapped sink if it meets the minimum level.
// Arguments:
//   - entry: LogEntry - The log entry to filter and forward
//
// Returns:
//   - error: Error from the wrapped sink, or nil when filtered
//
// Use case: Per-destination severity routing.
func (s *LeveledSink) Write(entry LogEntry) error {
	if entry.Level < s.minLevel {
		return nil
	}
	return s.sink.Write(entry)
}

// Close closes the wrapped sink.
// Returns:
//   - error: Error from the wrapped sink's Close
//
// Use case: Graceful shutdown of the wrapped destination.
func (s *LeveledSink) Close() error {
	return s.sink.Close()
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

// OverflowPolicy controls what happens when the async queue is full.
type OverflowPolicy int

const (
	// OverflowSyncFallback writes the entry synchronously from the calling
	// goroutine when the queue is full. No entries are lost, but the caller
	// blocks for the write and entries may appear slightly out of order.
	// This is the default.
	OverflowSyncFallback OverflowPolicy = iota

	// OverflowBlock blocks the calling goroutine until queue space frees up.
	// Preserves strict ordering at the cost of caller latency under load.
	OverflowBlock

	// OverflowDrop discards the entry when the queue is full. Never blocks
	// the caller; dropped entries are counted in Stats().DroppedEntries.
	OverflowDrop
)

// samplerState holds per-level counters for log sampling.
type samplerState struct {
	every    uint64                   // Keep 1 in every N entries
	maxLevel LogLevel                 // Levels at or below this are sampled
	counters [numLevels]atomic.Uint64 // Per-level entry counters
	skipped  atomic.Uint64            // Entries dropped by sampling
}

// LoggerStats reports counters about entries the logger did not write.
type LoggerStats struct {
	DroppedEntries uint64 // Entries dropped under OverflowDrop
	SampledEntries uint64 // Entries skipped by the sampler
}

// loggerCore holds all state shared by the Logger views derived from one
// NewLogger call. WithContext and With return lightweight views onto the same
// core, so sinks, the async queue, the worker goroutine, and the closed flag
// exist exactly once per logger — closing any view shuts everything down
// exactly once, and sink changes are visible to every view.
type loggerCore struct {
	level        atomic.Int32       // Minimum log level (atomic for lock-free reads)
	sinks        []LogSink          // List of output sinks
	sinksMu      sync.RWMutex       // Protects sinks slice modification
	async        bool               // Whether logging is asynchronous
	queue        chan LogEntry      // Buffer for async logging
	closeChan    chan struct{}      // Signal channel for shutdown
	wg           sync.WaitGroup     // Wait group for async worker
	closed       atomic.Bool        // Whether logger is closed (atomic)
	enableCaller bool               // Whether to include caller information
	callerSkip   int                // Extra stack frames to skip for caller info
	overflow     OverflowPolicy     // Behavior when the async queue is full
	dropped      atomic.Uint64      // Entries dropped under OverflowDrop
	errorHandler func(error)        // Callback for sink write failures (nil = stderr)
	extractors   []ContextExtractor // Custom context extractors for WithContext
	sampler      *samplerState      // Optional sampling state

	// Rate limiting for the default stderr error reporting
	errMu       sync.Mutex
	lastErrMsg  string
	lastErrTime time.Time
	suppressed  uint64
}

// Logger is the main logging interface.
// It creates LogEntry objects and dispatches them to configured sinks.
//
// A Logger is a cheap view onto shared state: WithContext and With derive new
// views carrying extra fields while sharing sinks and the async machinery.
// Calling Close on any view closes the shared logger exactly once.
type Logger struct {
	core          *loggerCore // Shared state (sinks, queue, worker, level)
	contextFields []Field     // Fields added to every entry; immutable per view
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
		l.core.level.Store(int32(level))
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
			l.core.sinks = append(l.core.sinks, sink)
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
//   - Behavior when the buffer is full is governed by WithOverflowPolicy
//     (default: the entry is written synchronously so it is never lost)
//
// Use case: High-performance logging where log writing shouldn't block application code.
func WithAsync(bufferSize int) LoggerOption {
	return func(l *Logger) {
		if bufferSize > 0 {
			l.core.async = true
			l.core.queue = make(chan LogEntry, bufferSize)
		}
	}
}

// WithOverflowPolicy sets the behavior when the async queue is full.
// Arguments:
//   - policy: OverflowPolicy - OverflowSyncFallback (default), OverflowBlock, or OverflowDrop
//
// Returns:
//   - LoggerOption: Configuration function
//
// Only meaningful together with WithAsync.
// Use case: Choosing between durability (SyncFallback), strict ordering
// (Block), and guaranteed non-blocking behavior (Drop) under load.
func WithOverflowPolicy(policy OverflowPolicy) LoggerOption {
	return func(l *Logger) {
		l.core.overflow = policy
	}
}

// WithCaller enables or disables caller information in logs.
// Arguments:
//   - enabled: bool - Whether to include caller info
//
// Returns:
//   - LoggerOption: Configuration function
//
// This is the single source of truth for caller info: formatters render the
// caller whenever the entry carries it.
// Use case: Controlling whether to include file:line:function info in logs.
func WithCaller(enabled bool) LoggerOption {
	return func(l *Logger) {
		l.core.enableCaller = enabled
	}
}

// WithCallerSkip adds extra stack frames to skip when resolving caller info.
// Arguments:
//   - skip: int - Number of additional frames to skip
//
// Returns:
//   - LoggerOption: Configuration function
//
// Use case: When wrapping the logger in your own helper functions, so caller
// info points at your application code instead of the wrapper.
func WithCallerSkip(skip int) LoggerOption {
	return func(l *Logger) {
		if skip > 0 {
			l.core.callerSkip = skip
		}
	}
}

// WithErrorHandler sets a callback invoked when a sink write fails.
// Arguments:
//   - handler: func(error) - Called with each sink write error
//
// Returns:
//   - LoggerOption: Configuration function
//
// When no handler is set, failures are reported to stderr with repeated
// identical errors rate-limited to once per second.
// Use case: Routing sink failures to metrics or an alerting system. The
// handler must not log through the same logger (risk of recursion).
func WithErrorHandler(handler func(error)) LoggerOption {
	return func(l *Logger) {
		l.core.errorHandler = handler
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

// WithContextExtractor registers a custom extractor used by WithContext.
// Arguments:
//   - extractor: ContextExtractor - Function mapping a context to fields
//
// Returns:
//   - LoggerOption: Configuration function
//
// Multiple extractors may be registered; each runs on every WithContext call.
// Use case: Pulling OpenTelemetry span IDs, tenant IDs, or other custom
// request metadata out of contexts.
func WithContextExtractor(extractor ContextExtractor) LoggerOption {
	return func(l *Logger) {
		if extractor != nil {
			l.core.extractors = append(l.core.extractors, extractor)
		}
	}
}

// WithSampler enables sampling for high-volume low-severity logs.
// Arguments:
//   - every: int - Keep 1 in every N entries (must be > 1 to enable)
//   - maxLevel: LogLevel - Only levels at or below this are sampled
//
// Returns:
//   - LoggerOption: Configuration function
//
// The first entry of each level is always kept. Entries above maxLevel are
// never sampled. Skipped entries are counted in Stats().SampledEntries.
// Use case: Keeping DEBUG/INFO volume manageable on hot paths without losing
// WARN/ERROR visibility.
func WithSampler(every int, maxLevel LogLevel) LoggerOption {
	return func(l *Logger) {
		if every > 1 {
			l.core.sampler = &samplerState{
				every:    uint64(every),
				maxLevel: maxLevel,
			}
		}
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
		core: &loggerCore{
			sinks:        []LogSink{},
			enableCaller: true,
			closeChan:    make(chan struct{}),
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(logger)
	}

	// Default sink if none provided
	if len(logger.core.sinks) == 0 {
		logger.core.sinks = append(logger.core.sinks, NewStdoutSink(PlainFormat()))
	}

	// Start async worker if enabled
	if logger.core.async && logger.core.queue != nil {
		logger.core.wg.Add(1)
		go logger.core.asyncWorker()
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
func (c *loggerCore) asyncWorker() {
	defer c.wg.Done()

	for {
		select {
		case entry := <-c.queue:
			c.writeSinks(entry)
		case <-c.closeChan:
			c.drainQueue()
			return
		}
	}
}

// drainQueue synchronously writes every entry currently buffered in the queue.
func (c *loggerCore) drainQueue() {
	for {
		select {
		case entry := <-c.queue:
			c.writeSinks(entry)
		default:
			return
		}
	}
}

// writeSinks writes a log entry to all configured sinks.
// Arguments:
//   - entry: LogEntry - The log entry to write
//
// Use case: Internal method that handles the actual sink writing logic.
func (c *loggerCore) writeSinks(entry LogEntry) {
	c.sinksMu.RLock()
	defer c.sinksMu.RUnlock()

	for _, sink := range c.sinks {
		if err := sink.Write(entry); err != nil {
			c.handleError(err)
		}
	}
}

// handleError reports a sink write failure. With no custom handler set,
// failures go to stderr with repeated identical errors rate-limited so a
// persistently failing sink cannot flood stderr.
func (c *loggerCore) handleError(err error) {
	if c.errorHandler != nil {
		c.errorHandler(err)
		return
	}

	msg := err.Error()
	now := time.Now()

	c.errMu.Lock()
	if msg == c.lastErrMsg && now.Sub(c.lastErrTime) < time.Second {
		c.suppressed++
		c.errMu.Unlock()
		return
	}
	suppressed := c.suppressed
	c.suppressed = 0
	c.lastErrMsg = msg
	c.lastErrTime = now
	c.errMu.Unlock()

	if suppressed > 0 {
		fmt.Fprintf(os.Stderr, "logger: sink write failed (%d similar errors suppressed): %v\n", suppressed, err)
	} else {
		fmt.Fprintf(os.Stderr, "logger: sink write failed: %v\n", err)
	}
}

// GetLevel returns the current minimum log level.
// Returns:
//   - LogLevel: The current log level setting
//
// Use case: Checking the current log level for conditional logging logic.
func (l *Logger) GetLevel() LogLevel {
	return LogLevel(l.core.level.Load())
}

// SetLevel changes the minimum log level.
// Arguments:
//   - level: LogLevel - The new minimum log level
//
// Use case: Dynamically adjusting log verbosity (e.g., via admin API or SIGHUP).
func (l *Logger) SetLevel(level LogLevel) {
	l.core.level.Store(int32(level))
}

// Stats returns counters about entries the logger did not write.
// Returns:
//   - LoggerStats: Dropped (overflow) and sampled entry counts
//
// Use case: Monitoring log loss under OverflowDrop or sampling in production.
func (l *Logger) Stats() LoggerStats {
	stats := LoggerStats{
		DroppedEntries: l.core.dropped.Load(),
	}
	if s := l.core.sampler; s != nil {
		stats.SampledEntries = s.skipped.Load()
	}
	return stats
}

// AddSink adds a new sink to the logger.
// Arguments:
//   - sink: LogSink - The sink to add
//
// The change is visible to every view derived from this logger.
// Use case: Adding log destinations at runtime (e.g., adding file logging after startup).
func (l *Logger) AddSink(sink LogSink) {
	if sink == nil {
		return
	}

	c := l.core
	c.sinksMu.Lock()
	defer c.sinksMu.Unlock()

	// Copy-on-write so concurrent readers never observe a partially
	// mutated slice.
	newSinks := make([]LogSink, len(c.sinks), len(c.sinks)+1)
	copy(newSinks, c.sinks)
	newSinks = append(newSinks, sink)
	c.sinks = newSinks
}

// RemoveSink removes a sink from the logger.
// Arguments:
//   - sink: LogSink - The sink to remove
//
// The change is visible to every view derived from this logger.
// Use case: Removing log destinations at runtime (e.g., disabling file logging).
func (l *Logger) RemoveSink(sink LogSink) {
	c := l.core
	c.sinksMu.Lock()
	defer c.sinksMu.Unlock()

	for i, s := range c.sinks {
		if s == sink {
			// Copy-on-write: never mutate the slice in place, since views
			// and in-flight readers may share the backing array.
			newSinks := make([]LogSink, 0, len(c.sinks)-1)
			newSinks = append(newSinks, c.sinks[:i]...)
			newSinks = append(newSinks, c.sinks[i+1:]...)
			c.sinks = newSinks
			return
		}
	}
}

// With returns a new logger view that includes the given fields on every entry.
// Arguments:
//   - fields: ...Field - Fields added to all entries logged through the view
//
// Returns:
//   - *Logger: A lightweight view sharing sinks and async machinery
//
// Use case: Deriving component- or request-scoped loggers:
//
//	dbLog := logger.With(grlog.String("component", "database"))
func (l *Logger) With(fields ...Field) *Logger {
	if len(fields) == 0 {
		return l
	}
	merged := make([]Field, 0, len(l.contextFields)+len(fields))
	merged = append(merged, l.contextFields...)
	merged = append(merged, fields...)
	return &Logger{core: l.core, contextFields: merged}
}

// WithContext returns a new logger view that includes context-derived fields.
// Arguments:
//   - ctx: context.Context - The context to extract values from
//
// Returns:
//   - *Logger: A new logger view with context fields
//
// Extracts values stored via this package's helpers:
//   - ContextWithRequestID -> "request_id"
//   - ContextWithTraceID   -> "trace_id"
//   - ContextWithUserID    -> "user_id"
//
// Additional extractors registered with WithContextExtractor also run.
// Use case: Adding request-scoped metadata to all logs within a request handler.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	if ctx == nil {
		return l
	}

	contextFields := make([]Field, 0, len(l.contextFields)+3)
	contextFields = append(contextFields, l.contextFields...)

	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		contextFields = append(contextFields, String("request_id", requestID))
	}
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		contextFields = append(contextFields, String("trace_id", traceID))
	}
	if userID, ok := ctx.Value(userIDKey).(string); ok {
		contextFields = append(contextFields, String("user_id", userID))
	}

	for _, extractor := range l.core.extractors {
		contextFields = append(contextFields, extractor(ctx)...)
	}

	return &Logger{core: l.core, contextFields: contextFields}
}

// Close shuts down the logger and closes all sinks.
// Returns:
//   - error: Combined errors from closing all sinks (joined via errors.Join)
//
// Steps:
//  1. Stop async worker and drain queue (no buffered entries are lost)
//  2. Close all sinks
//  3. Aggregate any errors
//
// Closing any view derived via With/WithContext closes the shared logger;
// subsequent Close calls on any view are no-ops.
// Use case: Graceful shutdown to ensure all logs are flushed and resources are released.
func (l *Logger) Close() error {
	c := l.core
	if !c.closed.CompareAndSwap(false, true) {
		return nil // Already closed
	}

	// Stop async worker
	if c.async {
		close(c.closeChan)
		c.wg.Wait()
		// Catch entries enqueued in the race window between a concurrent
		// log()'s closed-check and the CAS above.
		c.drainQueue()
	}

	// Close all sinks
	c.sinksMu.Lock()
	defer c.sinksMu.Unlock()

	var errs []error
	for _, sink := range c.sinks {
		if err := sink.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sink close errors: %w", errors.Join(errs...))
	}
	return nil
}

// shouldSample reports whether an entry at the given level should be kept.
func (c *loggerCore) shouldSample(level LogLevel) bool {
	s := c.sampler
	if s == nil || level > s.maxLevel || level < 0 || int(level) >= numLevels {
		return true
	}
	n := s.counters[level].Add(1)
	if (n-1)%s.every == 0 {
		return true
	}
	s.skipped.Add(1)
	return false
}

// dispatch routes a finished entry to the sinks, honoring async settings.
func (c *loggerCore) dispatch(entry LogEntry) {
	if !c.async || c.queue == nil {
		c.writeSinks(entry)
		return
	}

	switch c.overflow {
	case OverflowBlock:
		select {
		case c.queue <- entry:
		case <-c.closeChan:
			// Shutting down; write directly rather than blocking forever.
			c.writeSinks(entry)
		}
	case OverflowDrop:
		select {
		case c.queue <- entry:
		default:
			c.dropped.Add(1)
		}
	default: // OverflowSyncFallback
		select {
		case c.queue <- entry:
		default:
			// Queue full - log synchronously to avoid loss
			c.writeSinks(entry)
		}
	}
}

// log is the core logging method that creates and dispatches log entries.
// Arguments:
//   - level: LogLevel - The severity level
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// Steps:
//  1. Check if level meets minimum threshold (and sampling, if enabled)
//  2. Build LogEntry with all metadata
//  3. Dispatch to sinks (async or sync)
//
// Use case: Internal method called by all public logging methods.
func (l *Logger) log(level LogLevel, msg string, fields ...Field) {
	c := l.core
	if level < LogLevel(c.level.Load()) {
		return
	}

	if c.closed.Load() {
		return
	}

	if !c.shouldSample(level) {
		return
	}

	// Build entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   msg,
	}

	// Merge view fields with call-site fields. The variadic slice is copied
	// rather than stored, so it never escapes: filtered-out calls above stay
	// completely allocation-free.
	if n := len(l.contextFields) + len(fields); n > 0 {
		merged := make([]Field, 0, n)
		merged = append(merged, l.contextFields...)
		merged = append(merged, fields...)
		entry.Fields = merged
	}

	// Get caller info if enabled
	if c.enableCaller {
		entry.CallerInfo = getCallerInfo(3 + c.callerSkip) // Skip log -> Debug/Info/etc -> actual caller
	}

	c.dispatch(entry)
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

// osExit is indirected for testability of Fatal.
var osExit = os.Exit

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

// Fatal logs a message at FATAL level, closes the logger, and exits with status 1.
// Arguments:
//   - msg: string - The log message
//   - fields: ...Field - Structured fields for context
//
// The logger is closed first so async buffers are flushed before exit.
// Use case: Unrecoverable errors where the process cannot continue.
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.log(FATAL, msg, fields...)
	_ = l.Close()
	osExit(1)
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

// Fatalf logs a formatted message at FATAL level, closes the logger, and exits
// with status 1.
// Arguments:
//   - format: string - Format string (fmt.Printf style)
//   - args: ...interface{} - Arguments for the format string
//
// Use case: Unrecoverable errors where the process cannot continue.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FATAL, fmt.Sprintf(format, args...))
	_ = l.Close()
	osExit(1)
}
