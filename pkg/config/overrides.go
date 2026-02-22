// Package config provides configuration schema types for klaudiush validators.
package config

import "time"

// OverridesConfig holds override entries keyed by error code or validator name.
type OverridesConfig struct {
	// Entries maps error codes (e.g., "GIT014") or validator names (e.g., "git.commit")
	// to their override settings.
	Entries map[string]*OverrideEntry `json:"entries,omitempty" koanf:"entries" toml:"entries,omitempty"`
}

// OverrideEntry represents a single override for an error code or validator.
type OverrideEntry struct {
	// Disabled controls whether the target is disabled (true) or explicitly enabled (false).
	// Default: true when created via "klaudiush disable".
	Disabled *bool `json:"disabled,omitempty" koanf:"disabled" toml:"disabled,omitempty"`

	// Reason explains why this override exists.
	Reason string `json:"reason,omitempty" koanf:"reason" toml:"reason,omitempty"`

	// DisabledAt is the RFC3339 timestamp when the override was created.
	DisabledAt string `json:"disabled_at,omitempty" koanf:"disabled_at" toml:"disabled_at,omitempty"`

	// ExpiresAt is the RFC3339 timestamp when the override expires. Empty means permanent.
	ExpiresAt string `json:"expires_at,omitempty" koanf:"expires_at" toml:"expires_at,omitempty"`

	// DisabledBy records who created this override (e.g., "cli").
	DisabledBy string `json:"disabled_by,omitempty" koanf:"disabled_by" toml:"disabled_by,omitempty"`
}

// CodeToValidator maps error codes to their parent validator names.
// Used for (1) validating known codes and (2) checking validator-level overrides
// when a code-level override doesn't exist.
var CodeToValidator = map[string]string{
	// Git commit codes
	"GIT001": "git.commit",
	"GIT002": "git.commit",
	"GIT003": "git.commit",
	"GIT004": "git.commit",
	"GIT005": "git.commit",
	"GIT006": "git.commit",
	"GIT010": "git.commit",
	"GIT011": "git.commit",
	"GIT012": "git.commit",
	"GIT013": "git.commit",
	"GIT014": "git.commit",
	"GIT015": "git.commit",
	"GIT016": "git.commit",

	// Git push codes
	"GIT007": "git.push",
	"GIT008": "git.push",
	"GIT022": "git.push",
	"GIT025": "git.push",

	// Git add codes
	"GIT009": "git.add",
	"GIT019": "git.add",

	// Git branch codes
	"GIT020": "git.branch",

	// Git no-verify codes
	"GIT021": "git.no_verify",

	// Git merge codes
	"GIT017": "git.merge",
	"GIT018": "git.merge",

	// Git PR codes
	"GIT023": "git.pr",

	// Git fetch codes
	"GIT024": "git.fetch",

	// File codes
	"FILE001": "file.shellscript",
	"FILE002": "file.terraform",
	"FILE003": "file.terraform",
	"FILE004": "file.workflow",
	"FILE005": "file.markdown",
	"FILE006": "file.gofumpt",
	"FILE007": "file.python",
	"FILE008": "file.javascript",
	"FILE009": "file.rust",
	"FILE010": "file.linter_ignore",

	// Security codes
	"SEC001": "secrets",
	"SEC002": "secrets",
	"SEC003": "secrets",
	"SEC004": "secrets",
	"SEC005": "secrets",

	// Shell codes
	"SHELL001": "shell.backtick",

	// GitHub CLI codes
	"GH001": "github.issue",

	// Plugin codes
	"PLUG001": "plugins",
	"PLUG002": "plugins",
	"PLUG003": "plugins",
	"PLUG004": "plugins",
	"PLUG005": "plugins",
}

// IsDisabled checks if a key (error code or validator name) is disabled and not expired.
func (o *OverridesConfig) IsDisabled(key string) bool {
	if o == nil || len(o.Entries) == 0 {
		return false
	}

	entry, exists := o.Entries[key]
	if !exists {
		return false
	}

	return entry.IsActive() && entry.isDisabled()
}

// IsExplicitlyEnabled checks if a key has an active "enable" override.
func (o *OverridesConfig) IsExplicitlyEnabled(key string) bool {
	if o == nil || len(o.Entries) == 0 {
		return false
	}

	entry, exists := o.Entries[key]
	if !exists {
		return false
	}

	return entry.IsActive() && !entry.isDisabled()
}

// IsCodeDisabled checks if a specific error code is disabled.
// First checks for an exact code match, then falls back to checking
// the parent validator name.
func (o *OverridesConfig) IsCodeDisabled(code string) bool {
	if o == nil || len(o.Entries) == 0 {
		return false
	}

	// Check exact code match first
	if o.IsDisabled(code) {
		return true
	}

	// Check if the code is explicitly enabled (overrides parent)
	if o.IsExplicitlyEnabled(code) {
		return false
	}

	// Fall back to parent validator check
	if parent, ok := CodeToValidator[code]; ok {
		return o.IsDisabled(parent)
	}

	return false
}

// ActiveEntries returns all non-expired override entries.
func (o *OverridesConfig) ActiveEntries() map[string]*OverrideEntry {
	if o == nil || len(o.Entries) == 0 {
		return nil
	}

	result := make(map[string]*OverrideEntry)

	for key, entry := range o.Entries {
		if entry.IsActive() {
			result[key] = entry
		}
	}

	return result
}

// ExpiredEntries returns all expired override entries.
func (o *OverridesConfig) ExpiredEntries() map[string]*OverrideEntry {
	if o == nil || len(o.Entries) == 0 {
		return nil
	}

	result := make(map[string]*OverrideEntry)

	for key, entry := range o.Entries {
		if entry.IsExpired() {
			result[key] = entry
		}
	}

	return result
}

// IsKnownTarget returns true if the target is a known error code or validator name.
func IsKnownTarget(target string) bool {
	// Check if it's a known error code
	if _, ok := CodeToValidator[target]; ok {
		return true
	}

	// Check if it's a known validator name (appears as a value in CodeToValidator)
	for _, v := range CodeToValidator {
		if v == target {
			return true
		}
	}

	return false
}

// IsExpired returns true if the entry has an expiry and it has passed.
func (e *OverrideEntry) IsExpired() bool {
	if e == nil || e.ExpiresAt == "" {
		return false
	}

	t, err := time.Parse(time.RFC3339, e.ExpiresAt)
	if err != nil {
		return false
	}

	return time.Now().After(t)
}

// IsActive returns true if the entry is not expired.
func (e *OverrideEntry) IsActive() bool {
	if e == nil {
		return false
	}

	return !e.IsExpired()
}

// isDisabled returns whether this entry represents a "disable" override.
// Default is true (disabled) when the field is nil.
func (e *OverrideEntry) isDisabled() bool {
	if e == nil || e.Disabled == nil {
		return true
	}

	return *e.Disabled
}
