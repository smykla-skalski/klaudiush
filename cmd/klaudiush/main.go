// Package main provides the CLI entry point for klaudiush.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/config/factory"
	"github.com/smykla-labs/klaudiush/internal/dispatcher"
	"github.com/smykla-labs/klaudiush/internal/parser"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// ExitCodeAllow indicates the operation should be allowed.
	ExitCodeAllow = 0

	// ExitCodeBlock indicates the operation should be blocked.
	ExitCodeBlock = 2
)

var (
	hookType     string
	debugMode    bool
	traceMode    bool
	configPath   string
	globalConfig string
	disableList  []string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "klaudiush",
	Short: "Claude Code hooks validator",
	Long: `Claude Code hooks validator - validates tool invocations and file operations
before they are executed by Claude Code.`,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		checkVersionFlag()
	},
	RunE: run,
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

	// Build validator registry from configuration
	registryBuilder := factory.NewRegistryBuilder(log)
	registry := registryBuilder.Build(cfg)

	// Create dispatcher
	disp := dispatcher.NewDispatcher(registry, log)

	// Dispatch validation
	errs := disp.Dispatch(context.Background(), ctx)

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
