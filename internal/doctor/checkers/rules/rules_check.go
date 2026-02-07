// Package ruleschecker provides checkers for validation rules configuration.
package ruleschecker

import (
	"context"
	"fmt"
	"slices"
	"strings"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/stringutil"
)

// RuleIssue represents an issue found in a rule configuration.
type RuleIssue struct {
	RuleIndex int
	RuleName  string
	IssueType string
	Message   string
	Fixable   bool
}

// ConfigLoader defines the interface for configuration loading operations.
type ConfigLoader interface {
	HasProjectConfig() bool
	Load(flags map[string]any) (*config.Config, error)
	LoadWithoutValidation(flags map[string]any) (*config.Config, error)
}

// RulesChecker checks the validity of rules configuration.
type RulesChecker struct {
	loader    ConfigLoader
	loaderErr error
	issues    []RuleIssue
}

// NewRulesChecker creates a new rules checker.
func NewRulesChecker() *RulesChecker {
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return &RulesChecker{
			loaderErr: err,
		}
	}

	return &RulesChecker{
		loader: loader,
	}
}

// NewRulesCheckerWithLoader creates a RulesChecker with a custom loader (for testing).
func NewRulesCheckerWithLoader(loader ConfigLoader) *RulesChecker {
	return &RulesChecker{
		loader: loader,
	}
}

// Name returns the name of the check.
func (*RulesChecker) Name() string {
	return "Rules validation"
}

// Category returns the category of the check.
func (*RulesChecker) Category() doctor.Category {
	return doctor.CategoryConfig
}

// GetIssues returns the issues found during the last check.
func (c *RulesChecker) GetIssues() []RuleIssue {
	return c.issues
}

// Check performs the rules validation check.
func (c *RulesChecker) Check(_ context.Context) doctor.CheckResult {
	c.issues = nil

	// Check if loader initialization failed
	if c.loaderErr != nil {
		return doctor.FailError("Rules validation",
			fmt.Sprintf("config loader initialization failed: %v", c.loaderErr))
	}

	if !c.loader.HasProjectConfig() {
		return doctor.Skip("Rules validation", "No project config found")
	}

	// Use LoadWithoutValidation to allow checking invalid rules
	cfg, err := c.loader.LoadWithoutValidation(nil)
	if err != nil {
		// Config loading errors are handled by config checker
		return doctor.Skip("Rules validation", "Config load failed (see config check)")
	}

	if cfg.Rules == nil || len(cfg.Rules.Rules) == 0 {
		return doctor.Pass("Rules validation", "No rules configured")
	}

	// Validate each enabled rule
	enabledCount := 0

	for i := range cfg.Rules.Rules {
		// Skip validation for disabled rules
		if !cfg.Rules.Rules[i].IsRuleEnabled() {
			continue
		}

		enabledCount++

		c.validateRule(i, &cfg.Rules.Rules[i])
	}

	if len(c.issues) == 0 {
		msg := fmt.Sprintf("%d rule(s) validated", enabledCount)

		return doctor.Pass("Rules validation", msg)
	}

	// Build details from issues
	details := make([]string, 0, len(c.issues)+1)

	for _, issue := range c.issues {
		var prefix string
		if issue.RuleName != "" {
			prefix = fmt.Sprintf("Rule %q", issue.RuleName)
		} else {
			prefix = fmt.Sprintf("Rule #%d", issue.RuleIndex+1)
		}

		details = append(details, fmt.Sprintf("%s: %s", prefix, issue.Message))
	}

	// Count fixable issues
	fixableCount := 0

	for _, issue := range c.issues {
		if issue.Fixable {
			fixableCount++
		}
	}

	result := doctor.FailError("Rules validation",
		fmt.Sprintf("%d invalid rule(s) found", len(c.issues))).
		WithDetails(details...)

	if fixableCount > 0 {
		result = result.WithFixID("fix_invalid_rules")
	}

	return result
}

// validateRule validates a single rule and records issues.
func (c *RulesChecker) validateRule(index int, rule *config.RuleConfig) {
	ruleName := rule.Name

	// Check for missing match section
	if rule.Match == nil {
		c.issues = append(c.issues, RuleIssue{
			RuleIndex: index,
			RuleName:  ruleName,
			IssueType: "no_match_section",
			Message:   "missing match section (rule will never match)",
			Fixable:   true,
		})

		return // No point checking other fields if match is missing
	}

	// Check for empty match conditions (using centralized method)
	if !rule.Match.HasMatchConditions() {
		c.issues = append(c.issues, RuleIssue{
			RuleIndex: index,
			RuleName:  ruleName,
			IssueType: "empty_match",
			Message:   "match section is empty (rule will never match)",
			Fixable:   true,
		})
	}

	// Check for invalid event_type
	if rule.Match.EventType != "" {
		if !stringutil.ContainsCaseInsensitive(config.ValidEventTypes, rule.Match.EventType) {
			c.issues = append(c.issues, RuleIssue{
				RuleIndex: index,
				RuleName:  ruleName,
				IssueType: "invalid_event_type",
				Message: fmt.Sprintf("invalid event_type %q (valid: %s)",
					rule.Match.EventType, strings.Join(config.ValidEventTypes, ", ")),
				Fixable: true,
			})
		}
	}

	// Check for invalid tool_type
	if rule.Match.ToolType != "" {
		if !stringutil.ContainsCaseInsensitive(config.ValidToolTypes, rule.Match.ToolType) {
			c.issues = append(c.issues, RuleIssue{
				RuleIndex: index,
				RuleName:  ruleName,
				IssueType: "invalid_tool_type",
				Message: fmt.Sprintf("invalid tool_type %q (valid: %s)",
					rule.Match.ToolType, strings.Join(config.ValidToolTypes, ", ")),
				Fixable: true,
			})
		}
	}

	// Check for invalid action type
	if rule.Action != nil && rule.Action.Type != "" {
		if !slices.Contains(config.ValidActionTypes, rule.Action.Type) {
			c.issues = append(c.issues, RuleIssue{
				RuleIndex: index,
				RuleName:  ruleName,
				IssueType: "invalid_action_type",
				Message: fmt.Sprintf("invalid action type %q (valid: %s)",
					rule.Action.Type, strings.Join(config.ValidActionTypes, ", ")),
				Fixable: true,
			})
		}
	}
}
