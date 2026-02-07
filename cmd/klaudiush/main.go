// Package main provides the CLI entry point for klaudiush.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-labs/klaudiush/internal/backup"
	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/config/factory"
	"github.com/smykla-labs/klaudiush/internal/crashdump"
	"github.com/smykla-labs/klaudiush/internal/dispatcher"
	"github.com/smykla-labs/klaudiush/internal/parser"
	"github.com/smykla-labs/klaudiush/internal/session"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// ExitCodeAllow indicates the operation should be allowed.
	ExitCodeAllow = 0

	// ExitCodeBlock indicates the operation should be blocked.
	ExitCodeBlock = 2

	// ExitCodeCrash indicates an unexpected panic/crash occurred.
	ExitCodeCrash = 3

	// MigrationMarkerFile is used to track if first-run migration has completed.
	MigrationMarkerFile = ".migration_v1"
)

var (
	hookType     string
	debugMode    bool
	traceMode    bool
	configPath   string
	globalConfig string
	disableList  []string

	// crashContext stores the current hook context for crash recovery.
	// Set during validation dispatch and accessed by panic handler.
	crashContext *hook.Context

	// crashConfig stores the current configuration for crash recovery.
	// Set during validation dispatch and accessed by panic handler.
	crashConfig *config.Config
)

func main() {
	os.Exit(mainWithExitCode())
}

func mainWithExitCode() (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			handlePanic(r)

			exitCode = ExitCodeCrash
		}
	}()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)

		return 1
	}

	return ExitCodeAllow
}

var rootCmd = &cobra.Command{
	Use:   "klaudiush",
	Short: "Claude Code hooks validator",
	Long: `Claude Code hooks validator - validates tool invocations and file operations
before they are executed by Claude Code.`,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		checkVersionFlag()
	},
	RunE:              run,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
}

func init() {
	rootCmd.Flags().StringVarP(
		&hookType,
		"hook-type",
		"T",
		"",
		"Hook event type (PreToolUse, PostToolUse, Notification)",
	)
	rootCmd.Flags().BoolVar(&debugMode, "debug", true, "Enable debug logging")
	rootCmd.Flags().BoolVar(&traceMode, "trace", false, "Enable trace logging")
	rootCmd.Flags().StringVarP(
		&configPath,
		"config",
		"c",
		"",
		"Path to project configuration file (default: .klaudiush/config.toml or klaudiush.toml)",
	)
	rootCmd.Flags().StringVar(
		&globalConfig,
		"global-config",
		"",
		"Path to global configuration file (default: ~/.klaudiush/config.toml)",
	)
	rootCmd.Flags().StringSliceVar(
		&disableList,
		"disable",
		[]string{},
		"Comma-separated list of validators to disable (e.g., commit,markdown)",
	)
}

func run(_ *cobra.Command, _ []string) error {
	// Setup logger
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, debugMode, traceMode)
	if err != nil {
		return errors.Wrap(err, "failed to create logger")
	}

	// Perform first-run migration if needed
	if migErr := performFirstRunMigration(homeDir, log); migErr != nil {
		log.Error("first-run migration failed", "error", migErr)
	}

	// Determine event type using enumer-generated function
	eventType, err := hook.EventTypeString(hookType)
	if err != nil {
		eventType = hook.EventTypePreToolUse // Default to PreToolUse
	}

	log.Info("hook invoked",
		"eventType", eventType,
		"debug", debugMode,
		"trace", traceMode,
	)

	// Load configuration
	cfg, err := loadConfig(log)
	if err != nil {
		return errors.Wrap(err, "failed to load configuration")
	}

	// Parse JSON input
	jsonParser := parser.NewJSONParser(os.Stdin)

	ctx, err := jsonParser.Parse(eventType)
	if err != nil {
		if errors.Is(err, parser.ErrEmptyInput) {
			log.Info("no input provided, allowing")

			return nil
		}

		return errors.Wrap(err, "failed to parse input")
	}

	log.Info("context parsed",
		"tool", ctx.ToolName,
		"command", ctx.GetCommand(),
		"file", filepath.Base(ctx.GetFilePath()),
	)

	// Store context and config for crash recovery
	crashContext = ctx
	crashConfig = cfg

	// Build validator registry from configuration
	registryBuilder := factory.NewRegistryBuilder(log)
	registry := registryBuilder.Build(cfg)

	// Create and initialize session tracker if enabled
	sessionTracker := initSessionTracker(cfg, log)

	// Create dispatcher with session tracker
	disp := dispatcher.NewDispatcherWithOptions(
		registry,
		log,
		dispatcher.NewSequentialExecutor(log),
		dispatcher.WithSessionTracker(sessionTracker),
	)

	// Dispatch validation
	errs := disp.Dispatch(context.Background(), ctx)

	// Save session state after dispatch
	if sessionTracker != nil {
		if err := sessionTracker.Save(); err != nil {
			log.Info("failed to save session state", "error", err)
		}
	}

	// Check if we should block
	if dispatcher.ShouldBlock(errs) {
		errorMsg := dispatcher.FormatErrors(errs)
		fmt.Fprint(os.Stderr, errorMsg)

		log.Error("validation blocked",
			"errorCount", len(errs),
		)

		os.Exit(ExitCodeBlock)
	}

	// If there are warnings, log them
	if len(errs) > 0 {
		errorMsg := dispatcher.FormatErrors(errs)
		fmt.Fprint(os.Stderr, errorMsg)

		log.Info("validation passed with warnings",
			"warningCount", len(errs),
		)
	} else {
		log.Info("validation passed")
	}

	return nil
}

// loadConfig loads configuration from all sources with precedence.
func loadConfig(log logger.Logger) (*config.Config, error) {
	// Build flags map from CLI arguments
	flags := buildFlagsMap()

	// Create koanf loader
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config loader")
	}

	// Load configuration
	cfg, err := loader.Load(flags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	log.Debug("configuration loaded")

	return cfg, nil
}

// initSessionTracker creates and initializes a session tracker if enabled in the config.
func initSessionTracker(cfg *config.Config, log logger.Logger) *session.Tracker {
	sessionCfg := cfg.GetSession()
	if !sessionCfg.IsEnabled() {
		return nil
	}

	tracker := session.NewTracker(
		sessionCfg,
		session.WithLogger(log),
	)

	// Load existing session state
	if err := tracker.Load(); err != nil {
		log.Info("failed to load session state, starting fresh", "error", err)
	}

	log.Debug("session tracker initialized",
		"state_file", sessionCfg.GetStateFile(),
		"max_session_age", sessionCfg.GetMaxSessionAge(),
	)

	return tracker
}

// buildFlagsMap converts CLI flags to a map for the config provider.
func buildFlagsMap() map[string]any {
	flags := make(map[string]any)

	if configPath != "" {
		flags["config_path"] = configPath
	}

	if globalConfig != "" {
		flags["global_config"] = globalConfig
	}

	if len(disableList) > 0 {
		flags["disable"] = disableList
	}

	return flags
}

// performFirstRunMigration creates initial backups for existing configs on first run.
func performFirstRunMigration(homeDir string, log logger.Logger) error {
	// Check if migration already completed
	markerPath := filepath.Join(homeDir, internalconfig.GlobalConfigDir, MigrationMarkerFile)
	if _, err := os.Stat(markerPath); err == nil {
		// Migration already completed
		return nil
	}

	log.Info("performing first-run migration")

	// Backup global config if it exists
	globalConfigPath := filepath.Join(
		homeDir,
		internalconfig.GlobalConfigDir,
		internalconfig.GlobalConfigFile,
	)

	if err := backupConfigIfExists(
		globalConfigPath,
		backup.ConfigTypeGlobal,
		"",
		homeDir,
		log,
	); err != nil {
		log.Error("failed to backup global config", "error", err)
	}

	// Backup project config if it exists
	workDir, err := os.Getwd()
	if err != nil {
		log.Error("failed to get working directory", "error", err)
	} else {
		projectConfigPath := filepath.Join(workDir, internalconfig.ProjectConfigDir, internalconfig.ProjectConfigFile)

		if err := backupConfigIfExists(
			projectConfigPath,
			backup.ConfigTypeProject,
			workDir,
			homeDir,
			log,
		); err != nil {
			log.Error("failed to backup project config", "error", err)
		}
	}

	// Create migration marker file
	configDir := filepath.Join(homeDir, internalconfig.GlobalConfigDir)
	if err := os.MkdirAll(configDir, internalconfig.ConfigDirMode); err != nil {
		return errors.Wrap(err, "failed to create config directory")
	}

	if err := os.WriteFile(markerPath, []byte("v1"), internalconfig.ConfigFileMode); err != nil {
		return errors.Wrap(err, "failed to create migration marker")
	}

	log.Info("first-run migration completed")

	return nil
}

// backupConfigIfExists creates a backup of a config file if it exists.
func backupConfigIfExists(
	configPath string,
	configType backup.ConfigType,
	projectPath string,
	homeDir string,
	log logger.Logger,
) error {
	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	// Create storage
	baseDir := filepath.Join(homeDir, internalconfig.GlobalConfigDir)

	storage, err := backup.NewFilesystemStorage(baseDir, configType, projectPath)
	if err != nil {
		return errors.Wrap(err, "failed to create backup storage")
	}

	// Create backup manager with default config
	backupCfg := &config.BackupConfig{}

	manager, err := backup.NewManager(storage, backupCfg)
	if err != nil {
		return errors.Wrap(err, "failed to create backup manager")
	}

	// Create backup
	opts := backup.CreateBackupOptions{
		ConfigPath: configPath,
		Trigger:    backup.TriggerMigration,
		Metadata: backup.SnapshotMetadata{
			Command: "first-run migration",
		},
	}

	snapshot, err := manager.CreateBackup(opts)
	if err != nil {
		return errors.Wrap(err, "backup creation failed")
	}

	log.Info("created migration backup",
		"config", configPath,
		"snapshot", snapshot.ID,
	)

	return nil
}

// handlePanic handles a recovered panic value by creating a crash dump.
func handlePanic(recovered any) {
	// Get crash dump configuration
	var dumpDir string

	if crashConfig != nil && crashConfig.CrashDump != nil {
		if !crashConfig.CrashDump.IsEnabled() {
			// Crash dumps disabled, just print error
			fmt.Fprintf(os.Stderr, "panic: %v\n", recovered)

			return
		}

		dumpDir = crashConfig.CrashDump.GetDumpDir()
	} else {
		dumpDir = config.DefaultCrashDumpDir
	}

	// Create crash dump
	collector := crashdump.NewCollector(version)
	info := collector.Collect(recovered, crashContext, crashConfig)

	writer, err := crashdump.NewFilesystemWriter(dumpDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "panic: %v\n", recovered)
		fmt.Fprintf(os.Stderr, "failed to create crash dump writer: %v\n", err)

		return
	}

	path, err := writer.Write(info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "panic: %v\n", recovered)
		fmt.Fprintf(os.Stderr, "failed to write crash dump: %v\n", err)

		return
	}

	// Output crash information to stderr
	fmt.Fprintf(os.Stderr, "panic: %v\n", recovered)
	fmt.Fprintf(os.Stderr, "crash dump saved to: %s\n", path)
}
