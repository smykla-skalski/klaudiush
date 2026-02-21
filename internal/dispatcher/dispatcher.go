// Package dispatcher orchestrates validation of hook contexts.
package dispatcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
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
	// Format: https://klaudiu.sh/e/{CODE} (e.g., https://klaudiu.sh/e/GIT001).
	Reference validator.Reference

	// FixHint provides a short suggestion for fixing the issue.
	FixHint string

	// Bypassed indicates this error was bypassed via an exception token.
	// When true, ShouldBlock is false (converted to warning).
	Bypassed bool

	// BypassReason is the justification from the exception token.
	BypassReason string
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
	registry         *validator.Registry
	logger           logger.Logger
	executor         Executor
	exceptionChecker ExceptionChecker
	overrides        *config.OverridesConfig
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

// DispatcherOption configures a Dispatcher.
type DispatcherOption func(*Dispatcher)

// WithExceptionChecker sets the exception checker for the dispatcher.
func WithExceptionChecker(checker ExceptionChecker) DispatcherOption {
	return func(d *Dispatcher) {
		if checker != nil {
			d.exceptionChecker = checker
		}
	}
}

// WithOverrides sets the overrides config for the dispatcher.
func WithOverrides(overrides *config.OverridesConfig) DispatcherOption {
	return func(d *Dispatcher) {
		d.overrides = overrides
	}
}

// NewDispatcherWithOptions creates a new Dispatcher with options.
func NewDispatcherWithOptions(
	registry *validator.Registry,
	log logger.Logger,
	executor Executor,
	opts ...DispatcherOption,
) *Dispatcher {
	d := &Dispatcher{
		registry: registry,
		logger:   log,
		executor: executor,
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
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

	// Apply overrides to suppress disabled error codes
	validationErrors = d.applyOverrides(validationErrors)

	// Apply exception checking to blocking errors
	validationErrors = d.applyExceptionChecking(hookCtx, validationErrors)

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

// applyOverrides filters out validation errors whose error codes are disabled via overrides.
func (d *Dispatcher) applyOverrides(errors []*ValidationError) []*ValidationError {
	if d.overrides == nil {
		return errors
	}

	result := make([]*ValidationError, 0, len(errors))

	for _, verr := range errors {
		code := verr.Reference.Code()
		if code != "" && d.overrides.IsCodeDisabled(code) {
			d.logger.Info("validation error suppressed by override",
				"code", code,
				"validator", verr.Validator,
			)

			continue
		}

		result = append(result, verr)
	}

	return result
}

// applyExceptionChecking checks for exception tokens in blocking errors.
func (d *Dispatcher) applyExceptionChecking(
	hookCtx *hook.Context,
	errors []*ValidationError,
) []*ValidationError {
	if d.exceptionChecker == nil || !d.exceptionChecker.IsEnabled() {
		return errors
	}

	result := make([]*ValidationError, 0, len(errors))

	for _, verr := range errors {
		modifiedErr, bypassed := d.exceptionChecker.CheckException(hookCtx, verr)

		if bypassed {
			d.logger.Info("validation error bypassed via exception",
				"validator", verr.Validator,
				"reference", verr.Reference,
			)
		}

		// Include the modified error (nil if completely bypassed, non-blocking if converted to warning)
		if modifiedErr != nil {
			result = append(result, modifiedErr)
		}
	}

	return result
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
