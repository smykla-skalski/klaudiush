package config

import "time"

// Default values for patterns configuration.
const (
	// DefaultPatternsMinCount is the minimum observation count before warning.
	DefaultPatternsMinCount = 3

	// DefaultPatternsMaxAge is the default maximum age for patterns (90 days).
	DefaultPatternsMaxAge = 90 * 24 * time.Hour

	// DefaultPatternsMaxWarningsPerError caps warnings per individual error.
	DefaultPatternsMaxWarningsPerError = 2

	// DefaultPatternsMaxWarningsTotal caps total pattern warnings per response.
	DefaultPatternsMaxWarningsTotal = 3

	// DefaultPatternsProjectDataFile is the default project-local data file.
	DefaultPatternsProjectDataFile = ".klaudiush/patterns.json"

	// DefaultPatternsGlobalDataDir is the default global data directory.
	DefaultPatternsGlobalDataDir = "~/.klaudiush/patterns"
)

// PatternsConfig contains configuration for failure pattern tracking.
type PatternsConfig struct {
	// Enabled controls whether pattern tracking is active.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// MinCount is the minimum observation count before a pattern triggers a warning.
	// Default: 3
	MinCount int `json:"min_count,omitempty" koanf:"min_count" toml:"min_count"`

	// MaxAge is the maximum age for patterns before cleanup.
	// Default: "2160h" (90 days)
	MaxAge Duration `json:"max_age,omitempty" koanf:"max_age" toml:"max_age"`

	// MaxWarningsPerError caps how many pattern warnings are shown per error.
	// Default: 2
	MaxWarningsPerError int `json:"max_warnings_per_error,omitempty" koanf:"max_warnings_per_error" toml:"max_warnings_per_error"`

	// MaxWarningsTotal caps the total number of pattern warnings per response.
	// Default: 3
	MaxWarningsTotal int `json:"max_warnings_total,omitempty" koanf:"max_warnings_total" toml:"max_warnings_total"`

	// ProjectDataFile is the path to the project-local patterns file.
	// Default: ".klaudiush/patterns.json"
	ProjectDataFile string `json:"project_data_file,omitempty" koanf:"project_data_file" toml:"project_data_file"`

	// GlobalDataDir is the directory for global per-project pattern files.
	// Default: "~/.klaudiush/patterns"
	GlobalDataDir string `json:"global_data_dir,omitempty" koanf:"global_data_dir" toml:"global_data_dir"`

	// UseSeedData controls whether built-in seed patterns are loaded.
	// Default: true
	UseSeedData *bool `json:"use_seed_data,omitempty" koanf:"use_seed_data" toml:"use_seed_data"`
}

// IsEnabled returns true if pattern tracking is enabled.
// Returns true if Enabled is nil (default behavior).
func (p *PatternsConfig) IsEnabled() bool {
	if p == nil || p.Enabled == nil {
		return true
	}

	return *p.Enabled
}

// GetMinCount returns the minimum observation count.
// Returns DefaultPatternsMinCount if not set.
func (p *PatternsConfig) GetMinCount() int {
	if p == nil || p.MinCount == 0 {
		return DefaultPatternsMinCount
	}

	return p.MinCount
}

// GetMaxAge returns the maximum pattern age as time.Duration.
// Returns DefaultPatternsMaxAge if not set.
func (p *PatternsConfig) GetMaxAge() time.Duration {
	if p == nil || p.MaxAge == 0 {
		return DefaultPatternsMaxAge
	}

	return time.Duration(p.MaxAge)
}

// GetMaxWarningsPerError returns the per-error warning cap.
// Returns DefaultPatternsMaxWarningsPerError if not set.
func (p *PatternsConfig) GetMaxWarningsPerError() int {
	if p == nil || p.MaxWarningsPerError == 0 {
		return DefaultPatternsMaxWarningsPerError
	}

	return p.MaxWarningsPerError
}

// GetMaxWarningsTotal returns the total warning cap.
// Returns DefaultPatternsMaxWarningsTotal if not set.
func (p *PatternsConfig) GetMaxWarningsTotal() int {
	if p == nil || p.MaxWarningsTotal == 0 {
		return DefaultPatternsMaxWarningsTotal
	}

	return p.MaxWarningsTotal
}

// GetProjectDataFile returns the project-local data file path.
// Returns DefaultPatternsProjectDataFile if not set.
func (p *PatternsConfig) GetProjectDataFile() string {
	if p == nil || p.ProjectDataFile == "" {
		return DefaultPatternsProjectDataFile
	}

	return p.ProjectDataFile
}

// GetGlobalDataDir returns the global data directory path.
// Returns DefaultPatternsGlobalDataDir if not set.
func (p *PatternsConfig) GetGlobalDataDir() string {
	if p == nil || p.GlobalDataDir == "" {
		return DefaultPatternsGlobalDataDir
	}

	return p.GlobalDataDir
}

// IsUseSeedData returns true if seed data should be loaded.
// Returns true if UseSeedData is nil (default behavior).
func (p *PatternsConfig) IsUseSeedData() bool {
	if p == nil || p.UseSeedData == nil {
		return true
	}

	return *p.UseSeedData
}
