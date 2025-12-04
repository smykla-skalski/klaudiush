// Package config provides configuration schema types for klaudiush validators.
package config

// ShellConfig groups all shell-related validator configurations.
type ShellConfig struct {
	// Backtick validator configuration
	Backtick *BacktickValidatorConfig `json:"backtick,omitempty" koanf:"backtick" toml:"backtick"`
}

// BacktickValidatorConfig configures the backtick validator.
type BacktickValidatorConfig struct {
	ValidatorConfig
	// CheckAllCommands enables comprehensive backtick checking for all Bash commands,
	// not just git commit and gh pr/issue create. Default: false (specific commands only)
	CheckAllCommands bool `json:"check_all_commands,omitempty"    koanf:"check_all_commands"    toml:"check_all_commands"` //nolint:tagalign // golines formatting
	// CheckUnquoted enables detection of unquoted backticks. Default: true
	// Uses pointer to distinguish between unset (nil=true) and explicitly false.
	CheckUnquoted *bool `json:"check_unquoted,omitempty"        koanf:"check_unquoted"        toml:"check_unquoted"` //nolint:tagalign // golines formatting
	// SuggestSingleQuotes suggests using single quotes instead of double quotes
	// when the string contains no variables. Default: true
	// Uses pointer to distinguish between unset (nil=true) and explicitly false.
	SuggestSingleQuotes *bool `json:"suggest_single_quotes,omitempty" koanf:"suggest_single_quotes" toml:"suggest_single_quotes"`
}

// CheckUnquotedOrDefault returns the CheckUnquoted value, defaulting to true if nil.
func (c *BacktickValidatorConfig) CheckUnquotedOrDefault() bool {
	if c == nil || c.CheckUnquoted == nil {
		return true
	}

	return *c.CheckUnquoted
}

// SuggestSingleQuotesOrDefault returns the SuggestSingleQuotes value, defaulting to true if nil.
func (c *BacktickValidatorConfig) SuggestSingleQuotesOrDefault() bool {
	if c == nil || c.SuggestSingleQuotes == nil {
		return true
	}

	return *c.SuggestSingleQuotes
}
