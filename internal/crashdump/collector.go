package crashdump

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

const (
	// shortIDLength is the length of the short ID suffix.
	shortIDLength = 8

	// panicNilStr is the string representation of panic(nil).
	panicNilStr = "panic(nil)"
)

// formatPanicValue converts a recovered panic value to a string representation.
// Handles Go 1.21+ PanicNilError, error interface, and default cases.
func formatPanicValue(v any) string {
	if v == nil {
		return panicNilStr
	}

	// Go 1.21+ converts panic(nil) to *runtime.PanicNilError
	// Use type assertion with interface to avoid direct dependency
	type panicNilError interface {
		error
		RuntimeError()
	}

	if _, ok := v.(panicNilError); ok {
		return panicNilStr
	}

	// Handle error interface
	if err, ok := v.(error); ok {
		return err.Error()
	}

	// Default: use fmt.Sprintf
	return fmt.Sprintf("%v", v)
}

// Collector collects crash diagnostic information.
type Collector interface {
	// Collect gathers crash information from a recovered panic.
	Collect(recovered any, ctx *hook.Context, cfg *config.Config) *CrashInfo
}

// DefaultCollector is the default crash info collector.
type DefaultCollector struct {
	// Version is the klaudiush version.
	Version string

	// sanitizer handles config sanitization.
	sanitizer *Sanitizer
}

// NewCollector creates a new crash info collector.
func NewCollector(version string) *DefaultCollector {
	return &DefaultCollector{
		Version:   version,
		sanitizer: NewSanitizer(),
	}
}

// Collect gathers crash information from a recovered panic.
func (c *DefaultCollector) Collect(
	recovered any,
	ctx *hook.Context,
	cfg *config.Config,
) *CrashInfo {
	now := time.Now()
	panicValue := formatPanicValue(recovered)
	stackTrace := captureStack()

	info := &CrashInfo{
		ID:         generateCrashID(now, panicValue),
		Timestamp:  now,
		PanicValue: panicValue,
		StackTrace: stackTrace,
		Runtime:    collectRuntime(),
		Metadata:   c.collectMetadata(),
	}

	if ctx != nil {
		info.Context = collectContext(ctx)
	}

	if cfg != nil {
		info.Config = c.sanitizer.SanitizeConfig(cfg)
	}

	return info
}

// captureStack captures the stack trace of the panicking goroutine.
// Uses debug.Stack() which automatically sizes the buffer and returns
// the stack trace for the current goroutine.
func captureStack() string {
	return string(debug.Stack())
}

// collectRuntime gathers runtime information.
func collectRuntime() RuntimeInfo {
	return RuntimeInfo{
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		GoVersion:    runtime.Version(),
		NumGoroutine: runtime.NumGoroutine(),
		NumCPU:       runtime.NumCPU(),
	}
}

// collectContext extracts hook context information.
func collectContext(ctx *hook.Context) *ContextInfo {
	return &ContextInfo{
		EventType: ctx.EventType.String(),
		ToolName:  ctx.ToolName.String(),
		Command:   ctx.GetCommand(),
		FilePath:  ctx.GetFilePath(),
	}
}

// collectMetadata gathers additional diagnostic metadata.
func (c *DefaultCollector) collectMetadata() DumpMetadata {
	meta := DumpMetadata{
		Version: c.Version,
	}

	if u, err := user.Current(); err == nil {
		meta.User = u.Username
	}

	if hostname, err := os.Hostname(); err == nil {
		meta.Hostname = hostname
	}

	if wd, err := os.Getwd(); err == nil {
		meta.WorkingDir = wd
	}

	return meta
}

// generateCrashID generates a unique crash dump ID.
// Format: crash-{timestamp}-{shortHash}
func generateCrashID(timestamp time.Time, panicValue string) string {
	data := fmt.Sprintf("%d-%s", timestamp.UnixNano(), panicValue)
	hash := sha256.Sum256([]byte(data))
	shortHash := hex.EncodeToString(hash[:])[:shortIDLength]

	return fmt.Sprintf("crash-%s-%s", timestamp.Format("20060102T150405"), shortHash)
}
