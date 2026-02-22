// Package config provides configuration schema types for klaudiush validators.
package config

// ValidatorConfig represents the base configuration for all validators.
type ValidatorConfig struct {
	// Enabled controls whether the validator is active.
	// When false, the validator is completely skipped.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Severity determines whether validation failures block the operation.
	// "error" blocks the operation (default)
	// "warning" only warns without blocking
	Severity Severity `json:"severity,omitempty" koanf:"severity" toml:"severity,omitempty"`

	// RulesEnabled controls whether dynamic rules are checked for this validator.
	// When true (default), rules from the rules engine are checked before built-in validation.
	// When false, only built-in validation logic is used.
	// Default: true
	RulesEnabled *bool `json:"rules_enabled,omitempty" koanf:"rules_enabled" toml:"rules_enabled,omitempty"`
}

// IsEnabled returns true if the validator is enabled.
// Returns true if Enabled is nil (default behavior).
func (c *ValidatorConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

// GetSeverity returns the severity level, defaulting to Error if not set.
func (c *ValidatorConfig) GetSeverity() Severity {
	if c.Severity == SeverityUnknown {
		return SeverityError
	}

	return c.Severity
}

// AreRulesEnabled returns true if dynamic rules are enabled for this validator.
// Returns true if RulesEnabled is nil (default behavior).
func (c *ValidatorConfig) AreRulesEnabled() bool {
	if c.RulesEnabled == nil {
		return true
	}

	return *c.RulesEnabled
}
