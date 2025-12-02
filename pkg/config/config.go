// Package config provides configuration schema types for klaudiush validators.
package config

// Config represents the root configuration for klaudiush.
type Config struct {
	// Validators groups all validator configurations.
	Validators *ValidatorsConfig `json:"validators,omitempty" koanf:"validators" toml:"validators"`

	// Global settings that apply across all validators.
	Global *GlobalConfig `json:"global,omitempty" koanf:"global" toml:"global"`

	// Plugins contains configuration for external plugins.
	Plugins *PluginConfig `json:"plugins,omitempty" koanf:"plugins" toml:"plugins"`

	// Rules contains dynamic validation rule configuration.
	Rules *RulesConfig `json:"rules,omitempty" koanf:"rules" toml:"rules"`

	// Exceptions contains exception workflow configuration.
	Exceptions *ExceptionsConfig `json:"exceptions,omitempty" koanf:"exceptions" toml:"exceptions"`

	// Backup contains configuration for the backup system.
	Backup *BackupConfig `json:"backup,omitempty" koanf:"backup" toml:"backup"`
}

// ValidatorsConfig groups all validator configurations by category.
type ValidatorsConfig struct {
	// Git validator configurations.
	Git *GitConfig `json:"git,omitempty" koanf:"git" toml:"git"`

	// GitHub CLI validator configurations.
	GitHub *GitHubConfig `json:"github,omitempty" koanf:"github" toml:"github"`

	// File validator configurations.
	File *FileConfig `json:"file,omitempty" koanf:"file" toml:"file"`

	// Notification validator configurations.
	Notification *NotificationConfig `json:"notification,omitempty" koanf:"notification" toml:"notification"`

	// Secrets validator configurations.
	Secrets *SecretsConfig `json:"secrets,omitempty" koanf:"secrets" toml:"secrets"`

	// Shell validator configurations.
	Shell *ShellConfig `json:"shell,omitempty" koanf:"shell" toml:"shell"`
}

// GlobalConfig contains global settings that apply to all validators.
type GlobalConfig struct {
	// UseSDKGit controls whether to use the go-git SDK or CLI for git operations.
	// Default: true (use SDK for better performance)
	UseSDKGit *bool `json:"use_sdk_git,omitempty" koanf:"use_sdk_git" toml:"use_sdk_git"`

	// DefaultTimeout is the default timeout for all operations that support timeouts.
	// Individual validator timeouts override this value.
	// Default: "10s"
	DefaultTimeout Duration `json:"default_timeout,omitempty" koanf:"default_timeout" toml:"default_timeout"`

	// ParallelExecution enables parallel validator execution.
	// Default: false (sequential execution)
	ParallelExecution *bool `json:"parallel_execution,omitempty" koanf:"parallel_execution" toml:"parallel_execution"`

	// MaxCPUWorkers is the maximum number of concurrent CPU-bound validators.
	// Default: runtime.NumCPU()
	MaxCPUWorkers *int `json:"max_cpu_workers,omitempty" koanf:"max_cpu_workers" toml:"max_cpu_workers"`

	// MaxIOWorkers is the maximum number of concurrent I/O-bound validators.
	// Default: runtime.NumCPU() * 2
	MaxIOWorkers *int `json:"max_io_workers,omitempty" koanf:"max_io_workers" toml:"max_io_workers"`

	// MaxGitWorkers is the maximum number of concurrent git operations.
	// Default: 1 (serialized to avoid index lock contention)
	MaxGitWorkers *int `json:"max_git_workers,omitempty" koanf:"max_git_workers" toml:"max_git_workers"`
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
