package dispatcher

import (
	"fmt"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/session"
	"github.com/smykla-labs/klaudiush/internal/validator"
)

const (
	// poisonedSessionValidator is the validator name for poisoned session errors.
	poisonedSessionValidator = "session-poisoned"
)

// createPoisonedSessionError creates a validation error for a poisoned session.
// This error references the original blocking error that poisoned the session
// and includes machine-parseable unpoison instructions.
func createPoisonedSessionError(info *session.SessionInfo) *ValidationError {
	if info == nil || !info.IsPoisoned() {
		return nil
	}

	// Format timestamp with full date and time
	var timestamp string
	if info.PoisonedAt != nil {
		timestamp = info.PoisonedAt.Format("2006-01-02 15:04:05")
	}

	// Format codes for display (comma-separated with spaces)
	codesDisplay := strings.Join(info.PoisonCodes, ", ")

	// Format codes for token (comma-separated without spaces)
	codesToken := strings.Join(info.PoisonCodes, ",")

	// Build error message with "Blocked:" prefix to match documentation
	msg := fmt.Sprintf("Blocked: session poisoned by %s at %s", codesDisplay, timestamp)

	// Create details with original error and unpoison instructions
	details := make(map[string]string)

	if info.PoisonMessage != "" {
		details["original_error"] = "Original error: " + info.PoisonMessage
	}

	// Add machine-parseable unpoison instructions
	details["unpoison"] = fmt.Sprintf(
		"To unpoison: KLACK=\"SESS:%s\" command  # or comment: # SESS:%s",
		codesToken,
		codesToken,
	)

	// Build fix hint with unpoison instructions
	fixHint := fmt.Sprintf(
		"Acknowledge violations to unpoison: KLACK=\"SESS:%s\" your_command",
		codesToken,
	)

	return &ValidationError{
		Validator:   poisonedSessionValidator,
		Message:     msg,
		Details:     details,
		ShouldBlock: true,
		Reference:   validator.RefSessionPoisoned,
		FixHint:     fixHint,
	}
}

// extractSessionPoisonCodes extracts all error codes from blocking validation errors.
// Returns a slice of codes from all blocking errors with references.
func extractSessionPoisonCodes(errors []*ValidationError) []string {
	var codes []string

	for _, err := range errors {
		if err.ShouldBlock && err.Reference != "" {
			codes = append(codes, err.Reference.Code())
		}
	}

	return codes
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
