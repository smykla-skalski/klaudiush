// Package main provides the CLI entry point for klaudiush.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/smykla-labs/klaudiush/internal/dispatcher"
	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	githubpkg "github.com/smykla-labs/klaudiush/internal/github"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/parser"
	"github.com/smykla-labs/klaudiush/internal/validator"
	filevalidators "github.com/smykla-labs/klaudiush/internal/validators/file"
	gitvalidators "github.com/smykla-labs/klaudiush/internal/validators/git"
	notificationvalidators "github.com/smykla-labs/klaudiush/internal/validators/notification"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

const (
	// ExitCodeAllow indicates the operation should be allowed.
	ExitCodeAllow = 0

	// ExitCodeBlock indicates the operation should be blocked.
	ExitCodeBlock = 2

	// CommandDisplayLength is the maximum length of command to display in logs.
	CommandDisplayLength = 50

	// LinterTimeout is the timeout for linter operations.
	LinterTimeout = 10 * time.Second
)

var (
	hookType  string
	debugMode bool
	traceMode bool
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
}

func run(_ *cobra.Command, _ []string) error {
	// Setup logger
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, debugMode, traceMode)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Determine event type
	eventType := hook.EventType(hookType)
	if eventType == "" {
		eventType = hook.PreToolUse // Default to PreToolUse
	}

	log.Info("hook invoked",
		"eventType", eventType,
		"debug", debugMode,
		"trace", traceMode,
	)

	// Parse JSON input
	jsonParser := parser.NewJSONParser(os.Stdin)

	ctx, err := jsonParser.Parse(eventType)
	if err != nil {
		if errors.Is(err, parser.ErrEmptyInput) {
			log.Info("no input provided, allowing")

			return nil
		}

		return fmt.Errorf("failed to parse input: %w", err)
	}

	log.Info("context parsed",
		"tool", ctx.ToolName,
		"command", truncate(ctx.GetCommand(), CommandDisplayLength),
		"file", filepath.Base(ctx.GetFilePath()),
	)

	// Create validator registry
	registry := validator.NewRegistry()

	// Register validators
	registerValidators(registry, log)

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

func registerValidators(registry *validator.Registry, log logger.Logger) {
	registerGitValidators(registry, log)
	registerFileValidators(registry, log)
	registerNotificationValidators(registry, log)
}

func registerGitValidators(registry *validator.Registry, log logger.Logger) {
	registry.Register(
		gitvalidators.NewAddValidator(log, nil, nil), // nil uses RealGitRunner
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git add"),
		),
	)

	registry.Register(
		gitvalidators.NewNoVerifyValidator(log, nil),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git commit"),
		),
	)

	registry.Register(
		gitvalidators.NewCommitValidator(log, nil, nil), // nil uses RealGitRunner
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git commit"),
		),
	)

	registry.Register(
		gitvalidators.NewPushValidator(log, nil, nil), // nil uses RealGitRunner
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("git push"),
		),
	)

	registry.Register(
		gitvalidators.NewPRValidator(log),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.CommandContains("gh pr create"),
		),
	)

	registry.Register(
		gitvalidators.NewBranchValidator(log),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIs(hook.Bash),
			validator.Or(
				validator.CommandContains("git checkout -b"),
				validator.And(
					validator.CommandContains("git branch"),
					validator.Not(validator.Or(
						validator.CommandContains("-d"),
						validator.CommandContains("-D"),
						validator.CommandContains("--delete"),
					)),
				),
			),
		),
	)
}

func registerFileValidators(registry *validator.Registry, log logger.Logger) {
	// Initialize linters
	runner := execpkg.NewCommandRunner(LinterTimeout)
	shellChecker := linters.NewShellChecker(runner)
	terraformFormatter := linters.NewTerraformFormatter(runner)
	tfLinter := linters.NewTfLinter(runner)
	actionLinter := linters.NewActionLinter(runner)
	markdownLinter := linters.NewMarkdownLinter(runner)

	// Initialize GitHub client
	githubClient := githubpkg.NewClient()

	registry.Register(
		filevalidators.NewMarkdownValidator(markdownLinter, log),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIn(hook.Write, hook.Edit, hook.MultiEdit),
			validator.FileExtensionIs(".md"),
		),
	)

	registry.Register(
		filevalidators.NewTerraformValidator(terraformFormatter, tfLinter, log),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIn(hook.Write, hook.Edit, hook.MultiEdit),
			validator.FileExtensionIs(".tf"),
		),
	)

	registry.Register(
		filevalidators.NewShellScriptValidator(log, shellChecker),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIn(hook.Write, hook.Edit, hook.MultiEdit),
			validator.Or(
				validator.FileExtensionIs(".sh"),
				validator.FileExtensionIs(".bash"),
			),
		),
	)

	registry.Register(
		filevalidators.NewWorkflowValidator(actionLinter, githubClient, log),
		validator.And(
			validator.EventTypeIs(hook.PreToolUse),
			validator.ToolTypeIn(hook.Write, hook.Edit, hook.MultiEdit),
			validator.Or(
				validator.FilePathContains(".github/workflows/"),
				validator.FilePathContains(".github/actions/"),
			),
			validator.Or(
				validator.FileExtensionIs(".yml"),
				validator.FileExtensionIs(".yaml"),
			),
		),
	)
}

func registerNotificationValidators(registry *validator.Registry, log logger.Logger) {
	registry.Register(
		notificationvalidators.NewBellValidator(log),
		validator.EventTypeIs(hook.Notification),
	)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..."
}
