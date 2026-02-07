// Package config provides internal configuration loading and processing.
package config

import (
	"fmt"
	"slices"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/stringutil"
)

var (
	// ErrInvalidConfig is returned when the configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrInvalidLength is returned when a length value is invalid.
	ErrInvalidLength = errors.New("invalid length value")

	// ErrInvalidSeverity is returned when a severity value is invalid.
	ErrInvalidSeverity = errors.New("invalid severity value")

	// ErrEmptyValue is returned when a required value is empty.
	ErrEmptyValue = errors.New("empty value not allowed")

	// ErrInvalidOption is returned when an option value is invalid.
	ErrInvalidOption = errors.New("invalid option value")

	// ErrInvalidRule is returned when a rule configuration is invalid.
	ErrInvalidRule = errors.New("invalid rule configuration")

	// ErrEmptyMatchConditions is returned when a rule has no match conditions.
	ErrEmptyMatchConditions = errors.New("rule has no match conditions")
)

// Validator validates configuration semantics.
type Validator struct{}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate validates the entire configuration.
// Returns an error describing all validation failures.
func (v *Validator) Validate(cfg *config.Config) error {
	if cfg == nil {
		return errors.WithMessage(ErrInvalidConfig, "config is nil")
	}

	var validationErrors []error

	// Validate global config
	if cfg.Global != nil {
		if err := v.validateGlobalConfig(cfg.Global); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	// Validate validators config
	if cfg.Validators != nil {
		if err := v.validateValidatorsConfig(cfg.Validators); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	// Validate rules config
	if cfg.Rules != nil {
		if err := v.validateRulesConfig(cfg.Rules); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return errors.WithSecondaryError(
			errors.Wrapf(
				ErrInvalidConfig,
				"validation failed with %d error(s)",
				len(validationErrors),
			),
			combineErrors(validationErrors),
		)
	}

	return nil
}

// validateGlobalConfig validates global configuration.
func (*Validator) validateGlobalConfig(*config.GlobalConfig) error {
	// No specific validation needed for global config currently
	return nil
}

// validateValidatorsConfig validates the validators configuration.
func (v *Validator) validateValidatorsConfig(cfg *config.ValidatorsConfig) error {
	var validationErrors []error

	if cfg.Git != nil {
		if err := v.validateGitConfig(cfg.Git); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if cfg.File != nil {
		if err := v.validateFileConfig(cfg.File); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if cfg.Notification != nil {
		if err := v.validateNotificationConfig(cfg.Notification); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validateGitConfig validates git validators configuration.
func (v *Validator) validateGitConfig(cfg *config.GitConfig) error {
	var validationErrors []error

	if cfg.Commit != nil {
		if err := v.validateCommitConfig(cfg.Commit); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.git.commit"),
			)
		}
	}

	if cfg.Push != nil {
		if err := v.validatePushConfig(cfg.Push); err != nil {
			validationErrors = append(validationErrors, errors.Wrap(err, "validators.git.push"))
		}
	}

	if cfg.Add != nil {
		if err := v.validateAddConfig(cfg.Add); err != nil {
			validationErrors = append(validationErrors, errors.Wrap(err, "validators.git.add"))
		}
	}

	if cfg.PR != nil {
		if err := v.validatePRConfig(cfg.PR); err != nil {
			validationErrors = append(validationErrors, errors.Wrap(err, "validators.git.pr"))
		}
	}

	if cfg.Branch != nil {
		if err := v.validateBranchConfig(cfg.Branch); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.git.branch"),
			)
		}
	}

	if cfg.NoVerify != nil {
		if err := v.validateBaseConfig(&cfg.NoVerify.ValidatorConfig); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.git.no_verify"),
			)
		}
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validateFileConfig validates file validators configuration.
func (v *Validator) validateFileConfig(cfg *config.FileConfig) error {
	var validationErrors []error

	if cfg.Markdown != nil {
		if err := v.validateMarkdownConfig(cfg.Markdown); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.file.markdown"),
			)
		}
	}

	if cfg.ShellScript != nil {
		if err := v.validateShellScriptConfig(cfg.ShellScript); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.file.shellscript"),
			)
		}
	}

	if cfg.Terraform != nil {
		if err := v.validateTerraformConfig(cfg.Terraform); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.file.terraform"),
			)
		}
	}

	if cfg.Workflow != nil {
		if err := v.validateWorkflowConfig(cfg.Workflow); err != nil {
			validationErrors = append(
				validationErrors,
				errors.Wrap(err, "validators.file.workflow"),
			)
		}
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validateNotificationConfig validates notification validators configuration.
func (v *Validator) validateNotificationConfig(cfg *config.NotificationConfig) error {
	if cfg.Bell != nil {
		if err := v.validateBaseConfig(&cfg.Bell.ValidatorConfig); err != nil {
			return errors.Wrap(err, "validators.notification.bell")
		}
	}

	return nil
}

// validateCommitConfig validates commit validator configuration.
func (v *Validator) validateCommitConfig(cfg *config.CommitValidatorConfig) error {
	if err := v.validateBaseConfig(&cfg.ValidatorConfig); err != nil {
		return err
	}

	if cfg.Message != nil {
		if err := v.validateCommitMessageConfig(cfg.Message); err != nil {
			return errors.Wrap(err, "message")
		}
	}

	return nil
}

// validateCommitMessageConfig validates commit message configuration.
func (*Validator) validateCommitMessageConfig(cfg *config.CommitMessageConfig) error {
	var validationErrors []error

	if cfg.TitleMaxLength != nil && *cfg.TitleMaxLength <= 0 {
		validationErrors = append(
			validationErrors,
			errors.Wrapf(
				ErrInvalidLength,
				"title_max_length must be positive, got %d",
				*cfg.TitleMaxLength,
			),
		)
	}

	if cfg.BodyMaxLineLength != nil && *cfg.BodyMaxLineLength <= 0 {
		validationErrors = append(
			validationErrors,
			errors.Wrapf(
				ErrInvalidLength,
				"body_max_line_length must be positive, got %d",
				*cfg.BodyMaxLineLength,
			),
		)
	}

	if cfg.BodyLineTolerance != nil && *cfg.BodyLineTolerance < 0 {
		validationErrors = append(
			validationErrors,
			errors.Wrapf(
				ErrInvalidLength,
				"body_line_tolerance must be non-negative, got %d",
				*cfg.BodyLineTolerance,
			),
		)
	}

	if len(cfg.ValidTypes) > 0 {
		if slices.Contains(cfg.ValidTypes, "") {
			validationErrors = append(
				validationErrors,
				errors.WithMessage(ErrEmptyValue, "valid_types"),
			)
		}
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validatePushConfig validates push validator configuration.
func (v *Validator) validatePushConfig(cfg *config.PushValidatorConfig) error {
	return v.validateBaseConfig(&cfg.ValidatorConfig)
}

// validateAddConfig validates add validator configuration.
func (v *Validator) validateAddConfig(cfg *config.AddValidatorConfig) error {
	return v.validateBaseConfig(&cfg.ValidatorConfig)
}

// validatePRConfig validates PR validator configuration.
func (v *Validator) validatePRConfig(cfg *config.PRValidatorConfig) error {
	if err := v.validateBaseConfig(&cfg.ValidatorConfig); err != nil {
		return err
	}

	var validationErrors []error

	if cfg.TitleMaxLength != nil && *cfg.TitleMaxLength <= 0 {
		validationErrors = append(
			validationErrors,
			errors.Wrapf(
				ErrInvalidLength,
				"title_max_length must be positive, got %d",
				*cfg.TitleMaxLength,
			),
		)
	}

	if len(cfg.ValidTypes) > 0 {
		if slices.Contains(cfg.ValidTypes, "") {
			validationErrors = append(
				validationErrors,
				errors.WithMessage(ErrEmptyValue, "valid_types"),
			)
		}
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validateBranchConfig validates branch validator configuration.
func (v *Validator) validateBranchConfig(cfg *config.BranchValidatorConfig) error {
	if err := v.validateBaseConfig(&cfg.ValidatorConfig); err != nil {
		return err
	}

	if len(cfg.ValidTypes) > 0 {
		if slices.Contains(cfg.ValidTypes, "") {
			return errors.WithMessage(ErrEmptyValue, "valid_types")
		}
	}

	return nil
}

// validateMarkdownConfig validates markdown validator configuration.
func (v *Validator) validateMarkdownConfig(cfg *config.MarkdownValidatorConfig) error {
	if err := v.validateBaseConfig(&cfg.ValidatorConfig); err != nil {
		return err
	}

	if cfg.ContextLines != nil && *cfg.ContextLines < 0 {
		return errors.Wrapf(
			ErrInvalidLength,
			"context_lines must be non-negative, got %d",
			*cfg.ContextLines,
		)
	}

	return nil
}

// validateShellScriptConfig validates shell script validator configuration.
func (v *Validator) validateShellScriptConfig(cfg *config.ShellScriptValidatorConfig) error {
	if err := v.validateBaseConfig(&cfg.ValidatorConfig); err != nil {
		return err
	}

	if cfg.ContextLines != nil && *cfg.ContextLines < 0 {
		return errors.Wrapf(
			ErrInvalidLength,
			"context_lines must be non-negative, got %d",
			*cfg.ContextLines,
		)
	}

	// Validate shellcheck severity
	if cfg.ShellcheckSeverity != "" {
		validSeverities := []string{"error", "warning", "info", "style"}

		valid := slices.Contains(validSeverities, cfg.ShellcheckSeverity)

		if !valid {
			return errors.Wrapf(
				ErrInvalidOption,
				"shellcheck_severity must be one of %v, got %q",
				validSeverities,
				cfg.ShellcheckSeverity,
			)
		}
	}

	return nil
}

// validateTerraformConfig validates terraform validator configuration.
func (v *Validator) validateTerraformConfig(cfg *config.TerraformValidatorConfig) error {
	if err := v.validateBaseConfig(&cfg.ValidatorConfig); err != nil {
		return err
	}

	if cfg.ContextLines != nil && *cfg.ContextLines < 0 {
		return errors.Wrapf(
			ErrInvalidLength,
			"context_lines must be non-negative, got %d",
			*cfg.ContextLines,
		)
	}

	// Validate tool preference
	if cfg.ToolPreference != "" {
		validPreferences := []string{"tofu", "terraform", "auto"}

		valid := slices.Contains(validPreferences, cfg.ToolPreference)

		if !valid {
			return errors.Wrapf(
				ErrInvalidOption,
				"tool_preference must be one of %v, got %q",
				validPreferences,
				cfg.ToolPreference,
			)
		}
	}

	return nil
}

// validateWorkflowConfig validates workflow validator configuration.
func (v *Validator) validateWorkflowConfig(cfg *config.WorkflowValidatorConfig) error {
	return v.validateBaseConfig(&cfg.ValidatorConfig)
}

// validateBaseConfig validates the base validator configuration.
func (*Validator) validateBaseConfig(cfg *config.ValidatorConfig) error {
	if cfg.Severity != config.SeverityUnknown && !cfg.Severity.IsASeverity() {
		return errors.Wrapf(
			ErrInvalidSeverity,
			"must be %q or %q, got %q",
			config.SeverityError.String(),
			config.SeverityWarning.String(),
			cfg.Severity.String(),
		)
	}

	return nil
}

// validateRulesConfig validates the rules configuration.
func (v *Validator) validateRulesConfig(cfg *config.RulesConfig) error {
	if cfg == nil || len(cfg.Rules) == 0 {
		return nil
	}

	var validationErrors []error

	for i := range cfg.Rules {
		// Skip validation for disabled rules
		if !cfg.Rules[i].IsRuleEnabled() {
			continue
		}

		ruleID := v.getRuleIdentifier(cfg.Rules[i], i)

		if err := v.validateRule(&cfg.Rules[i], ruleID); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return errors.Wrap(combineErrors(validationErrors), "rules")
	}

	return nil
}

// getRuleIdentifier returns a human-readable identifier for a rule.
func (*Validator) getRuleIdentifier(rule config.RuleConfig, index int) string {
	if rule.Name != "" {
		return fmt.Sprintf("rule[%q]", rule.Name)
	}

	return fmt.Sprintf("rule[%d]", index)
}

// validateRule validates a single rule configuration.
func (v *Validator) validateRule(rule *config.RuleConfig, ruleID string) error {
	var validationErrors []error

	// Validate match conditions exist
	if err := v.validateRuleMatchConditions(rule.Match, ruleID); err != nil {
		validationErrors = append(validationErrors, err)
	}

	// Validate match field values
	if rule.Match != nil {
		if err := v.validateRuleMatchFields(rule.Match, ruleID); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	// Validate action
	if err := v.validateRuleAction(rule.Action, ruleID); err != nil {
		validationErrors = append(validationErrors, err)
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validateRuleMatchConditions validates that a rule has at least one match condition.
func (*Validator) validateRuleMatchConditions(match *config.RuleMatchConfig, ruleID string) error {
	if match == nil {
		return errors.Wrapf(ErrEmptyMatchConditions, "%s has no match section", ruleID)
	}

	// Use centralized method on RuleMatchConfig
	if !match.HasMatchConditions() {
		return errors.Wrapf(
			ErrEmptyMatchConditions,
			"%s has empty match section (rule will never match)",
			ruleID,
		)
	}

	return nil
}

// validateRuleMatchFields validates the field values in a rule's match section.
func (*Validator) validateRuleMatchFields(match *config.RuleMatchConfig, ruleID string) error {
	var validationErrors []error

	// Validate event_type if specified
	if match.EventType != "" {
		if !stringutil.ContainsCaseInsensitive(config.ValidEventTypes, match.EventType) {
			validationErrors = append(
				validationErrors,
				errors.Wrapf(
					ErrInvalidRule,
					"%s has invalid event_type %q (valid: %v)",
					ruleID,
					match.EventType,
					config.ValidEventTypes,
				),
			)
		}
	}

	// Validate tool_type if specified
	if match.ToolType != "" {
		if !stringutil.ContainsCaseInsensitive(config.ValidToolTypes, match.ToolType) {
			validationErrors = append(
				validationErrors,
				errors.Wrapf(
					ErrInvalidRule,
					"%s has invalid tool_type %q (valid: %v)",
					ruleID,
					match.ToolType,
					config.ValidToolTypes,
				),
			)
		}
	}

	if len(validationErrors) > 0 {
		return combineErrors(validationErrors)
	}

	return nil
}

// validateRuleAction validates a rule's action configuration.
func (*Validator) validateRuleAction(action *config.RuleActionConfig, ruleID string) error {
	if action == nil {
		// No action is valid - defaults to "block"
		return nil
	}

	// Validate action type if specified
	if action.Type != "" && !slices.Contains(config.ValidActionTypes, action.Type) {
		return errors.Wrapf(
			ErrInvalidRule,
			"%s has invalid action type %q (valid: %v)",
			ruleID,
			action.Type,
			config.ValidActionTypes,
		)
	}

	return nil
}

// combineErrors combines multiple errors into a single error.
func combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}

	if len(errs) == 1 {
		return errs[0]
	}

	return errors.Join(errs...)
}
