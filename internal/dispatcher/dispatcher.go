// Package dispatcher orchestrates validation of hook contexts.
package dispatcher

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/session"
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

// SessionTracker manages session state for fast-fail behavior.
type SessionTracker interface {
	// IsPoisoned checks if a session is poisoned.
	IsPoisoned(sessionID string) (bool, *session.SessionInfo)

	// Poison marks a session as poisoned with the given error codes and message.
	Poison(sessionID string, codes []string, message string)

	// Unpoison clears the poisoned state of a session.
	Unpoison(sessionID string)

	// RecordCommand increments the command count for a session.
	RecordCommand(sessionID string)

	// IsEnabled returns true if session tracking is enabled.
	IsEnabled() bool
}

// SessionAuditLogger logs session events (poison/unpoison) for audit purposes.
type SessionAuditLogger interface {
	// Log writes an audit entry to the log file.
	Log(entry *session.AuditEntry) error

	// IsEnabled returns true if audit logging is enabled.
	IsEnabled() bool
}

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
	registry           *validator.Registry
	logger             logger.Logger
	executor           Executor
	exceptionChecker   ExceptionChecker
	sessionTracker     SessionTracker
	sessionAuditLogger SessionAuditLogger
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

// WithSessionTracker sets the session tracker for the dispatcher.
func WithSessionTracker(tracker SessionTracker) DispatcherOption {
	return func(d *Dispatcher) {
		if tracker != nil {
			d.sessionTracker = tracker
		}
	}
}

// WithSessionAuditLogger sets the session audit logger for the dispatcher.
func WithSessionAuditLogger(auditLogger SessionAuditLogger) DispatcherOption {
	return func(d *Dispatcher) {
		if auditLogger != nil {
			d.sessionAuditLogger = auditLogger
		}
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

	// Check if session tracking is enabled and session is poisoned
	if d.sessionTracker != nil && d.sessionTracker.IsEnabled() && hookCtx.HasSessionID() {
		if poisoned, info := d.sessionTracker.IsPoisoned(hookCtx.SessionID); poisoned {
			d.logger.Info("session is poisoned",
				"session_id", hookCtx.SessionID,
				"poison_codes", info.PoisonCodes,
			)

			// Check for unpoison acknowledgment token
			if !d.checkUnpoisonAcknowledgment(hookCtx, info) {
				return []*ValidationError{createPoisonedSessionError(info)}
			}

			d.logger.Info("session unpoisoned via acknowledgment",
				"session_id", hookCtx.SessionID,
			)
		}
	}

	// Run validators on the main context
	validationErrors := d.runValidators(ctx, hookCtx)

	// If this is a Bash PreToolUse, also validate synthetic Write contexts for file writes
	if hookCtx.EventType == hook.EventTypePreToolUse && hookCtx.ToolName == hook.ToolTypeBash {
		syntheticErrors := d.validateBashFileWrites(ctx, hookCtx)
		validationErrors = append(validationErrors, syntheticErrors...)
	}

	// Poison session if there are blocking errors, otherwise record command
	if d.sessionTracker != nil && d.sessionTracker.IsEnabled() && hookCtx.HasSessionID() {
		if ShouldBlock(validationErrors) {
			codes := extractSessionPoisonCodes(validationErrors)
			message := extractSessionPoisonMessage(validationErrors)

			d.logger.Info("poisoning session due to blocking error",
				"session_id", hookCtx.SessionID,
				"error_codes", codes,
			)

			d.sessionTracker.Poison(hookCtx.SessionID, codes, message)

			// Log audit entry for poison
			d.logSessionAuditEntry(
				hookCtx,
				session.AuditActionPoison,
				codes,
				"", // no source for poison (it's from validation failure)
				message,
			)
		} else {
			// Record command when validation passes or only has warnings (no blocking errors)
			d.sessionTracker.RecordCommand(hookCtx.SessionID)
		}
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

// checkUnpoisonAcknowledgment checks if the current command contains an unpoison token
// that acknowledges all poison codes. If all codes are acknowledged, it unpoisons
// the session and returns true. Otherwise, returns false.
//
// IMPORTANT: To prevent deadlocks, this function is lenient when it detects an
// unpoison attempt. If the command contains unpoison markers (KLACK= or # SESS:)
// but parsing fails, the command is allowed through to prevent the session from
// becoming permanently stuck.
func (d *Dispatcher) checkUnpoisonAcknowledgment(
	hookCtx *hook.Context,
	info *session.SessionInfo,
) bool {
	// Get command from context (only Bash commands have commands to check)
	command := hookCtx.GetCommand()
	if command == "" {
		return false
	}

	// Quick check: does this look like an unpoison attempt?
	hasUnpoisonAttempt := session.ContainsUnpoisonAttempt(command)

	// Check if the command contains an unpoison acknowledgment token
	result, err := session.CheckUnpoisonAcknowledgmentFull(
		command,
		info.PoisonCodes,
	)
	if err != nil {
		d.logger.Debug("failed to parse unpoison token",
			"error", err,
		)

		// If it looks like an unpoison attempt but parsing failed,
		// be lenient and allow the command through to prevent deadlocks.
		// The command will still be validated by normal validators.
		if hasUnpoisonAttempt {
			d.logger.Info("allowing command with unpoison attempt despite parse error",
				"session_id", hookCtx.SessionID,
				"error", err,
			)

			// Unpoison since we're being lenient (user is clearly trying)
			d.sessionTracker.Unpoison(hookCtx.SessionID)

			// Log audit entry for lenient unpoison
			d.logSessionAuditEntry(
				hookCtx,
				session.AuditActionUnpoison,
				info.PoisonCodes,
				"lenient_fallback",
				"",
			)

			return true
		}

		return false
	}

	if !result.Acknowledged {
		if len(result.UnacknowledgedCodes) > 0 &&
			len(result.UnacknowledgedCodes) < len(info.PoisonCodes) {
			// Partial acknowledgment - log which codes still need acknowledgment
			d.logger.Info("partial unpoison acknowledgment",
				"session_id", hookCtx.SessionID,
				"unacknowledged_codes", result.UnacknowledgedCodes,
			)
		}

		return false
	}

	// All codes acknowledged - unpoison the session
	d.sessionTracker.Unpoison(hookCtx.SessionID)

	// Log audit entry for unpoison
	d.logSessionAuditEntry(
		hookCtx,
		session.AuditActionUnpoison,
		info.PoisonCodes,
		result.Source.String(),
		"",
	)

	return true
}

// logSessionAuditEntry logs a session audit entry if audit logging is enabled.
func (d *Dispatcher) logSessionAuditEntry(
	hookCtx *hook.Context,
	action session.AuditAction,
	codes []string,
	source string,
	poisonMessage string,
) {
	if d.sessionAuditLogger == nil || !d.sessionAuditLogger.IsEnabled() {
		return
	}

	// Truncate command to prevent sensitive data leakage
	command := hookCtx.GetCommand()

	const maxCommandLength = 500
	if len(command) > maxCommandLength {
		command = command[:maxCommandLength] + "..."
	}

	// Get working directory
	workingDir, _ := os.Getwd()

	entry := &session.AuditEntry{
		Timestamp:     time.Now(),
		Action:        action,
		SessionID:     hookCtx.SessionID,
		PoisonCodes:   codes,
		Source:        source,
		Command:       command,
		PoisonMessage: poisonMessage,
		WorkingDir:    workingDir,
	}

	if err := d.sessionAuditLogger.Log(entry); err != nil {
		d.logger.Error("failed to log session audit entry",
			"action", action.String(),
			"error", err,
		)
	}
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
