// Package dispatcher provides validation orchestration.
package dispatcher

import (
	"strings"

	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// ExceptionChecker checks if validation errors can be bypassed via exception tokens.
type ExceptionChecker interface {
	// CheckException determines if a validation error should be bypassed.
	// Returns the modified error (nil if bypassed) and whether it was bypassed.
	CheckException(
		hookCtx *hook.Context,
		verr *ValidationError,
	) (*ValidationError, bool)

	// IsEnabled returns whether exception checking is enabled.
	IsEnabled() bool
}

// DefaultExceptionChecker uses the exceptions.Handler to check for exceptions.
type DefaultExceptionChecker struct {
	handler *exceptions.Handler
	logger  logger.Logger
}

// ExceptionCheckerOption configures the DefaultExceptionChecker.
type ExceptionCheckerOption func(*DefaultExceptionChecker)

// WithExceptionCheckerLogger sets the logger.
func WithExceptionCheckerLogger(log logger.Logger) ExceptionCheckerOption {
	return func(c *DefaultExceptionChecker) {
		if log != nil {
			c.logger = log
		}
	}
}

// NewExceptionChecker creates a new exception checker.
func NewExceptionChecker(
	handler *exceptions.Handler,
	opts ...ExceptionCheckerOption,
) *DefaultExceptionChecker {
	c := &DefaultExceptionChecker{
		handler: handler,
		logger:  logger.NewNoOpLogger(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CheckException determines if a validation error should be bypassed.
func (c *DefaultExceptionChecker) CheckException(
	hookCtx *hook.Context,
	verr *ValidationError,
) (*ValidationError, bool) {
	if c.handler == nil || !c.handler.IsEnabled() {
		return verr, false
	}

	// Only check blocking errors
	if !verr.ShouldBlock {
		return verr, false
	}

	// Extract error code from reference URL
	errorCode := extractErrorCode(verr.Reference)
	if errorCode == "" {
		c.logger.Debug("no error code found in validation error",
			"validator", verr.Validator,
		)

		return verr, false
	}

	// Check for exception
	resp := c.handler.Check(&exceptions.CheckRequest{
		HookContext:   hookCtx,
		ValidatorName: verr.Validator,
		ErrorCode:     errorCode,
		ErrorMessage:  verr.Message,
	})

	if !resp.Bypassed {
		c.logger.Debug("exception not allowed",
			"validator", verr.Validator,
			"error_code", errorCode,
			"reason", resp.Reason,
		)

		return verr, false
	}

	c.logger.Info("exception allowed, bypassing validation error",
		"validator", verr.Validator,
		"error_code", errorCode,
		"token_reason", resp.TokenReason,
	)

	// Convert to a non-blocking warning with bypass info
	bypassedErr := &ValidationError{
		Validator:   verr.Validator,
		Message:     formatBypassedMessage(verr.Message, resp),
		Details:     verr.Details,
		ShouldBlock: false, // No longer blocks
		Reference:   verr.Reference,
		FixHint:     verr.FixHint,
	}

	return bypassedErr, true
}

// IsEnabled returns whether exception checking is enabled.
func (c *DefaultExceptionChecker) IsEnabled() bool {
	if c.handler == nil {
		return false
	}

	return c.handler.IsEnabled()
}

// extractErrorCode extracts the error code from a reference URL.
// Reference format: https://klaudiu.sh/{CODE}
func extractErrorCode(ref validator.Reference) string {
	if ref == "" {
		return ""
	}

	refStr := string(ref)

	const prefix = "https://klaudiu.sh/"

	if !strings.HasPrefix(refStr, prefix) {
		return ""
	}

	return strings.TrimSuffix(refStr[len(prefix):], "/")
}

// formatBypassedMessage formats the message to indicate it was bypassed.
func formatBypassedMessage(originalMsg string, resp *exceptions.CheckResponse) string {
	if resp.TokenReason != "" {
		return originalMsg + " [BYPASSED: " + resp.TokenReason + "]"
	}

	return originalMsg + " [BYPASSED]"
}

// NoOpExceptionChecker is an exception checker that never allows exceptions.
type NoOpExceptionChecker struct{}

// CheckException always returns the error unchanged.
func (*NoOpExceptionChecker) CheckException(
	_ *hook.Context,
	verr *ValidationError,
) (*ValidationError, bool) {
	return verr, false
}

// IsEnabled always returns false.
func (*NoOpExceptionChecker) IsEnabled() bool {
	return false
}

// Verify interface compliance.
var (
	_ ExceptionChecker = (*DefaultExceptionChecker)(nil)
	_ ExceptionChecker = (*NoOpExceptionChecker)(nil)
)
