// Package main provides the CLI entry point for klaudiush.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalcolor "github.com/smykla-skalski/klaudiush/internal/color"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
	backupchecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/backup"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/binary"
	configchecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/config"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/hook"
	overrideschecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/overrides"
	patternschecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/patterns"
	ruleschecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/rules"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/tools"
	"github.com/smykla-skalski/klaudiush/internal/doctor/fixers"
	"github.com/smykla-skalski/klaudiush/internal/doctor/reporters"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var (
	verboseFlag  bool
	fixFlag      bool
	categoryFlag []string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose klaudiush setup and configuration",
	Long: `Diagnose klaudiush setup and configuration issues.

Checks:
- Binary availability and permissions
- Hook registration in Claude settings
- Configuration file validity
- Pattern learning system health
- Backup system health
- Overrides configuration health
- Optional tool dependencies (shellcheck, terraform, etc.)

Examples:
  klaudiush doctor              # Run all checks
  klaudiush doctor --verbose    # Run with detailed output
  klaudiush doctor --fix        # Automatically fix issues
  klaudiush doctor --category binary,hook  # Check specific categories`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	doctorCmd.Flags().BoolVarP(
		&verboseFlag,
		"verbose",
		"v",
		false,
		"Enable verbose output with detailed context",
	)

	doctorCmd.Flags().BoolVar(
		&fixFlag,
		"fix",
		false,
		"Automatically fix issues without prompting",
	)

	doctorCmd.Flags().StringSliceVar(
		&categoryFlag,
		"category",
		[]string{},
		"Filter checks by category (binary, hook, config, tools, patterns, backup, overrides)",
	)
}

func runDoctor(_ *cobra.Command, _ []string) error {
	// Setup logger
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, true, false)
	if err != nil {
		return errors.Wrap(err, "failed to create logger")
	}

	log.Info("starting doctor command",
		"verbose", verboseFlag,
		"fix", fixFlag,
		"categories", categoryFlag,
	)

	// Build registry
	registry := buildDoctorRegistry()

	// Create prompter for interactive mode
	prompter := prompt.NewStdPrompter()

	// Register fixers
	registerFixers(registry, prompter)

	// Create reporter based on terminal capabilities
	reporter := selectReporter()

	// Create runner
	runner := doctor.NewRunner(registry, reporter, prompter, log)

	// Parse categories
	categories := parseCategories(categoryFlag)

	// Build run options
	opts := doctor.RunOptions{
		Verbose:     verboseFlag,
		AutoFix:     fixFlag,
		Interactive: !fixFlag && isInteractive(),
		Categories:  categories,
		Global:      true,
		Project:     true,
	}

	// Run doctor
	ctx := context.Background()

	if err := runner.Run(ctx, opts); err != nil {
		// Error means checks failed
		if errors.Is(err, errors.New("health checks failed")) {
			os.Exit(1)
		}

		return errors.Wrap(err, "doctor command failed")
	}

	return nil
}

// buildDoctorRegistry creates and populates the health check registry.
func buildDoctorRegistry() *doctor.Registry {
	registry := doctor.NewRegistry()

	// Register binary checkers
	registry.RegisterChecker(binary.NewExistsChecker())
	registry.RegisterChecker(binary.NewPermissionsChecker())
	registry.RegisterChecker(binary.NewLocationChecker())

	// Register hook checkers
	registry.RegisterChecker(hook.NewUserRegistrationChecker())
	registry.RegisterChecker(hook.NewProjectRegistrationChecker())
	registry.RegisterChecker(hook.NewProjectLocalRegistrationChecker())
	registry.RegisterChecker(hook.NewUserPreToolUseChecker())
	registry.RegisterChecker(hook.NewProjectPreToolUseChecker())
	registry.RegisterChecker(hook.NewPathValidationChecker())

	// Register config checkers
	registry.RegisterChecker(configchecker.NewGlobalChecker())
	registry.RegisterChecker(configchecker.NewProjectChecker())
	registry.RegisterChecker(configchecker.NewPermissionsChecker())

	// Register rules checkers
	registry.RegisterChecker(ruleschecker.NewRulesChecker())

	// Register tools checkers
	registry.RegisterChecker(tools.NewShellcheckChecker())
	registry.RegisterChecker(tools.NewTerraformChecker())
	registry.RegisterChecker(tools.NewTflintChecker())
	registry.RegisterChecker(tools.NewActionlintChecker())
	registry.RegisterChecker(tools.NewMarkdownlintChecker())

	// Register patterns checkers
	registry.RegisterChecker(patternschecker.NewSeedDataChecker())
	registry.RegisterChecker(patternschecker.NewDataFileChecker())
	registry.RegisterChecker(patternschecker.NewDescriptionChecker())

	// Register backup checkers
	registry.RegisterChecker(backupchecker.NewDirectoryChecker())
	registry.RegisterChecker(backupchecker.NewMetadataChecker())
	registry.RegisterChecker(backupchecker.NewIntegrityChecker())

	// Register overrides checkers
	registry.RegisterChecker(overrideschecker.NewExpiredChecker())
	registry.RegisterChecker(overrideschecker.NewUnknownTargetChecker())
	registry.RegisterChecker(overrideschecker.NewRedundantChecker())

	return registry
}

// registerFixers registers all available fixers.
func registerFixers(registry *doctor.Registry, prompter prompt.Prompter) {
	registry.RegisterFixer(fixers.NewInstallHookFixer(prompter))
	registry.RegisterFixer(fixers.NewPermissionsFixer(prompter))
	registry.RegisterFixer(fixers.NewConfigFixer(prompter))
	registry.RegisterFixer(fixers.NewInstallBinaryFixer(prompter))
	registry.RegisterFixer(fixers.NewRulesFixer(prompter))
	registry.RegisterFixer(fixers.NewPatternsFixer(prompter))
	registry.RegisterFixer(fixers.NewBackupFixer(prompter))
	registry.RegisterFixer(fixers.NewOverridesFixer(prompter))
}

// parseCategories converts string category names to Category types.
func parseCategories(names []string) []doctor.Category {
	if len(names) == 0 {
		return nil
	}

	categoryMap := map[string]doctor.Category{
		"binary":    doctor.CategoryBinary,
		"hook":      doctor.CategoryHook,
		"config":    doctor.CategoryConfig,
		"tools":     doctor.CategoryTools,
		"patterns":  doctor.CategoryPatterns,
		"backup":    doctor.CategoryBackup,
		"overrides": doctor.CategoryOverrides,
	}

	var categories []doctor.Category

	for _, name := range names {
		if cat, ok := categoryMap[name]; ok {
			categories = append(categories, cat)
		} else {
			fmt.Fprintf(os.Stderr, "Warning: unknown category %q, ignoring\n", name)
		}
	}

	return categories
}

// isInteractive returns true if stdin is a terminal.
func isInteractive() bool {
	fileInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// selectReporter picks the right reporter based on TTY and color settings.
//
//	TTY + colors     -> InteractiveReporter (spinners + colored table)
//	TTY + no colors  -> InteractiveReporter (spinners + plain table)
//	non-TTY + colors -> ColoredReporter (static colored table, no spinners)
//	non-TTY + no color -> SimpleReporter (plain text, backward compat)
//
//nolint:ireturn // factory function selecting reporter implementation by environment
func selectReporter() doctor.Reporter {
	colorEnabled := internalcolor.Profile(noColorFlag)
	tty := internalcolor.IsTerminal(os.Stdout)
	theme := internalcolor.NewTheme(colorEnabled)

	if tty {
		return reporters.NewInteractiveReporter(theme)
	}

	if colorEnabled {
		return reporters.NewColoredReporter(theme)
	}

	return reporters.NewSimpleReporter()
}
