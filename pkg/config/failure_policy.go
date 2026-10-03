package config

import "time"

// Failure policy modes. FailureModeIgnore is accepted only for missing_tools.
const (
	FailureModeWarn   = "warn"
	FailureModeBlock  = "block"
	FailureModeIgnore = "ignore"
)

// DefaultFailureDeadline bounds one hook run. It stays below the 30 second
// timeout klaudiush registers with every provider, leaving time to answer
// before the harness kills the hook and lets the action through.
const DefaultFailureDeadline = 20 * time.Second

// FailurePolicyConfig decides what happens when validation cannot run: a
// linter is missing or times out, a plugin fails, a validator crashes, the
// hook input or configuration cannot be read, or the hook runs out of time.
//
// Example configuration:
//
//	[failure_policy]
//	mode = "block"
//	missing_tools = "warn"
//	critical = ["git.commit", "secrets"]
//	deadline = "20s"
type FailurePolicyConfig struct {
	// Mode applies to every check that could not run and to failures of the
	// hook itself. "warn" lets the action through with a "Validation
	// unavailable" warning, "block" denies it where the event can be denied.
	// Unset keeps each check's own choice (plugin failures block, everything
	// else warns).
	Mode string `json:"mode,omitempty" jsonschema:"enum=warn,enum=block" koanf:"mode" toml:"mode,omitempty"`

	// MissingTools applies when a linter a check needs is not installed:
	// "ignore" (default) skips the check silently, "warn" reports it, "block"
	// denies the action.
	MissingTools string `json:"missing_tools,omitempty" jsonschema:"enum=ignore,enum=warn,enum=block" koanf:"missing_tools" toml:"missing_tools,omitempty"`

	// Critical lists validators that block whenever they cannot run, for any
	// reason including a missing tool. Accepts runtime names ("commit",
	// "shellscript") and override names ("git.commit", "file.shellscript").
	Critical []string `json:"critical,omitempty" koanf:"critical" toml:"critical,omitempty"`

	// Deadline bounds one hook run. Checks still running are reported as
	// timed out and the policy applies. Keep it below the provider hook
	// timeout. Default: "20s".
	Deadline Duration `json:"deadline,omitempty" koanf:"deadline" toml:"deadline,omitempty"`
}

// GetMode returns the configured mode, or "" when unset.
func (f *FailurePolicyConfig) GetMode() string {
	if f == nil {
		return ""
	}

	return f.Mode
}

// GetMissingTools returns the missing tool action, defaulting to "ignore".
func (f *FailurePolicyConfig) GetMissingTools() string {
	if f == nil || f.MissingTools == "" {
		return FailureModeIgnore
	}

	return f.MissingTools
}

// GetCritical returns the critical validator names.
func (f *FailurePolicyConfig) GetCritical() []string {
	if f == nil {
		return nil
	}

	return f.Critical
}

// GetDeadline returns the hook deadline, defaulting to DefaultFailureDeadline.
func (f *FailurePolicyConfig) GetDeadline() time.Duration {
	if f == nil || f.Deadline <= 0 {
		return DefaultFailureDeadline
	}

	return time.Duration(f.Deadline)
}

// GetFailurePolicy returns the failure policy config, creating it if missing.
func (c *Config) GetFailurePolicy() *FailurePolicyConfig {
	if c.FailurePolicy == nil {
		c.FailurePolicy = &FailurePolicyConfig{}
	}

	return c.FailurePolicy
}
