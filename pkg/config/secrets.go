package config

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// SecretsConfig groups all secrets-related validator configurations.
type SecretsConfig struct {
	// Secrets validator configuration
	Secrets *SecretsValidatorConfig `json:"secrets,omitempty" koanf:"secrets" toml:"secrets"`
}

// SecretsValidatorConfig configures the secrets detection validator.
type SecretsValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// UseGitleaks enables gitleaks integration if available.
	// When enabled and gitleaks is installed, it runs as a second-tier check
	// after the built-in pattern detection.
	// Default: false
	UseGitleaks *bool `json:"use_gitleaks,omitempty" koanf:"use_gitleaks" toml:"use_gitleaks"`

	// GitleaksPath is the path to the gitleaks binary.
	// Default: "" (use PATH)
	GitleaksPath string `json:"gitleaks_path,omitempty" koanf:"gitleaks_path" toml:"gitleaks_path"`

	// MaxFileSize is the maximum file size to scan in bytes.
	// Files larger than this are skipped to avoid performance issues.
	// Default: "1MB"
	MaxFileSize ByteSize `json:"max_file_size,omitempty" koanf:"max_file_size" toml:"max_file_size"`

	// BlockOnDetection determines if secret detection should block the operation.
	// When false, secrets are reported as warnings instead of failures.
	// Default: true
	BlockOnDetection *bool `json:"block_on_detection,omitempty" koanf:"block_on_detection" toml:"block_on_detection"`

	// AllowList is a list of patterns that should be ignored even if they match.
	// Useful for test fixtures, documentation examples, or known false positives.
	// Each entry is a regex pattern that, if it matches the detected secret,
	// will cause the finding to be ignored.
	AllowList []string `json:"allow_list,omitempty" koanf:"allow_list" toml:"allow_list"`

	// CustomPatterns allows adding custom regex patterns for detection.
	// These are in addition to the built-in patterns.
	CustomPatterns []CustomPatternConfig `json:"custom_patterns,omitempty" koanf:"custom_patterns" toml:"custom_patterns"`

	// DisabledPatterns is a list of built-in pattern names to disable.
	// Use this to reduce false positives from specific pattern types.
	DisabledPatterns []string `json:"disabled_patterns,omitempty" koanf:"disabled_patterns" toml:"disabled_patterns"`
}

// CustomPatternConfig defines a custom secret detection pattern.
type CustomPatternConfig struct {
	// Name is a unique identifier for this pattern.
	Name string `json:"name" koanf:"name" toml:"name"`

	// Description explains what this pattern detects.
	Description string `json:"description" koanf:"description" toml:"description"`

	// Regex is the regular expression pattern.
	Regex string `json:"regex" koanf:"regex" toml:"regex"`
}

// ByteSize represents a byte size value that can be parsed from strings like "1MB".
type ByteSize int64

// Common byte size constants.
const (
	KB ByteSize = 1024
	MB ByteSize = 1024 * KB
	GB ByteSize = 1024 * MB
)

// DefaultMaxFileSize is the default maximum file size for secrets scanning.
const DefaultMaxFileSize = MB

// JSONSchema returns the JSON Schema for the ByteSize type.
func (ByteSize) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "integer",
		Minimum:     json.Number("0"),
		Description: "Size in bytes",
		Examples:    []any{1048576, 10485760},
	}
}

// IsUseGitleaksEnabled returns whether gitleaks integration is enabled.
func (c *SecretsValidatorConfig) IsUseGitleaksEnabled() bool {
	if c == nil || c.UseGitleaks == nil {
		return false
	}

	return *c.UseGitleaks
}

// IsBlockOnDetectionEnabled returns whether detection should block operations.
func (c *SecretsValidatorConfig) IsBlockOnDetectionEnabled() bool {
	if c == nil || c.BlockOnDetection == nil {
		return true // default to blocking
	}

	return *c.BlockOnDetection
}

// GetMaxFileSize returns the configured max file size or the default.
func (c *SecretsValidatorConfig) GetMaxFileSize() ByteSize {
	if c == nil || c.MaxFileSize == 0 {
		return DefaultMaxFileSize
	}

	return c.MaxFileSize
}
