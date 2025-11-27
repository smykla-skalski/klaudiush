package factory

import (
	"regexp"
	"time"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/secrets"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// SecretsValidatorFactory creates secrets validators from configuration.
type SecretsValidatorFactory struct {
	log logger.Logger
}

// NewSecretsValidatorFactory creates a new SecretsValidatorFactory.
func NewSecretsValidatorFactory(log logger.Logger) *SecretsValidatorFactory {
	return &SecretsValidatorFactory{log: log}
}

// CreateValidators creates all secrets validators based on configuration.
func (f *SecretsValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	// Get secrets config with nil safety
	secretsCfg := f.getSecretsConfig(cfg)
	if secretsCfg == nil || !secretsCfg.IsEnabled() {
		return validators
	}

	// Determine timeout from config or use default
	timeout := DefaultLinterTimeout
	if cfg.Global != nil && cfg.Global.DefaultTimeout.ToDuration() > 0 {
		timeout = cfg.Global.DefaultTimeout.ToDuration()
	}

	// Create detector with patterns
	detector := f.createDetector(secretsCfg)

	// Create gitleaks checker
	gitleaks := f.createGitleaksChecker(timeout)

	validators = append(validators, ValidatorWithPredicate{
		Validator: secrets.NewSecretsValidator(f.log, detector, gitleaks, secretsCfg),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIn(hook.ToolTypeWrite, hook.ToolTypeEdit),
		),
	})

	return validators
}

// getSecretsConfig safely retrieves the secrets validator config.
func (*SecretsValidatorFactory) getSecretsConfig(
	cfg *config.Config,
) *config.SecretsValidatorConfig {
	if cfg == nil || cfg.Validators == nil || cfg.Validators.Secrets == nil {
		return nil
	}

	return cfg.Validators.Secrets.Secrets
}

// createDetector creates a pattern detector with custom patterns if configured.
//
//nolint:ireturn // interface for polymorphism
func (f *SecretsValidatorFactory) createDetector(
	cfg *config.SecretsValidatorConfig,
) secrets.Detector {
	detector := secrets.NewDefaultPatternDetector()

	// Add custom patterns from config
	if cfg != nil && len(cfg.CustomPatterns) > 0 {
		customPatterns := f.buildCustomPatterns(cfg.CustomPatterns)
		detector.AddPatterns(customPatterns...)
	}

	return detector
}

// buildCustomPatterns converts config patterns to detector patterns.
func (f *SecretsValidatorFactory) buildCustomPatterns(
	cfgPatterns []config.CustomPatternConfig,
) []secrets.Pattern {
	patterns := make([]secrets.Pattern, 0, len(cfgPatterns))

	for _, cp := range cfgPatterns {
		re, err := regexp.Compile(cp.Regex)
		if err != nil {
			f.log.Error("invalid custom pattern regex",
				"name", cp.Name,
				"regex", cp.Regex,
				"error", err)

			continue
		}

		patterns = append(patterns, secrets.Pattern{
			Name:        cp.Name,
			Description: cp.Description,
			Regex:       re,
			Reference:   validator.RefSecretsAPIKey, // default reference for custom patterns
		})
	}

	return patterns
}

// createGitleaksChecker creates a gitleaks checker.
//
//nolint:ireturn // interface for polymorphism
func (*SecretsValidatorFactory) createGitleaksChecker(
	timeout time.Duration,
) linters.GitleaksChecker {
	runner := execpkg.NewCommandRunner(timeout)

	return linters.NewGitleaksChecker(runner)
}
