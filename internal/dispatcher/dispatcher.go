// Package dispatcher orchestrates validation of hook contexts.
package dispatcher

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/secrets"
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

	// Findings lists every actionable violation behind this error.
	Findings []validator.Finding

	// Unavailable reports that the check could not run.
	Unavailable bool
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

// BypassPolicy decides whether a context escapes validation because the
// session runs without approval prompts.
type BypassPolicy interface {
	// SkipValidation reports whether validation should be skipped entirely.
	SkipValidation(hookCtx *hook.Context) bool
}

// Dispatcher orchestrates validation of hook contexts.
type Dispatcher struct {
	registry         *validator.Registry
	logger           logger.Logger
	executor         Executor
	exceptionChecker ExceptionChecker
	overrides        *config.OverridesConfig
	bypassPolicy     BypassPolicy
	pathResolver     parser.Resolver
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

// WithBypassPolicy sets the permission bypass policy. Without one, validation
// runs regardless of the session permission mode.
func WithBypassPolicy(policy BypassPolicy) DispatcherOption {
	return func(d *Dispatcher) {
		if policy != nil {
			d.bypassPolicy = policy
		}
	}
}

// WithPathResolver sets what expands ~ in the paths a shell command writes.
// Without one, the running system answers.
func WithPathResolver(resolver parser.Resolver) DispatcherOption {
	return func(d *Dispatcher) {
		if resolver != nil {
			d.pathResolver = resolver
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

	if d.bypassPolicy != nil && d.bypassPolicy.SkipValidation(hookCtx) {
		d.logger.Info("skipping validation: configured to skip in bypass permission mode",
			"permissionMode", hookCtx.PermissionMode,
		)

		return nil
	}

	if len(hookCtx.PatchFiles) > 0 {
		return d.validatePatchFiles(ctx, hookCtx)
	}

	validationErrors := afterToolFindings(hookCtx, d.runValidators(ctx, hookCtx))

	// Validate the files a Bash command writes, before and after it runs.
	if hookCtx.ToolName == hook.ToolTypeBash && (hookCtx.Event == hook.CanonicalEventBeforeTool ||
		hookCtx.Event == hook.CanonicalEventAfterTool ||
		hookCtx.EventType == hook.EventTypePreToolUse ||
		hookCtx.EventType == hook.EventTypePostToolUse) {
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
				"message", secrets.Redact(verr.Message),
			)
		} else {
			d.logger.Info("validator warned",
				"validator", name,
				"message", secrets.Redact(verr.Message),
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

	if hookCtx == nil ||
		(hookCtx.Event != hook.CanonicalEventBeforeTool &&
			hookCtx.Event != hook.CanonicalEventElicitation &&
			hookCtx.Event != hook.CanonicalEventElicitationResult &&
			hookCtx.EventType != hook.EventTypePreToolUse &&
			!hookCtx.MatchesEventName(string(hook.CanonicalEventBeforeTool))) {
		return errors
	}

	result := make([]*ValidationError, 0, len(errors))

	for _, verr := range errors {
		result = append(result, d.checkException(hookCtx, verr)...)
	}

	return result
}

// checkException applies exceptions to one error. Each code among its
// findings is checked on its own, so a token for a secondary code waives
// those findings while the rest stay blocking. Errors without findings, or
// whose findings all share the primary code, are checked whole.
func (d *Dispatcher) checkException(
	hookCtx *hook.Context,
	verr *ValidationError,
) []*ValidationError {
	codes := findingCodes(verr.Findings)
	if len(codes) == 0 || (len(codes) == 1 && codes[0] == verr.Reference.Code()) {
		return d.checkWhole(hookCtx, verr)
	}

	waived := make([]*ValidationError, 0, len(codes))
	cleared := make(map[string]bool, len(codes))

	for _, code := range codes {
		part := errorForCode(verr, code)

		checked, bypassed := d.exceptionChecker.CheckException(hookCtx, part)
		if checked == nil {
			cleared[code] = true

			continue
		}

		if !bypassed {
			continue
		}

		d.logBypass(checked)

		waived = append(waived, checked)
		cleared[code] = true
	}

	if len(cleared) == 0 {
		return []*ValidationError{verr}
	}

	rest := make([]validator.Finding, 0, len(verr.Findings))

	for _, f := range verr.Findings {
		if !cleared[f.Code()] {
			rest = append(rest, f)
		}
	}

	if len(rest) == 0 {
		return waived
	}

	remaining := errorFromFindings(verr, rest)
	if !cleared[verr.Reference.Code()] {
		remaining = withHeaderOf(verr, rest)
	}

	return append(waived, remaining)
}

// checkWhole applies an exception to the error as a single unit.
func (d *Dispatcher) checkWhole(
	hookCtx *hook.Context,
	verr *ValidationError,
) []*ValidationError {
	checked, bypassed := d.exceptionChecker.CheckException(hookCtx, verr)
	if checked == nil {
		return nil
	}

	if bypassed {
		d.logBypass(checked)
	}

	return []*ValidationError{checked}
}

func (d *Dispatcher) logBypass(verr *ValidationError) {
	d.logger.Info("validation error bypassed via exception",
		"validator", verr.Validator,
		"reference", verr.Reference,
	)
}

// findingCodes lists the distinct finding codes in order of first appearance.
func findingCodes(findings []validator.Finding) []string {
	seen := make(map[string]bool, len(findings))
	codes := make([]string, 0, len(findings))

	for _, f := range findings {
		if code := f.Code(); !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}

	return codes
}

// errorForCode narrows an error to the findings with one code, keeping the
// original header when the code is the primary one.
func errorForCode(verr *ValidationError, code string) *ValidationError {
	findings := make([]validator.Finding, 0, len(verr.Findings))

	for _, f := range verr.Findings {
		if f.Code() == code {
			findings = append(findings, f)
		}
	}

	if code == verr.Reference.Code() {
		return withHeaderOf(verr, findings)
	}

	return errorFromFindings(verr, findings)
}

// withHeaderOf copies the error with its findings replaced.
func withHeaderOf(verr *ValidationError, findings []validator.Finding) *ValidationError {
	narrowed := *verr
	narrowed.Findings = findings

	return &narrowed
}

// errorFromFindings builds a blocking error headed by its first finding.
func errorFromFindings(verr *ValidationError, findings []validator.Finding) *ValidationError {
	return &ValidationError{
		Validator:   verr.Validator,
		Message:     findings[0].Message,
		ShouldBlock: true,
		Reference:   findings[0].Reference,
		FixHint:     validator.GetSuggestion(findings[0].Reference),
		Findings:    findings,
		Unavailable: verr.Unavailable,
	}
}

// validateBashFileWrites validates each file a Bash command writes as a
// synthetic Write. Before the command runs that is the content parsed from
// it; afterwards it is the file on disk, including files the provider reports
// as changed that the parser could not see.
func (d *Dispatcher) validateBashFileWrites(
	ctx context.Context,
	bashCtx *hook.Context,
) []*ValidationError {
	result, err := bashCtx.ParsedCommand()
	if err != nil {
		d.logger.Debug("failed to parse bash command for file writes",
			"error", err,
		)

		return nil
	}

	targets := bashWriteTargets(bashCtx, result.FileWrites, d.resolver())
	if len(targets) == 0 {
		return nil
	}

	d.logger.Info("detected bash file writes",
		"count", len(targets),
	)

	allErrors := make([]*ValidationError, 0)

	for _, target := range targets {
		if bashCtx.IsAfterTool() && unchangedSinceBeforeTool(target) {
			d.logger.Debug("skipping write checked before the tool", "file", target.path)

			continue
		}

		syntheticCtx := &hook.Context{
			Provider:       bashCtx.Provider,
			Event:          bashCtx.Event,
			RawEventName:   bashCtx.EventName(),
			EventType:      bashCtx.EventType,
			RawToolName:    hook.ToolTypeWrite.String(),
			ToolFamily:     hook.ToolFamilyWrite,
			ToolName:       hook.ToolTypeWrite,
			WorkingDir:     bashCtx.WorkingDir,
			PermissionMode: bashCtx.PermissionMode,
			SessionID:      bashCtx.SessionID,
			ToolUseID:      bashCtx.ToolUseID,
			ToolExecuted:   bashCtx.ToolExecuted,
			ToolSucceeded:  bashCtx.ToolSucceeded,
			ToolError:      bashCtx.ToolError,
			Derived:        true,
			ToolInput: hook.ToolInput{
				FilePath: target.path,
				Content:  target.content,
			},
		}

		d.logger.Debug("validating synthetic write context",
			"file", target.path,
		)

		errs := d.runValidators(ctx, syntheticCtx)
		if bashCtx.IsAfterTool() {
			errs = namedAfter(target.path, advisory(errs))
		}

		allErrors = append(allErrors, errs...)
	}

	return allErrors
}

// validatePatchFiles validates each file of a multi-file patch as its own Write
// or Edit, so path rules and content checks apply to every file rather than
// only the first one.
func (d *Dispatcher) validatePatchFiles(
	ctx context.Context,
	patchCtx *hook.Context,
) []*ValidationError {
	allErrors := make([]*ValidationError, 0, len(patchCtx.PatchFiles))

	for _, file := range patchCtx.PatchFiles {
		fileCtx := &hook.Context{
			Provider:       patchCtx.Provider,
			Event:          patchCtx.Event,
			RawEventName:   patchCtx.EventName(),
			EventType:      patchCtx.EventType,
			RawToolName:    patchCtx.RawToolName,
			ToolFamily:     file.ToolFamily,
			ToolName:       file.ToolName,
			ToolInput:      file.Input,
			RawJSON:        patchCtx.RawJSON,
			WorkingDir:     patchCtx.WorkingDir,
			PermissionMode: patchCtx.PermissionMode,
			Model:          patchCtx.Model,
			SessionID:      patchCtx.SessionID,
			ToolUseID:      patchCtx.ToolUseID,
			TurnID:         patchCtx.TurnID,
			ToolExecuted:   patchCtx.ToolExecuted,
			ToolSucceeded:  patchCtx.ToolSucceeded,
			ToolError:      patchCtx.ToolError,
			Derived:        true,
			AffectedPaths:  []string{file.Input.FilePath},
		}

		d.logger.Debug("validating patch file", "file", file.Input.FilePath)

		errs := afterToolFindings(fileCtx, d.runValidators(ctx, fileCtx))
		allErrors = append(allErrors, errs...)
	}

	return allErrors
}

// afterToolFindings makes the findings about a file a tool already changed
// advisory and names the file in each, as for shell writes.
func afterToolFindings(hookCtx *hook.Context, errs []*ValidationError) []*ValidationError {
	if len(errs) == 0 || !hookCtx.IsAfterTool() || !hookCtx.IsFileTool() {
		return errs
	}

	return namedAfter(hookCtx.GetFilePath(), advisory(errs))
}

func (d *Dispatcher) resolver() parser.Resolver {
	if d.pathResolver == nil {
		return &parser.OSResolver{}
	}

	return d.pathResolver
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
