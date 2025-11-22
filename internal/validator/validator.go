package validator

import (
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

// Validator validates a hook context.
type Validator interface {
	// Name returns the validator name.
	Name() string

	// Validate validates the given context and returns a result.
	Validate(ctx *hook.Context) *Result
}

// Result represents the validation result.
type Result struct {
	// Passed indicates whether the validation passed.
	Passed bool

	// Message is the human-readable message.
	Message string

	// Details contains additional details about the validation.
	Details map[string]string

	// ShouldBlock indicates whether this failure should block the operation.
	// Some validators may only warn without blocking.
	ShouldBlock bool
}

// Pass creates a passing validation result.
func Pass() *Result {
	return &Result{
		Passed:      true,
		ShouldBlock: false,
	}
}

// PassWithMessage creates a passing validation result with a message.
func PassWithMessage(message string) *Result {
	return &Result{
		Passed:      true,
		Message:     message,
		ShouldBlock: false,
	}
}

// Fail creates a failing validation result that blocks the operation.
func Fail(message string) *Result {
	return &Result{
		Passed:      false,
		Message:     message,
		ShouldBlock: true,
	}
}

// FailWithDetails creates a failing validation result with details.
func FailWithDetails(message string, details map[string]string) *Result {
	return &Result{
		Passed:      false,
		Message:     message,
		Details:     details,
		ShouldBlock: true,
	}
}

// Warn creates a failing validation result that only warns without blocking.
func Warn(message string) *Result {
	return &Result{
		Passed:      false,
		Message:     message,
		ShouldBlock: false,
	}
}

// WarnWithDetails creates a warning validation result with details.
func WarnWithDetails(message string, details map[string]string) *Result {
	return &Result{
		Passed:      false,
		Message:     message,
		Details:     details,
		ShouldBlock: false,
	}
}

// AddDetail adds a detail to the result.
func (r *Result) AddDetail(key, value string) *Result {
	if r.Details == nil {
		r.Details = make(map[string]string)
	}

	r.Details[key] = value

	return r
}

// String returns a string representation of the result.
func (r *Result) String() string {
	if r.Passed {
		return "PASS"
	}

	if r.ShouldBlock {
		return "BLOCK"
	}

	return "WARN"
}

// BaseValidator provides common validator functionality.
type BaseValidator struct {
	name   string
	logger logger.Logger
}

// NewBaseValidator creates a new BaseValidator.
func NewBaseValidator(name string, logger logger.Logger) *BaseValidator {
	return &BaseValidator{
		name:   name,
		logger: logger,
	}
}

// Name returns the validator name.
func (v *BaseValidator) Name() string {
	return v.name
}

// Logger returns the logger.
func (v *BaseValidator) Logger() logger.Logger {
	return v.logger
}

// LogValidation logs the validation result.
func (v *BaseValidator) LogValidation(ctx *hook.Context, result *Result) {
	if result.Passed {
		v.logger.Info("validation passed",
			"validator", v.name,
			"tool", ctx.ToolName,
		)

		return
	}

	logMsg := "validation " + result.String()
	kvs := []any{
		"validator", v.name,
		"tool", ctx.ToolName,
		"message", result.Message,
	}

	if len(result.Details) > 0 {
		for k, val := range result.Details {
			kvs = append(kvs, k, val)
		}
	}

	if result.ShouldBlock {
		v.logger.Error(logMsg, kvs...)
	} else {
		v.logger.Info(logMsg, kvs...)
	}
}
