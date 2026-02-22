package config

import (
	"slices"
	"time"

	"github.com/invopop/jsonschema"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	// defaultPluginTimeout is the default timeout for plugin operations.
	defaultPluginTimeout = 5 * time.Second
)

// PluginConfig contains configuration for the plugin system.
type PluginConfig struct {
	// Enabled controls whether plugin support is enabled.
	// Default: false
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Directory is the path where plugins are located.
	// Default: "~/.klaudiush/plugins"
	Directory string `json:"directory,omitempty" koanf:"directory" toml:"directory,omitempty"`

	// Plugins is the list of plugin configurations.
	Plugins []*PluginInstanceConfig `json:"plugins,omitempty" koanf:"plugins" toml:"plugins,omitempty"`

	// DefaultTimeout is the default timeout for plugin operations.
	// Default: "5s"
	DefaultTimeout Duration `json:"default_timeout,omitempty" koanf:"default_timeout" toml:"default_timeout,omitempty"`
}

// PluginInstanceConfig configures a single plugin instance.
type PluginInstanceConfig struct {
	// Name is the unique identifier for this plugin instance.
	Name string `json:"name" koanf:"name" toml:"name,omitempty"`

	// Type specifies the plugin type ("exec").
	Type PluginType `json:"type" koanf:"type" toml:"type,omitempty"`

	// Enabled controls whether this plugin is enabled.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled,omitempty"`

	// Path is the file path for exec plugins.
	// Example: "~/.klaudiush/plugins/my-plugin.sh"
	Path string `json:"path,omitempty" koanf:"path" toml:"path,omitempty"`

	// Args are command-line arguments for exec plugins.
	Args []string `json:"args,omitempty" koanf:"args" toml:"args,omitempty"`

	// Timeout is the maximum time to wait for plugin operations.
	// Default: inherited from PluginConfig.DefaultTimeout
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout,omitempty"`

	// Predicate configures when this plugin should be invoked.
	Predicate *PluginPredicate `json:"predicate,omitempty" koanf:"predicate" toml:"predicate,omitempty"`

	// Config contains plugin-specific configuration passed to the plugin.
	// The structure is defined by the plugin author.
	Config map[string]any `json:"config,omitempty" koanf:"config" toml:"config,omitempty"`

	// ProjectRoot is the project root directory, set by the loader for path validation.
	// This field is not serialized and is populated at runtime.
	ProjectRoot string `json:"-" koanf:"-" toml:"-"`
}

// PluginType represents the type of plugin loader to use.
type PluginType string

const (
	// PluginTypeExec executes plugins as subprocesses with JSON I/O.
	PluginTypeExec PluginType = "exec"
)

// JSONSchema returns the JSON Schema for the PluginType type.
func (PluginType) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "string",
		Enum: []any{"exec"},
	}
}

// PluginPredicate configures when a plugin should be invoked.
type PluginPredicate struct {
	// EventTypes filters by event type.
	// Example: ["PreToolUse", "PostToolUse"]
	EventTypes []string `json:"event_types,omitempty" koanf:"event_types" toml:"event_types,omitempty"`

	// ToolTypes filters by tool type.
	// Example: ["Bash", "Write", "Edit"]
	ToolTypes []string `json:"tool_types,omitempty" koanf:"tool_types" toml:"tool_types,omitempty"`

	// FilePatterns filters by file path patterns (glob syntax).
	// Only applies to file operation tools (Write, Edit, MultiEdit).
	// Example: ["*.go", "**/*.tf"]
	FilePatterns []string `json:"file_patterns,omitempty" koanf:"file_patterns" toml:"file_patterns,omitempty"`

	// CommandPatterns filters by command patterns (regex).
	// Only applies to Bash tool.
	// Example: ["^git commit", "terraform apply"]
	CommandPatterns []string `json:"command_patterns,omitempty" koanf:"command_patterns" toml:"command_patterns,omitempty"`
}

// IsEnabled returns whether the plugin system is enabled.
func (p *PluginConfig) IsEnabled() bool {
	if p == nil || p.Enabled == nil {
		return false
	}

	return *p.Enabled
}

// GetDefaultTimeout returns the default timeout for plugin operations.
func (p *PluginConfig) GetDefaultTimeout() time.Duration {
	if p == nil || p.DefaultTimeout == 0 {
		return defaultPluginTimeout
	}

	return time.Duration(p.DefaultTimeout)
}

// GetDirectory returns the plugin directory from config, or empty string if not set.
// Callers should use xdg.PluginDir() as default when this returns empty.
func (p *PluginConfig) GetDirectory() string {
	if p == nil || p.Directory == "" {
		return ""
	}

	return p.Directory
}

// IsInstanceEnabled returns whether this plugin instance is enabled.
func (c *PluginInstanceConfig) IsInstanceEnabled() bool {
	if c.Enabled == nil {
		return true
	}

	return *c.Enabled
}

// GetTimeout returns the timeout for this plugin, falling back to the provided default.
func (c *PluginInstanceConfig) GetTimeout(defaultTimeout time.Duration) time.Duration {
	if c.Timeout == 0 {
		return defaultTimeout
	}

	return time.Duration(c.Timeout)
}

// MatchesEventType returns whether this predicate matches the given event type.
func (p *PluginPredicate) MatchesEventType(eventType hook.EventType) bool {
	if p == nil || len(p.EventTypes) == 0 {
		return true
	}

	eventTypeStr := eventType.String()

	return slices.Contains(p.EventTypes, eventTypeStr)
}

// MatchesToolType returns whether this predicate matches the given tool type.
func (p *PluginPredicate) MatchesToolType(toolType hook.ToolType) bool {
	if p == nil || len(p.ToolTypes) == 0 {
		return true
	}

	toolTypeStr := toolType.String()

	return slices.Contains(p.ToolTypes, toolTypeStr)
}

// GetPlugin returns the plugin config, creating it if it doesn't exist.
func (v *ValidatorsConfig) GetPlugin() *PluginConfig {
	if v == nil {
		return nil
	}

	return nil // Plugins are stored at the root level, not under validators
}
