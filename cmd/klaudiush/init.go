// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/backup"
	"github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/tui"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

var (
	globalFlag bool
	forceFlag  bool
	noTUIFlag  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize klaudiush configuration",
	Long: `Initialize klaudiush configuration file.

By default, creates a project-local configuration file (.klaudiush/config.toml).
Use --global or -g to create a global configuration file (~/.klaudiush/config.toml).

The initialization process will prompt you to configure:
- Git commit signoff (default: from git config user.name and user.email)
- Whether to add the config file to .git/info/exclude (project-local only)

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
}

func runInit(_ *cobra.Command, _ []string) error {
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
