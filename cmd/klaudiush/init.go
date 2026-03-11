// Package main provides the CLI entry point for klaudiush.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/backup"
	"github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/internal/tui"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const defaultHookTimeout = 30

var (
	globalFlag         bool
	forceFlag          bool
	noTUIFlag          bool
	installHooksFlag   bool
	providersFlag      []string
	codexHooksFlag     string
	geminiSettingsFlag string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize klaudiush configuration",
	Long: `Initialize klaudiush configuration file and register hooks.

By default, creates a project-local configuration file (.klaudiush/config.toml)
and registers supported hooks for enabled providers. Claude installation is enabled by default; Codex installation runs when providers.codex is enabled with experimental=true and hooks_config_path set; Gemini installation runs when providers.gemini is enabled with settings_path set.

Use --global or -g to create a global configuration file (~/.klaudiush/config.toml).
Use --install-hooks to register hooks only (skip TUI).
Use --install-hooks=false to skip hook registration.
Use --force to overwrite an existing configuration file.
Use --no-tui to use simple prompts instead of the interactive TUI.`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(
		&globalFlag,
		"global",
		"g",
		false,
		"Initialize global configuration",
	)

	initCmd.Flags().BoolVarP(
		&forceFlag,
		"force",
		"f",
		false,
		"Overwrite existing configuration file",
	)

	initCmd.Flags().BoolVar(
		&noTUIFlag,
		"no-tui",
		false,
		"Use simple prompts instead of interactive TUI",
	)

	initCmd.Flags().BoolVar(
		&installHooksFlag,
		"install-hooks",
		true,
		"Register klaudiush hooks for enabled providers",
	)

	initCmd.Flags().StringSliceVar(
		&providersFlag,
		"providers",
		nil,
		"Providers to configure (claude,codex,gemini)",
	)

	initCmd.Flags().StringVar(
		&codexHooksFlag,
		"codex-hooks-path",
		"",
		"Codex hooks.json path to configure during init",
	)

	initCmd.Flags().StringVar(
		&geminiSettingsFlag,
		"gemini-settings-path",
		"",
		"Gemini settings.json path to configure during init",
	)
}

func runInit(cmd *cobra.Command, _ []string) error {
	// Explicit --install-hooks: skip TUI, just install hooks
	if cmd.Flags().Changed("install-hooks") && installHooksFlag {
		return runInstallHooks()
	}

	if err := normalizeInitProviderFlags(cmd); err != nil {
		return err
	}

	writer := config.NewWriter()

	// Check if config already exists
	configPath, existingConfig := resolveInitConfigPath(writer)
	if handled, err := handleExistingInitConfig(
		writer,
		configPath,
		existingConfig,
	); handled || err != nil {
		return err
	}

	return runFreshInit(writer, configPath, existingConfig)
}

func normalizeInitProviderFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("providers") && len(providersFlag) == 0 {
		return errors.New("--providers requires at least one provider")
	}

	if (codexHooksFlag != "" || geminiSettingsFlag != "") && !cmd.Flags().Changed("providers") {
		providersFlag = []string{"claude"}

		if codexHooksFlag != "" {
			providersFlag = append(providersFlag, "codex")
		}

		if geminiSettingsFlag != "" {
			providersFlag = append(providersFlag, "gemini")
		}
	}

	return nil
}

func handleExistingInitConfig(
	writer *config.Writer,
	configPath string,
	existingConfig bool,
) (bool, error) {
	if !existingConfig || forceFlag {
		return false, nil
	}

	updated, handled, err := maybeUpdateExistingConfig(writer, configPath)
	if err != nil {
		return true, err
	}

	if handled {
		if updated != nil {
			fmt.Println()
			fmt.Println("Configuration updated successfully!")
		}

		return true, nil
	}

	return true, errors.Errorf(
		"configuration file already exists: %s\nUse --force to overwrite",
		configPath,
	)
}

func runFreshInit(
	writer *config.Writer,
	configPath string,
	existingConfig bool,
) error {
	backupExistingConfig(configPath, existingConfig)

	showGitExclude := !globalFlag && git.IsInGitRepo()
	ui := tui.NewWithFallback(noTUIFlag)

	cfg, addToExclude, err := ui.RunInitForm(tui.InitFormOptions{
		Global:         globalFlag,
		DefaultSignoff: getDefaultSignoff(),
		ShowGitExclude: showGitExclude,
	})
	if err != nil {
		return errors.Wrap(err, "configuration form failed")
	}

	if cfg, err = applyProviderFlags(cfg); err != nil {
		return err
	}

	if err := validateConfigForWrite(cfg); err != nil {
		return err
	}

	if err := writeConfig(writer, cfg, configPath); err != nil {
		return err
	}

	maybeAddConfigToExclude(addToExclude, showGitExclude)
	maybeInstallInitHooks(cfg)

	fmt.Println()
	fmt.Println("Configuration initialized successfully!")

	return nil
}

func backupExistingConfig(configPath string, existingConfig bool) {
	if !forceFlag || !existingConfig {
		return
	}

	if backupErr := backupBeforeForce(configPath); backupErr != nil {
		fmt.Fprintf(
			os.Stderr,
			"⚠️  Warning: failed to backup existing config: %v\n",
			backupErr,
		)
	}
}

func maybeAddConfigToExclude(addToExclude bool, showGitExclude bool) {
	if !addToExclude || !showGitExclude {
		return
	}

	if err := addConfigToExclude(); err != nil {
		fmt.Fprintf(
			os.Stderr,
			"⚠️  Warning: failed to add to .git/info/exclude: %v\n",
			err,
		)

		return
	}

	fmt.Println("✅ Added to .git/info/exclude")
}

func maybeInstallInitHooks(cfg *pkgConfig.Config) {
	if !installHooksFlag {
		return
	}

	if err := tryInstallHooks(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: hook registration failed: %v\n", err)
	}
}

func resolveInitConfigPath(writer *config.Writer) (string, bool) {
	var (
		configPath   string
		configExists bool
	)

	if globalFlag {
		configPath = writer.GlobalConfigPath()
		configExists = writer.IsGlobalConfigExists()
	} else {
		configPath = writer.ProjectConfigPath()
		configExists = writer.IsProjectConfigExists()
	}

	return configPath, configExists
}

func maybeUpdateExistingConfig(
	writer *config.Writer,
	configPath string,
) (*pkgConfig.Config, bool, error) {
	existingCfg, err := loadConfigFile(configPath)
	if err != nil {
		return nil, false, err
	}

	if len(providersFlag) > 0 || codexHooksFlag != "" || geminiSettingsFlag != "" {
		return updateExistingConfigFromFlags(writer, configPath, existingCfg)
	}

	if !isInteractive() {
		return nil, false, nil
	}

	return updateExistingConfigInteractively(writer, configPath, existingCfg)
}

func updateExistingConfigFromFlags(
	writer *config.Writer,
	configPath string,
	existingCfg *pkgConfig.Config,
) (*pkgConfig.Config, bool, error) {
	selection, resolveErr := resolveProviderSelection(
		providersFlag,
		codexHooksFlag,
		geminiSettingsFlag,
		existingCfg,
	)
	if resolveErr != nil {
		return nil, false, resolveErr
	}

	updated, applyErr := applyProviderSelection(existingCfg, selection)
	if applyErr != nil {
		return nil, false, applyErr
	}

	if validateErr := validateConfigForWrite(updated); validateErr != nil {
		return nil, false, validateErr
	}

	diff, diffErr := renderConfigDiff(configPath, existingCfg, updated)
	if diffErr != nil {
		return nil, false, diffErr
	}

	if diff == "" {
		fmt.Printf("No configuration changes are needed for %s\n", configPath)
		return nil, true, nil
	}

	fmt.Printf("Applying provider changes to %s:\n%s", configPath, diff)

	if writeErr := writeConfig(writer, updated, configPath); writeErr != nil {
		return nil, false, writeErr
	}

	return updated, true, nil
}

func updateExistingConfigInteractively(
	writer *config.Writer,
	configPath string,
	existingCfg *pkgConfig.Config,
) (*pkgConfig.Config, bool, error) {
	updated, handled, err := promptProviderUpdate(
		prompt.NewStdPrompter(),
		os.Stdout,
		configPath,
		existingCfg,
	)
	if err != nil {
		return nil, false, err
	}

	if !handled {
		return nil, false, nil
	}

	if updated == nil {
		fmt.Println("Configuration update cancelled.")
		return nil, true, nil
	}

	diff, err := renderConfigDiff(configPath, existingCfg, updated)
	if err != nil {
		return nil, false, err
	}

	if diff == "" {
		return updated, true, nil
	}

	if validateErr := validateConfigForWrite(updated); validateErr != nil {
		return nil, false, validateErr
	}

	if writeErr := writeConfig(writer, updated, configPath); writeErr != nil {
		return nil, false, writeErr
	}

	return updated, true, nil
}

func applyProviderFlags(cfg *pkgConfig.Config) (*pkgConfig.Config, error) {
	if len(providersFlag) == 0 && codexHooksFlag == "" && geminiSettingsFlag == "" {
		return cfg, nil
	}

	selection, err := resolveProviderSelection(
		providersFlag,
		codexHooksFlag,
		geminiSettingsFlag,
		cfg,
	)
	if err != nil {
		return nil, err
	}

	return applyProviderSelection(cfg, selection)
}

// backupBeforeForce creates a backup before overwriting config with --force.
func backupBeforeForce(configPath string) error {
	// Get home directory for backup storage
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(err, "failed to get home directory")
	}

	// Determine config type
	var configType backup.ConfigType

	if globalFlag {
		configType = backup.ConfigTypeGlobal
	} else {
		configType = backup.ConfigTypeProject
	}

	// Create storage
	baseDir := filepath.Join(homeDir, config.GlobalConfigDir)
	projectPath := ""

	if !globalFlag {
		projectPath, err = os.Getwd()
		if err != nil {
			return errors.Wrap(err, "failed to get working directory")
		}
	}

	storage, err := backup.NewFilesystemStorage(baseDir, configType, projectPath)
	if err != nil {
		return errors.Wrap(err, "failed to create backup storage")
	}

	// Create backup manager with default config
	backupCfg := &pkgConfig.BackupConfig{}

	manager, err := backup.NewManager(storage, backupCfg)
	if err != nil {
		return errors.Wrap(err, "failed to create backup manager")
	}

	// Create backup
	opts := backup.CreateBackupOptions{
		ConfigPath: configPath,
		Trigger:    backup.TriggerBeforeInit,
		Metadata: backup.SnapshotMetadata{
			Command: "init --force",
		},
	}

	snapshot, err := manager.CreateBackup(opts)
	if err != nil {
		return errors.Wrap(err, "backup creation failed")
	}

	fmt.Printf("✅ Backed up existing config: %s\n", snapshot.ID)

	return nil
}

// getDefaultSignoff gets the default signoff from git config.
func getDefaultSignoff() string {
	if !git.IsInGitRepo() {
		return ""
	}

	cfgReader, err := git.NewConfigReader()
	if err != nil {
		return ""
	}

	signoff, err := cfgReader.GetSignoff()
	if err != nil {
		return ""
	}

	return signoff
}

// writeConfig writes the configuration to the appropriate location.
func writeConfig(writer *config.Writer, cfg *pkgConfig.Config, configPath string) error {
	if globalFlag {
		if err := writer.WriteGlobal(cfg); err != nil {
			return errors.Wrap(err, "failed to write global configuration")
		}
	} else {
		if err := writer.WriteProject(cfg); err != nil {
			return errors.Wrap(err, "failed to write project configuration")
		}
	}

	fmt.Printf("\n✅ Configuration written to: %s\n", configPath)

	return nil
}

// addConfigToExclude adds the config file pattern to .git/info/exclude.
func addConfigToExclude() error {
	excludeMgr, err := git.NewExcludeManager()
	if err != nil {
		return errors.Wrap(err, "failed to create exclude manager")
	}

	// Add both config file patterns
	patterns := []string{
		filepath.Join(config.ProjectConfigDir, config.ProjectConfigFile),
		config.ProjectConfigFileAlt,
	}

	for _, pattern := range patterns {
		if err := excludeMgr.AddEntry(pattern); err != nil {
			if !errors.Is(err, git.ErrEntryAlreadyExists) {
				return errors.Wrapf(err, "failed to add %s", pattern)
			}
		}
	}

	return nil
}

// runInstallHooks registers hooks without running the TUI.
func runInstallHooks() error {
	binaryPath, err := exec.LookPath("klaudiush")
	if err != nil {
		return errors.Wrap(err, "klaudiush not found in PATH")
	}

	cfg, cfgErr := loadHookInstallConfig()
	if cfgErr != nil {
		fmt.Fprintf(
			os.Stderr,
			"Warning: failed to load config, installing default Claude hooks only: %v\n",
			cfgErr,
		)
	}

	return performConfiguredInstall(resolveSettingsPath(), binaryPath, cfg)
}

// tryInstallHooks attempts hook registration, returning error on failure.
func tryInstallHooks(cfg *pkgConfig.Config) error {
	binaryPath, err := exec.LookPath("klaudiush")
	if err != nil {
		return errors.Wrap(err, "klaudiush not found in PATH")
	}

	return performConfiguredInstall(resolveSettingsPath(), binaryPath, cfg)
}

// resolveSettingsPath returns the settings.json path based on --global flag.
func resolveSettingsPath() string {
	if globalFlag {
		return settings.GetUserSettingsPath()
	}

	return settings.GetProjectSettingsPath()
}

func performClaudeInstall(settingsPath, binaryPath string) error {
	registered, err := settings.InstallClaudeDispatcher(settingsPath, binaryPath)
	if err != nil {
		return err
	}

	if registered {
		fmt.Printf("klaudiush is already registered in %s\n", settingsPath)
		return nil
	}

	fmt.Printf("klaudiush registered in %s\n", settingsPath)

	return nil
}

func performCodexInstall(hooksPath, binaryPath string) error {
	registered, err := settings.InstallCodexDispatcher(hooksPath, binaryPath)
	if err != nil {
		return err
	}

	if registered {
		fmt.Printf("klaudiush is already registered in %s\n", hooksPath)
		return nil
	}

	fmt.Printf("klaudiush registered in %s\n", hooksPath)

	return nil
}

func performGeminiInstall(settingsPath, binaryPath string) error {
	registered, err := settings.InstallGeminiDispatcher(settingsPath, binaryPath)
	if err != nil {
		return err
	}

	if registered {
		fmt.Printf("klaudiush is already registered in %s\n", settingsPath)
		return nil
	}

	fmt.Printf("klaudiush registered in %s\n", settingsPath)

	return nil
}

func performConfiguredInstall(
	claudeSettingsPath string,
	binaryPath string,
	cfg *pkgConfig.Config,
) error {
	claudeEnabled := true
	codexHooksPath := ""
	geminiSettingsPath := ""

	if cfg != nil {
		providers := cfg.GetProviders()
		claudeEnabled = providers.GetClaude().IsEnabled()

		codexCfg := providers.GetCodex()
		if codexCfg.IsEnabled() &&
			codexCfg.IsExperimentalEnabled() &&
			codexCfg.HasHooksConfigPath() {
			codexHooksPath = codexCfg.HooksConfigPath
		}

		geminiCfg := providers.GetGemini()
		if geminiCfg.IsEnabled() && geminiCfg.HasSettingsPath() {
			geminiSettingsPath = geminiCfg.SettingsPath
		}
	}

	if claudeEnabled {
		if err := performClaudeInstall(claudeSettingsPath, binaryPath); err != nil {
			return err
		}
	}

	if codexHooksPath != "" {
		if err := performCodexInstall(codexHooksPath, binaryPath); err != nil {
			return err
		}
	}

	if geminiSettingsPath != "" {
		if err := performGeminiInstall(geminiSettingsPath, binaryPath); err != nil {
			return err
		}
	}

	return nil
}

func loadHookInstallConfig() (*pkgConfig.Config, error) {
	return loadConfig(logger.NewNoOpLogger(), "")
}

// loadRawSettings reads and parses a JSON settings file.
func loadRawSettings(path string) (map[string]any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from CLI flags or settings helper
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}

		return nil, errors.Wrap(err, "failed to read settings")
	}

	if len(data) == 0 {
		return make(map[string]any), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.Wrap(err, "failed to parse settings")
	}

	return raw, nil
}

// addHookToSettings adds a PreToolUse hook entry for klaudiush.
func addHookToSettings(raw map[string]any, binaryPath string) {
	hooks, ok := raw["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		raw["hooks"] = hooks
	}

	entry := map[string]any{
		"matcher": "Bash|Write|Edit",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": binaryPath + " --hook-type PreToolUse",
				"timeout": defaultHookTimeout,
			},
		},
	}

	existing, ok := hooks["PreToolUse"].([]any)
	if !ok {
		existing = nil
	}

	hooks["PreToolUse"] = append(existing, entry)
}
