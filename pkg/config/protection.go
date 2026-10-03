package config

import "slices"

// Claude ConfigChange sources. policy_settings changes cannot be blocked.
const (
	ConfigSourceUserSettings    = "user_settings"
	ConfigSourceProjectSettings = "project_settings"
	ConfigSourceLocalSettings   = "local_settings"
	ConfigSourcePolicySettings  = "policy_settings"
	ConfigSourceSkills          = "skills"
)

// ProtectionConfig keeps the agent from changing the files that enforce
// policy on it: klaudiush configuration and state, hook registrations of
// every supported harness, evidence check scripts, the klaudiush binary,
// and any extra paths listed here. Disabled by default.
//
// Example configuration:
//
//	[protection]
//	enabled = true
//	paths = ["scripts/ci/**"]
//	allow = [".claude/settings.local.json"]
type ProtectionConfig struct {
	// Enabled turns protection on. Default: false.
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Paths lists extra protected paths or glob patterns. Relative entries
	// are resolved against the project root; "~" is the home directory.
	// A pattern without a slash matches that name in any directory.
	Paths []string `json:"paths,omitempty" koanf:"paths" toml:"paths,omitempty"`

	// Allow lists paths or glob patterns, in the same form as Paths, that
	// the agent may change even though they are protected. This is how
	// policy maintenance is authorized: there is no automatic bypass.
	Allow []string `json:"allow,omitempty" koanf:"allow" toml:"allow,omitempty"`

	// ConfigChangeSources lists the Claude ConfigChange sources whose
	// changes are blocked from taking effect mid-session. Default:
	// user_settings, project_settings. local_settings is left out because
	// Claude Code writes "don't ask again" permissions there. An empty list
	// blocks none. policy_settings cannot be blocked.
	ConfigChangeSources []string `json:"config_change_sources,omitempty" jsonschema:"enum=user_settings,enum=project_settings,enum=local_settings,enum=policy_settings,enum=skills" koanf:"config_change_sources" toml:"config_change_sources,omitempty"`
}

// IsEnabled reports whether protection is on. Defaults to false.
func (p *ProtectionConfig) IsEnabled() bool {
	if p == nil || p.Enabled == nil {
		return false
	}

	return *p.Enabled
}

// GetPaths returns the extra protected paths.
func (p *ProtectionConfig) GetPaths() []string {
	if p == nil {
		return nil
	}

	return p.Paths
}

// GetAllow returns the paths exempt from protection.
func (p *ProtectionConfig) GetAllow() []string {
	if p == nil {
		return nil
	}

	return p.Allow
}

// DefaultConfigChangeSources returns the ConfigChange sources blocked by
// default: the shared settings files that register hooks. Local settings
// are left out because Claude Code itself writes permission rules there.
func DefaultConfigChangeSources() []string {
	return []string{
		ConfigSourceUserSettings,
		ConfigSourceProjectSettings,
	}
}

// GetConfigChangeSources returns the blocked ConfigChange sources. A nil
// list means the defaults; an explicitly empty one blocks none.
func (p *ProtectionConfig) GetConfigChangeSources() []string {
	if p == nil || p.ConfigChangeSources == nil {
		return DefaultConfigChangeSources()
	}

	return p.ConfigChangeSources
}

// BlocksConfigChange reports whether changes from source are blocked.
func (p *ProtectionConfig) BlocksConfigChange(source string) bool {
	return slices.Contains(p.GetConfigChangeSources(), source)
}

// GetProtection returns the protection config, creating it if missing.
func (c *Config) GetProtection() *ProtectionConfig {
	if c.Protection == nil {
		c.Protection = &ProtectionConfig{}
	}

	return c.Protection
}
