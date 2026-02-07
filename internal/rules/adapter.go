package rules

import (
	"context"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// RuleValidatorAdapter adapts the rule engine for use in validators.
// It provides a simple interface for validators to check rules.
type RuleValidatorAdapter struct {
	engine        *RuleEngine
	validatorType ValidatorType
	logger        logger.Logger

	// GitContextProvider is called to get git context for rule matching.
	// This is optional and allows validators to provide git-specific data.
	GitContextProvider func() *GitContext

	// FileContextProvider is called to get file context for rule matching.
	// This is optional and allows validators to provide file-specific data.
	FileContextProvider func() *FileContext
}

// AdapterOption configures a RuleValidatorAdapter.
type AdapterOption func(*RuleValidatorAdapter)

// WithAdapterLogger sets the logger for the adapter.
func WithAdapterLogger(log logger.Logger) AdapterOption {
	return func(a *RuleValidatorAdapter) {
		a.logger = log
	}
}

// WithGitContextProvider sets the git context provider.
func WithGitContextProvider(provider func() *GitContext) AdapterOption {
	return func(a *RuleValidatorAdapter) {
		a.GitContextProvider = provider
	}
}

// WithFileContextProvider sets the file context provider.
func WithFileContextProvider(provider func() *FileContext) AdapterOption {
	return func(a *RuleValidatorAdapter) {
		a.FileContextProvider = provider
	}
}

// NewRuleValidatorAdapter creates a new adapter for the given engine and validator type.
func NewRuleValidatorAdapter(
	engine *RuleEngine,
	validatorType ValidatorType,
	opts ...AdapterOption,
) *RuleValidatorAdapter {
	adapter := &RuleValidatorAdapter{
		engine:        engine,
		validatorType: validatorType,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	if adapter.logger == nil {
		adapter.logger = logger.NewNoOpLogger()
	}

	return adapter
}

// CheckRules evaluates rules for the given hook context.
// Returns a validator.Result if a rule matched, or nil to continue with built-in logic.
func (a *RuleValidatorAdapter) CheckRules(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	if a.engine == nil {
		return nil
	}

	// Build match context.
	matchCtx := &MatchContext{
		HookContext:   hookCtx,
		ValidatorType: a.validatorType,
	}

	if hookCtx != nil {
		matchCtx.Command = hookCtx.GetCommand()
	}

	// Get git context if provider is set.
	if a.GitContextProvider != nil {
		matchCtx.GitContext = a.GitContextProvider()
	}

	// Get file context if provider is set.
	if a.FileContextProvider != nil {
		matchCtx.FileContext = a.FileContextProvider()
	}

	// Evaluate rules.
	result := a.engine.Evaluate(ctx, matchCtx)

	// If no rule matched, return nil to continue with built-in logic.
	if !result.Matched {
		return nil
	}

	// Convert rule result to validator result.
	return a.convertResult(result)
}

// CheckRulesWithContext evaluates rules with explicit git and file context.
// This is useful when the validator already has the context available.
func (a *RuleValidatorAdapter) CheckRulesWithContext(
	ctx context.Context,
	hookCtx *hook.Context,
	gitCtx *GitContext,
	fileCtx *FileContext,
) *validator.Result {
	if a.engine == nil {
		return nil
	}

	// Build match context.
	matchCtx := &MatchContext{
		HookContext:   hookCtx,
		GitContext:    gitCtx,
		FileContext:   fileCtx,
		ValidatorType: a.validatorType,
	}

	if hookCtx != nil {
		matchCtx.Command = hookCtx.GetCommand()
	}

	// Evaluate rules.
	result := a.engine.Evaluate(ctx, matchCtx)

	// If no rule matched, return nil to continue with built-in logic.
	if !result.Matched {
		return nil
	}

	// Convert rule result to validator result.
	return a.convertResult(result)
}

// convertResult converts a RuleResult to a validator.Result.
func (*RuleValidatorAdapter) convertResult(result *RuleResult) *validator.Result {
	switch result.Action {
	case ActionBlock:
		if result.Reference != "" {
			return validator.FailWithRef(
				validator.Reference(result.Reference),
				result.Message,
			)
		}

		return validator.Fail(result.Message)

	case ActionWarn:
		if result.Reference != "" {
			return validator.WarnWithRef(
				validator.Reference(result.Reference),
				result.Message,
			)
		}

		return validator.Warn(result.Message)

	case ActionAllow:
		return validator.Pass()

	default:
		return nil
	}
}

// HasRulesForValidator returns true if there are any rules for this validator type.
func (a *RuleValidatorAdapter) HasRulesForValidator() bool {
	if a.engine == nil {
		return false
	}

	rules := a.engine.FilterByValidator(a.validatorType)

	return len(rules) > 0
}

// GetApplicableRules returns all rules that apply to this validator type.
func (a *RuleValidatorAdapter) GetApplicableRules() []*Rule {
	if a.engine == nil {
		return nil
	}

	return a.engine.FilterByValidator(a.validatorType)
}

// Verify interface compliance.
var _ ValidatorAdapter = (*RuleValidatorAdapter)(nil)
