// Package config provides internal configuration loading and processing.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/knadh/koanf/maps"
	tomlparser "github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// deepMergeOpt enables recursive map merging for koanf.Load calls.
// Without this, koanf replaces entire sub-maps when a higher-priority source
// sets any key in a nested map, wiping defaults for unset keys.
var deepMergeOpt = koanf.WithMergeFunc(func(src, dest map[string]any) error {
	maps.Merge(src, dest)
	return nil
})

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
	defaultTimeoutStr      = "10s"
	defaultGHAPITimeoutStr = "5s"
	defaultContextLines    = 2

	// Exception defaults.
	defaultExceptionTokenPrefix     = "EXC"
	defaultExceptionRateLimitPerH   = 10
	defaultExceptionRateLimitPerD   = 50
	defaultExceptionAuditMaxSizeMB  = 10
	defaultExceptionAuditMaxAgeDays = 30
	defaultExceptionAuditMaxBackups = 3
	defaultExceptionMinReasonLength = 10
)

// defaultValidTypes is the list of valid commit types.
var defaultValidTypes = []string{
	commitTypeBuild,
	commitTypeChore,
	commitTypeCI,
	commitTypeDocs,
	commitTypeFeat,
	commitTypeFix,
	commitTypePerf,
	commitTypeRefactor,
	commitTypeRevert,
	commitTypeStyle,
	commitTypeTest,
}

// defaultBranchValidTypes is the list of valid branch type prefixes (excludes "revert").
var defaultBranchValidTypes = []string{
	commitTypeBuild, commitTypeChore, commitTypeCI, commitTypeDocs, commitTypeFeat,
	commitTypeFix, commitTypePerf, commitTypeRefactor, commitTypeStyle, commitTypeTest,
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
	paths    xdg.PathResolver
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

	return makeKoanfLoader(homeDir, workDir, xdg.DefaultResolver()), nil
}

// NewKoanfLoaderWithDirs creates a new KoanfLoader with custom directories (for testing).
func NewKoanfLoaderWithDirs(homeDir, workDir string) (*KoanfLoader, error) {
	return makeKoanfLoader(homeDir, workDir, xdg.ResolverFor(homeDir)), nil
}

func makeKoanfLoader(homeDir, workDir string, paths xdg.PathResolver) *KoanfLoader {
	k := koanf.New(".")

	return &KoanfLoader{
		k:       k,
		homeDir: homeDir,
		workDir: workDir,
		paths:   paths,
		tomlOpts: koanf.UnmarshalConf{
			Tag:       "koanf",
			FlatPaths: false,
		},
	}
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

	if err := l.k.Load(env.Provider(".", envOpt), nil, deepMergeOpt); err != nil {
		return nil, errors.Wrap(err, "failed to load env vars")
	}

	// 5. CLI flags (highest priority)
	if len(flags) > 0 {
		flagConfig := l.flagsToConfig(flags)
		if err := l.k.Load(confmap.Provider(flagConfig, "."), nil, deepMergeOpt); err != nil {
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

		if ruleK.Exists(keyEnabled) {
			enabled := ruleK.Bool(keyEnabled)
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
	if err := checkConfigPermissions(path); err != nil {
		return err
	}

	return l.k.Load(file.Provider(path), tomlparser.Parser(), deepMergeOpt)
}

// checkConfigPermissions rejects world-writable config files. Every path that
// reads a config must go through this, including the isolated loaders the
// rewriting commands use.
func checkConfigPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.Mode().Perm()&0o002 != 0 {
		return errors.Wrapf(
			ErrInvalidPermissions,
			"%s is world-writable (mode: %s)",
			path,
			info.Mode().Perm(),
		)
	}

	return nil
}

// envHierarchy maps each valid parent path to its known child segment names.
// Child segments may contain underscores (e.g. "no_verify", "crash_dump").
// Segments not listed here are treated as leaf field names with underscores preserved.
var envHierarchy = map[string][]string{
	"": {
		sectionGlobal,
		sectionValidators,
		sectionRules,
		sectionExceptions,
		"backup",
		sectionCrashDump,
		sectionPatterns,
		"plugins",
		"overrides",
		sectionBypassPerms,
		"output",
	},
	"overrides":       {"entries"},
	sectionValidators: {sectionGit, sectionFile, sectionNotification, sectionSecrets, sectionShell},
	"validators.git": {
		validatorCommit,
		validatorPush,
		validatorFetch,
		validatorAdd,
		validatorPR,
		validatorBranch,
		validatorNoVerify,
		validatorMerge,
	},
	"validators.git.commit": {validatorMessage},
	"validators.git.merge":  {validatorMessage},
	"validators.file": {
		validatorMarkdown,
		validatorShellScript,
		validatorTerraform,
		validatorWorkflow,
		validatorGofumpt,
		validatorPython,
		validatorJavaScript,
		validatorRust,
		validatorLinterIgnore,
	},
	"validators.notification": {validatorBell},
	"validators.secrets":      {sectionSecrets},
	"validators.shell":        {validatorBacktick},
	sectionExceptions:         {"rate_limit", "audit", "policies"},
	"backup":                  {"delta"},
}

// envTransform transforms environment variable names to config paths.
// It uses the config hierarchy to distinguish path separators from
// underscores that are part of field names.
//
//	KLAUDIUSH_VALIDATORS_GIT_COMMIT_ENABLED → validators.git.commit.enabled
//	KLAUDIUSH_VALIDATORS_FILE_MARKDOWN_USE_MARKDOWNLINT → validators.file.markdown.use_markdownlint
//	KLAUDIUSH_CRASH_DUMP_MAX_DUMPS → crash_dump.max_dumps
func (*KoanfLoader) envTransform(key, value string) (string, any) {
	key = strings.TrimPrefix(key, "KLAUDIUSH_")
	key = strings.ToLower(key)

	parts := strings.Split(key, "_")
	path := envMatchHierarchy(parts)

	return path, value
}

// envMatchHierarchy walks the underscore-separated parts and builds a
// dot-separated koanf path, using envHierarchy to decide which underscores
// are hierarchy separators vs. part of field names.
func envMatchHierarchy(parts []string) string {
	var pathSegments []string

	parent := ""
	pos := 0

	for pos < len(parts) {
		children, hasChildren := envHierarchy[parent]
		if !hasChildren {
			// No more hierarchy levels - rest is the field name.
			break
		}

		matched := false

		// Try longest child segments first (e.g. "crash_dump" before "crash").
		// Children with more underscores need more parts to match.
		for _, child := range children {
			childParts := strings.Split(child, "_")
			n := len(childParts)

			if pos+n > len(parts) {
				continue
			}

			candidate := strings.Join(parts[pos:pos+n], "_")
			if candidate == child {
				pathSegments = append(pathSegments, child)

				if parent == "" {
					parent = child
				} else {
					parent = parent + "." + child
				}

				pos += n
				matched = true

				break
			}
		}

		if !matched {
			break
		}
	}

	// Remaining parts form the field name (underscores preserved).
	if pos < len(parts) {
		fieldName := strings.Join(parts[pos:], "_")
		pathSegments = append(pathSegments, fieldName)
	}

	return strings.Join(pathSegments, ".")
}

// GlobalConfigPath returns the path to the global configuration file.
// Checks XDG location first, falls back to legacy ~/.klaudiush/config.toml.
func (l *KoanfLoader) GlobalConfigPath() string {
	xdgPath := l.paths.GlobalConfigFile()
	legacyPath := filepath.Join(l.homeDir, GlobalConfigDir, GlobalConfigFile)

	return xdg.ResolveFile(xdgPath, legacyPath)
}

// ProjectConfigPaths returns the paths to check for project configuration.
func (l *KoanfLoader) ProjectConfigPaths() []string {
	return []string{
		filepath.Join(l.workDir, ProjectConfigDir, ProjectConfigFile),
		filepath.Join(l.workDir, ProjectConfigFileAlt),
	}
}

// findProjectConfig checks for project config files and returns the first found.
// Falls back to walking up parent directories if nothing is found in the cwd.
func (l *KoanfLoader) findProjectConfig() string {
	for _, path := range l.ProjectConfigPaths() {
		if fileExists(path) {
			return path
		}
	}

	return l.walkUpForConfig()
}

// walkUpForConfig walks parent directories looking for a project config file.
// Skips any candidate that matches the global config path to avoid double-loading.
func (l *KoanfLoader) walkUpForConfig() string {
	globalPath := l.GlobalConfigPath()
	dir := filepath.Dir(l.workDir)

	for {
		for _, candidate := range []string{
			filepath.Join(dir, ProjectConfigDir, ProjectConfigFile),
			filepath.Join(dir, ProjectConfigFileAlt),
		} {
			if candidate != globalPath && fileExists(candidate) {
				return candidate
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
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

	if err := checkConfigPermissions(projectPath); err != nil {
		return nil, projectPath, errors.Wrap(err, "failed to read project config")
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

	if cfg.Version == 0 {
		cfg.Version = config.CurrentConfigVersion
	}

	return &cfg, projectPath, nil
}

// LoadGlobalConfigOnly loads the global config file in isolation, without
// defaults, project config, environment variables, or flags merged in.
// Returns a nil config when no global file exists.
//
// Commands that rewrite the global config must read it through this so the
// rest of the file survives the round trip.
func (l *KoanfLoader) LoadGlobalConfigOnly() (*config.Config, string, error) {
	globalPath := l.GlobalConfigPath()

	if err := checkConfigPermissions(globalPath); err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}

		return nil, globalPath, errors.Wrap(err, "failed to read global config")
	}

	// Create a fresh koanf instance for isolated loading
	k := koanf.New(".")

	if err := k.Load(file.Provider(globalPath), tomlparser.Parser()); err != nil {
		return nil, globalPath, errors.Wrap(err, "failed to load global config")
	}

	var cfg config.Config

	if err := k.UnmarshalWithConf("", &cfg, l.tomlOpts); err != nil {
		return nil, globalPath, errors.Wrap(err, "failed to unmarshal global config")
	}

	if cfg.Version == 0 {
		cfg.Version = config.CurrentConfigVersion
	}

	return &cfg, globalPath, nil
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
				globalMap := ensureMapKey(result, sectionGlobal)
				globalMap["use_sdk_git"] = boolVal
			}

		case keyTimeout:
			if strVal, ok := value.(string); ok {
				globalMap := ensureMapKey(result, sectionGlobal)
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
		validatorCommit:       {sectionGit, validatorCommit},
		validatorPush:         {sectionGit, validatorPush},
		validatorAdd:          {sectionGit, validatorAdd},
		validatorPR:           {sectionGit, validatorPR},
		validatorBranch:       {sectionGit, validatorBranch},
		validatorNoVerify:     {sectionGit, validatorNoVerify},
		validatorMerge:        {sectionGit, validatorMerge},
		validatorFetch:        {sectionGit, validatorFetch},
		validatorMarkdown:     {sectionFile, validatorMarkdown},
		validatorShellScript:  {sectionFile, validatorShellScript},
		validatorTerraform:    {sectionFile, validatorTerraform},
		validatorWorkflow:     {sectionFile, validatorWorkflow},
		validatorGofumpt:      {sectionFile, validatorGofumpt},
		validatorPython:       {sectionFile, validatorPython},
		validatorJavaScript:   {sectionFile, validatorJavaScript},
		validatorRust:         {sectionFile, validatorRust},
		validatorLinterIgnore: {sectionFile, validatorLinterIgnore},
		sectionSecrets:        {sectionSecrets, sectionSecrets},
		validatorBacktick:     {sectionShell, validatorBacktick},
		"issue":               {sectionGitHub, "issue"},
		validatorGHAPI:        {sectionGitHub, validatorGHAPI},
		validatorBell:         {sectionNotification, validatorBell},
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
		finalMap[keyEnabled] = false
	}
}

// defaultsToMap converts DefaultConfig to a map for koanf loading.
func defaultsToMap() map[string]any {
	return map[string]any{
		"version":     config.CurrentConfigVersion,
		sectionGlobal: defaultGlobalMap(),
		"providers":   defaultProvidersMap(),
		"validators":  defaultValidatorsMap(),
		"rules":       defaultRulesMap(),
		"exceptions":  defaultExceptionsMap(),
		"patterns":    defaultPatternsMap(),
	}
}

func defaultProvidersMap() map[string]any {
	return map[string]any{
		"claude": map[string]any{
			keyEnabled: true,
		},
		"codex": map[string]any{
			keyEnabled:          false,
			"experimental":      false,
			"hooks_config_path": "",
		},
		"gemini": map[string]any{
			keyEnabled:      false,
			"settings_path": "~/.gemini/settings.json",
		},
	}
}

func defaultRulesMap() map[string]any {
	return map[string]any{
		keyEnabled:            true,
		"stop_on_first_match": true,
		"rules":               []any{},
	}
}

func defaultExceptionsMap() map[string]any {
	return map[string]any{
		keyEnabled:     true,
		"token_prefix": defaultExceptionTokenPrefix,
		"policies":     defaultExceptionPoliciesMap(),
		"rate_limit": map[string]any{
			keyEnabled:     true,
			"max_per_hour": defaultExceptionRateLimitPerH,
			"max_per_day":  defaultExceptionRateLimitPerD,
			"state_file":   xdg.ExceptionStateFile(),
		},
		"audit": map[string]any{
			keyEnabled:     true,
			"log_file":     xdg.ExceptionAuditFile(),
			"max_size_mb":  defaultExceptionAuditMaxSizeMB,
			"max_age_days": defaultExceptionAuditMaxAgeDays,
			"max_backups":  defaultExceptionAuditMaxBackups,
		},
	}
}

// defaultExceptionPoliciesMap denies an exception token for the codes whose
// whole point is that the artefact outlives the session. AI attribution in a
// commit message or pull request description is permanent and public, and the
// assistant writing the commit can just as easily write its own exception
// token, so GIT012 ships with the escape hatch closed. Set
// allow_exception = true under [exceptions.policies.GIT012] to reopen it.
func defaultExceptionPoliciesMap() map[string]any {
	return map[string]any{
		"GIT012": map[string]any{
			keyEnabled:        true,
			"allow_exception": false,
		},
	}
}

func defaultGlobalMap() map[string]any {
	return map[string]any{
		"use_sdk_git":     true,
		"default_timeout": defaultTimeoutStr,
	}
}

func defaultPatternsMap() map[string]any {
	return map[string]any{
		keyEnabled:               true,
		"min_count":              config.DefaultPatternsMinCount,
		"max_age":                config.DefaultPatternsMaxAge.String(),
		"max_warnings_per_error": config.DefaultPatternsMaxWarningsPerError,
		"max_warnings_total":     config.DefaultPatternsMaxWarningsTotal,
		"project_data_file":      config.DefaultPatternsProjectDataFile,
		"global_data_dir":        xdg.PatternsGlobalDir(),
		"session_max_age":        config.DefaultPatternsSessionMaxAge.String(),
		"use_seed_data":          true,
		"max_patterns":           config.DefaultPatternsMaxPatterns,
		"max_sessions":           config.DefaultPatternsMaxSessions,
	}
}

func defaultValidatorsMap() map[string]any {
	return map[string]any{
		sectionGit:          defaultGitValidatorsMap(),
		sectionFile:         defaultFileValidatorsMap(),
		sectionGitHub:       defaultGitHubValidatorsMap(),
		sectionNotification: defaultNotificationValidatorsMap(),
	}
}

func defaultGitHubValidatorsMap() map[string]any {
	return map[string]any{
		validatorGHAPI: defaultGHAPIMap(),
	}
}

func defaultGHAPIMap() map[string]any {
	return map[string]any{
		keyEnabled:                  true,
		keySeverity:                 severityError,
		"blocked_endpoints":         config.DefaultBlockedGHAPIEndpoints(),
		"blocked_graphql_mutations": config.DefaultBlockedGHAPIMutations(),
		"block_unverifiable_calls":  true,
		"check_http_clients":        true,
		"hosts":                     config.DefaultGitHubAPIHosts(),
		"blocked_client_calls":      config.DefaultBlockedGHAPIClientCalls(),
	}
}

func defaultGitValidatorsMap() map[string]any {
	return map[string]any{
		validatorCommit:   defaultCommitMap(),
		validatorPush:     defaultPushMap(),
		validatorFetch:    defaultFetchMap(),
		validatorAdd:      defaultAddMap(),
		validatorPR:       defaultPRMap(),
		validatorBranch:   defaultBranchMap(),
		validatorNoVerify: defaultNoVerifyMap(),
	}
}

func defaultCommitMap() map[string]any {
	return map[string]any{
		keyEnabled:           true,
		keySeverity:          severityError,
		"required_flags":     []string{flagSignoff, flagGPGSign},
		"check_staging_area": true,
		validatorMessage: map[string]any{
			keyEnabled:                 true,
			"title_max_length":         config.DefaultTitleMaxLength,
			"body_max_line_length":     config.DefaultBodyMaxLineLength,
			"body_line_tolerance":      config.DefaultBodyLineTolerance,
			"conventional_commits":     true,
			"require_scope":            true,
			"block_infra_scope_misuse": true,
			"block_pr_references":      true,
			"block_ai_attribution":     true,
			keyValidTypes:              defaultValidTypes,
			"expected_signoff":         "",
		},
	}
}

func defaultPushMap() map[string]any {
	return map[string]any{
		keyEnabled:         true,
		keySeverity:        severityError,
		"blocked_remotes":  []string{},
		"require_tracking": true,
	}
}

func defaultFetchMap() map[string]any {
	return map[string]any{
		keyEnabled:  true,
		keySeverity: severityError,
	}
}

func defaultAddMap() map[string]any {
	return map[string]any{
		keyEnabled:         true,
		keySeverity:        severityError,
		"blocked_patterns": []string{"tmp/*"},
	}
}

func defaultPRMap() map[string]any {
	return map[string]any{
		keyEnabled:                   true,
		keySeverity:                  severityError,
		"title_max_length":           config.DefaultTitleMaxLength,
		"title_conventional_commits": true,
		"require_changelog":          false,
		"check_ci_labels":            true,
		"require_body":               true,
		keyValidTypes:                defaultValidTypes,
		"markdown_disabled_rules":    []string{mdRuleLineLength, mdRuleBareURLs, mdRuleFirstLine},
	}
}

func defaultBranchMap() map[string]any {
	return map[string]any{
		keyEnabled:           true,
		keySeverity:          severityError,
		"protected_branches": []string{branchMain, "master"},
		"require_type":       true,
		"allow_uppercase":    false,
		keyValidTypes:        defaultBranchValidTypes,
	}
}

func defaultNoVerifyMap() map[string]any {
	return map[string]any{
		keyEnabled:  true,
		keySeverity: severityError,
	}
}

func defaultFileValidatorsMap() map[string]any {
	return map[string]any{
		validatorMarkdown:    defaultMarkdownMap(),
		validatorShellScript: defaultShellscriptMap(),
		validatorTerraform:   defaultTerraformMap(),
		validatorWorkflow:    defaultWorkflowMap(),
	}
}

func defaultMarkdownMap() map[string]any {
	return map[string]any{
		keyEnabled:              true,
		keySeverity:             severityError,
		keyTimeout:              defaultTimeoutStr,
		keyContextLines:         defaultContextLines,
		"heading_spacing":       true,
		"code_block_formatting": true,
		"list_formatting":       true,
		"use_markdownlint":      true,
	}
}

func defaultShellscriptMap() map[string]any {
	return map[string]any{
		keyEnabled:            true,
		keySeverity:           severityError,
		keyTimeout:            defaultTimeoutStr,
		keyContextLines:       defaultContextLines,
		"use_shellcheck":      true,
		"shellcheck_severity": severityWarning,
		"exclude_rules":       []string{},
	}
}

func defaultTerraformMap() map[string]any {
	return map[string]any{
		keyEnabled:        true,
		keySeverity:       severityError,
		keyTimeout:        defaultTimeoutStr,
		keyContextLines:   defaultContextLines,
		"tool_preference": "auto",
		"check_format":    true,
		"use_tflint":      true,
	}
}

func defaultWorkflowMap() map[string]any {
	return map[string]any{
		keyEnabled:                true,
		keySeverity:               severityError,
		keyTimeout:                defaultTimeoutStr,
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
			keyEnabled:  true,
			keySeverity: severityError,
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
