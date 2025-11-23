// Package logger provides structured logging for Claude Code hooks.
package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Logger provides structured logging interface.
type Logger interface {
	// Debug logs debug-level messages with optional key-value pairs.
	Debug(msg string, keysAndValues ...any)

	// Info logs info-level messages with optional key-value pairs.
	Info(msg string, keysAndValues ...any)

	// Error logs error-level messages with optional key-value pairs.
	Error(msg string, keysAndValues ...any)

	// With returns a new logger with additional key-value pairs.
	With(keysAndValues ...any) Logger
}

// Level represents the log level.
type Level string

const (
	// LevelDebug represents debug-level logging.
	LevelDebug Level = "DEBUG"

	// LevelInfo represents info-level logging.
	LevelInfo Level = "INFO"

	// LevelError represents error-level logging.
	LevelError Level = "ERROR"

	// LogFilePermissions defines the file permissions for log files (owner read/write only).
	LogFilePermissions = 0o600
)

// FileLogger implements Logger interface with file output only.
type FileLogger struct {
	file      io.Writer
	baseKVs   []any
	debugMode bool
	traceMode bool
}

// NewFileLogger creates a new FileLogger that writes to a log file.
func NewFileLogger(filePath string, debugMode, traceMode bool) (*FileLogger, error) {
	//nolint:gosec // File path is controlled and within user home directory
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFilePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &FileLogger{
		file:      file,
		debugMode: debugMode,
		traceMode: traceMode,
	}, nil
}

// NewFileLoggerWithWriter creates a new FileLogger with a custom writer.
func NewFileLoggerWithWriter(file io.Writer, debugMode, traceMode bool) *FileLogger {
	return &FileLogger{
		file:      file,
		debugMode: debugMode,
		traceMode: traceMode,
	}
}

// Debug logs debug-level messages.
func (l *FileLogger) Debug(msg string, keysAndValues ...any) {
	if !l.traceMode {
		return
	}

	l.log(LevelDebug, msg, keysAndValues...)
}

// Info logs info-level messages.
func (l *FileLogger) Info(msg string, keysAndValues ...any) {
	if !l.debugMode && !l.traceMode {
		return
	}

	l.log(LevelInfo, msg, keysAndValues...)
}

// Error logs error-level messages.
func (l *FileLogger) Error(msg string, keysAndValues ...any) {
	l.log(LevelError, msg, keysAndValues...)
}

// With returns a new logger with additional base key-value pairs.
//
//nolint:ireturn // With is intended to return an interface for chaining
func (l *FileLogger) With(keysAndValues ...any) Logger {
	newKVs := make([]any, len(l.baseKVs)+len(keysAndValues))
	copy(newKVs, l.baseKVs)
	copy(newKVs[len(l.baseKVs):], keysAndValues)

	return &FileLogger{
		file:      l.file,
		baseKVs:   newKVs,
		debugMode: l.debugMode,
		traceMode: l.traceMode,
	}
}

// log writes a log entry to the file only (not stderr).
func (l *FileLogger) log(level Level, msg string, keysAndValues ...any) {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	var builder strings.Builder

	builder.WriteString(timestamp)
	builder.WriteString(" ")
	builder.WriteString(string(level))
	builder.WriteString(" ")
	builder.WriteString(msg)

	// Add base key-value pairs
	if len(l.baseKVs) > 0 {
		l.writeKeyValues(&builder, l.baseKVs)
	}

	// Add message key-value pairs
	if len(keysAndValues) > 0 {
		l.writeKeyValues(&builder, keysAndValues)
	}

	builder.WriteString("\n")

	output := builder.String()

	// Write to file only
	if l.file != nil {
		_, _ = l.file.Write([]byte(output))
	}
}

// writeKeyValues formats key-value pairs and appends to builder.
func (l *FileLogger) writeKeyValues(builder *strings.Builder, kvs []any) {
	for i := 0; i < len(kvs); i += 2 {
		if i+1 >= len(kvs) {
			// Odd number of arguments, skip the last one
			break
		}

		key := fmt.Sprintf("%v", kvs[i])
		value := fmt.Sprintf("%v", kvs[i+1])

		builder.WriteString(" ")
		builder.WriteString(key)
		builder.WriteString("=")

		// Quote value if it contains spaces or special characters
		if strings.ContainsAny(value, " \t\n\"") {
			builder.WriteString(l.quote(value))
		} else {
			builder.WriteString(value)
		}
	}
}

// quote escapes and quotes a string value.
func (*FileLogger) quote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")

	return "\"" + s + "\""
}

// NoOpLogger is a logger that does nothing.
type NoOpLogger struct{}

// NewNoOpLogger creates a new NoOpLogger.
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

// Debug does nothing.
func (*NoOpLogger) Debug(string, ...any) {}

// Info does nothing.
func (*NoOpLogger) Info(string, ...any) {}

// Error does nothing.
func (*NoOpLogger) Error(string, ...any) {}

// With returns the same NoOpLogger.
//
//nolint:ireturn // With is intended to return an interface for chaining
func (n *NoOpLogger) With(...any) Logger {
	return n
}
