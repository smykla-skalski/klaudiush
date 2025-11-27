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

	// Reference is the URL that uniquely identifies this error type.
	// Format: https://klaudiu.sh/{CODE} (e.g., https://klaudiu.sh/GIT001).
	Reference validator.Reference

	// FixHint provides a short suggestion for fixing the issue.
	FixHint string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Validator, e.Message)
	}

	return e.Validator
}

// shortName returns the validator name without the "validate-" prefix.
func shortName(name string) string {
	return strings.TrimPrefix(name, "validate-")
}

// Dispatcher orchestrates validation of hook contexts.
type Dispatcher struct {
	registry *validator.Registry
	logger   logger.Logger
	executor Executor
}

// NewDispatcher creates a new Dispatcher with sequential execution.
func NewDispatcher(registry *validator.Registry, logger logger.Logger) *Dispatcher {
	return &Dispatcher{
		registry: registry,
		logger:   logger,
		executor: NewSequentialExecutor(logger),
	}
}

// NewDispatcherWithExecutor creates a new Dispatcher with a custom executor.
func NewDispatcherWithExecutor(
	registry *validator.Registry,
	logger logger.Logger,
	executor Executor,
) *Dispatcher {
	return &Dispatcher{
		registry: registry,
		logger:   logger,
		executor: executor,
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
	if hookCtx.EventType == hook.EventTypePreToolUse && hookCtx.ToolName == hook.ToolTypeBash {
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

	// Use executor to run validators (sequential or parallel)
	validationErrors := d.executor.Execute(ctx, hookCtx, validators)

	// Log results
	for _, verr := range validationErrors {
		name := shortName(verr.Validator)

		if verr.ShouldBlock {
			d.logger.Error("validator failed",
				"validator", name,
				"message", verr.Message,
			)
		} else {
			d.logger.Info("validator warned",
				"validator", name,
				"message", verr.Message,
			)
		}
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
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
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

	formatErrorHeader(&builder, header, errors)

	for _, err := range errors {
		formatSingleError(&builder, err)
	}

	return builder.String()
}

// formatErrorHeader writes the header with validator names.
func formatErrorHeader(builder *strings.Builder, header string, errors []*ValidationError) {
	builder.WriteString("\n")
	builder.WriteString(header)

	for _, err := range errors {
		builder.WriteString(" ")
		builder.WriteString(shortName(err.Validator))
	}

	builder.WriteString("\n\n")
}

// formatSingleError writes a single validation error to the builder.
func formatSingleError(builder *strings.Builder, err *ValidationError) {
	builder.WriteString(err.Message)
	builder.WriteString("\n")

	formatErrorMetadata(builder, err)
	formatErrorDetails(builder, err.Details)

	builder.WriteString("\n")
}

// formatErrorMetadata writes fix hint and reference if present.
func formatErrorMetadata(builder *strings.Builder, err *ValidationError) {
	if err.FixHint != "" {
		builder.WriteString("   Fix: ")
		builder.WriteString(err.FixHint)
		builder.WriteString("\n")
	}

	if err.Reference != "" {
		builder.WriteString("   Reference: ")
		builder.WriteString(string(err.Reference))
		builder.WriteString("\n")
	}
}

// formatErrorDetails writes error details to the builder.
func formatErrorDetails(builder *strings.Builder, details map[string]string) {
	if len(details) == 0 {
		return
	}

	builder.WriteString("\n")

	for _, v := range details {
		lines := strings.SplitSeq(strings.TrimSpace(v), "\n")
		for line := range lines {
			if line != "" {
				builder.WriteString(line)
				builder.WriteString("\n")
			}
		}
	}
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
