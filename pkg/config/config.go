// Package config provides configuration schema types for klaudiush validators.
package config

// CurrentConfigVersion is the latest config schema version.
const CurrentConfigVersion = 1

// Config represents the root configuration for klaudiush.
type Config struct {
	// Version is the config schema version. Defaults to 1 when omitted.
	Version int `json:"version,omitempty" koanf:"version" toml:"version,omitempty"`

	// Validators groups all validator configurations.
	Validators *ValidatorsConfig `json:"validators,omitempty" koanf:"validators" toml:"validators,omitempty"`

	// Global settings that apply across all validators.
	Global *GlobalConfig `json:"global,omitempty" koanf:"global" toml:"global,omitempty"`

	// Plugins contains configuration for external plugins.
	Plugins *PluginConfig `json:"plugins,omitempty" koanf:"plugins" toml:"plugins,omitempty"`

	// Rules contains dynamic validation rule configuration.
	Rules *RulesConfig `json:"rules,omitempty" koanf:"rules" toml:"rules,omitempty"`

	// Exceptions contains exception workflow configuration.
	Exceptions *ExceptionsConfig `json:"exceptions,omitempty" koanf:"exceptions" toml:"exceptions,omitempty"`

	// Backup contains configuration for the backup system.
	Backup *BackupConfig `json:"backup,omitempty" koanf:"backup" toml:"backup,omitempty"`

	// CrashDump contains configuration for the crash dump system.
	CrashDump *CrashDumpConfig `json:"crash_dump,omitempty" koanf:"crash_dump" toml:"crash_dump,omitempty"`

	// Patterns contains configuration for failure pattern tracking.
	Patterns *PatternsConfig `json:"patterns,omitempty" koanf:"patterns" toml:"patterns,omitempty"`

	// Overrides contains persistent disable/enable overrides for error codes and validators.
	Overrides *OverridesConfig `json:"overrides,omitempty" koanf:"overrides" toml:"overrides,omitempty"`
}

// ValidatorsConfig groups all validator configurations by category.
type ValidatorsConfig struct {
	// Git validator configurations.
	Git *GitConfig `json:"git,omitempty" koanf:"git" toml:"git,omitempty"`

	// GitHub CLI validator configurations.
	GitHub *GitHubConfig `json:"github,omitempty" koanf:"github" toml:"github,omitempty"`

	// File validator configurations.
	File *FileConfig `json:"file,omitempty" koanf:"file" toml:"file,omitempty"`

	// Notification validator configurations.
	Notification *NotificationConfig `json:"notification,omitempty" koanf:"notification" toml:"notification,omitempty"`

	// Secrets validator configurations.
	Secrets *SecretsConfig `json:"secrets,omitempty" koanf:"secrets" toml:"secrets,omitempty"`

	// Shell validator configurations.
	Shell *ShellConfig `json:"shell,omitempty" koanf:"shell" toml:"shell,omitempty"`
}

// GlobalConfig contains global settings that apply to all validators.
type GlobalConfig struct {
	// UseSDKGit controls whether to use the go-git SDK or CLI for git operations.
	// Default: true (use SDK for better performance)
	UseSDKGit *bool `json:"use_sdk_git,omitempty" koanf:"use_sdk_git" toml:"use_sdk_git,omitempty"`

	// DefaultTimeout is the default timeout for all operations that support timeouts.
	// Individual validator timeouts override this value.
	// Default: "10s"
	DefaultTimeout Duration `json:"default_timeout,omitempty" koanf:"default_timeout" toml:"default_timeout,omitempty"`

	// ParallelExecution enables parallel validator execution.
	// Default: false (sequential execution)
	ParallelExecution *bool `json:"parallel_execution,omitempty" koanf:"parallel_execution" toml:"parallel_execution,omitempty"`

	// MaxCPUWorkers is the maximum number of concurrent CPU-bound validators.
	// Default: runtime.NumCPU()
	MaxCPUWorkers *int `json:"max_cpu_workers,omitempty" koanf:"max_cpu_workers" toml:"max_cpu_workers,omitempty"`

	// MaxIOWorkers is the maximum number of concurrent I/O-bound validators.
	// Default: runtime.NumCPU() * 2
	MaxIOWorkers *int `json:"max_io_workers,omitempty" koanf:"max_io_workers" toml:"max_io_workers,omitempty"`

	// MaxGitWorkers is the maximum number of concurrent git operations.
	// Default: 1 (serialized to avoid index lock contention)
	MaxGitWorkers *int `json:"max_git_workers,omitempty" koanf:"max_git_workers" toml:"max_git_workers,omitempty"`
}

// IsParallelExecutionEnabled returns whether parallel execution is enabled.
func (g *GlobalConfig) IsParallelExecutionEnabled() bool {
	if g == nil || g.ParallelExecution == nil {
		return false
	}

	return *g.ParallelExecution
}

// GetValidators returns the validators config, creating it if it doesn't exist.
func (c *Config) GetValidators() *ValidatorsConfig {
	if c.Validators == nil {
		c.Validators = &ValidatorsConfig{}
	}

	return c.Validators
}

// GetGlobal returns the global config, creating it if it doesn't exist.
func (c *Config) GetGlobal() *GlobalConfig {
	if c.Global == nil {
		c.Global = &GlobalConfig{}
	}

	return c.Global
}

// GetGit returns the git validators config, creating it if it doesn't exist.
func (v *ValidatorsConfig) GetGit() *GitConfig {
	if v.Git == nil {
		v.Git = &GitConfig{}
	}

	return v.Git
}

// GetGitHub returns the GitHub CLI validators config, creating it if it doesn't exist.
func (v *ValidatorsConfig) GetGitHub() *GitHubConfig {
	if v.GitHub == nil {
		v.GitHub = &GitHubConfig{}
	}

	return v.GitHub
}

// GetFile returns the file validators config, creating it if it doesn't exist.
func (v *ValidatorsConfig) GetFile() *FileConfig {
	if v.File == nil {
		v.File = &FileConfig{}
	}

	return v.File
}

// GetNotification returns the notification validators config, creating it if it doesn't exist.
func (v *ValidatorsConfig) GetNotification() *NotificationConfig {
	if v.Notification == nil {
		v.Notification = &NotificationConfig{}
	}

	return v.Notification
}

// GetSecrets returns the secrets validators config, creating it if it doesn't exist.
func (v *ValidatorsConfig) GetSecrets() *SecretsConfig {
	if v.Secrets == nil {
		v.Secrets = &SecretsConfig{}
	}

	return v.Secrets
}

// GetPlugins returns the plugins config, creating it if it doesn't exist.
func (c *Config) GetPlugins() *PluginConfig {
	if c.Plugins == nil {
		c.Plugins = &PluginConfig{}
	}

	return c.Plugins
}

// GetRules returns the rules config, creating it if it doesn't exist.
func (c *Config) GetRules() *RulesConfig {
	if c.Rules == nil {
		c.Rules = &RulesConfig{}
	}

	return c.Rules
}

// GetExceptions returns the exceptions config, creating it if it doesn't exist.
func (c *Config) GetExceptions() *ExceptionsConfig {
	if c.Exceptions == nil {
		c.Exceptions = &ExceptionsConfig{}
	}

	return c.Exceptions
}

// GetBackup returns the backup config, creating it if it doesn't exist.
func (c *Config) GetBackup() *BackupConfig {
	if c.Backup == nil {
		c.Backup = &BackupConfig{}
	}

	return c.Backup
}

// GetCrashDump returns the crash dump config, creating it if it doesn't exist.
func (c *Config) GetCrashDump() *CrashDumpConfig {
	if c.CrashDump == nil {
		c.CrashDump = &CrashDumpConfig{}
	}

	return c.CrashDump
}

// GetPatterns returns the patterns config, creating it if it doesn't exist.
func (c *Config) GetPatterns() *PatternsConfig {
	if c.Patterns == nil {
		c.Patterns = &PatternsConfig{}
	}

	return c.Patterns
}

// GetOverrides returns the overrides config, creating it if it doesn't exist.
func (c *Config) GetOverrides() *OverridesConfig {
	if c.Overrides == nil {
		c.Overrides = &OverridesConfig{}
	}

	return c.Overrides
}
