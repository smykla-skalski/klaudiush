// Package config provides internal configuration loading and processing.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	tomlparser "github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var (
	// ErrConfigNotFound is returned when no configuration file is found.
	ErrConfigNotFound = errors.New("configuration file not found")

	// ErrInvalidTOML is returned when the TOML file cannot be parsed.
	ErrInvalidTOML = errors.New("invalid TOML")

	// ErrInvalidPermissions is returned when config file has insecure permissions.
	ErrInvalidPermissions = errors.New("config file has insecure permissions")
)

const (
	// GlobalConfigFile is the name of the global configuration file.
	GlobalConfigFile = "config.toml"

	// GlobalConfigDir is the directory name for global configuration.
	GlobalConfigDir = ".klaudiush"

	// ProjectConfigDir is the directory name for project configuration.
	ProjectConfigDir = ".klaudiush"

	// ProjectConfigFile is the primary project configuration file name.
	ProjectConfigFile = "config.toml"

	// ProjectConfigFileAlt is the alternative project configuration file name.
	ProjectConfigFileAlt = "klaudiush.toml"
)

// Default configuration constants for koanf map defaults.
const (
	defaultTimeoutStr        = "10s"
	defaultGHAPITimeoutStr   = "5s"
	defaultTitleMaxLength    = 50
	defaultBodyMaxLineLength = 72
	defaultBodyLineTolerance = 5
	defaultContextLines      = 2

	// Exception defaults.
	defaultExceptionTokenPrefix     = "EXC"
	defaultExceptionRateLimitPerH   = 10
	defaultExceptionRateLimitPerD   = 50
	defaultExceptionAuditMaxSizeMB  = 10
	defaultExceptionAuditMaxAgeDays = 30
	defaultExceptionAuditMaxBackups = 3
	defaultExceptionMinReasonLength = 10

	// Session defaults.
	defaultSessionStateFile = "~/.klaudiush/session_state.json"
	defaultSessionMaxAgeStr = "24h"
)

// defaultValidTypes is the list of valid commit types.
var defaultValidTypes = []string{
	"build", "chore", "ci", "docs", "feat",
	"fix", "perf", "refactor", "revert", "style", "test",
}

// defaultBranchValidTypes is the list of valid branch type prefixes (excludes "revert").
var defaultBranchValidTypes = []string{
	"build", "chore", "ci", "docs", "feat",
	"fix", "perf", "refactor", "style", "test",
}

// KoanfLoader handles configuration loading from multiple sources using koanf.
// Precedence order (highest to lowest):
// 1. CLI Flags
// 2. Environment Variables (KLAUDIUSH_*)
// 3. Project Config (.klaudiush/config.toml or klaudiush.toml)
// 4. Global Config (~/.klaudiush/config.toml)
// 5. Defaults
type KoanfLoader struct {
	k        *koanf.Koanf
	homeDir  string
	workDir  string
	tomlOpts koanf.UnmarshalConf
}

// NewKoanfLoader creates a new KoanfLoader with default directories.
func NewKoanfLoader() (*KoanfLoader, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get home directory")
	}

	workDir, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get working directory")
	}

	return NewKoanfLoaderWithDirs(homeDir, workDir)
}

// NewKoanfLoaderWithDirs creates a new KoanfLoader with custom directories (for testing).
func NewKoanfLoaderWithDirs(homeDir, workDir string) (*KoanfLoader, error) {
	k := koanf.New(".")

	return &KoanfLoader{
		k:       k,
		homeDir: homeDir,
		workDir: workDir,
		tomlOpts: koanf.UnmarshalConf{
			Tag:       "koanf",
			FlatPaths: false,
		},
	}, nil
}

// Load loads configuration from all sources with precedence.
// Defaults → Global TOML → Project TOML → Env Vars → CLI Flags
//
// Rules have special merge semantics:
// - Rules with the same name: project overrides global
// - Rules with different names: combined (both included)
func (l *KoanfLoader) Load(flags map[string]any) (*config.Config, error) {
	cfg, err := l.LoadWithoutValidation(flags)
	if err != nil {
		return nil, err
	}

	// Validate
	validator := NewValidator()
	if err := validator.Validate(cfg); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return cfg, nil
}

// LoadWithoutValidation loads configuration without running validation.
// This is useful for tools that need to fix invalid configurations.
func (l *KoanfLoader) LoadWithoutValidation(flags map[string]any) (*config.Config, error) {
	// Reset koanf instance for fresh load
	l.k = koanf.New(".")

	// Track rules from each source for proper merging
	var globalRules []config.RuleConfig

	var projectRules []config.RuleConfig

	// 1. Load defaults first (lowest priority)
	defaults := defaultsToMap()
	if err := l.k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return nil, errors.Wrap(err, "failed to load defaults")
	}

	// 2. Global config: ~/.klaudiush/config.toml
	globalPath := l.GlobalConfigPath()
	if err := l.loadTOMLFile(globalPath); err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, "failed to load global config")
	} else if err == nil {
		globalRules = l.extractRules()
	}

	// 3. Project config: .klaudiush/config.toml or klaudiush.toml
	projectPath := l.findProjectConfig()
	if projectPath != "" {
		if err := l.loadTOMLFile(projectPath); err != nil {
			return nil, errors.Wrap(err, "failed to load project config")
		}

		projectRules = l.extractRules()
	}

	// 4. Environment variables: KLAUDIUSH_*
	envOpt := env.Opt{
		Prefix:        "KLAUDIUSH_",
		TransformFunc: l.envTransform,
	}

	if err := l.k.Load(env.Provider(".", envOpt), nil); err != nil {
		return nil, errors.Wrap(err, "failed to load env vars")
	}

	// 5. CLI flags (highest priority)
	if len(flags) > 0 {
		flagConfig := l.flagsToConfig(flags)
		if err := l.k.Load(confmap.Provider(flagConfig, "."), nil); err != nil {
			return nil, errors.Wrap(err, "failed to load flags")
		}
	}

	// Unmarshal into config struct
	var cfg config.Config
	if err := l.k.UnmarshalWithConf("", &cfg, l.tomlOpts); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	// Merge rules: project overrides global by name, different names are combined
	mergedRules := mergeRules(globalRules, projectRules)

	if cfg.Rules == nil {
		cfg.Rules = &config.RulesConfig{}
	}

	cfg.Rules.Rules = mergedRules

	return &cfg, nil
}

// extractRules extracts rules from the current koanf state.
func (l *KoanfLoader) extractRules() []config.RuleConfig {
	rulesSlice := l.k.Slices("rules.rules")
	rules := make([]config.RuleConfig, 0, len(rulesSlice))

	for _, ruleK := range rulesSlice {
		var rule config.RuleConfig

		// Extract rule fields from the slice element
		rule.Name = ruleK.String("name")
		rule.Description = ruleK.String("description")
		rule.Priority = ruleK.Int("priority")

		if ruleK.Exists("enabled") {
			enabled := ruleK.Bool("enabled")
			rule.Enabled = &enabled
		}

		// Extract match conditions
		if ruleK.Exists("match") {
			rule.Match = &config.RuleMatchConfig{
				ValidatorType:  ruleK.String("match.validator_type"),
				RepoPattern:    ruleK.String("match.repo_pattern"),
				Remote:         ruleK.String("match.remote"),
				BranchPattern:  ruleK.String("match.branch_pattern"),
				FilePattern:    ruleK.String("match.file_pattern"),
				ContentPattern: ruleK.String("match.content_pattern"),
				CommandPattern: ruleK.String("match.command_pattern"),
				ToolType:       ruleK.String("match.tool_type"),
				EventType:      ruleK.String("match.event_type"),
			}
		}

		// Extract action
		if ruleK.Exists("action") {
			rule.Action = &config.RuleActionConfig{
				Type:      ruleK.String("action.type"),
				Message:   ruleK.String("action.message"),
				Reference: ruleK.String("action.reference"),
			}
		}

		rules = append(rules, rule)
	}

	return rules
}

// mergeRules merges global and project rules.
// Rules with the same name: project overrides global.
// Rules with different names: combined (both included).
func mergeRules(globalRules, projectRules []config.RuleConfig) []config.RuleConfig {
	if len(globalRules) == 0 {
		return projectRules
	}

	if len(projectRules) == 0 {
		return globalRules
	}

	// Build a map of project rules by name for quick lookup
	projectRulesByName := make(map[string]config.RuleConfig)

	for _, rule := range projectRules {
		if rule.Name != "" {
			projectRulesByName[rule.Name] = rule
		}
	}

	// Start with global rules, replacing with project rules where names match
	merged := make([]config.RuleConfig, 0, len(globalRules)+len(projectRules))
	seenNames := make(map[string]bool)

	for _, globalRule := range globalRules {
		if globalRule.Name != "" {
			if projectRule, exists := projectRulesByName[globalRule.Name]; exists {
				// Project rule overrides global rule with same name
				merged = append(merged, projectRule)
				seenNames[globalRule.Name] = true
			} else {
				merged = append(merged, globalRule)
				seenNames[globalRule.Name] = true
			}
		} else {
			// Rules without names are always included
			merged = append(merged, globalRule)
		}
	}

	// Add project rules that weren't in global

	for _, projectRule := range projectRules {
		if projectRule.Name != "" && !seenNames[projectRule.Name] {
			merged = append(merged, projectRule)
		} else if projectRule.Name == "" {
			// Rules without names are always included
			merged = append(merged, projectRule)
		}
	}

	return merged
}

// loadTOMLFile loads a TOML configuration file with security checks.
func (l *KoanfLoader) loadTOMLFile(path string) error {
	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Security check: reject world-writable files
	if info.Mode().Perm()&0o002 != 0 {
		return errors.Wrapf(
			ErrInvalidPermissions,
			"%s is world-writable (mode: %s)",
			path,
			info.Mode().Perm(),
		)
	}

	return l.k.Load(file.Provider(path), tomlparser.Parser())
}

// envTransform transforms environment variable names to config paths.
// KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED → validators.git.commit.enabled
func (*KoanfLoader) envTransform(key, value string) (string, any) {
	key = strings.TrimPrefix(key, "KLAUDIUSH_")
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", ".")

	return key, value
}

// GlobalConfigPath returns the path to the global configuration file.
func (l *KoanfLoader) GlobalConfigPath() string {
	return filepath.Join(l.homeDir, GlobalConfigDir, GlobalConfigFile)
}

// ProjectConfigPaths returns the paths to check for project configuration.
func (l *KoanfLoader) ProjectConfigPaths() []string {
	return []string{
		filepath.Join(l.workDir, ProjectConfigDir, ProjectConfigFile),
		filepath.Join(l.workDir, ProjectConfigFileAlt),
	}
}

// findProjectConfig checks for project config files and returns the first found.
func (l *KoanfLoader) findProjectConfig() string {
	for _, path := range l.ProjectConfigPaths() {
		if fileExists(path) {
			return path
		}
	}

	return ""
}

// HasGlobalConfig checks if a global configuration file exists.
func (l *KoanfLoader) HasGlobalConfig() bool {
	return fileExists(l.GlobalConfigPath())
}

// HasProjectConfig checks if a project configuration file exists.
func (l *KoanfLoader) HasProjectConfig() bool {
	return l.findProjectConfig() != ""
}

// FindProjectConfigPath returns the path to the project config file if one exists.
// Returns empty string if no project config file is found.
func (l *KoanfLoader) FindProjectConfigPath() string {
	return l.findProjectConfig()
}

// LoadProjectConfigOnly loads only the project configuration file without merging
// with defaults, global config, or environment variables.
// This is useful for tools that need to edit and write back the project config
// without contaminating it with values from other sources.
// Returns nil if no project config file exists.
func (l *KoanfLoader) LoadProjectConfigOnly() (*config.Config, string, error) {
	projectPath := l.findProjectConfig()
	if projectPath == "" {
		return nil, "", nil
	}

	// Create a fresh koanf instance for isolated loading
	k := koanf.New(".")

	// Load only the project config file
	if err := k.Load(file.Provider(projectPath), tomlparser.Parser()); err != nil {
		return nil, projectPath, errors.Wrap(err, "failed to load project config")
	}

	// Unmarshal into config struct
	var cfg config.Config

	tomlOpts := koanf.UnmarshalConf{
		Tag:       "koanf",
		FlatPaths: false,
	}

	if err := k.UnmarshalWithConf("", &cfg, tomlOpts); err != nil {
		return nil, projectPath, errors.Wrap(err, "failed to unmarshal project config")
	}

	return &cfg, projectPath, nil
}

// flagsToConfig converts CLI flags to a configuration map.
func (*KoanfLoader) flagsToConfig(flags map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range flags {
		switch key {
		case "disable":
			// Handle --disable=commit,markdown,push
			if disableList, ok := value.([]string); ok {
				applyDisableFlags(result, disableList)
			}

		case "use-sdk-git":
			if boolVal, ok := value.(bool); ok {
				globalMap := ensureMapKey(result, "global")
				globalMap["use_sdk_git"] = boolVal
			}

		case "timeout":
			if strVal, ok := value.(string); ok {
				globalMap := ensureMapKey(result, "global")
				globalMap["default_timeout"] = strVal
			}
		}
	}

	return result
}

// ensureMapKey ensures a key exists as a map and returns it.
func ensureMapKey(cfg map[string]any, key string) map[string]any {
	if _, ok := cfg[key]; !ok {
		cfg[key] = make(map[string]any)
	}

	result, _ := cfg[key].(map[string]any)

	return result
}

// applyDisableFlags applies --disable flags to the config map.
func applyDisableFlags(cfg map[string]any, validatorNames []string) {
	validatorPaths := map[string][]string{
		"commit":      {"git", "commit"},
		"push":        {"git", "push"},
		"add":         {"git", "add"},
		"pr":          {"git", "pr"},
		"branch":      {"git", "branch"},
		"no_verify":   {"git", "no_verify"},
		"markdown":    {"file", "markdown"},
		"shellscript": {"file", "shellscript"},
		"terraform":   {"file", "terraform"},
		"workflow":    {"file", "workflow"},
		"bell":        {"notification", "bell"},
	}

	for _, name := range validatorNames {
		name = strings.TrimSpace(name)

		path, ok := validatorPaths[name]
		if !ok {
			continue
		}

		validators := ensureMapKey(cfg, "validators")
		current := validators

		// Navigate/create path
		for i := range len(path) - 1 {
			current = ensureMapKey(current, path[i])
		}

		// Set enabled = false on the final level
		finalMap := ensureMapKey(current, path[len(path)-1])
		finalMap["enabled"] = false
	}
}

// defaultsToMap converts DefaultConfig to a map for koanf loading.
func defaultsToMap() map[string]any {
	return map[string]any{
		"global":     defaultGlobalMap(),
		"validators": defaultValidatorsMap(),
		"rules":      defaultRulesMap(),
		"exceptions": defaultExceptionsMap(),
		"session":    defaultSessionMap(),
	}
}

func defaultRulesMap() map[string]any {
	return map[string]any{
		"enabled":             true,
		"stop_on_first_match": true,
		"rules":               []any{},
	}
}

func defaultExceptionsMap() map[string]any {
	return map[string]any{
		"enabled":      true,
		"token_prefix": defaultExceptionTokenPrefix,
		"policies":     map[string]any{},
		"rate_limit": map[string]any{
			"enabled":      true,
			"max_per_hour": defaultExceptionRateLimitPerH,
			"max_per_day":  defaultExceptionRateLimitPerD,
			"state_file":   "~/.klaudiush/exception_state.json",
		},
		"audit": map[string]any{
			"enabled":      true,
			"log_file":     "~/.klaudiush/exception_audit.jsonl",
			"max_size_mb":  defaultExceptionAuditMaxSizeMB,
			"max_age_days": defaultExceptionAuditMaxAgeDays,
			"max_backups":  defaultExceptionAuditMaxBackups,
		},
	}
}

func defaultSessionMap() map[string]any {
	return map[string]any{
		"enabled":         false,
		"state_file":      defaultSessionStateFile,
		"max_session_age": defaultSessionMaxAgeStr,
	}
}

func defaultGlobalMap() map[string]any {
	return map[string]any{
		"use_sdk_git":     true,
		"default_timeout": defaultTimeoutStr,
	}
}

func defaultValidatorsMap() map[string]any {
	return map[string]any{
		"git":          defaultGitValidatorsMap(),
		"file":         defaultFileValidatorsMap(),
		"notification": defaultNotificationValidatorsMap(),
	}
}

func defaultGitValidatorsMap() map[string]any {
	return map[string]any{
		"commit":    defaultCommitMap(),
		"push":      defaultPushMap(),
		"fetch":     defaultFetchMap(),
		"add":       defaultAddMap(),
		"pr":        defaultPRMap(),
		"branch":    defaultBranchMap(),
		"no_verify": defaultNoVerifyMap(),
	}
}

func defaultCommitMap() map[string]any {
	return map[string]any{
		"enabled":            true,
		"severity":           "error",
		"required_flags":     []string{"-s", "-S"},
		"check_staging_area": true,
		"message": map[string]any{
			"enabled":                  true,
			"title_max_length":         defaultTitleMaxLength,
			"body_max_line_length":     defaultBodyMaxLineLength,
			"body_line_tolerance":      defaultBodyLineTolerance,
			"conventional_commits":     true,
			"require_scope":            true,
			"block_infra_scope_misuse": true,
			"block_pr_references":      true,
			"block_ai_attribution":     true,
			"valid_types":              defaultValidTypes,
			"expected_signoff":         "",
		},
	}
}

func defaultPushMap() map[string]any {
	return map[string]any{
		"enabled":          true,
		"severity":         "error",
		"blocked_remotes":  []string{},
		"require_tracking": true,
	}
}

func defaultFetchMap() map[string]any {
	return map[string]any{
		"enabled":  true,
		"severity": "error",
	}
}

func defaultAddMap() map[string]any {
	return map[string]any{
		"enabled":          true,
		"severity":         "error",
		"blocked_patterns": []string{"tmp/*"},
	}
}

func defaultPRMap() map[string]any {
	return map[string]any{
		"enabled":                    true,
		"severity":                   "error",
		"title_max_length":           defaultTitleMaxLength,
		"title_conventional_commits": true,
		"require_changelog":          false,
		"check_ci_labels":            true,
		"require_body":               true,
		"valid_types":                defaultValidTypes,
		"markdown_disabled_rules":    []string{"MD013", "MD034", "MD041"},
	}
}

func defaultBranchMap() map[string]any {
	return map[string]any{
		"enabled":            true,
		"severity":           "error",
		"protected_branches": []string{"main", "master"},
		"require_type":       true,
		"allow_uppercase":    false,
		"valid_types":        defaultBranchValidTypes,
	}
}

func defaultNoVerifyMap() map[string]any {
	return map[string]any{
		"enabled":  true,
		"severity": "error",
	}
}

func defaultFileValidatorsMap() map[string]any {
	return map[string]any{
		"markdown":    defaultMarkdownMap(),
		"shellscript": defaultShellscriptMap(),
		"terraform":   defaultTerraformMap(),
		"workflow":    defaultWorkflowMap(),
	}
}

func defaultMarkdownMap() map[string]any {
	return map[string]any{
		"enabled":               true,
		"severity":              "error",
		"timeout":               defaultTimeoutStr,
		"context_lines":         defaultContextLines,
		"heading_spacing":       true,
		"code_block_formatting": true,
		"list_formatting":       true,
		"use_markdownlint":      true,
	}
}

func defaultShellscriptMap() map[string]any {
	return map[string]any{
		"enabled":             true,
		"severity":            "error",
		"timeout":             defaultTimeoutStr,
		"context_lines":       defaultContextLines,
		"use_shellcheck":      true,
		"shellcheck_severity": "warning",
		"exclude_rules":       []string{},
	}
}

func defaultTerraformMap() map[string]any {
	return map[string]any{
		"enabled":         true,
		"severity":        "error",
		"timeout":         defaultTimeoutStr,
		"context_lines":   defaultContextLines,
		"tool_preference": "auto",
		"check_format":    true,
		"use_tflint":      true,
	}
}

func defaultWorkflowMap() map[string]any {
	return map[string]any{
		"enabled":                 true,
		"severity":                "error",
		"timeout":                 defaultTimeoutStr,
		"gh_api_timeout":          defaultGHAPITimeoutStr,
		"enforce_digest_pinning":  true,
		"require_version_comment": true,
		"check_latest_version":    true,
		"use_actionlint":          true,
	}
}

func defaultNotificationValidatorsMap() map[string]any {
	return map[string]any{
		"bell": map[string]any{
			"enabled":  true,
			"severity": "error",
		},
	}
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// mustGetwd returns the current working directory or panics.
func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic("failed to get working directory: " + err.Error())
	}

	return wd
}
