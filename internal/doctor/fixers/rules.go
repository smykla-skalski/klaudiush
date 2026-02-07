// Package fixers provides auto-fix functionality for doctor checks.
package fixers

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	ruleschecker "github.com/smykla-labs/klaudiush/internal/doctor/checkers/rules"
	"github.com/smykla-labs/klaudiush/internal/prompt"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

const (
	// disabledNote is the full description for rules with no existing description.
	disabledNote = "DISABLED BY DOCTOR: fix the rule configuration and re-enable"

	// disabledSuffix is appended to existing descriptions.
	disabledSuffix = "[DISABLED BY DOCTOR: fix and re-enable]"
)

// RulesFixer fixes invalid rules by disabling them.
type RulesFixer struct {
	prompter     prompt.Prompter
	rulesChecker *ruleschecker.RulesChecker
	loader       *internalconfig.KoanfLoader
}

// NewRulesFixer creates a new RulesFixer.
func NewRulesFixer(prompter prompt.Prompter) *RulesFixer {
	loader, _ := internalconfig.NewKoanfLoader()

	return &RulesFixer{
		prompter:     prompter,
		rulesChecker: ruleschecker.NewRulesChecker(),
		loader:       loader,
	}
}

// ID returns the fixer identifier.
func (*RulesFixer) ID() string {
	return "fix_invalid_rules"
}

// Description returns a human-readable description.
func (*RulesFixer) Description() string {
	return "Disable invalid rules in configuration (rules can be re-enabled after manual fix)"
}

// CanFix checks if this fixer can fix the given result.
func (*RulesFixer) CanFix(result doctor.CheckResult) bool {
	return result.FixID == "fix_invalid_rules" && result.Status == doctor.StatusFail
}

// Fix disables invalid rules in the configuration.
func (f *RulesFixer) Fix(ctx context.Context, interactive bool) error {
	// Run the rules checker to get current issues
	f.rulesChecker.Check(ctx)
	issues := f.rulesChecker.GetIssues()

	if len(issues) == 0 {
		return nil
	}

	// Load only the project config (not merged with defaults/global/env)
	// This ensures we only write back what was in the project config
	cfg, configPath, err := f.loadProjectConfig()
	if err != nil {
		return err
	}

	if cfg == nil || cfg.Rules == nil || len(cfg.Rules.Rules) == 0 {
		return nil
	}

	// Collect indices of rules to disable
	rulesToDisable := f.collectFixableRules(issues)
	if len(rulesToDisable) == 0 {
		return nil
	}

	// Confirm with user if interactive
	if interactive {
		if !f.confirmFix(len(rulesToDisable)) {
			return nil
		}
	}

	// Disable the invalid rules
	f.disableRules(cfg, rulesToDisable)

	// Write back to the same project config file that was loaded
	writer := internalconfig.NewWriter()
	if err := writer.WriteFile(configPath, cfg); err != nil {
		return errors.Wrapf(err, "failed to write config to %s", configPath)
	}

	return nil
}

// loadProjectConfig loads only the project configuration file.
// This ensures we don't contaminate the project config with defaults/global/env values.
func (f *RulesFixer) loadProjectConfig() (*config.Config, string, error) {
	if f.loader == nil {
		loader, err := internalconfig.NewKoanfLoader()
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to create config loader")
		}

		f.loader = loader
	}

	// Load only the project config file (not merged with defaults/global/env)
	cfg, path, err := f.loader.LoadProjectConfigOnly()
	if err != nil {
		return nil, path, errors.Wrap(err, "failed to load project config")
	}

	return cfg, path, nil
}

// collectFixableRules collects indices of rules that can be fixed.
func (*RulesFixer) collectFixableRules(issues []ruleschecker.RuleIssue) map[int]bool {
	rulesToDisable := make(map[int]bool)

	for _, issue := range issues {
		if issue.Fixable {
			rulesToDisable[issue.RuleIndex] = true
		}
	}

	return rulesToDisable
}

// confirmFix prompts the user for confirmation.
func (f *RulesFixer) confirmFix(count int) bool {
	msg := fmt.Sprintf(
		"Disable %d invalid rule(s)? (They can be re-enabled after manual fix)",
		count,
	)

	confirmed, err := f.prompter.Confirm(msg, true)
	if err != nil {
		return false
	}

	return confirmed
}

// disableRules marks the specified rules as disabled.
func (*RulesFixer) disableRules(cfg *config.Config, rulesToDisable map[int]bool) {
	for idx := range rulesToDisable {
		if idx < len(cfg.Rules.Rules) {
			disabled := false
			cfg.Rules.Rules[idx].Enabled = &disabled

			// Add a description note if not present
			desc := cfg.Rules.Rules[idx].Description
			if desc == "" {
				cfg.Rules.Rules[idx].Description = disabledNote
			} else if !containsDisabledNote(desc) {
				cfg.Rules.Rules[idx].Description = desc + " " + disabledSuffix
			}
		}
	}
}

// containsDisabledNote checks if description already has the disabled note.
func containsDisabledNote(desc string) bool {
	if desc == "" {
		return false
	}

	return desc == disabledNote || strings.HasSuffix(desc, disabledSuffix)
}
