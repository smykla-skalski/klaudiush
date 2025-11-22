// Package dispatcher orchestrates validation of hook contexts.
package dispatcher

import (
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

var (
	// ErrValidationFailed is returned when one or more validators fail.
	ErrValidationFailed = errors.New("validation failed")

	// ErrNoValidators is returned when no validators match the context.
	ErrNoValidators = errors.New("no validators found")
)

// ValidationError represents a validation failure.
type ValidationError struct {
	// Validator is the name of the validator that failed.
	Validator string

	// Message is the error message.
	Message string

	// Details contains additional error details.
	Details map[string]string

	// ShouldBlock indicates whether this error should block the operation.
	ShouldBlock bool
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Validator, e.Message)
	}

	return e.Validator
}

// Dispatcher orchestrates validation of hook contexts.
type Dispatcher struct {
	registry *validator.Registry
	logger   logger.Logger
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(registry *validator.Registry, logger logger.Logger) *Dispatcher {
	return &Dispatcher{
		registry: registry,
		logger:   logger,
	}
}

// Dispatch validates the context using all matching validators.
// Returns a slice of validation errors (empty if all pass).
func (d *Dispatcher) Dispatch(ctx *hook.Context) []*ValidationError {
	d.logger.Info("dispatching",
		"event", ctx.EventType,
		"tool", ctx.ToolName,
	)

	validators := d.registry.FindValidators(ctx)

	if len(validators) == 0 {
		d.logger.Info("no validators found",
			"event", ctx.EventType,
			"tool", ctx.ToolName,
		)

		return nil
	}

	d.logger.Info("validators found",
		"count", len(validators),
	)

	validationErrors := make([]*ValidationError, 0, len(validators))

	for _, v := range validators {
		d.logger.Debug("running validator",
			"validator", v.Name(),
		)

		result := v.Validate(ctx)

		if result.Passed {
			d.logger.Info("validator passed",
				"validator", v.Name(),
			)

			continue
		}

		// Log based on whether it blocks
		if result.ShouldBlock {
			d.logger.Error("validator failed",
				"validator", v.Name(),
				"message", result.Message,
			)
		} else {
			d.logger.Info("validator warned",
				"validator", v.Name(),
				"message", result.Message,
			)
		}

		validationErrors = append(validationErrors, &ValidationError{
			Validator:   v.Name(),
			Message:     result.Message,
			Details:     result.Details,
			ShouldBlock: result.ShouldBlock,
		})
	}

	return validationErrors
}

// ShouldBlock returns true if any validation error should block the operation.
func ShouldBlock(errors []*ValidationError) bool {
	for _, err := range errors {
		if err.ShouldBlock {
			return true
		}
	}

	return false
}

// categorizeErrors separates validation errors into blocking errors and warnings.
func categorizeErrors(errors []*ValidationError) (blocking, warnings []*ValidationError) {
	blockingErrors := make([]*ValidationError, 0)
	warningErrors := make([]*ValidationError, 0)

	for _, err := range errors {
		if err.ShouldBlock {
			blockingErrors = append(blockingErrors, err)
		} else {
			warningErrors = append(warningErrors, err)
		}
	}

	return blockingErrors, warningErrors
}

// formatErrorList formats a list of errors with a header.
func formatErrorList(header string, errors []*ValidationError) string {
	if len(errors) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(header)
	builder.WriteString("\n\n")

	for _, err := range errors {
		builder.WriteString(fmt.Sprintf("  %s\n", err.Message))

		if len(err.Details) > 0 {
			for k, v := range err.Details {
				builder.WriteString(fmt.Sprintf("    %s: %s\n", k, v))
			}
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

// FormatErrors formats validation errors for display.
func FormatErrors(errors []*ValidationError) string {
	if len(errors) == 0 {
		return ""
	}

	blockingErrors, warnings := categorizeErrors(errors)

	result := formatErrorList("❌ Validation Failed:", blockingErrors)
	result += formatErrorList("⚠️  Warnings:", warnings)

	return result
}
