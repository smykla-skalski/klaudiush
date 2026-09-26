package shell

import (
	"context"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// NestingValidator blocks a command that nests launchers, scripts or aliases
// deeper than the parser follows. What such a command finally runs is never
// seen, so it could hide a git command from every other validator.
type NestingValidator struct {
	validator.BaseValidator
}

// NewNestingValidator creates a new NestingValidator instance.
func NewNestingValidator(log logger.Logger) *NestingValidator {
	return &NestingValidator{
		BaseValidator: *validator.NewBaseValidator("validate-nesting", log),
	}
}

// Validate fails when the parsed command was cut off at the nesting limit.
func (*NestingValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	parsed, err := hookCtx.ParsedCommand()
	if err != nil || !parsed.Truncated {
		return validator.Pass()
	}

	return validator.FailWithRef(
		validator.RefShellNesting,
		"Command nests launchers, scripts or aliases too deeply to inspect",
	)
}

// Category returns the validator category for parallel execution.
func (*NestingValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}
