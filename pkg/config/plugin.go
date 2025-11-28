package config

import (
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/smykla-labs/klaudiush/pkg/hook"
)

const (
	// defaultPluginTimeout is the default timeout for plugin operations.
	defaultPluginTimeout = 5 * time.Second
)

// PluginConfig contains configuration for the plugin system.
type PluginConfig struct {
	// Enabled controls whether plugin support is enabled.
	// Default: false
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// Directory is the path where plugins are located.
	// Default: "~/.klaudiush/plugins"
	Directory string `json:"directory,omitempty" koanf:"directory" toml:"directory"`

	// Plugins is the list of plugin configurations.
	Plugins []*PluginInstanceConfig `json:"plugins,omitempty" koanf:"plugins" toml:"plugins"`

	// DefaultTimeout is the default timeout for plugin operations.
	// Default: "5s"
	DefaultTimeout Duration `json:"default_timeout,omitempty" koanf:"default_timeout" toml:"default_timeout"`
}

// TLSConfig configures TLS for gRPC plugin connections.
type TLSConfig struct {
	// Enabled controls TLS usage.
	// nil = auto (TLS for remote, insecure for localhost)
	// true = require TLS
	// false = disable TLS (blocked for remote unless AllowInsecureRemote is set)
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// CertFile is the path to client certificate (for mTLS).
	CertFile string `json:"cert_file,omitempty" koanf:"cert_file" toml:"cert_file"`

	// KeyFile is the path to client private key (for mTLS).
	KeyFile string `json:"key_file,omitempty" koanf:"key_file" toml:"key_file"`

	// CAFile is the path to CA certificate for server verification.
	CAFile string `json:"ca_file,omitempty" koanf:"ca_file" toml:"ca_file"`

	// InsecureSkipVerify disables server certificate verification.
	// WARNING: Only use for development/testing.
	InsecureSkipVerify *bool `json:"insecure_skip_verify,omitempty" koanf:"insecure_skip_verify" toml:"insecure_skip_verify"`

	// AllowInsecureRemote explicitly allows insecure connection to remote host.
	// WARNING: This is a security risk and should only be used in trusted networks.
	AllowInsecureRemote *bool `json:"allow_insecure_remote,omitempty" koanf:"allow_insecure_remote" toml:"allow_insecure_remote"`
}

// IsEnabled returns whether TLS is explicitly enabled.
func (t *TLSConfig) IsEnabled() *bool {
	if t == nil {
		return nil
	}

	return t.Enabled
}

// ShouldSkipVerify returns whether to skip server certificate verification.
func (t *TLSConfig) ShouldSkipVerify() bool {
	if t == nil || t.InsecureSkipVerify == nil {
		return false
	}

	return *t.InsecureSkipVerify
}

// AllowsInsecureRemote returns whether insecure remote connections are allowed.
func (t *TLSConfig) AllowsInsecureRemote() bool {
	if t == nil || t.AllowInsecureRemote == nil {
		return false
	}

	return *t.AllowInsecureRemote
}

// PluginInstanceConfig configures a single plugin instance.
type PluginInstanceConfig struct {
	// Name is the unique identifier for this plugin instance.
	Name string `json:"name" koanf:"name" toml:"name"`

	// Type specifies the plugin type ("go", "grpc", or "exec").
	Type PluginType `json:"type" koanf:"type" toml:"type"`

	// Enabled controls whether this plugin is enabled.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// Path is the file path for Go plugins or exec plugins.
	// Example: "~/.klaudiush/plugins/my-plugin.so"
	Path string `json:"path,omitempty" koanf:"path" toml:"path"`

	// Address is the network address for gRPC plugins.
	// Example: "localhost:50051"
	Address string `json:"address,omitempty" koanf:"address" toml:"address"`

	// Args are command-line arguments for exec plugins.
	Args []string `json:"args,omitempty" koanf:"args" toml:"args"`

	// Timeout is the maximum time to wait for plugin operations.
	// Default: inherited from PluginConfig.DefaultTimeout
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout"`

	// Predicate configures when this plugin should be invoked.
	Predicate *PluginPredicate `json:"predicate,omitempty" koanf:"predicate" toml:"predicate"`

	// Config contains plugin-specific configuration passed to the plugin.
	// The structure is defined by the plugin author.
	Config map[string]any `json:"config,omitempty" koanf:"config" toml:"config"`

	// TLS contains TLS configuration for gRPC plugins.
	TLS *TLSConfig `json:"tls,omitempty" koanf:"tls" toml:"tls"`

	// ProjectRoot is the project root directory, set by the loader for path validation.
	// This field is not serialized and is populated at runtime.
	ProjectRoot string `json:"-" koanf:"-" toml:"-"`
}

// PluginType represents the type of plugin loader to use.
type PluginType string

const (
	// PluginTypeGo loads native Go plugins (.so files).
	PluginTypeGo PluginType = "go"

	// PluginTypeGRPC communicates with plugins over gRPC.
	PluginTypeGRPC PluginType = "grpc"

	// PluginTypeExec executes plugins as subprocesses with JSON I/O.
	PluginTypeExec PluginType = "exec"
)

// PluginPredicate configures when a plugin should be invoked.
type PluginPredicate struct {
	// EventTypes filters by event type.
	// Example: ["PreToolUse", "PostToolUse"]
	EventTypes []string `json:"event_types,omitempty" koanf:"event_types" toml:"event_types"`

	// ToolTypes filters by tool type.
	// Example: ["Bash", "Write", "Edit"]
	ToolTypes []string `json:"tool_types,omitempty" koanf:"tool_types" toml:"tool_types"`

	// FilePatterns filters by file path patterns (glob syntax).
	// Only applies to file operation tools (Write, Edit, MultiEdit).
	// Example: ["*.go", "**/*.tf"]
	FilePatterns []string `json:"file_patterns,omitempty" koanf:"file_patterns" toml:"file_patterns"`

	// CommandPatterns filters by command patterns (regex).
	// Only applies to Bash tool.
	// Example: ["^git commit", "terraform apply"]
	CommandPatterns []string `json:"command_patterns,omitempty" koanf:"command_patterns" toml:"command_patterns"`
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

// GetDirectory returns the plugin directory, with default fallback.
// Expands ~ to user home directory if present at the start of the path.
func (p *PluginConfig) GetDirectory() string {
	var dir string
	if p == nil || p.Directory == "" {
		dir = "~/.klaudiush/plugins"
	} else {
		dir = p.Directory
	}

	// Expand ~ to home directory
	if len(dir) > 0 && dir[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			// Fallback to unexpanded path if home dir cannot be determined
			return dir
		}

		if len(dir) == 1 {
			return homeDir
		}

		if dir[1] == '/' || dir[1] == filepath.Separator {
			return filepath.Join(homeDir, dir[2:])
		}
	}

	return dir
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
