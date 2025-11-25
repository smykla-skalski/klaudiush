package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

const initialBufferCapacity = 256

// CustomHandler writes log entries with custom formatting.
// It supports both files and generic io.Writers.
type CustomHandler struct {
	writer io.Writer
	mu     sync.Mutex
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

// FileHandler is an alias for CustomHandler for backward compatibility.
type FileHandler = CustomHandler

// NewFileHandler creates a new FileHandler that writes to the specified file.
func NewFileHandler(path string, level Level) (*CustomHandler, error) {
	//nolint:gosec // File path is controlled and within user home directory
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, LogFilePermissions)
	if err != nil {
		return nil, err
	}

	return &CustomHandler{
		writer: file,
		level:  level.ToSlogLevel(),
	}, nil
}

// NewWriterHandler creates a new handler that writes to the specified writer.
func NewWriterHandler(w io.Writer, level Level) *CustomHandler {
	return &CustomHandler{
		writer: w,
		level:  level.ToSlogLevel(),
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *CustomHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle handles the log record.
func (h *CustomHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	buf := make([]byte, 0, initialBufferCapacity)

	// Format: "2006-01-02T15:04:05-07:00 LEVEL msg key=value\n"
	buf = append(buf, r.Time.Local().Format("2006-01-02T15:04:05-07:00")...)
	buf = append(buf, ' ')
	buf = append(buf, r.Level.String()...)
	buf = append(buf, ' ')
	buf = append(buf, r.Message...)

	// Add pre-set attrs with group prefix
	for _, a := range h.attrs {
		buf = h.appendAttr(buf, a)
	}

	// Add record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf = h.appendAttr(buf, a)

		return true
	})

	buf = append(buf, '\n')

	_, err := h.writer.Write(buf)

	return err
}

// appendAttr appends an attribute to the buffer.
func (h *CustomHandler) appendAttr(buf []byte, a slog.Attr) []byte {
	// Skip empty attrs
	if a.Equal(slog.Attr{}) {
		return buf
	}

	buf = append(buf, ' ')

	// Add group prefix
	if len(h.groups) > 0 {
		buf = append(buf, strings.Join(h.groups, ".")...)
		buf = append(buf, '.')
	}

	buf = append(buf, a.Key...)
	buf = append(buf, '=')

	val := a.Value.String()
	if needsQuoting(val) {
		buf = append(buf, quoteValue(val)...)
	} else {
		buf = append(buf, val...)
	}

	return buf
}

// needsQuoting returns true if the string value needs to be quoted.
func needsQuoting(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' || c == '"' {
			return true
		}
	}

	return false
}

// quoteValue escapes and quotes a string value.
func quoteValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")

	return "\"" + s + "\""
}

// WithAttrs returns a new handler with the given attributes added.
func (h *CustomHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &CustomHandler{
		writer: h.writer,
		level:  h.level,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with the given group name added.
func (h *CustomHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &CustomHandler{
		writer: h.writer,
		level:  h.level,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// Close closes the underlying writer if it implements io.Closer.
func (h *CustomHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if closer, ok := h.writer.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}
