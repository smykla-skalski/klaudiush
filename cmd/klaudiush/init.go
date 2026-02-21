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
	"github.com/smykla-skalski/klaudiush/internal/doctor/fixers"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/tui"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

const defaultHookTimeout = 30

var (
	globalFlag       bool
	forceFlag        bool
	noTUIFlag        bool
	installHooksFlag bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize klaudiush configuration",
	Long: `Initialize klaudiush configuration file and register hooks.

By default, creates a project-local configuration file (.klaudiush/config.toml)
and registers klaudiush as a PreToolUse hook in Claude Code settings.

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
		"Register klaudiush hooks in Claude Code settings",
	)
}

func runInit(cmd *cobra.Command, _ []string) error {
	// Explicit --install-hooks: skip TUI, just install hooks
	if cmd.Flags().Changed("install-hooks") && installHooksFlag {
		return runInstallHooks()
	}

	writer := config.NewWriter()

	// Check if config already exists
	configPath, existingConfig, err := checkExistingConfig(writer)
	if err != nil {
		return err
	}

	// If --force and config exists, create backup before overwriting
	if forceFlag && existingConfig {
		if backupErr := backupBeforeForce(configPath); backupErr != nil {
			// Log warning but don't fail
			fmt.Fprintf(
				os.Stderr,
				"⚠️  Warning: failed to backup existing config: %v\n",
				backupErr,
			)
		}
	}

	// Get default signoff from git config
	defaultSignoff := getDefaultSignoff()

	// Determine if we should show git exclude option
	showGitExclude := !globalFlag && git.IsInGitRepo()

	// Create UI (TUI or fallback based on terminal capabilities and flags)
	ui := tui.NewWithFallback(noTUIFlag)

	// Run the init form
	cfg, addToExclude, err := ui.RunInitForm(tui.InitFormOptions{
		Global:         globalFlag,
		DefaultSignoff: defaultSignoff,
		ShowGitExclude: showGitExclude,
	})
	if err != nil {
		return errors.Wrap(err, "configuration form failed")
	}

	// Write configuration
	if err := writeConfig(writer, cfg, configPath); err != nil {
		return err
	}

	// Handle .git/info/exclude for project config
	if addToExclude && showGitExclude {
		if err := addConfigToExclude(); err != nil {
			fmt.Fprintf(
				os.Stderr,
				"⚠️  Warning: failed to add to .git/info/exclude: %v\n",
				err,
			)
		} else {
			fmt.Println("✅ Added to .git/info/exclude")
		}
	}

	// Install hooks if enabled (default true, non-fatal)
	if installHooksFlag {
		if err := tryInstallHooks(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: hook registration failed: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("Configuration initialized successfully!")

	return nil
}

// checkExistingConfig checks if config already exists and returns the path and existence flag.
func checkExistingConfig(writer *config.Writer) (string, bool, error) {
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

	if configExists && !forceFlag {
		return "", false, errors.Errorf(
			"configuration file already exists: %s\nUse --force to overwrite",
			configPath,
		)
	}

	return configPath, configExists, nil
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

	return performInstall(resolveSettingsPath(), binaryPath)
}

// tryInstallHooks attempts hook registration, returning error on failure.
func tryInstallHooks() error {
	binaryPath, err := exec.LookPath("klaudiush")
	if err != nil {
		return errors.Wrap(err, "klaudiush not found in PATH")
	}

	return performInstall(resolveSettingsPath(), binaryPath)
}

// resolveSettingsPath returns the settings.json path based on --global flag.
func resolveSettingsPath() string {
	if globalFlag {
		return settings.GetUserSettingsPath()
	}

	return settings.GetProjectSettingsPath()
}

// performInstall registers klaudiush in the given settings file.
func performInstall(settingsPath, binaryPath string) error {
	// Check if already registered
	parser := settings.NewSettingsParser(settingsPath)

	registered, err := parser.IsDispatcherRegistered(binaryPath)
	if err != nil {
		return errors.Wrap(err, "failed to check settings")
	}

	if registered {
		fmt.Printf("klaudiush is already registered in %s\n", settingsPath)
		return nil
	}

	// Load existing settings as raw map to preserve unknown fields
	raw, err := loadRawSettings(settingsPath)
	if err != nil {
		return err
	}

	addHookToSettings(raw, binaryPath)

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal settings")
	}

	data = append(data, '\n')

	if err := fixers.AtomicWriteFile(settingsPath, data, true); err != nil {
		return errors.Wrap(err, "failed to write settings")
	}

	fmt.Printf("klaudiush registered in %s\n", settingsPath)

	return nil
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
