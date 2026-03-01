// Package config provides configuration schema types for klaudiush validators.
package config

// NotificationConfig groups all notification-related validator configurations.
type NotificationConfig struct {
	// Bell validator configuration
	Bell *BellValidatorConfig `json:"bell,omitempty" koanf:"bell" toml:"bell,omitempty"`
}

// BellValidatorConfig configures the notification bell validator.
type BellValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// CustomCommand is an optional command to run instead of sending a bell character.
	// When set, this command will be executed for notification events instead of
	// writing ASCII 7 to /dev/tty.
	// The command is executed via shell and can be any valid command string.
	// Example: "osascript -e 'display notification \"Claude Code\" with title \"Notification\"'"
	// Default: "" (use bell character)
	CustomCommand string `json:"custom_command,omitempty" koanf:"custom_command" toml:"custom_command,omitempty"`
}
