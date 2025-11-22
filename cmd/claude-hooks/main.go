// Package main provides the CLI entry point for claude-hooks.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/smykla-labs/claude-hooks/internal/dispatcher"
	"github.com/smykla-labs/claude-hooks/internal/parser"
	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

const (
	// ExitCodeAllow indicates the operation should be allowed.
	ExitCodeAllow = 0

	// ExitCodeBlock indicates the operation should be blocked.
	ExitCodeBlock = 2

	// CommandDisplayLength is the maximum length of command to display in logs.
	CommandDisplayLength = 50
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
	Use:   "claude-hooks",
	Short: "Claude Code hooks validator",
	Long: `Claude Code hooks validator - validates tool invocations and file operations
before they are executed by Claude Code.`,
	RunE: run,
}

func init() {
	rootCmd.Flags().StringVar(&hookType, "hook-type", "", "Hook event type (PreToolUse, PostToolUse, Notification)")
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

	// Handle Notification event
	if eventType == hook.Notification {
		// Send bell character to trigger dock bounce
		_, _ = fmt.Fprint(os.Stdout, "\a")
		log.Info("notification bell sent")

		return nil
	}

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

	// TODO: Register validators here
	// This is where we'll register all validators in future phases

	// Create dispatcher
	disp := dispatcher.NewDispatcher(registry, log)

	// Dispatch validation
	errors := disp.Dispatch(ctx)

	// Check if we should block
	if dispatcher.ShouldBlock(errors) {
		errorMsg := dispatcher.FormatErrors(errors)
		fmt.Fprint(os.Stderr, errorMsg)

		log.Error("validation blocked",
			"errorCount", len(errors),
		)

		os.Exit(ExitCodeBlock)
	}

	// If there are warnings, log them
	if len(errors) > 0 {
		errorMsg := dispatcher.FormatErrors(errors)
		fmt.Fprint(os.Stderr, errorMsg)

		log.Info("validation passed with warnings",
			"warningCount", len(errors),
		)
	} else {
		log.Info("validation passed")
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..."
}
