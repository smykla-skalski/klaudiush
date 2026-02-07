package rules

import (
	"context"

	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// RuleEngine is the main implementation of the Engine interface.
type RuleEngine struct {
	registry  *Registry
	evaluator *Evaluator
	logger    logger.Logger

	// Configuration options.
	stopOnFirstMatch bool
	defaultAction    ActionType
}

// EngineOption configures a RuleEngine.
type EngineOption func(*RuleEngine)

// WithLogger sets the logger for the engine.
func WithLogger(log logger.Logger) EngineOption {
	return func(e *RuleEngine) {
		e.logger = log
	}
}

// WithEngineStopOnFirstMatch configures the engine to stop after the first match.
func WithEngineStopOnFirstMatch(stop bool) EngineOption {
	return func(e *RuleEngine) {
		e.stopOnFirstMatch = stop
	}
}

// WithEngineDefaultAction sets the default action when no rules match.
func WithEngineDefaultAction(action ActionType) EngineOption {
	return func(e *RuleEngine) {
		e.defaultAction = action
	}
}

// NewRuleEngine creates a new RuleEngine with the given rules.
func NewRuleEngine(rules []*Rule, opts ...EngineOption) (*RuleEngine, error) {
	engine := &RuleEngine{
		registry:         NewRegistry(),
		stopOnFirstMatch: true,
		defaultAction:    ActionAllow,
	}

	// Apply options.
	for _, opt := range opts {
		opt(engine)
	}

	// Set up logger.
	if engine.logger == nil {
		engine.logger = logger.NewNoOpLogger()
	}

	// Add rules to registry.
	if err := engine.registry.AddAll(rules); err != nil {
		return nil, err
	}

	// Create evaluator.
	engine.evaluator = NewEvaluator(
		engine.registry,
		WithStopOnFirstMatch(engine.stopOnFirstMatch),
		WithDefaultAction(engine.defaultAction),
	)

	return engine, nil
}

// Evaluate evaluates rules against the given match context.
func (e *RuleEngine) Evaluate(_ context.Context, matchCtx *MatchContext) *RuleResult {
	result := e.evaluator.Evaluate(matchCtx)

	if result.Matched {
		e.logger.Debug("rule matched",
			"rule", result.Rule.Name,
			"action", result.Action,
			"validator", matchCtx.ValidatorType,
		)
	}

	return result
}

// EvaluateHook evaluates rules for a hook context with additional git/file context.
// This is a convenience method that builds the match context from hook context.
func (e *RuleEngine) EvaluateHook(
	ctx context.Context,
	hookCtx *hook.Context,
	validatorType ValidatorType,
	gitCtx *GitContext,
	fileCtx *FileContext,
) *RuleResult {
	matchCtx := &MatchContext{
		HookContext:   hookCtx,
		GitContext:    gitCtx,
		FileContext:   fileCtx,
		ValidatorType: validatorType,
	}

	if hookCtx != nil {
		matchCtx.Command = hookCtx.GetCommand()
	}

	return e.Evaluate(ctx, matchCtx)
}

// AddRule adds a rule to the engine.
func (e *RuleEngine) AddRule(rule *Rule) error {
	return e.registry.Add(rule)
}

// RemoveRule removes a rule by name.
func (e *RuleEngine) RemoveRule(name string) bool {
	return e.registry.Remove(name)
}

// GetRule returns a rule by name.
func (e *RuleEngine) GetRule(name string) *Rule {
	compiled := e.registry.Get(name)
	if compiled == nil {
		return nil
	}

	return compiled.Rule
}

// GetAllRules returns all rules.
func (e *RuleEngine) GetAllRules() []*Rule {
	compiled := e.registry.GetAll()
	rules := make([]*Rule, len(compiled))

	for i, c := range compiled {
		rules[i] = c.Rule
	}

	return rules
}

// GetEnabledRules returns all enabled rules.
func (e *RuleEngine) GetEnabledRules() []*Rule {
	compiled := e.registry.GetEnabled()
	rules := make([]*Rule, len(compiled))

	for i, c := range compiled {
		rules[i] = c.Rule
	}

	return rules
}

// Size returns the number of rules.
func (e *RuleEngine) Size() int {
	return e.registry.Size()
}

// FilterByValidator returns rules that apply to the given validator type.
func (e *RuleEngine) FilterByValidator(validatorType ValidatorType) []*Rule {
	compiled := e.evaluator.FilterByValidator(validatorType)
	rules := make([]*Rule, len(compiled))

	for i, c := range compiled {
		rules[i] = c.Rule
	}

	return rules
}

// Merge combines rules from another engine into this one.
func (e *RuleEngine) Merge(other *RuleEngine) {
	if other == nil {
		return
	}

	e.registry.Merge(other.registry)
}

// Verify interface compliance.
var _ Engine = (*RuleEngine)(nil)
