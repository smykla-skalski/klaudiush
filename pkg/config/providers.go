package config

// ProvidersConfig contains provider-specific integration configuration.
type ProvidersConfig struct {
	Claude *ClaudeProviderConfig `json:"claude,omitempty" koanf:"claude" toml:"claude,omitempty"`
	Codex  *CodexProviderConfig  `json:"codex,omitempty"  koanf:"codex"  toml:"codex,omitempty"`
	Gemini *GeminiProviderConfig `json:"gemini,omitempty" koanf:"gemini" toml:"gemini,omitempty"`
}

// ClaudeProviderConfig contains Claude-specific integration toggles.
type ClaudeProviderConfig struct {
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`
}

// CodexProviderConfig contains Codex-specific integration toggles.
type CodexProviderConfig struct {
	Enabled         *bool  `json:"enabled,omitempty"           koanf:"enabled"           toml:"enabled,omitempty"`
	Experimental    *bool  `json:"experimental,omitempty"      koanf:"experimental"      toml:"experimental,omitempty"`
	HooksConfigPath string `json:"hooks_config_path,omitempty" koanf:"hooks_config_path" toml:"hooks_config_path,omitempty"`
}

// GeminiProviderConfig contains Gemini-specific integration toggles.
type GeminiProviderConfig struct {
	Enabled      *bool  `json:"enabled,omitempty"       koanf:"enabled"       toml:"enabled,omitempty"`
	SettingsPath string `json:"settings_path,omitempty" koanf:"settings_path" toml:"settings_path,omitempty"`
}

// GetClaude returns the Claude provider config, creating it if needed.
func (p *ProvidersConfig) GetClaude() *ClaudeProviderConfig {
	if p.Claude == nil {
		p.Claude = &ClaudeProviderConfig{}
	}

	return p.Claude
}

// GetCodex returns the Codex provider config, creating it if needed.
func (p *ProvidersConfig) GetCodex() *CodexProviderConfig {
	if p.Codex == nil {
		p.Codex = &CodexProviderConfig{}
	}

	return p.Codex
}

// GetGemini returns the Gemini provider config, creating it if needed.
func (p *ProvidersConfig) GetGemini() *GeminiProviderConfig {
	if p.Gemini == nil {
		p.Gemini = &GeminiProviderConfig{}
	}

	return p.Gemini
}

// IsEnabled returns whether Claude integration is enabled.
func (c *ClaudeProviderConfig) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

// IsEnabled returns whether Codex integration is enabled.
func (c *CodexProviderConfig) IsEnabled() bool {
	if c == nil || c.Enabled == nil {
		return false
	}

	return *c.Enabled
}

// IsExperimentalEnabled returns whether Codex experimental automation is enabled.
func (c *CodexProviderConfig) IsExperimentalEnabled() bool {
	if c == nil || c.Experimental == nil {
		return false
	}

	return *c.Experimental
}

// HasHooksConfigPath returns true when the Codex hooks config path is configured.
func (c *CodexProviderConfig) HasHooksConfigPath() bool {
	return c != nil && c.HooksConfigPath != ""
}

// IsEnabled returns whether Gemini integration is enabled.
func (g *GeminiProviderConfig) IsEnabled() bool {
	if g == nil || g.Enabled == nil {
		return false
	}

	return *g.Enabled
}

// HasSettingsPath returns true when the Gemini settings path is configured.
func (g *GeminiProviderConfig) HasSettingsPath() bool {
	return g != nil && g.SettingsPath != ""
}
