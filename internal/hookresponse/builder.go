package hookresponse

import (
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
)

// Build constructs a HookResponse from validation errors.
// Returns nil when there are no errors (clean pass, no output needed).
func Build(eventName string, errs []*dispatcher.ValidationError) *HookResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, warnings, bypassed := categorize(errs)

	resp := &HookResponse{
		SystemMessage: FormatSystemMessage(errs),
	}

	switch {
	case len(blocking) > 0:
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:            eventName,
			PermissionDecision:       "deny",
			PermissionDecisionReason: formatDecisionReason(blocking),
			AdditionalContext:        formatAdditionalContext(blocking, warnings, bypassed),
		}
	case len(bypassed) > 0:
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:      eventName,
			PermissionDecision: "allow",
			AdditionalContext:  formatAdditionalContext(nil, warnings, bypassed),
		}
	case len(warnings) > 0:
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:      eventName,
			PermissionDecision: "allow",
			AdditionalContext:  formatAdditionalContext(nil, warnings, nil),
		}
	}

	return resp
}

// categorize splits errors into blocking, warnings, and bypassed.
func categorize(errs []*dispatcher.ValidationError) (
	blocking, warnings, bypassed []*dispatcher.ValidationError,
) {
	for _, e := range errs {
		switch {
		case e.Bypassed:
			bypassed = append(bypassed, e)
		case e.ShouldBlock:
			blocking = append(blocking, e)
		default:
			warnings = append(warnings, e)
		}
	}

	return blocking, warnings, bypassed
}
