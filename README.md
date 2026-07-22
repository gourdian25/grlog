# 🎃 grlog - Production-Ready Structured Logging for Go

[![CI](https://github.com/gourdian25/grlog/actions/workflows/ci.yml/badge.svg)](https://github.com/gourdian25/grlog/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/gourdian25/grlog.svg)](https://pkg.go.dev/github.com/gourdian25/grlog)
[![Go Version](https://img.shields.io/badge/go-1.26.4+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

A high-performance structured logging library for Go with pluggable sinks, formatters, and advanced features like async logging, file rotation, a `log/slog` adapter, and comprehensive race condition safety.

## 🌐 Part of the gourdian25 ecosystem

grlog is one of several small, independent Go libraries meant to be used
together — it's the recommended logging choice satisfied structurally (no
adapter code) by every other repo's optional `Logger` interface:

- [gourdiantoken](https://github.com/gourdian25/gourdiantoken) — JWT
  access/refresh token issuance, verification, revocation, and rotation.
- [grcache](https://github.com/gourdian25/grcache) — backend-agnostic
  caching abstraction (Redis, Postgres, Mongo, memcached, in-memory).
- [grevents](https://github.com/gourdian25/grevents) — an in-process event
  bus for decoupling producers of state changes from consumers that react
  to them.
- [graudit](https://github.com/gourdian25/graudit) — an append-only,
  tamper-evident audit log with pluggable storage backends.
- [grpolicy](https://github.com/gourdian25/grpolicy) — attribute-based
  policy evaluation (RBAC/ABAC), independent of any notion of "user" or
  "role".

## 🌟 Why grlog?

grlog is built for **production systems** that demand:

- ⚡ **Blazing Fast**: Zero-allocation typed fields, zero-allocation formatting via pooled buffers, ~2.7ns filtered-out calls
- 🛡️ **Production-Safe**: Extensive race detection tests, panic-free shutdown, no lost logs on Close
- 🔧 **Modular Design**: Pluggable sinks (writer, file, multi, leveled, custom) and formatters (plain text, JSON)
- 🚀 **Cloud-Native**: Deterministic JSON output, `log/slog` handler, typed context keys for request tracing
- 📦 **Batteries Included**: File rotation with compression, sampling, overflow policies, caller info, dynamic configuration

### Performance Highlights

Measured on Apple M-series (Go 1.26); run `make bench` for your hardware:

```
Field Construction (zero allocation):
- String field:        0.22 ns/op    0 B/op    0 allocs/op
- Int field:           0.23 ns/op    0 B/op    0 allocs/op
- Mixed 10 fields:     0.22 ns/op    0 B/op    0 allocs/op

Logging Performance:
- Sync log, no fields:  125 ns/op      0 B/op   0 allocs/op
- Sync log, 3 fields:   177 ns/op    128 B/op   1 allocs/op
- Async log, no fields: 159 ns/op      0 B/op   0 allocs/op
- Level filtered out:   2.7 ns/op      0 B/op   0 allocs/op

Formatting (pooled buffers):
- Plain, 3 fields:      109 ns/op      0 B/op   0 allocs/op
- JSON, 3 fields:       205 ns/op      4 B/op   1 allocs/op

Real-World Scenarios:
- HTTP request log:     563 ns/op    324 B/op   2 allocs/op
- Error log:            1.2 μs/op    600 B/op   8 allocs/op
- Business event:       945 ns/op    581 B/op   2 allocs/op
```

## 📚 Table of Contents

- [Installation](#-installation)
- [Quick Start](#-quick-start)
- [Core Concepts](#-core-concepts)
- [Log Levels](#-log-levels)
- [Structured Logging](#-structured-logging)
- [All Available Sinks](#-all-available-sinks)
- [All Available Formatters](#-all-available-formatters)
- [Complete Configuration Examples](#-complete-configuration-examples)
- [Advanced Usage](#-advanced-usage)
- [Production Examples](#-production-examples)
- [Best Practices](#-best-practices)
- [Performance & Benchmarks](#-performance--benchmarks)
- [Testing](#-testing)
- [Contributing](#-contributing)
- [License](#-license)

## 📦 Installation

```bash
go get github.com/gourdian25/grlog
```

**Requirements**: Go 1.26.4+ (ecosystem-aligned minimum; `log/slog` interoperability and `errors.Join` only actually need 1.21+)

## 🚀 Quick Start

### Basic Usage

```go
package main

import (
    "github.com/gourdian25/grlog"
)

func main() {
    // Create logger with sensible defaults
    logger := grlog.NewDefaultLogger()
    defer logger.Close() // Important: flushes async buffers
    
    logger.Info("Application started")
    logger.Debug("Initializing components...")
    logger.Warn("High memory usage detected")
    logger.Error("Failed to connect", grlog.Err(err))
}
```

### Structured Logging

```go
logger.Info("User authenticated",
    grlog.String("user_id", "12345"),
    grlog.String("email", "user@example.com"),
    grlog.Int("attempts", 3),
    grlog.Bool("success", true),
    grlog.Duration("latency", 45*time.Millisecond),
)
```

**Output (Plain Text)**:
```
2025-12-17 06:45:58.801 [INFO] main.go:42:main User authenticated {user_id=12345, email=user@example.com, attempts=3, success=true, latency=45ms}
```

## 🎯 Core Concepts

### Architecture

```
Logger → LogEntry → Formatter → LogSink
```

1. **Logger**: Main interface, creates LogEntry objects
2. **LogEntry**: Canonical structure with timestamp, level, message, fields
3. **Formatter**: Converts LogEntry to bytes (PlainFormatter, JSONFormatter)
4. **LogSink**: Writes formatted bytes to destination (StdoutSink, FileSink, MultiSink, CustomSink)

### Log Entry Lifecycle

```go
// 1. Logger creates entry with metadata
entry := LogEntry{
    Timestamp:  time.Now(),
    Level:      INFO,
    Message:    "Request processed",
    Fields:     []Field{String("user", "alice")},
    CallerInfo: "handler.go:42:ProcessRequest",
}

// 2. Formatter converts to bytes
bytes := formatter.Format(entry)

// 3. Sink writes to destination
sink.Write(entry)
```

## 📶 Log Levels

grlog provides 5 log levels with atomic level changes, plus `OFF` to disable output:

| Level | When to Use | Example |
|-------|-------------|---------|
| 🐛 **DEBUG** | Detailed diagnostic information | `logger.Debug("Cache hit", String("key", key))` |
| ℹ️ **INFO** | General operational messages | `logger.Info("Server started on :8080")` |
| ⚠️ **WARN** | Warning conditions | `logger.Warn("High memory usage", Int("percent", 85))` |
| ❌ **ERROR** | Error conditions | `logger.Error("Database error", Err(err))` |
| 💀 **FATAL** | Unrecoverable errors | `logger.Fatal("Cannot bind port", Err(err))` — flushes, closes, then `os.Exit(1)` |
| 🔇 **OFF** | Disable all logging | `logger.SetLevel(grlog.OFF)` |

### Dynamic Level Control

```go
// Set level at runtime (thread-safe)
logger.SetLevel(grlog.WARN)

// Get current level
level := logger.GetLevel()

// Parse from string ("debug", "info", "warn", "error", "fatal", "off")
level, err := grlog.ParseLogLevel("info")
```

## 📝 Structured Logging

### All Available Typed Fields (Zero Allocation)

```go
// String fields
grlog.String("key", "value")

// Numeric fields
grlog.Int("count", 42)
grlog.Int64("size", 1234567890)
grlog.Uint64("offset", 987654321)
grlog.Float64("ratio", 0.75)

// Boolean fields
grlog.Bool("success", true)

// Time fields
grlog.Duration("latency", 45*time.Millisecond)
grlog.Time("expires_at", expiry) // RFC3339Nano

// Error fields (nil-safe: Err(nil) is skipped entirely in output)
grlog.Err(err)  // Automatically uses "error" as key

// Any type (fallback, uses reflection)
grlog.Any("data", complexStruct)
```

### Complete Field Usage Example

```go
logger.Info("Transaction completed",
    // Strings
    grlog.String("transaction_id", "tx_789012"),
    grlog.String("user_id", "user_12345"),
    grlog.String("currency", "USD"),
    
    // Numbers
    grlog.Int("status_code", 200),
    grlog.Int64("amount_cents", 4999),
    
    // Booleans
    grlog.Bool("verified", true),
    grlog.Bool("fraud_check_passed", true),
    
    // Duration
    grlog.Duration("processing_time", 142*time.Millisecond),
    
    // Error (if any)
    grlog.Err(validationErr),
    
    // Complex types
    grlog.Any("metadata", transactionMetadata),
)
```

### Formatted Logging Methods

```go
// Printf-style formatting for all levels
logger.Debugf("Cache stats: hit=%d miss=%d ratio=%.2f", hits, misses, ratio)
logger.Infof("Processing %s request from %s", method, ip)
logger.Warnf("Memory usage at %d%% (threshold: %d%%)", current, threshold)
logger.Errorf("Failed after %d attempts: %v", retries, err)
```

## 🔌 All Available Sinks

### 1. StdoutSink - Console/Terminal Output

Perfect for development and containerized environments where stdout is captured.

```go
// Basic stdout sink with plain text
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())

logger := grlog.NewLogger(
    grlog.WithSink(stdoutSink),
    grlog.WithLevel(grlog.INFO),
)
```

```go
// Stdout sink with JSON format (for log processors)
stdoutSink := grlog.NewStdoutSink(grlog.JSONFormat())

logger := grlog.NewLogger(
    grlog.WithSink(stdoutSink),
)
```

```go
// Stdout sink with custom formatter
customFormatter := &grlog.PlainFormatter{
    TimestampFormat: "2006-01-02 15:04:05.000",
}
stdoutSink := grlog.NewStdoutSink(customFormatter)

// Or write to any io.Writer (stderr, buffers in tests, sockets, ...)
stderrSink := grlog.NewWriterSink(os.Stderr, customFormatter)
```

**Use Cases:**
- ✅ Development and debugging
- ✅ Docker/Kubernetes (logs collected from stdout)
- ✅ Cloud-native applications
- ✅ CI/CD pipelines

---

### 2. FileSink - File-Based Logging with Rotation

Writes logs to files with automatic rotation based on size.

```go
// Basic file sink
config := grlog.FileSinkConfig{
    Filename:    "myapp",                  // Creates myapp.log
    Dir:         "logs",                   // In ./logs directory
    MaxBytes:    10 * 1024 * 1024,         // Rotate at 10MB
    BackupCount: 5,                        // Keep 5 backups
    Formatter:   grlog.PlainFormat(),
}

fileSink, err := grlog.NewFileSink(config)
if err != nil {
    log.Fatal(err)
}

logger := grlog.NewLogger(
    grlog.WithSink(fileSink),
)
```

```go
// Production file sink with JSON, compression, and age-based cleanup
config := grlog.FileSinkConfig{
    Filename:    "production",
    Dir:         "/var/log/myapp",
    MaxBytes:    100 * 1024 * 1024,        // 100MB files
    BackupCount: 20,                       // Keep 20 backups
    MaxAge:      30 * 24 * time.Hour,      // Also remove backups older than 30 days
    Compress:    true,                     // Gzip rotated backups in the background
    Formatter:   grlog.JSONFormat(),
}

fileSink, err := grlog.NewFileSink(config)
```

Rotation is crash-safe: backup names are collision-proof (nanosecond
timestamps plus a sequence suffix), and if a rotation fails the sink reopens
the current file and keeps writing — log entries are never dropped because of
a rotation error.

```go
// High-volume application configuration
config := grlog.FileSinkConfig{
    Filename:    "highvolume",
    Dir:         "/mnt/logs",
    MaxBytes:    500 * 1024 * 1024,        // 500MB files
    BackupCount: 50,                       // Keep 50 backups (25GB total)
    Formatter:   grlog.JSONFormat(),
}

fileSink, err := grlog.NewFileSink(config)
```

```go
// Minimal configuration (uses defaults)
config := grlog.FileSinkConfig{
    Filename: "app",
}
// Defaults:
// - Dir: "logs"
// - MaxBytes: 10MB
// - BackupCount: 5
// - Formatter: PlainFormat()

fileSink, err := grlog.NewFileSink(config)
```

**File Naming Convention:**
- `myapp.log` (current log file)
- `myapp_20251217_064558.log` (rotated backup)
- `myapp_20251216_120000.log` (older backup)

**Use Cases:**
- ✅ Traditional server applications
- ✅ Audit logging (compliance requirements)
- ✅ Offline log analysis
- ✅ Long-term log retention

---

### 3. MultiSink - Multiple Simultaneous Destinations

Fan-out logs to multiple sinks at once.

```go
// Console + File (common development setup)
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())
fileSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename: "app",
})

multiSink := grlog.NewMultiSink(stdoutSink, fileSink)

logger := grlog.NewLogger(
    grlog.WithSink(multiSink),
)
```

```go
// Console + Audit File + Error File
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())

auditSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename: "audit",
    Dir:      "/var/log/audit",
    Formatter: grlog.JSONFormat(),
})

errorSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename: "errors",
    Dir:      "/var/log/errors",
    Formatter: grlog.JSONFormat(),
})

multiSink := grlog.NewMultiSink(stdoutSink, auditSink, errorSink)
```

```go
// All sink types combined
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())
fileSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{Filename: "app"})
customSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    // Send to external service
    return nil
})

multiSink := grlog.NewMultiSink(stdoutSink, fileSink, customSink)
```

**Error Handling:**
- Continues writing to all sinks even if some fail
- Returns combined errors from all failed sinks

**Use Cases:**
- ✅ Development (console + file)
- ✅ Compliance (multiple audit trails)
- ✅ Hybrid environments (local + remote)
- ✅ Redundant logging

---

### 4. CustomSink - Your Own Log Destinations

Implement any custom logging logic.

#### MongoDB Example

```go
// MongoDB sink
type MongoDBSink struct {
    collection *mongo.Collection
}

func NewMongoDBSink(client *mongo.Client, database, collection string) *MongoDBSink {
    return &MongoDBSink{
        collection: client.Database(database).Collection(collection),
    }
}

mongoSink := grlog.NewCustomSinkWithClose(
    func(entry grlog.LogEntry) error {
        doc := bson.M{
            "timestamp": entry.Timestamp,
            "level":     entry.Level.String(),
            "message":   entry.Message,
            "caller":    entry.CallerInfo,
        }
        
        // Add fields
        for _, field := range entry.Fields {
            doc[field.Key] = field.Value
        }
        
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        
        _, err := mongoSink.collection.InsertOne(ctx, doc)
        return err
    },
    func() error {
        // Cleanup if needed
        return nil
    },
)

logger := grlog.NewLogger(
    grlog.WithSink(mongoSink),
)
```

#### GORM/Database Example

```go
// GORM sink for structured log storage
type LogEntry struct {
    ID        uint      `gorm:"primaryKey"`
    Timestamp time.Time `gorm:"index"`
    Level     string    `gorm:"index"`
    Message   string    `gorm:"type:text"`
    Fields    string    `gorm:"type:json"`
    Caller    string
}

type GORMSink struct {
    db *gorm.DB
}

gormSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    // Convert fields to JSON
    fieldsJSON, _ := json.Marshal(entry.Fields)
    
    logEntry := LogEntry{
        Timestamp: entry.Timestamp,
        Level:     entry.Level.String(),
        Message:   entry.Message,
        Fields:    string(fieldsJSON),
        Caller:    entry.CallerInfo,
    }
    
    return db.Create(&logEntry).Error
})

logger := grlog.NewLogger(
    grlog.WithSink(gormSink),
)
```

#### Network/TCP Sink Example

```go
// TCP sink (e.g., Logstash, Fluentd)
type TCPSink struct {
    conn net.Conn
    mu   sync.Mutex
}

func NewTCPSink(address string) (*TCPSink, error) {
    conn, err := net.Dial("tcp", address)
    if err != nil {
        return nil, err
    }
    return &TCPSink{conn: conn}, nil
}

tcpSink, err := NewTCPSink("logs.example.com:514")
if err != nil {
    log.Fatal(err)
}

networkSink := grlog.NewCustomSinkWithClose(
    func(entry grlog.LogEntry) error {
        tcpSink.mu.Lock()
        defer tcpSink.mu.Unlock()
        
        // Format as JSON
        formatter := grlog.JSONFormat()
        data := formatter.Format(entry)
        
        _, err := tcpSink.conn.Write(data)
        return err
    },
    func() error {
        return tcpSink.conn.Close()
    },
)

logger := grlog.NewLogger(
    grlog.WithSink(networkSink),
)
```

#### HTTP/Webhook Sink Example

```go
// HTTP webhook sink
type WebhookSink struct {
    url    string
    client *http.Client
}

webhookSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    payload := map[string]interface{}{
        "timestamp": entry.Timestamp,
        "level":     entry.Level.String(),
        "message":   entry.Message,
        "fields":    entry.Fields,
    }
    
    jsonData, _ := json.Marshal(payload)
    
    resp, err := http.Post(
        "https://hooks.example.com/log",
        "application/json",
        bytes.NewBuffer(jsonData),
    )
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return fmt.Errorf("webhook failed: %d", resp.StatusCode)
    }
    return nil
})
```

#### Redis/Queue Sink Example

```go
// Redis pub/sub sink
type RedisSink struct {
    client *redis.Client
    channel string
}

redisSink := grlog.NewCustomSinkWithClose(
    func(entry grlog.LogEntry) error {
        formatter := grlog.JSONFormat()
        data := formatter.Format(entry)
        
        return redisClient.Publish(
            context.Background(),
            "logs:channel",
            string(data),
        ).Err()
    },
    func() error {
        return redisClient.Close()
    },
)
```

#### Elasticsearch Sink Example

```go
// Elasticsearch sink
type ElasticsearchSink struct {
    client *elasticsearch.Client
    index  string
}

esSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    doc := map[string]interface{}{
        "timestamp": entry.Timestamp,
        "level":     entry.Level.String(),
        "message":   entry.Message,
        "caller":    entry.CallerInfo,
    }
    
    for _, field := range entry.Fields {
        doc[field.Key] = field.Value
    }
    
    data, _ := json.Marshal(doc)
    
    res, err := esClient.Index(
        "logs",
        bytes.NewReader(data),
    )
    if err != nil {
        return err
    }
    defer res.Body.Close()
    
    return nil
})
```

#### Metrics/Monitoring Sink Example

```go
// Prometheus/StatsD metrics sink
metricsSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    // Increment counters based on log level
    switch entry.Level {
    case grlog.ERROR:
        errorCounter.Inc()
    case grlog.WARN:
        warnCounter.Inc()
    }
    
    // Extract custom metrics from fields
    for _, field := range entry.Fields {
        if field.Key == "duration" && field.Type == grlog.DurationType {
            duration := field.Value.(time.Duration)
            latencyHistogram.Observe(duration.Seconds())
        }
    }
    
    return nil
})
```

#### Syslog Sink Example

```go
// Syslog sink
syslogWriter, err := syslog.New(syslog.LOG_INFO|syslog.LOG_LOCAL0, "myapp")
if err != nil {
    log.Fatal(err)
}

syslogSink := grlog.NewCustomSinkWithClose(
    func(entry grlog.LogEntry) error {
        formatter := grlog.PlainFormat()
        data := formatter.Format(entry)
        
        switch entry.Level {
        case grlog.ERROR:
            return syslogWriter.Err(string(data))
        case grlog.WARN:
            return syslogWriter.Warning(string(data))
        case grlog.INFO:
            return syslogWriter.Info(string(data))
        case grlog.DEBUG:
            return syslogWriter.Debug(string(data))
        }
        return nil
    },
    func() error {
        return syslogWriter.Close()
    },
)
```

#### Simple CustomSink (No Cleanup Needed)

```go
// Simple custom sink without close function
simpleSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    // Your custom logic here
    fmt.Printf("[CUSTOM] %s: %s\n", entry.Level, entry.Message)
    return nil
})
```

**Use Cases for CustomSink:**
- ✅ Database logging (MongoDB, PostgreSQL, MySQL)
- ✅ Message queues (Kafka, RabbitMQ, Redis)
- ✅ Log aggregation (Elasticsearch, Splunk, Datadog)
- ✅ Monitoring/metrics (Prometheus, StatsD, New Relic)
- ✅ Webhooks and HTTP endpoints
- ✅ Cloud services (AWS CloudWatch, Google Cloud Logging)
- ✅ Custom business logic

---

### 5. LeveledSink - Per-Destination Severity Routing

Wraps any sink and drops entries below a minimum level. This is how you send
everything to stdout but only errors to a file (or an alerting sink):

```go
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())
errorFile, _ := grlog.NewFileSink(grlog.FileSinkConfig{Filename: "errors"})

logger := grlog.NewLogger(
    grlog.WithSink(grlog.NewMultiSink(
        stdoutSink,                              // receives everything
        grlog.NewLeveledSink(errorFile, grlog.ERROR), // ERROR and FATAL only
    )),
)
```

---

### 6. WriterSink - Any io.Writer

```go
// stderr, network connections, in-memory buffers in tests...
sink := grlog.NewWriterSink(os.Stderr, grlog.JSONFormat())

// Perfect for asserting on log output in tests:
var buf bytes.Buffer
testLogger := grlog.NewLogger(grlog.WithSink(grlog.NewWriterSink(&buf, grlog.PlainFormat())))
```

---

## 🎨 All Available Formatters

### 1. PlainFormatter - Human-Readable Text

```go
// Default plain formatter
formatter := grlog.PlainFormat()
```

```go
// Custom plain formatter
formatter := &grlog.PlainFormatter{
    TimestampFormat: "15:04:05", // Time only
}
```

Caller info is controlled on the logger via `grlog.WithCaller(false)`;
formatters render it whenever the entry carries it.

**Output Format:**
```
2025-12-17 06:45:58.801 [INFO] main.go:42:main User authenticated {user_id=12345, email=user@example.com, attempts=3}
```

**Use Cases:**
- ✅ Development and debugging
- ✅ Console output
- ✅ Human reading
- ✅ Grep-friendly logs

---

### 2. JSONFormatter - Machine-Readable Structured Logs

```go
// Default JSON formatter
formatter := grlog.JSONFormat()
```

```go
// JSON formatter with custom timestamp
formatter := &grlog.JSONFormatter{
    TimestampFormat: time.RFC3339Nano,
    PrettyPrint:     false,
}
```

```go
// Pretty-printed JSON (for debugging)
formatter := &grlog.JSONFormatter{
    TimestampFormat: time.RFC3339,
    PrettyPrint:     true,
}
```

```go
// JSON with custom fields (added to every log)
formatter := &grlog.JSONFormatter{
    TimestampFormat: time.RFC3339Nano,
    CustomFields: map[string]interface{}{
        "service":     "user-api",
        "version":     "1.0.0",
        "environment": "production",
        "host":        hostname,
    },
}
```

**Output Format (compact):**
```json
{"timestamp":"2025-12-17T06:45:58.801Z","level":"INFO","message":"Request processed","caller":"handler.go:42:ProcessRequest","user_id":"12345","status":200,"latency":"45.2ms"}
```

**Output Format (pretty):**
```json
{
  "timestamp": "2025-12-17T06:45:58.801Z",
  "level": "INFO",
  "message": "Request processed",
  "caller": "handler.go:42:ProcessRequest",
  "user_id": "12345",
  "status": 200,
  "latency": "45.2ms"
}
```

**Use Cases:**
- ✅ Log aggregation systems (ELK, Splunk)
- ✅ Structured log analysis
- ✅ Machine parsing
- ✅ Production environments

---

## ⚙️ Complete Configuration Examples

### 1. Minimal Configuration (Quick Start)

```go
// Absolute minimum - uses all defaults
logger := grlog.NewDefaultLogger()
defer logger.Close()
```

**Defaults:**
- Level: INFO
- Sink: StdoutSink
- Format: Plain text
- Async: Disabled (synchronous)
- Caller info: Enabled

---

### 2. Development Configuration

```go
// Console logging with debug level
logger := grlog.NewLogger(
    grlog.WithLevel(grlog.DEBUG),
    grlog.WithSink(
        grlog.NewStdoutSink(grlog.PlainFormat()),
    ),
    grlog.WithCaller(true),
)
defer logger.Close()
```

**Best for:**
- Local development
- Debugging
- Testing

---

### 3. Production Configuration (Single File)

```go
// Production file logging with rotation
fileSink, err := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename:    "production",
    Dir:         "/var/log/myapp",
    MaxBytes:    100 * 1024 * 1024,  // 100MB
    BackupCount: 20,
    Formatter:   grlog.JSONFormat(),
})
if err != nil {
    log.Fatal(err)
}

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(fileSink),
    grlog.WithAsync(10000),
    grlog.WithCaller(false),  // Better performance
    grlog.WithContextFields(
        grlog.String("service", "user-api"),
        grlog.String("version", "1.0.0"),
        grlog.String("environment", "production"),
    ),
)
defer logger.Close()
```

**Best for:**
- Production servers
- VMs and bare metal
- Audit requirements

---

### 4. Cloud-Native Configuration (Stdout JSON)

```go
// Kubernetes/Docker optimized
logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(
        grlog.NewStdoutSink(grlog.JSONFormat()),
    ),
    grlog.WithAsync(5000),
    grlog.WithCaller(false),
    grlog.WithContextFields(
        grlog.String("service", os.Getenv("SERVICE_NAME")),
        grlog.String("pod", os.Getenv("POD_NAME")),
        grlog.String("namespace", os.Getenv("NAMESPACE")),
    ),
)
defer logger.Close()
```

**Best for:**
- Docker containers
- Kubernetes pods
- Cloud platforms (AWS, GCP, Azure)

---

### 5. High-Performance Configuration

```go
// Optimized for maximum throughput
logger := grlog.NewLogger(
    grlog.WithLevel(grlog.WARN),  // Minimal logging
    grlog.WithSink(
        grlog.NewStdoutSink(grlog.PlainFormat()),
    ),
    grlog.WithAsync(50000),  // Large buffer
    grlog.WithCaller(false), // No caller overhead
)
defer logger.Close()
```

**Best for:**
- High-throughput services
- Performance-critical paths
- Latency-sensitive applications

---

### 6. Multi-Destination Configuration

```go
// Console + File + Custom (comprehensive logging)
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())

fileSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename:  "audit",
    Dir:       "/var/log/audit",
    MaxBytes:  50 * 1024 * 1024,
    Formatter: grlog.JSONFormat(),
})

metricsSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    // Send metrics
    if entry.Level >= grlog.ERROR {
        metrics.IncrementErrorCount()
    }
    return nil
})

multiSink := grlog.NewMultiSink(stdoutSink, fileSink, metricsSink)

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(multiSink),
    grlog.WithAsync(10000),
)
defer logger.Close()
```

**Best for:**
- Complex logging requirements
- Compliance and audit
- Hybrid environments

---

### 7. Context-Aware Configuration

```go
// Request-scoped logging
baseLogger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(
        grlog.NewStdoutSink(grlog.JSONFormat()),
    ),
    grlog.WithContextFields(
        grlog.String("service", "api"),
        grlog.String("version", "2.1.0"),
    ),
)
defer baseLogger.Close()

// In request handler
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := grlog.ContextWithRequestID(r.Context(), generateRequestID())
    ctx = grlog.ContextWithUserID(ctx, getUserID(r))

    requestLogger := baseLogger.WithContext(ctx)
    requestLogger.Info("Request started")
    // Automatically includes request_id and user_id
}
```

**Best for:**
- Web applications
- Microservices
- Distributed tracing

---

### 8. Database Logging Configuration

```go
// MongoDB sink
mongoClient, _ := mongo.Connect(context.Background(), options.Client().ApplyURI("mongodb://localhost:27017"))
mongoSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    collection := mongoClient.Database("logs").Collection("application")
    
    doc := bson.M{
        "timestamp": entry.Timestamp,
        "level":     entry.Level.String(),
        "message":   entry.Message,
    }
    for _, field := range entry.Fields {
        doc[field.Key] = field.Value
    }
    
    _, err := collection.InsertOne(context.Background(), doc)
    return err
})

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(mongoSink),
)
defer logger.Close()
```

**Best for:**
- Structured log storage
- Advanced querying
- Long-term analysis

---

### 9. Network/Remote Logging Configuration

```go
// TCP sink to log aggregator
tcpConn, _ := net.Dial("tcp", "logstash.example.com:5000")
networkSink := grlog.NewCustomSinkWithClose(
    func(entry grlog.LogEntry) error {
        formatter := grlog.JSONFormat()
        data := formatter.Format(entry)
        _, err := tcpConn.Write(data)
        return err
    },
    func() error {
        return tcpConn.Close()
    },
)

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(networkSink),
    grlog.WithAsync(5000),  // Important for network latency
)
defer logger.Close()
```

**Best for:**
- Centralized logging
- Log aggregation systems
- Remote monitoring

---

### 10. Testing Configuration

```go
// Custom sink for capturing logs in tests
var capturedLogs []grlog.LogEntry
testSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    capturedLogs = append(capturedLogs, entry)
    return nil
})

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.DEBUG),
    grlog.WithSink(testSink),
    grlog.WithCaller(false),  // Cleaner test output
)
defer logger.Close()

// In your test
func TestMyFunction(t *testing.T) {
    capturedLogs = nil  // Reset
    
    myFunction(logger)
    
    if len(capturedLogs) != 1 {
        t.Errorf("Expected 1 log, got %d", len(capturedLogs))
    }
}
```

**Best for:**
- Unit testing
- Integration testing
- Log verification

---

### 11. Separate Error Logging Configuration

```go
// Separate files for different log levels
infoFileSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename: "info",
    Dir:      "logs",
    Formatter: grlog.PlainFormat(),
})

errorFileSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename: "errors",
    Dir:      "logs",
    Formatter: grlog.JSONFormat(),
})

// Filter errors to separate file with custom sink
errorOnlySink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    if entry.Level >= grlog.ERROR {
        return errorFileSink.Write(entry)
    }
    return nil
})

multiSink := grlog.NewMultiSink(infoFileSink, errorOnlySink)

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(multiSink),
)
defer logger.Close()
```

**Best for:**
- Error tracking
- Audit requirements
- Compliance

---

### 12. Complete Production Configuration

```go
// Production-ready with all features
hostname, _ := os.Hostname()
version := "1.2.3"

fileSink, err := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename:    "production",
    Dir:         "/var/log/myapp",
    MaxBytes:    100 * 1024 * 1024,
    BackupCount: 30,
    Formatter: &grlog.JSONFormatter{
        TimestampFormat: time.RFC3339Nano,
        CustomFields: map[string]interface{}{
            "service":     "user-api",
            "version":     version,
            "environment": os.Getenv("ENV"),
            "host":        hostname,
            "pid":         os.Getpid(),
        },
    },
})
if err != nil {
    log.Fatalf("Failed to create file sink: %v", err)
}

// Also log to stdout for container logs
stdoutSink := grlog.NewStdoutSink(grlog.PlainFormat())

// Metrics integration
metricsSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
    logCounter.WithLabelValues(entry.Level.String()).Inc()
    return nil
})

multiSink := grlog.NewMultiSink(fileSink, stdoutSink, metricsSink)

logger := grlog.NewLogger(
    grlog.WithLevel(grlog.INFO),
    grlog.WithSink(multiSink),
    grlog.WithAsync(20000),
    grlog.WithCaller(false),
    grlog.WithContextFields(
        grlog.String("service", "user-api"),
        grlog.String("version", version),
    ),
)
defer logger.Close()

// Graceful shutdown
c := make(chan os.Signal, 1)
signal.Notify(c, os.Interrupt, syscall.SIGTERM)
go func() {
    <-c
    logger.Close()
    os.Exit(0)
}()
```

**Features:**
- File + console logging
- Metrics integration
- Custom fields
- Graceful shutdown
- Environment-aware

---

## 🔥 Advanced Usage

### Asynchronous Logging

```go
// Enable async with buffer
logger := grlog.NewLogger(
    grlog.WithAsync(10000),  // 10,000 entry buffer
)
defer logger.Close()  // Always close to drain queue

// Logs are buffered and written by background worker
logger.Info("This is non-blocking")

// If buffer fills, automatically falls back to sync (no loss)
```

**Key Points:**
- ✅ Buffer overflow behavior is configurable via `WithOverflowPolicy`:
  `OverflowSyncFallback` (default, never loses entries), `OverflowBlock`
  (strict ordering), or `OverflowDrop` (never blocks; drops are counted in
  `logger.Stats().DroppedEntries`)
- ✅ `Close()` drains the queue — no buffered entries are lost

---

### Context-Aware Logging

Context values are stored under private typed keys via helper functions, so
they can never collide with other packages' context values:

```go
// Base logger
baseLogger := grlog.NewDefaultLogger()

// Store request metadata with the typed helpers
ctx := grlog.ContextWithRequestID(context.Background(), "req-123")
ctx = grlog.ContextWithTraceID(ctx, "trace-456")
ctx = grlog.ContextWithUserID(ctx, "user-789")

// Create context logger
requestLogger := baseLogger.WithContext(ctx)

// All logs include context fields
requestLogger.Info("Processing request")
// Output includes: request_id=req-123, trace_id=trace-456, user_id=user-789
```

**Built-in Context Helpers:**
- `grlog.ContextWithRequestID` → `request_id`
- `grlog.ContextWithTraceID` → `trace_id`
- `grlog.ContextWithUserID` → `user_id`

**Custom extraction** (e.g. OpenTelemetry span IDs):

```go
logger := grlog.NewLogger(
    grlog.WithContextExtractor(func(ctx context.Context) []grlog.Field {
        if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
            return []grlog.Field{
                grlog.String("otel_trace_id", span.SpanContext().TraceID().String()),
            }
        }
        return nil
    }),
)
```

---

### Dynamic Sink Management

```go
logger := grlog.NewDefaultLogger()

// Add sink at runtime
newSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
    Filename: "runtime",
})
logger.AddSink(newSink)

// Remove sink at runtime
logger.RemoveSink(oldSink)
```

---

### Derived Loggers with With()

`With` returns a lightweight view that adds permanent fields; views share
sinks and the async machinery, and closing any view closes the logger exactly
once:

```go
logger := grlog.NewDefaultLogger()
defer logger.Close()

dbLog := logger.With(grlog.String("component", "database"))
cacheLog := logger.With(grlog.String("component", "cache"))

dbLog.Info("query executed", grlog.Duration("took", 3*time.Millisecond))
// ... component=database is on every entry
```

---

### log/slog Interoperability

grlog can act as the backend for the standard library's `log/slog`, so
third-party code that logs via slog flows through your grlog sinks:

```go
logger := grlog.NewLogger(
    grlog.WithSink(grlog.NewStdoutSink(grlog.JSONFormat())),
)
defer logger.Close()

slog.SetDefault(slog.New(grlog.NewSlogHandler(logger)))

slog.Info("from slog", "status", 200) // rendered by grlog's JSON formatter
```

Groups become dotted keys (`req.status`), `slog.Logger.With` attributes are
preserved, and levels map Debug→DEBUG, Info→INFO, Warn→WARN, Error→ERROR.

---

### Sampling High-Volume Logs

Keep 1 in N entries for noisy low-severity levels; higher levels are never
sampled:

```go
logger := grlog.NewLogger(
    grlog.WithSampler(100, grlog.INFO), // keep 1 in 100 DEBUG/INFO entries
)

// Observability:
skipped := logger.Stats().SampledEntries
```

---

### Sink Failure Handling

By default, sink write failures are reported to stderr (rate-limited so a
dead sink cannot flood it). For production, route them to your metrics:

```go
logger := grlog.NewLogger(
    grlog.WithSink(remoteSink),
    grlog.WithErrorHandler(func(err error) {
        sinkErrorCounter.Inc() // do NOT log through the same logger here
    }),
)
```

---

### Caller Info in Wrappers

If you wrap grlog in your own helper package, add caller skip so log lines
point at your application code instead of the wrapper:

```go
logger := grlog.NewLogger(
    grlog.WithCaller(true),
    grlog.WithCallerSkip(1), // one extra frame for your wrapper function
)
```

---

## 🏭 Production Examples

### 1. Complete Microservice Setup

```go
package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    
    "github.com/gourdian25/grlog"
)

func main() {
    // Initialize logger
    logger := setupLogger()
    defer logger.Close()
    
    logger.Info("Service starting",
        grlog.String("version", version),
        grlog.String("env", os.Getenv("ENV")),
    )
    
    // Setup HTTP server
    mux := http.NewServeMux()
    mux.HandleFunc("/health", healthCheckHandler(logger))
    mux.HandleFunc("/api/users", usersHandler(logger))
    
    server := &http.Server{
        Addr:    ":8080",
        Handler: loggingMiddleware(logger)(mux),
    }
    
    // Start server
    go func() {
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("Server failed", grlog.Err(err))
        }
    }()
    
    logger.Info("Server started on :8080")
    
    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
    <-quit
    
    logger.Info("Shutting down server...")
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    if err := server.Shutdown(ctx); err != nil {
        logger.Error("Server forced to shutdown", grlog.Err(err))
    }
    
    logger.Info("Server exited")
}

func setupLogger() *grlog.Logger {
    hostname, _ := os.Hostname()
    
    fileSink, _ := grlog.NewFileSink(grlog.FileSinkConfig{
        Filename:    "service",
        Dir:         "/var/log/myservice",
        MaxBytes:    100 * 1024 * 1024,
        BackupCount: 20,
        Formatter:   grlog.JSONFormat(),
    })
    
    stdoutSink := grlog.NewStdoutSink(grlog.JSONFormat())
    
    return grlog.NewLogger(
        grlog.WithLevel(grlog.INFO),
        grlog.WithSink(grlog.NewMultiSink(fileSink, stdoutSink)),
        grlog.WithAsync(10000),
        grlog.WithContextFields(
            grlog.String("service", "user-api"),
            grlog.String("host", hostname),
        ),
    )
}

func loggingMiddleware(logger *grlog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Generate request ID
            requestID := generateRequestID()
            ctx := grlog.ContextWithRequestID(r.Context(), requestID)
            
            // Create request-scoped logger
            reqLogger := logger.WithContext(ctx)
            
            // Wrap response writer
            wrapped := &responseWriter{ResponseWriter: w, statusCode: 200}
            
            // Process request
            next.ServeHTTP(wrapped, r.WithContext(ctx))
            
            // Log request
            reqLogger.Info("HTTP request",
                grlog.String("method", r.Method),
                grlog.String("path", r.URL.Path),
                grlog.String("remote_addr", r.RemoteAddr),
                grlog.Int("status", wrapped.statusCode),
                grlog.Duration("latency", time.Since(start)),
                grlog.String("user_agent", r.UserAgent()),
            )
        })
    }
}

type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

func generateRequestID() string {
    // Your implementation
    return "req-" + time.Now().Format("20060102150405")
}
```

---

### 2. Database Integration Example

```go
package main

import (
    "database/sql"
    "time"
    
    "github.com/gourdian25/grlog"
)

type User struct {
    ID        int64
    Email     string
    CreatedAt time.Time
}

func CreateUser(db *sql.DB, logger *grlog.Logger, user User) error {
    start := time.Now()
    
    result, err := db.Exec(
        "INSERT INTO users (email, created_at) VALUES (?, ?)",
        user.Email, user.CreatedAt,
    )
    
    if err != nil {
        logger.Error("Failed to create user",
            grlog.Err(err),
            grlog.String("email", user.Email),
            grlog.Duration("duration", time.Since(start)),
        )
        return err
    }
    
    id, _ := result.LastInsertId()
    
    logger.Info("User created",
        grlog.Int64("user_id", id),
        grlog.String("email", user.Email),
        grlog.Duration("duration", time.Since(start)),
    )
    
    return nil
}

func GetUser(db *sql.DB, logger *grlog.Logger, id int64) (*User, error) {
    start := time.Now()
    
    var user User
    err := db.QueryRow(
        "SELECT id, email, created_at FROM users WHERE id = ?",
        id,
    ).Scan(&user.ID, &user.Email, &user.CreatedAt)
    
    if err != nil {
        if err == sql.ErrNoRows {
            logger.Warn("User not found",
                grlog.Int64("user_id", id),
                grlog.Duration("duration", time.Since(start)),
            )
        } else {
            logger.Error("Failed to get user",
                grlog.Err(err),
                grlog.Int64("user_id", id),
                grlog.Duration("duration", time.Since(start)),
            )
        }
        return nil, err
    }
    
    logger.Debug("User retrieved",
        grlog.Int64("user_id", id),
        grlog.Duration("duration", time.Since(start)),
    )
    
    return &user, nil
}
```

---

### 3. Worker/Background Jobs Example

```go
package main

import (
    "context"
    "time"
    
    "github.com/gourdian25/grlog"
)

type Job struct {
    ID      string
    Type    string
    Payload map[string]interface{}
}

func ProcessJob(ctx context.Context, logger *grlog.Logger, job Job) error {
    start := time.Now()
    
    logger.Info("Job started",
        grlog.String("job_id", job.ID),
        grlog.String("job_type", job.Type),
    )
    
    // Add job context
    jobLogger := logger.WithContext(ctx)
    
    // Process job
    switch job.Type {
    case "email":
        err := sendEmail(jobLogger, job.Payload)
        if err != nil {
            jobLogger.Error("Email job failed",
                grlog.String("job_id", job.ID),
                grlog.Err(err),
                grlog.Duration("duration", time.Since(start)),
            )
            return err
        }
        
    case "report":
        err := generateReport(jobLogger, job.Payload)
        if err != nil {
            jobLogger.Error("Report job failed",
                grlog.String("job_id", job.ID),
                grlog.Err(err),
                grlog.Duration("duration", time.Since(start)),
            )
            return err
        }
    }
    
    jobLogger.Info("Job completed",
        grlog.String("job_id", job.ID),
        grlog.String("job_type", job.Type),
        grlog.Duration("duration", time.Since(start)),
    )
    
    return nil
}

func sendEmail(logger *grlog.Logger, payload map[string]interface{}) error {
    // Implementation
    logger.Debug("Sending email", grlog.Any("payload", payload))
    return nil
}

func generateReport(logger *grlog.Logger, payload map[string]interface{}) error {
    // Implementation
    logger.Debug("Generating report", grlog.Any("payload", payload))
    return nil
}
```

---

### 4. Error Tracking Integration

```go
package main

import (
    "runtime/debug"
    
    "github.com/getsentry/sentry-go"
    "github.com/gourdian25/grlog"
)

func LogAndTrackError(logger *grlog.Logger, err error, fields ...grlog.Field) {
    // Log locally with full context
    allFields := append(fields,
        grlog.Err(err),
        grlog.String("stack_trace", string(debug.Stack())),
    )
    
    logger.Error("Application error", allFields...)
    
    // Send to Sentry
    go func() {
        sentry.CaptureException(err)
    }()
}

func RecoverPanic(logger *grlog.Logger) {
    if r := recover(); r != nil {
        logger.Error("Panic recovered",
            grlog.Any("panic", r),
            grlog.String("stack", string(debug.Stack())),
        )
        
        // Send to Sentry
        sentry.CurrentHub().Recover(r)
        sentry.Flush(2 * time.Second)
    }
}

// Usage in HTTP handler
func SafeHandler(logger *grlog.Logger, fn http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        defer RecoverPanic(logger)
        fn(w, r)
    }
}
```

---

### 5. Metrics Integration Example

```go
package main

import (
    "time"
    
    "github.com/gourdian25/grlog"
    "github.com/prometheus/client_golang/prometheus"
)

var (
    logCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "logs_total",
            Help: "Total number of log entries by level",
        },
        []string{"level"},
    )
    
    errorLogCounter = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "errors_total",
            Help: "Total number of errors logged",
        },
    )
)

func init() {
    prometheus.MustRegister(logCounter, errorLogCounter)
}

func setupLoggerWithMetrics() *grlog.Logger {
    // Metrics sink
    metricsSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
        logCounter.WithLabelValues(entry.Level.String()).Inc()
        
        if entry.Level == grlog.ERROR {
            errorLogCounter.Inc()
        }
        
        return nil
    })
    
    // Regular logging
    stdoutSink := grlog.NewStdoutSink(grlog.JSONFormat())
    
    return grlog.NewLogger(
        grlog.WithLevel(grlog.INFO),
        grlog.WithSink(grlog.NewMultiSink(stdoutSink, metricsSink)),
        grlog.WithAsync(5000),
    )
}
```

---

## 🏆 Best Practices

### 1. Always Close the Logger

```go
logger := grlog.NewDefaultLogger()
defer logger.Close()  // ✅ Flushes async buffers and closes files
```

### 2. Choose Appropriate Log Levels

| Environment | Recommended Level | Reason |
|-------------|------------------|--------|
| Development | `DEBUG` | Maximum visibility |
| Staging | `INFO` | Operational insight |
| Production | `INFO` or `WARN` | Balance performance/visibility |
| High-volume | `WARN` or `ERROR` | Minimal overhead |

### 3. Use Typed Fields (Not `Any`)

```go
// ✅ Good - zero allocation
logger.Info("User action",
    grlog.String("user_id", userID),
    grlog.Int("attempts", attempts),
)

// ❌ Avoid - uses reflection
logger.Info("User action",
    grlog.Any("user_id", userID),
    grlog.Any("attempts", attempts),
)
```

### 4. Size Async Buffers Correctly

| Application Type | Buffer Size | Reasoning |
|-----------------|-------------|-----------|
| Low volume | 100-1000 | Small memory footprint |
| Medium volume | 1000-10000 | Balance latency/memory |
| High volume | 10000-50000 | Handle bursts |

### 5. Configure File Rotation

| Volume | MaxBytes | BackupCount | Total Storage |
|--------|----------|-------------|---------------|
| Low | 10MB | 5 | ~50MB |
| Medium | 50MB | 10 | ~500MB |
| High | 100MB | 20 | ~2GB |

### 6. Performance Optimization

```go
// For hot paths
grlog.WithCaller(false)             // Saves ~500ns per log (runtime.Caller)
grlog.WithLevel(grlog.WARN)         // Filtered-out calls cost ~2.7ns
grlog.WithSampler(100, grlog.INFO)  // Keep 1-in-100 DEBUG/INFO entries
```

### 7. Error Handling

```go
sink, err := grlog.NewFileSink(config)
if err != nil {
    // ✅ Fallback to stdout instead of crashing
    sink = grlog.NewStdoutSink(grlog.PlainFormat())
    log.Printf("File sink failed, using stdout: %v", err)
}
```

### 8. Testing

```go
func TestMyFunction(t *testing.T) {
    var logs []grlog.LogEntry
    testSink := grlog.NewCustomSink(func(entry grlog.LogEntry) error {
        logs = append(logs, entry)
        return nil
    })
    
    logger := grlog.NewLogger(
        grlog.WithSink(testSink),
        grlog.WithCaller(false),  // Cleaner output
    )
    
    myFunction(logger)
    
    // Verify logs
    if len(logs) != 1 {
        t.Errorf("Expected 1 log, got %d", len(logs))
    }
}
```

### 9. Structured Data Guidelines

```go
// ✅ Good - specific, searchable fields
logger.Info("Payment processed",
    grlog.String("payment_id", paymentID),
    grlog.String("customer_id", customerID),
    grlog.Int64("amount_cents", amount),
    grlog.String("currency", currency),
)

// ❌ Avoid - unstructured message
logger.Infof("Payment %s processed for customer %s: $%.2f %s",
    paymentID, customerID, float64(amount)/100, currency)
```

### 10. Security Considerations

```go
// ✅ Good - redact sensitive data
logger.Info("User login",
    grlog.String("user_id", userID),
    grlog.String("email", maskEmail(email)),
    grlog.String("ip", ip),
)

// ❌ Avoid - logging passwords or tokens
logger.Info("Authentication",
    grlog.String("password", password),  // Never!
    grlog.String("token", token),        // Never!
)
```

---

## 📊 Performance & Benchmarks

### Benchmark Results (Apple M-series, Go 1.26)

```
Field Construction (zero allocation):
BenchmarkFields_String-10             0.22 ns/op      0 B/op    0 allocs/op
BenchmarkFields_Int-10                0.23 ns/op      0 B/op    0 allocs/op
BenchmarkFields_Mixed10Fields-10      0.22 ns/op      0 B/op    0 allocs/op

Formatting (pooled buffers):
BenchmarkFormatter_Plain_NoFields-10   78 ns/op       0 B/op    0 allocs/op
BenchmarkFormatter_Plain_3Fields-10   109 ns/op       0 B/op    0 allocs/op
BenchmarkFormatter_JSON_NoFields-10    82 ns/op       0 B/op    0 allocs/op
BenchmarkFormatter_JSON_3Fields-10    205 ns/op       4 B/op    1 allocs/op

Logger Performance:
BenchmarkLogger_Sync_NoFields-10      125 ns/op       0 B/op    0 allocs/op
BenchmarkLogger_Sync_3Fields-10       177 ns/op     128 B/op    1 allocs/op
BenchmarkLogger_Async_NoFields-10     159 ns/op       0 B/op    0 allocs/op
BenchmarkLogger_FilteredOut_Info-10   2.7 ns/op       0 B/op    0 allocs/op

Real-World Scenarios:
BenchmarkRealWorld_HTTPRequestLog-10  563 ns/op     324 B/op    2 allocs/op
BenchmarkRealWorld_ErrorLog-10        1.2 μs/op     600 B/op    8 allocs/op
BenchmarkRealWorld_BusinessEvent-10   945 ns/op     581 B/op    2 allocs/op
```

### Key Takeaways

- ⚡ **Typed fields**: Sub-nanosecond overhead, zero allocations
- 📄 **Formatting**: Zero allocations via pooled buffers on both plain and JSON paths
- 🚀 **Logging**: Zero allocations for entries without fields; one for the field slice
- 🔍 **Filtering**: Filtered-out logs cost ~2.7ns and never allocate
- 📊 **Real-world**: HTTP request logging in ~0.6 microseconds
- ⚠️ Caller info (`WithCaller(true)`) adds ~500ns per entry (`runtime.Caller`); disable it on hot paths

### Running Benchmarks

```bash
# Run all benchmarks
make bench

# Specific benchmark
go test -bench=BenchmarkLogger_Sync -benchmem

# Compare performance
go test -bench=. -benchmem > new.txt
benchstat baseline.txt new.txt
```

---

## 🧪 Testing

### Test Coverage

| Metric | Value | Details |
|--------|-------|---------|
| **Statement Coverage** | 95.0-95.1% | 150+ tests, verified via `make coverage-summary` / `make coverage-check` on 2026-07-22 |
| **Race Detection Tests** | 20+ | All concurrent scenarios, run in CI with `-race` |
| **Regression Tests** | ✅ | Shutdown drain, rotation safety, JSON collisions, view close semantics |

### Running Tests

```bash
# All tests
make test

# With coverage
make coverage

# Race detection
env CGO_ENABLED=1 go test -race -v ./...

# Coverage report (HTML)
make coverage
open test_coverage/coverage.html
```

### Race Condition Safety

✅ **Tested Scenarios:**
- Concurrent logging from multiple goroutines
- Level changes during logging
- Sink addition/removal during logging
- Async queue overflow handling
- Logger close during active logging
- Context field concurrent access
- Multi-sink concurrent writes

All tests pass with `-race` flag enabled.

---

## 🤝 Contributing

We welcome contributions! Here's how:

### Development Setup

```bash
git clone https://github.com/gourdian25/grlog.git
cd grlog

# Run tests
make test

# Run benchmarks
make bench

# Check coverage
make coverage
```

### Contribution Process

1. Fork the repository
2. Create feature branch: `git checkout -b feat/amazing-feature`
3. Write tests for your changes (concurrency-sensitive changes need coverage in `grlog_race_test.go`)
4. Ensure everything passes: `make ci` and `make test-race`
5. Run benchmarks: `make bench` — no performance regressions
6. Update `CHANGELOG.md` under `[Unreleased]`
7. Submit pull request (CI runs lint, tests, and the race detector)

### Code Standards

- Follow Go conventions (`go fmt`, `make lint` / golangci-lint)
- Add godoc comments for public APIs
- Include examples in documentation
- Keep test coverage high (`make coverage-summary`)
- No performance regressions

---

## 📜 License

MIT License - see [LICENSE](LICENSE) file.

Copyright (c) 2024 grlog Contributors

**Permission is hereby granted**, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so.

**See [LICENSE](LICENSE) for full details.**

---

## 📞 Support & Resources

- 🐛 **Bug Reports**: [GitHub Issues](https://github.com/gourdian25/grlog/issues)
- 💡 **Feature Requests**: [GitHub Issues](https://github.com/gourdian25/grlog/issues)
- 📖 **Full API Documentation**: [pkg.go.dev/github.com/gourdian25/grlog](https://pkg.go.dev/github.com/gourdian25/grlog) (or [docs.go](docs.go))
- 📝 **Release History**: [CHANGELOG.md](CHANGELOG.md)
- ⚡ **Performance Questions**: Include benchmark results
- 🔒 **Security**: See [SECURITY.md](SECURITY.md)

---

## 🛠️ Maintainers

| Role                | Contributors                          |
|---------------------|---------------------------------------|
| Core Development    | [@gourdian25](https://github.com/gourdian25) |
| Performance Tuning  | [@lordofthemind](https://github.com/lordofthemind) |

---

## 🌟 Acknowledgments

grlog is built by Go developers for the Go community. Special thanks to:

- All our contributors 👥
- The amazing Go open-source ecosystem 🦾
- Early adopters who provided valuable feedback 💡
- The logging libraries that inspired us (zap, logrus, zerolog)

We believe logging should be:
- ✅ **Elegant** - Clean API that's joy to use
- ✅ **Efficient** - Zero-allocation performance
- ✅ **Production-safe** - Battle-tested concurrency
- ✅ **Developer-friendly** - Comprehensive docs and examples

Join us in making grlog even better! 🚀

---

**Made with 🎃 by the grlog team**