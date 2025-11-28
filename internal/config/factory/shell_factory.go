package factory

import (
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	shellvalidators "github.com/smykla-labs/klaudiush/internal/validators/shell"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// ShellValidatorFactory creates shell validators from configuration.
type ShellValidatorFactory struct {
	cfg        *config.Config
	log        logger.Logger
	ruleEngine *rules.RuleEngine
}

// NewShellValidatorFactory creates a new ShellValidatorFactory.
func NewShellValidatorFactory(log logger.Logger) *ShellValidatorFactory {
	return &ShellValidatorFactory{log: log}
}

// SetRuleEngine sets the rule engine for the factory.
func (f *ShellValidatorFactory) SetRuleEngine(engine *rules.RuleEngine) {
	f.ruleEngine = engine
}

// CreateValidators creates all shell validators based on configuration.
func (f *ShellValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	f.cfg = cfg // Store config for use in create methods

	var validators []ValidatorWithPredicate

	// Check if Shell config exists
	if cfg.Validators.Shell == nil {
		return validators
	}

	if cfg.Validators.Shell.Backtick != nil && cfg.Validators.Shell.Backtick.IsEnabled() {
		validators = append(validators, f.createBacktickValidator(cfg.Validators.Shell.Backtick))
	}

	return validators
}

func (f *ShellValidatorFactory) createBacktickValidator(
	cfg *config.BacktickValidatorConfig,
) ValidatorWithPredicate {
	var ruleAdapter *rules.RuleValidatorAdapter
	if f.ruleEngine != nil {
		ruleAdapter = rules.NewRuleValidatorAdapter(
			f.ruleEngine,
			rules.ValidatorShellBacktick,
			rules.WithAdapterLogger(f.log),
		)
	}

	return ValidatorWithPredicate{
		Validator: shellvalidators.NewBacktickValidator(f.log, cfg, ruleAdapter),
		Predicate: validator.And(
			validator.EventTypeIs(hook.EventTypePreToolUse),
			validator.ToolTypeIs(hook.ToolTypeBash),
			validator.Or(
				// git commit with -m or --message
				validator.CommandContains("git commit"),
				// gh pr create
				validator.CommandContains("gh pr create"),
				// gh issue create
				validator.CommandContains("gh issue create"),
			),
		),
	}
}
