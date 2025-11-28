// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var validatorFilter string

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug klaudiush configuration",
	Long: `Debug klaudiush configuration and internal state.

Subcommands:
  rules     Show loaded validation rules`,
}

var debugRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Show loaded validation rules",
	Long: `Show loaded validation rules from configuration.

Displays all rules with their match conditions, actions, and priorities.
Rules are shown in evaluation order (highest priority first).

Examples:
  klaudiush debug rules                    # Show all rules
  klaudiush debug rules --validator git.push  # Show rules for git.push validator`,
	RunE: runDebugRules,
}

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.AddCommand(debugRulesCmd)

	debugRulesCmd.Flags().StringVar(
		&validatorFilter,
		"validator",
		"",
		"Filter rules by validator type (e.g., git.push, file.*, secrets.secrets)",
	)
}

func runDebugRules(_ *cobra.Command, _ []string) error {
	// Setup logger
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return errors.Wrap(err, "failed to get home directory")
	}

	logFile := filepath.Join(homeDir, ".claude", "hooks", "dispatcher.log")

	log, err := logger.NewFileLogger(logFile, false, false)
	if err != nil {
		return errors.Wrap(err, "failed to create logger")
	}

	log.Info("debug rules command invoked",
		"validatorFilter", validatorFilter,
	)

	// Load configuration
	cfg, err := loadConfigForDebug(log)
	if err != nil {
		return errors.Wrap(err, "failed to load configuration")
	}

	// Display rules configuration
	displayRulesConfig(cfg, validatorFilter)

	return nil
}

func loadConfigForDebug(log logger.Logger) (*config.Config, error) {
	flags := buildFlagsMap()

	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create config loader")
	}

	cfg, err := loader.Load(flags)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load config")
	}

	log.Debug("configuration loaded for debug")

	return cfg, nil
}

func displayRulesConfig(cfg *config.Config, filter string) {
	rules := cfg.GetRules()
	if rules == nil || len(rules.Rules) == 0 {
		fmt.Println("No rules configured.")
		fmt.Println("")
		fmt.Println("To configure rules, add them to:")
		fmt.Println("  - Global: ~/.klaudiush/config.toml")
		fmt.Println("  - Project: .klaudiush/config.toml")
		fmt.Println("")
		fmt.Println("See docs/RULES_GUIDE.md for configuration examples.")

		return
	}

	// Header
	fmt.Println("Validation Rules")
	fmt.Println("================")
	fmt.Println("")

	// Rules engine status
	fmt.Printf("Engine Enabled: %v\n", rules.IsEnabled())
	fmt.Printf("Stop on First Match: %v\n", rules.ShouldStopOnFirstMatch())
	fmt.Printf("Total Rules: %d\n", len(rules.Rules))
	fmt.Println("")

	// Filter rules if needed
	filteredRules := filterRules(rules.Rules, filter)

	if len(filteredRules) == 0 {
		fmt.Printf("No rules match filter: %s\n", filter)

		return
	}

	// Display rules
	for i, rule := range filteredRules {
		displayRule(i+1, &rule)
	}
}

func filterRules(rules []config.RuleConfig, filter string) []config.RuleConfig {
	if filter == "" {
		return rules
	}

	var filtered []config.RuleConfig

	for _, rule := range rules {
		if rule.Match == nil {
			continue
		}

		validatorType := rule.Match.ValidatorType

		// Check exact match
		if validatorType == filter {
			filtered = append(filtered, rule)

			continue
		}

		// Check wildcard match (e.g., "git.*" matches "git.push")
		if before, ok := strings.CutSuffix(filter, ".*"); ok {
			prefix := before
			if strings.HasPrefix(validatorType, prefix+".") {
				filtered = append(filtered, rule)

				continue
			}
		}

		// Check if filter is a wildcard that matches rule's validator type
		if before, ok := strings.CutSuffix(validatorType, ".*"); ok {
			prefix := before
			if strings.HasPrefix(filter, prefix+".") {
				filtered = append(filtered, rule)

				continue
			}
		}

		// Check "*" matches everything
		if validatorType == "*" {
			filtered = append(filtered, rule)
		}
	}

	return filtered
}

func displayRule(index int, rule *config.RuleConfig) {
	enabledStr := "enabled"
	if !rule.IsRuleEnabled() {
		enabledStr = "DISABLED"
	}

	fmt.Printf("Rule #%d: %s [%s]\n", index, rule.Name, enabledStr)
	fmt.Printf("  Priority: %d\n", rule.Priority)

	if rule.Description != "" {
		fmt.Printf("  Description: %s\n", rule.Description)
	}

	// Match conditions
	if rule.Match != nil {
		fmt.Println("  Match:")
		displayMatchCondition("    ", rule.Match)
	}

	// Action
	if rule.Action != nil {
		fmt.Println("  Action:")
		fmt.Printf("    Type: %s\n", rule.Action.GetActionType())

		if rule.Action.Message != "" {
			fmt.Printf("    Message: %s\n", rule.Action.Message)
		}

		if rule.Action.Reference != "" {
			fmt.Printf("    Reference: %s\n", rule.Action.Reference)
		}
	}

	fmt.Println("")
}

func displayMatchCondition(indent string, match *config.RuleMatchConfig) {
	if match.ValidatorType != "" {
		fmt.Printf("%sValidator Type: %s\n", indent, match.ValidatorType)
	}

	if match.RepoPattern != "" {
		fmt.Printf("%sRepo Pattern: %s\n", indent, match.RepoPattern)
	}

	if match.Remote != "" {
		fmt.Printf("%sRemote: %s\n", indent, match.Remote)
	}

	if match.BranchPattern != "" {
		fmt.Printf("%sBranch Pattern: %s\n", indent, match.BranchPattern)
	}

	if match.FilePattern != "" {
		fmt.Printf("%sFile Pattern: %s\n", indent, match.FilePattern)
	}

	if match.ContentPattern != "" {
		fmt.Printf("%sContent Pattern: %s\n", indent, match.ContentPattern)
	}

	if match.CommandPattern != "" {
		fmt.Printf("%sCommand Pattern: %s\n", indent, match.CommandPattern)
	}

	if match.ToolType != "" {
		fmt.Printf("%sTool Type: %s\n", indent, match.ToolType)
	}

	if match.EventType != "" {
		fmt.Printf("%sEvent Type: %s\n", indent, match.EventType)
	}
}
