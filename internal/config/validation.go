// Package config provides internal configuration loading and processing.
package config

import (
	"slices"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/pkg/config"
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
