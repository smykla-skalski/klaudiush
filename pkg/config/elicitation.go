// Package config provides configuration schema types for klaudiush validators.
package config

// ElicitationConfig groups all elicitation-related validator configurations.
type ElicitationConfig struct {
	// Server validator configuration for MCP server allow/deny lists.
	Server *ElicitationServerConfig `json:"server,omitempty" koanf:"server" toml:"server,omitempty"`
}

// ElicitationServerConfig configures the MCP server elicitation validator.
type ElicitationServerConfig struct {
	ValidatorConfig `koanf:",squash"`

	// AllowedServers is a list of allowed MCP server name patterns (glob).
	// When set, only servers matching a pattern are allowed.
	AllowedServers []string `json:"allowed_servers,omitempty" koanf:"allowed_servers" toml:"allowed_servers,omitempty"`

	// DeniedServers is a list of denied MCP server name patterns (glob).
	// When set, servers matching a pattern are blocked.
	DeniedServers []string `json:"denied_servers,omitempty" koanf:"denied_servers" toml:"denied_servers,omitempty"`

	// BlockURLMode blocks URL-mode elicitations when true.
	BlockURLMode *bool `json:"block_url_mode,omitempty" koanf:"block_url_mode" toml:"block_url_mode,omitempty"`
}

// IsBlockURLMode returns true if URL mode is blocked.
func (c *ElicitationServerConfig) IsBlockURLMode() bool {
	if c == nil || c.BlockURLMode == nil {
		return false
	}

	return *c.BlockURLMode
}
