package dispatcher

import (
	"fmt"
	"time"

	"github.com/smykla-labs/klaudiush/internal/session"
	"github.com/smykla-labs/klaudiush/internal/validator"
)

const (
	// poisonedSessionValidator is the validator name for poisoned session errors.
	poisonedSessionValidator = "session-poisoned"
)

// createPoisonedSessionError creates a validation error for a poisoned session.
// This error references the original blocking error that poisoned the session.
func createPoisonedSessionError(info *session.SessionInfo) *ValidationError {
	if info == nil || !info.IsPoisoned() {
		return nil
	}

	// Format timestamp
	var timestamp string
	if info.PoisonedAt != nil {
		timestamp = info.PoisonedAt.Format(time.Kitchen)
	}

	// Build error message
	msg := fmt.Sprintf("Session poisoned by %s at %s", info.PoisonCode, timestamp)

	// Create details with original error
	details := make(map[string]string)
	if info.PoisonMessage != "" {
		details["original_error"] = "Original error: " + info.PoisonMessage
	}

	return &ValidationError{
		Validator:   poisonedSessionValidator,
		Message:     msg,
		Details:     details,
		ShouldBlock: true,
		Reference:   validator.RefSessionPoisoned,
		FixHint:     "Resolve the original error before continuing",
	}
}

// extractSessionPoisonCode extracts the error code from validation errors for session poisoning.
// Returns the code from the first blocking error with a reference.
func extractSessionPoisonCode(errors []*ValidationError) string {
	for _, err := range errors {
		if err.ShouldBlock && err.Reference != "" {
			return err.Reference.Code()
		}
	}

	return ""
}

// extractSessionPoisonMessage extracts the error message from validation errors for session poisoning.
// Returns the message from the first blocking error.
func extractSessionPoisonMessage(errors []*ValidationError) string {
	for _, err := range errors {
		if err.ShouldBlock && err.Message != "" {
			return err.Message
		}
	}

	return ""
}

// hasBlockingError returns true if any validation error should block.
func hasBlockingError(errors []*ValidationError) bool {
	return ShouldBlock(errors)
}
