package factory

import (
	"github.com/smykla-skalski/klaudiush/internal/validator"
	policyvalidators "github.com/smykla-skalski/klaudiush/internal/validators/policy"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// PolicyValidatorFactory creates the protection and MCP trust validators.
// Neither consults the rule engine: a rule is not the explicit
// protection.allow or mcp_trust entry these checks require.
type PolicyValidatorFactory struct {
	log     logger.Logger
	locator func(cfg *config.Config) policyvalidators.Locator
}

// NewPolicyValidatorFactory creates a PolicyValidatorFactory that locates
// policy files on the running system.
func NewPolicyValidatorFactory(log logger.Logger) *PolicyValidatorFactory {
	return &PolicyValidatorFactory{log: log, locator: policyvalidators.NewLocator}
}

// CreateValidators creates the enabled policy validators.
func (f *PolicyValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	var validators []ValidatorWithPredicate

	if cfg.Protection.IsEnabled() && !isValidatorOverridden(cfg.Overrides, "policy.protection") {
		validators = append(validators, ValidatorWithPredicate{
			Validator: policyvalidators.NewProtectionValidator(
				f.log,
				cfg.Protection,
				f.locator(cfg),
			),
			Predicate: protectionPredicate(),
		})
	}

	if cfg.MCPTrust.IsEnabled() && !isValidatorOverridden(cfg.Overrides, "policy.mcp_trust") {
		validators = append(validators, ValidatorWithPredicate{
			Validator: policyvalidators.NewMCPTrustValidator(f.log, cfg.MCPTrust),
			Predicate: validator.And(
				validator.EventIs(hook.CanonicalEventBeforeTool),
				func(ctx *hook.Context) bool { return ctx.IsMCPTool() },
			),
		})
	}

	return validators
}

// protectionPredicate selects every pre-tool call, Claude ConfigChange, and
// after-tool events that list files a shell command changed.
func protectionPredicate() validator.Predicate {
	return validator.Or(
		validator.EventIs(hook.CanonicalEventBeforeTool),
		validator.EventIs(hook.CanonicalEventConfigChange),
		validator.And(
			validator.EventIs(hook.CanonicalEventAfterTool),
			func(ctx *hook.Context) bool { return len(ctx.ChangedFiles) > 0 },
		),
	)
}
