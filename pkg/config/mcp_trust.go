package config

// MCP trust actions.
const (
	MCPTrustActionBlock = "block"
	MCPTrustActionWarn  = "warn"
	MCPTrustActionAllow = "allow"
)

// MCPTrustConfig decides which MCP servers the agent may call, based on the
// provenance the harness reports (Claude mcp_server.source, Gemini
// mcp_context transport) rather than on the server name, which a configured
// server picks itself and which the mcp__<server>__ tool name repeats.
// Disabled by default.
//
// Example configuration:
//
//	[mcp_trust]
//	enabled = true
//	trusted_sources = ["managed", "enterprise", "user"]
//	untrusted = "block"
//	unknown_provenance = "warn"
//
//	[[mcp_trust.servers]]
//	name = "github"
//	source = "project"
type MCPTrustConfig struct {
	// Enabled turns MCP trust checks on. Default: false.
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// TrustedSources lists Claude mcp_server.source values whose servers
	// are trusted whatever their name, such as sdk, plugin, user, project,
	// local, dynamic, managed, enterprise, claudeai or agent.
	TrustedSources []string `json:"trusted_sources,omitempty" koanf:"trusted_sources" toml:"trusted_sources,omitempty"`

	// Servers lists individually trusted servers. Each entry must name at
	// least one provenance field (source, command or url): a name alone
	// can be claimed by any server.
	Servers []*MCPTrustedServer `json:"servers,omitempty" koanf:"servers" toml:"servers,omitempty"`

	// Untrusted is what happens to a call whose reported provenance matches
	// nothing trusted: "block" (default) or "warn".
	Untrusted string `json:"untrusted,omitempty" jsonschema:"enum=block,enum=warn" koanf:"untrusted" toml:"untrusted,omitempty"`

	// UnknownProvenance is what happens to an MCP call whose payload
	// carries no provenance (Codex, opencode, Claude before 2.1.274): "block"
	// (default), "warn" or "allow". A Claude source klaudiush does not know
	// is provenance all the same: it is trusted only when listed exactly.
	UnknownProvenance string `json:"unknown_provenance,omitempty" jsonschema:"enum=block,enum=warn,enum=allow" koanf:"unknown_provenance" toml:"unknown_provenance,omitempty"`
}

// MCPTrustedServer trusts one server. Every field that is set must match
// the provenance the harness reports. Glob patterns are accepted.
type MCPTrustedServer struct {
	// Name matches the server name the harness reports. Not enough on its own.
	Name string `json:"name,omitempty" koanf:"name" toml:"name,omitempty"`

	// Source matches Claude mcp_server.source.
	Source string `json:"source,omitempty" koanf:"source" toml:"source,omitempty"`

	// Command matches the stdio command Gemini reports in mcp_context.
	Command string `json:"command,omitempty" koanf:"command" toml:"command,omitempty"`

	// URL matches the HTTP or SSE endpoint Gemini reports in mcp_context.
	URL string `json:"url,omitempty" koanf:"url" toml:"url,omitempty"`

	// Tools limits the trust to these tool names (globs). Default: all.
	Tools []string `json:"tools,omitempty" koanf:"tools" toml:"tools,omitempty"`
}

// HasProvenance reports whether the entry names a provenance field.
func (s *MCPTrustedServer) HasProvenance() bool {
	return s != nil && (s.Source != "" || s.Command != "" || s.URL != "")
}

// IsEnabled reports whether MCP trust checks are on. Defaults to false.
func (m *MCPTrustConfig) IsEnabled() bool {
	if m == nil || m.Enabled == nil {
		return false
	}

	return *m.Enabled
}

// GetUntrusted returns the action for untrusted servers, default "block".
func (m *MCPTrustConfig) GetUntrusted() string {
	if m == nil || m.Untrusted == "" {
		return MCPTrustActionBlock
	}

	return m.Untrusted
}

// GetUnknownProvenance returns the action for calls without provenance,
// default "block".
func (m *MCPTrustConfig) GetUnknownProvenance() string {
	if m == nil || m.UnknownProvenance == "" {
		return MCPTrustActionBlock
	}

	return m.UnknownProvenance
}

// GetMCPTrust returns the MCP trust config, creating it if missing.
func (c *Config) GetMCPTrust() *MCPTrustConfig {
	if c.MCPTrust == nil {
		c.MCPTrust = &MCPTrustConfig{}
	}

	return c.MCPTrust
}
