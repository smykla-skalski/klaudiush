// Package logger provides structured logging for Claude Code hooks.
package logger

//go:generate mockgen -source=logger.go -destination=logger_mock.go -package=logger

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// LogFilePermissions defines the file permissions for log files (owner read/write only).
const LogFilePermissions = 0o600

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

// SlogAdapter adapts *slog.Logger to the Logger interface.
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter creates a new SlogAdapter wrapping a slog.Logger.
func NewSlogAdapter(logger *slog.Logger) *SlogAdapter {
	return &SlogAdapter{logger: logger}
}

// Debug logs debug-level messages.
func (s *SlogAdapter) Debug(msg string, args ...any) {
	s.logger.Debug(msg, args...)
}

// Info logs info-level messages.
func (s *SlogAdapter) Info(msg string, args ...any) {
	s.logger.Info(msg, args...)
}

// Error logs error-level messages.
func (s *SlogAdapter) Error(msg string, args ...any) {
	s.logger.Error(msg, args...)
}

// With returns a new logger with additional key-value pairs.
//

func (s *SlogAdapter) With(args ...any) Logger {
	return &SlogAdapter{logger: s.logger.With(args...)}
}

// Slog returns the underlying *slog.Logger.
func (s *SlogAdapter) Slog() *slog.Logger {
	return s.logger
}

// NewFileLogger creates a logger that writes to a file.
// For backward compatibility, accepts debug and trace boolean flags.
func NewFileLogger(path string, debug, trace bool) (*SlogAdapter, error) {
	level := LevelFromFlags(debug, trace)

	handler, err := NewFileHandler(path, level)
	if err != nil {
		return nil, err
	}

	return NewSlogAdapter(slog.New(handler)), nil
}

// NewFileLoggerWithLevel creates a logger that writes to a file with a specific level.
func NewFileLoggerWithLevel(path string, level Level) (*SlogAdapter, error) {
	handler, err := NewFileHandler(path, level)
	if err != nil {
		return nil, err
	}

	return NewSlogAdapter(slog.New(handler)), nil
}

// NewFileLoggerWithWriter creates a new SlogAdapter with a custom writer.
// Uses the same custom formatting as NewFileLogger for consistency.
func NewFileLoggerWithWriter(w io.Writer, debug, trace bool) *SlogAdapter {
	level := LevelFromFlags(debug, trace)

	handler := NewWriterHandler(w, level)

	return NewSlogAdapter(slog.New(handler))
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

func (n *NoOpLogger) With(...any) Logger {
	return n
}

// contextKey is the type for context keys in this package.
type contextKey struct{}

var loggerKey = contextKey{}

// FromContext retrieves a logger from context.
// Returns a default logger if none is found.
//

func FromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(loggerKey).(Logger); ok {
		return l
	}

	return NewSlogAdapter(slog.Default())
}

// WithContext adds a logger to the context.
func WithContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// Default returns a default logger that writes to stderr.
//

func Default() Logger {
	return NewSlogAdapter(slog.New(slog.NewTextHandler(os.Stderr, nil)))
}
