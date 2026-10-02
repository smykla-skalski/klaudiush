package validator

import (
	"context"

	"github.com/cockroachdb/errors"
)

// UnavailableReason says why a check could not run. It keeps an unavailable
// check apart from a pass and from a violation, so the failure policy can
// decide what the action gets.
type UnavailableReason string

// Reasons a check could not run. Describe gives each one's wording.
const (
	ReasonMissingTool     UnavailableReason = "missing_tool"
	ReasonTimeout         UnavailableReason = "timeout"
	ReasonCanceled        UnavailableReason = "canceled"
	ReasonPanic           UnavailableReason = "panic"
	ReasonMalformedOutput UnavailableReason = "malformed_output"
	ReasonMalformedInput  UnavailableReason = "malformed_input"
	ReasonConfig          UnavailableReason = "config"
	ReasonState           UnavailableReason = "state"
	ReasonError           UnavailableReason = "error"
)

// Describe returns a short phrase for the reason, for messages.
func (r UnavailableReason) Describe() string {
	switch r {
	case ReasonMissingTool:
		return "required tool not installed"
	case ReasonTimeout:
		return "timed out"
	case ReasonCanceled:
		return "canceled"
	case ReasonPanic:
		return "crashed"
	case ReasonMalformedOutput:
		return "unreadable output"
	case ReasonMalformedInput:
		return "unreadable hook input"
	case ReasonConfig:
		return "configuration error"
	case ReasonState:
		return "session state unavailable"
	case ReasonError:
		return "failed to run"
	default:
		return "failed to run"
	}
}

// Unavailable creates a result for a check that could not run. It neither
// passes nor blocks on its own; the failure policy decides that.
func Unavailable(reason UnavailableReason, message string) *Result {
	return &Result{
		Passed:            false,
		Message:           message,
		ShouldBlock:       false,
		Reference:         RefValidationUnavailable,
		FixHint:           GetSuggestion(RefValidationUnavailable),
		Unavailable:       true,
		UnavailableReason: reason,
	}
}

// ReasonFromContext classifies why ctx ended: a deadline is a timeout,
// anything else a cancellation. It returns "" while ctx is still live.
func ReasonFromContext(ctx context.Context) UnavailableReason {
	switch err := ctx.Err(); {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonTimeout
	default:
		return ReasonCanceled
	}
}

// ReasonOf returns the reason of an unavailable result, defaulting to
// ReasonError when the result did not say.
func (r *Result) ReasonOf() UnavailableReason {
	if !r.Unavailable {
		return ""
	}

	if r.UnavailableReason == "" {
		return ReasonError
	}

	return r.UnavailableReason
}
