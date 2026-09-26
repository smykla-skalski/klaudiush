package shell

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// NestingValidator blocks a command whose final program the parser cannot
// see: one that does not parse, or one that nests launchers, scripts or
// aliases deeper than the parser follows. Either could hide a git command
// from every other validator.
type NestingValidator struct {
	validator.BaseValidator
}

// NewNestingValidator creates a new NestingValidator instance.
func NewNestingValidator(log logger.Logger) *NestingValidator {
	return &NestingValidator{
		BaseValidator: *validator.NewBaseValidator("validate-nesting", log),
	}
}

// Validate fails when the command could not be parsed or was cut off.
func (*NestingValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	parsed, err := hookCtx.ParsedCommand()

	switch {
	case errors.Is(err, parser.ErrParseFailed):
		return validator.FailWithRef(
			validator.RefShellNesting,
			"Command does not parse as shell, so what it runs cannot be inspected",
		)
	case err != nil || !parsed.Truncated:
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
