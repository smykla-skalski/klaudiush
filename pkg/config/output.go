// Package config provides configuration schema types for klaudiush validators.
package config

// OutputConfig controls what klaudiush shows the user and asks of the agent.
//
// Example configuration:
//
//	[output]
//	user_messages = true
//	validation_messages = true
//	agent_summary = true
type OutputConfig struct {
	// UserMessages is the master switch for everything klaudiush shows the user
	// directly: validation details, the bypass reminder, and update notices.
	// Default: true
	UserMessages *bool `json:"user_messages,omitempty" koanf:"user_messages" toml:"user_messages,omitempty"`

	// ValidationMessages controls the block and warning details shown to the user.
	// Default: true
	ValidationMessages *bool `json:"validation_messages,omitempty" koanf:"validation_messages" toml:"validation_messages,omitempty"`

	// AgentSummary asks the agent to tell the user in one plain sentence what
	// klaudiush blocked and why.
	// Default: true
	AgentSummary *bool `json:"agent_summary,omitempty" koanf:"agent_summary" toml:"agent_summary,omitempty"`
}

// IsUserMessagesEnabled reports whether any user-facing output is shown. Defaults to true.
func (o *OutputConfig) IsUserMessagesEnabled() bool {
	if o == nil || o.UserMessages == nil {
		return true
	}

	return *o.UserMessages
}

// IsValidationMessagesEnabled reports whether validation details are shown to
// the user. The UserMessages master switch overrides it. Defaults to true.
func (o *OutputConfig) IsValidationMessagesEnabled() bool {
	if !o.IsUserMessagesEnabled() {
		return false
	}

	if o == nil || o.ValidationMessages == nil {
		return true
	}

	return *o.ValidationMessages
}

// IsAgentSummaryEnabled reports whether the agent is asked to summarize blocks
// for the user. Defaults to true.
func (o *OutputConfig) IsAgentSummaryEnabled() bool {
	if o == nil || o.AgentSummary == nil {
		return true
	}

	return *o.AgentSummary
}
