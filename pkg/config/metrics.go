package config

import "time"

// DefaultMetricsRetention is the default report window and prune age.
const DefaultMetricsRetention = 30 * 24 * time.Hour

// DefaultMetricsMaxFileSizeMB caps the active metrics log. Past the cap it
// rotates into a single backup, so at most twice this is kept.
const DefaultMetricsMaxFileSizeMB = 8

// MaxMetricsFileSizeMB is the largest accepted max_file_size_mb.
const MaxMetricsFileSizeMB = 256

const bytesPerMB = 1 << 20

// MetricsConfig controls the local enforcement outcome metrics: one
// redacted line per hook recording what the response did, which codes it
// reported, which checks could not run, and how long the hook and each
// validator took. Nothing leaves the machine; no command, message, path or
// session ID is stored.
//
// Example configuration:
//
//	[metrics]
//	enabled = true
//	retention = "720h"
//	max_file_size_mb = 8
type MetricsConfig struct {
	// Enabled turns recording on. Default: true.
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Retention is the default report window and the age prune drops.
	// Default: "720h" (30 days).
	Retention Duration `json:"retention,omitempty" koanf:"retention" toml:"retention,omitempty"`

	// MaxFileSizeMB caps the active log before it rotates into one backup.
	// Default: 8. Values above 256 count as 256; zero or less as the default.
	MaxFileSizeMB int `json:"max_file_size_mb,omitempty" jsonschema:"minimum=0,maximum=256" koanf:"max_file_size_mb" toml:"max_file_size_mb,omitempty"`
}

// IsEnabled reports whether metrics are recorded. Defaults to true.
func (m *MetricsConfig) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return true
	}

	return *m.Enabled
}

// GetRetention returns the report window and prune age.
func (m *MetricsConfig) GetRetention() time.Duration {
	if m == nil || m.Retention <= 0 {
		return DefaultMetricsRetention
	}

	return time.Duration(m.Retention)
}

// GetMaxFileSize returns the rotation threshold in bytes.
func (m *MetricsConfig) GetMaxFileSize() int64 {
	if m == nil || m.MaxFileSizeMB <= 0 {
		return DefaultMetricsMaxFileSizeMB * bytesPerMB
	}

	return int64(min(m.MaxFileSizeMB, MaxMetricsFileSizeMB)) * bytesPerMB
}
