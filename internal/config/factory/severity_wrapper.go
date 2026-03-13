package factory

import (
	"context"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

type severityConfig interface {
	GetSeverity() config.Severity
}

func wrapValidatorWithSeverity(
	base validator.Validator,
	cfg severityConfig,
) validator.Validator {
	if base == nil || cfg == nil {
		return base
	}

	severity := cfg.GetSeverity()
	if severity.ShouldBlock() {
		return base
	}

	return &severityWrappedValidator{
		Validator: base,
		severity:  severity,
	}
}

type severityWrappedValidator struct {
	validator.Validator
	severity config.Severity
}

func (v *severityWrappedValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	result := v.Validator.Validate(ctx, hookCtx)
	if result == nil || result.Passed || !result.ShouldBlock {
		return result
	}

	if v.severity.ShouldBlock() {
		return result
	}

	cloned := *result
	cloned.ShouldBlock = false

	return &cloned
}
