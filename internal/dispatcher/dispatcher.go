// Package dispatcher orchestrates validation of hook contexts.
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
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
func (d *Dispatcher) Dispatch(ctx context.Context, hookCtx *hook.Context) []*ValidationError {
	d.logger.Info("dispatching",
		"event", hookCtx.EventType,
		"tool", hookCtx.ToolName,
	)

	// Run validators on the main context
	validationErrors := d.runValidators(ctx, hookCtx)

	// If this is a Bash PreToolUse, also validate synthetic Write contexts for file writes
	if hookCtx.EventType == hook.PreToolUse && hookCtx.ToolName == hook.Bash {
		syntheticErrors := d.validateBashFileWrites(ctx, hookCtx)
		validationErrors = append(validationErrors, syntheticErrors...)
	}

	return validationErrors
}

// runValidators runs validators on a context and returns validation errors.
func (d *Dispatcher) runValidators(ctx context.Context, hookCtx *hook.Context) []*ValidationError {
	validators := d.registry.FindValidators(hookCtx)

	if len(validators) == 0 {
		d.logger.Info("no validators found",
			"event", hookCtx.EventType,
			"tool", hookCtx.ToolName,
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

		result := v.Validate(ctx, hookCtx)

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

// validateBashFileWrites parses Bash commands for file writes and validates them
// as synthetic Write operations.
func (d *Dispatcher) validateBashFileWrites(
	ctx context.Context,
	bashCtx *hook.Context,
) []*ValidationError {
	// Parse the bash command
	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(bashCtx.GetCommand())
	if err != nil {
		d.logger.Debug("failed to parse bash command for file writes",
			"error", err,
		)

		return nil
	}

	// No file writes found
	if len(result.FileWrites) == 0 {
		return nil
	}

	d.logger.Info("detected bash file writes",
		"count", len(result.FileWrites),
	)

	allErrors := make([]*ValidationError, 0)

	// Create synthetic Write context for each file write
	for _, fw := range result.FileWrites {
		syntheticCtx := &hook.Context{
			EventType: hook.PreToolUse,
			ToolName:  hook.Write,
			ToolInput: hook.ToolInput{
				FilePath: fw.Path,
				Content:  fw.Content,
			},
		}

		d.logger.Debug("validating synthetic write context",
			"file", fw.Path,
			"operation", fw.Operation,
		)

		// Run validators on the synthetic context
		errors := d.runValidators(ctx, syntheticCtx)
		allErrors = append(allErrors, errors...)
	}

	return allErrors
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

	// Header with validator names
	builder.WriteString("\n")
	builder.WriteString(header)

	// Add validator names after header
	for _, err := range errors {
		builder.WriteString(" ")
		// Remove "validate-" prefix if present
		validatorName := err.Validator
		validatorName = strings.TrimPrefix(validatorName, "validate-")
		builder.WriteString(validatorName)
	}

	builder.WriteString("\n\n")

	for _, err := range errors {
		// Main message
		builder.WriteString(err.Message)
		builder.WriteString("\n")

		// Details - just the values without the key label
		if len(err.Details) > 0 {
			builder.WriteString("\n")

			for _, v := range err.Details {
				// Handle multi-line detail values
				lines := strings.SplitSeq(strings.TrimSpace(v), "\n")
				for line := range lines {
					if line != "" {
						builder.WriteString(line)
						builder.WriteString("\n")
					}
				}
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
