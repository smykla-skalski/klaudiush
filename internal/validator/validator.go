package validator

//go:generate mockgen -source=validator.go -destination=validator_mock.go -package=validator

import (
	"context"

	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// ValidatorCategory represents the type of workload a validator performs.
// Used to select the appropriate worker pool for parallel execution.
type ValidatorCategory int

const (
	// CategoryCPU is for pure computation validators (regex, parsing).
	// These are lightweight and CPU-bound.
	CategoryCPU ValidatorCategory = iota

	// CategoryIO is for validators that invoke external processes (shellcheck, terraform, tflint).
	// These are I/O-bound and benefit from higher concurrency.
	CategoryIO

	// CategoryGit is for validators that perform git operations.
	// These should be serialized to avoid index lock contention.
	CategoryGit
)

// String returns a string representation of the category.
func (c ValidatorCategory) String() string {
	switch c {
	case CategoryCPU:
		return "CPU"
	case CategoryIO:
		return "IO"
	case CategoryGit:
		return "Git"
	default:
		return "Unknown"
	}
}

// Validator validates a hook context.
type Validator interface {
	// Name returns the validator name.
	Name() string

	// Validate validates the given context and returns a result.
	Validate(ctx context.Context, hookCtx *hook.Context) *Result

	// Category returns the validator's workload category for parallel execution.
	// Default: CategoryCPU
	Category() ValidatorCategory
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

	// Reference is the URL that uniquely identifies this error type.
	// Format: https://klaudiu.sh/{CODE} (e.g., https://klaudiu.sh/GIT001).
	Reference Reference

	// FixHint provides a short suggestion for fixing the issue.
	FixHint string
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

// FailWithRef creates a failing validation result with a reference URL.
// Automatically populates FixHint from the suggestions registry.
func FailWithRef(ref Reference, message string) *Result {
	return &Result{
		Passed:      false,
		Message:     message,
		ShouldBlock: true,
		Reference:   ref,
		FixHint:     GetSuggestion(ref),
	}
}

// WarnWithRef creates a warning validation result with a reference URL.
// Automatically populates FixHint from the suggestions registry.
func WarnWithRef(ref Reference, message string) *Result {
	return &Result{
		Passed:      false,
		Message:     message,
		ShouldBlock: false,
		Reference:   ref,
		FixHint:     GetSuggestion(ref),
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
//
//nolint:ireturn // interface for polymorphism
func (v *BaseValidator) Logger() logger.Logger {
	return v.logger
}

// Category returns the default category (CPU) for validators.
// Validators that perform I/O or Git operations should override this.
func (*BaseValidator) Category() ValidatorCategory {
	return CategoryCPU
}

// LogValidation logs the validation result.
func (v *BaseValidator) LogValidation(hookCtx *hook.Context, result *Result) {
	if result.Passed {
		v.logger.Info("validation passed",
			"validator", v.name,
			"tool", hookCtx.ToolName,
		)

		return
	}

	logMsg := "validation " + result.String()
	kvs := []any{
		"validator", v.name,
		"tool", hookCtx.ToolName,
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
