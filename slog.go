// File: slog.go

package grlog

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SlogHandler adapts a grlog Logger to the standard library's slog.Handler
// interface, so code (or dependencies) written against log/slog can route
// records through grlog's sinks and formatters.
type SlogHandler struct {
	logger *Logger
	attrs  []Field  // Attributes accumulated via WithAttrs
	groups []string // Group names accumulated via WithGroup
}

// NewSlogHandler creates a slog.Handler backed by a grlog Logger.
// Arguments:
//   - logger: *Logger - The grlog logger records are dispatched to
//
// Returns:
//   - slog.Handler: A handler suitable for slog.New
//
// Level mapping: slog Debug->DEBUG, Info->INFO, Warn->WARN, Error->ERROR.
// Use case: `slog.SetDefault(slog.New(grlog.NewSlogHandler(logger)))` routes
// all slog output through grlog.
func NewSlogHandler(logger *Logger) slog.Handler {
	return &SlogHandler{logger: logger}
}

// levelFromSlog maps a slog.Level to the nearest grlog LogLevel.
func levelFromSlog(level slog.Level) LogLevel {
	switch {
	case level < slog.LevelInfo:
		return DEBUG
	case level < slog.LevelWarn:
		return INFO
	case level < slog.LevelError:
		return WARN
	default:
		return ERROR
	}
}

// Enabled reports whether the handler processes records at the given level.
// Arguments:
//   - level: slog.Level - The record level to check
//
// Returns:
//   - bool: true if the mapped grlog level meets the logger's minimum
//
// Use case: Lets slog skip building records that would be filtered anyway.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	c := h.logger.core
	return levelFromSlog(level) >= h.logger.GetLevel() && !c.closed.Load()
}

// Handle converts a slog.Record into a LogEntry and dispatches it.
// Arguments:
//   - r: slog.Record - The record to process
//
// Returns:
//   - error: Always nil (sink failures go through the logger's error handler)
//
// Use case: Called by slog.Logger methods; not usually invoked directly.
func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	c := h.logger.core
	level := levelFromSlog(r.Level)
	if level < h.logger.GetLevel() || c.closed.Load() {
		return nil
	}
	if !c.shouldSample(level) {
		return nil
	}

	entry := LogEntry{
		Timestamp: r.Time,
		Level:     level,
		Message:   r.Message,
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	fields := make([]Field, 0, len(h.logger.contextFields)+len(h.attrs)+r.NumAttrs())
	fields = append(fields, h.logger.contextFields...)
	fields = append(fields, h.attrs...)
	prefix := h.groupPrefix()
	r.Attrs(func(a slog.Attr) bool {
		fields = appendSlogAttr(fields, prefix, a)
		return true
	})
	entry.Fields = fields

	if c.enableCaller {
		entry.CallerInfo = callerFromPC(r.PC)
	}

	c.dispatch(entry)
	return nil
}

// WithAttrs returns a handler that includes the given attributes on every record.
// Arguments:
//   - attrs: []slog.Attr - Attributes to add
//
// Returns:
//   - slog.Handler: A derived handler carrying the attributes
//
// Use case: Implements slog.Logger.With.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	newAttrs := make([]Field, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	prefix := h.groupPrefix()
	for _, a := range attrs {
		newAttrs = appendSlogAttr(newAttrs, prefix, a)
	}
	return &SlogHandler{logger: h.logger, attrs: newAttrs, groups: h.groups}
}

// WithGroup returns a handler that prefixes subsequent attribute keys with name.
// Arguments:
//   - name: string - The group name
//
// Returns:
//   - slog.Handler: A derived handler with the group applied
//
// Use case: Implements slog.Logger.WithGroup; keys become "group.key".
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups), len(h.groups)+1)
	copy(newGroups, h.groups)
	return &SlogHandler{logger: h.logger, attrs: h.attrs, groups: append(newGroups, name)}
}

// groupPrefix returns the accumulated "a.b." key prefix for open groups.
func (h *SlogHandler) groupPrefix() string {
	if len(h.groups) == 0 {
		return ""
	}
	return strings.Join(h.groups, ".") + "."
}

// appendSlogAttr converts one slog.Attr (recursively for groups) into fields.
func appendSlogAttr(fields []Field, prefix string, a slog.Attr) []Field {
	if a.Equal(slog.Attr{}) {
		return fields
	}
	v := a.Value.Resolve()
	key := prefix + a.Key

	switch v.Kind() {
	case slog.KindGroup:
		groupPrefix := prefix
		if a.Key != "" {
			groupPrefix = key + "."
		}
		for _, ga := range v.Group() {
			fields = appendSlogAttr(fields, groupPrefix, ga)
		}
		return fields
	case slog.KindString:
		return append(fields, String(key, v.String()))
	case slog.KindInt64:
		return append(fields, Int64(key, v.Int64()))
	case slog.KindUint64:
		return append(fields, Uint64(key, v.Uint64()))
	case slog.KindFloat64:
		return append(fields, Float64(key, v.Float64()))
	case slog.KindBool:
		return append(fields, Bool(key, v.Bool()))
	case slog.KindDuration:
		return append(fields, Duration(key, v.Duration()))
	case slog.KindTime:
		return append(fields, Time(key, v.Time()))
	default:
		return append(fields, Any(key, v.Any()))
	}
}

// callerFromPC formats caller info from a program counter, matching the
// "file:line:function" layout produced by getCallerInfo.
func callerFromPC(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.File == "" {
		return ""
	}

	fnName := frame.Function
	if lastSlash := strings.LastIndex(fnName, "/"); lastSlash >= 0 {
		fnName = fnName[lastSlash+1:]
	}
	if lastDot := strings.LastIndex(fnName, "."); lastDot >= 0 {
		fnName = fnName[lastDot+1:]
	}
	if fnName == "" {
		return fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
	}
	return fmt.Sprintf("%s:%d:%s", filepath.Base(frame.File), frame.Line, fnName)
}
