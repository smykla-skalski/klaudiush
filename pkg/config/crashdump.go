// Package config provides configuration schema types for klaudiush validators.
package config

import "time"

const (
	// DefaultCrashDumpDir is the legacy default directory for crash dumps.
	// Kept for backward compatibility with existing user configs.
	//
	// Deprecated: Use xdg.CrashDumpDir() from internal/xdg for the XDG-compliant path.
	DefaultCrashDumpDir = "~/.klaudiush/crash_dumps"

	// DefaultMaxDumps is the default maximum number of crash dumps to keep.
	DefaultMaxDumps = 10

	// DefaultMaxAgeDays is the default maximum age of crash dumps in days.
	DefaultMaxAgeDays = 30
)

// CrashDumpConfig contains configuration for the crash dump system.
//
// Example configuration:
//
//	[crash_dump]
//	enabled = true
//	dump_dir = "~/.klaudiush/crash_dumps"
//	max_dumps = 10
//	max_age = "720h"  # 30 days
//	include_config = true
//	include_context = true
type CrashDumpConfig struct {
	// Enabled controls whether crash dumps are created on panic.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// DumpDir is the directory where crash dumps are stored.
	// Default: "~/.klaudiush/crash_dumps"
	DumpDir *string `json:"dump_dir,omitempty" koanf:"dump_dir" toml:"dump_dir,omitempty"`

	// MaxDumps is the maximum number of crash dumps to keep.
	// Default: 10
	MaxDumps *int `json:"max_dumps,omitempty" koanf:"max_dumps" toml:"max_dumps,omitempty"`

	// MaxAge is the maximum age of crash dumps before they are pruned.
	// Default: "720h" (30 days)
	MaxAge Duration `json:"max_age,omitempty" koanf:"max_age" toml:"max_age,omitempty"`

	// IncludeConfig controls whether the sanitized config is included in dumps.
	// Default: true
	IncludeConfig *bool `json:"include_config,omitempty" koanf:"include_config" toml:"include_config,omitempty"`

	// IncludeContext controls whether hook context is included in dumps.
	// Default: true
	IncludeContext *bool `json:"include_context,omitempty" koanf:"include_context" toml:"include_context,omitempty"`
}

// IsEnabled returns whether crash dumps are enabled.
func (c *CrashDumpConfig) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

// GetDumpDir returns the dump directory, using default if not set.
func (c *CrashDumpConfig) GetDumpDir() string {
	if c == nil || c.DumpDir == nil {
		return DefaultCrashDumpDir
	}

	return *c.DumpDir
}

// GetMaxDumps returns the maximum number of dumps, using default if not set.
func (c *CrashDumpConfig) GetMaxDumps() int {
	if c == nil || c.MaxDumps == nil {
		return DefaultMaxDumps
	}

	return *c.MaxDumps
}

// GetMaxAge returns the maximum age duration, using default if not set.
func (c *CrashDumpConfig) GetMaxAge() Duration {
	if c == nil || c.MaxAge.ToDuration() == 0 {
		return Duration(DefaultMaxAgeDays * 24 * time.Hour)
	}

	return c.MaxAge
}

// IsIncludeConfig returns whether config should be included in dumps.
func (c *CrashDumpConfig) IsIncludeConfig() bool {
	if c == nil || c.IncludeConfig == nil {
		return true
	}

	return *c.IncludeConfig
}

// IsIncludeContext returns whether hook context should be included in dumps.
func (c *CrashDumpConfig) IsIncludeContext() bool {
	if c == nil || c.IncludeContext == nil {
		return true
	}

	return *c.IncludeContext
}
