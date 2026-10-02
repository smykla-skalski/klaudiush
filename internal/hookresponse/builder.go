package hookresponse

import (
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Permission decision values emitted in hook responses. There is no allow:
// a validation finding is never permission approval, so non-blocking results
// omit the decision and leave the action to the harness permission flow.
const (
	decisionDeny  = "deny"
	decisionBlock = "block"
)

// Build constructs a HookResponse from validation errors.
// Returns nil when there are no errors (clean pass, no output needed).
func Build(eventName string, errs []*dispatcher.ValidationError) *HookResponse {
	return BuildWithPatterns(eventName, errs, nil)
}

// BuildWithPatterns constructs a HookResponse with optional pattern warnings.
// Pattern warnings are appended to the additionalContext for blocking errors.
//
// Only blocking findings set permissionDecision. Warnings and accepted
// exceptions carry additionalContext alone, which Claude treats like
// "defer": the normal permission flow (rules, mode, prompt) still decides.
// An allow here would skip the user's permission prompt.
func BuildWithPatterns(
	eventName string,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *HookResponse {
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
			PermissionDecision:       decisionDeny,
			PermissionDecisionReason: formatDecisionReason(blocking),
			AdditionalContext: formatAdditionalContext(
				blocking,
				warnings,
				bypassed,
				patternWarnings,
			),
		}
	default:
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:     eventName,
			AdditionalContext: formatAdditionalContext(nil, warnings, bypassed, nil),
		}
	}

	return resp
}

// BuildForContext constructs a provider-specific hook response shaped for the
// native event the hook received. Returns nil when there are no errors (clean
// pass, no output needed).
func BuildForContext(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) any {
	if len(errs) == 0 {
		return nil
	}

	if hookCtx == nil {
		return BuildWithPatterns("", errs, patternWarnings)
	}

	switch hookCtx.Provider {
	case hook.ProviderCodex:
		if hookCtx.IsPermissionRequest() {
			return BuildPermissionRequest(errs)
		}

		return BuildCodex(hookCtx, errs, patternWarnings)
	case hook.ProviderGemini:
		return BuildGemini(hookCtx, errs, patternWarnings)
	case hook.ProviderOpenCode:
		return BuildOpenCode(hookCtx, errs, patternWarnings)
	case hook.ProviderUnknown, hook.ProviderClaude:
		return BuildClaude(hookCtx, errs, patternWarnings)
	default:
		return BuildClaude(hookCtx, errs, patternWarnings)
	}
}

// BuildGemini constructs a Gemini command-hook response.
func BuildGemini(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *GeminiCommandResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, warnings, bypassed := categorize(errs)
	additionalContext := formatAdditionalContext(blocking, warnings, bypassed, patternWarnings)

	resp := &GeminiCommandResponse{
		SystemMessage: FormatSystemMessage(errs),
	}

	switch hookCtx.Event {
	case hook.CanonicalEventBeforeTool:
		if len(blocking) > 0 {
			resp.Decision = decisionDeny
			resp.Reason = formatDecisionReason(blocking)

			return resp
		}

		if additionalContext != "" {
			resp.HookSpecificOutput = &GeminiHookSpecificOutput{
				HookEventName:     hookCtx.EventName(),
				AdditionalContext: additionalContext,
			}
		}
	case hook.CanonicalEventAfterTool, hook.CanonicalEventSessionStart:
		if additionalContext != "" {
			resp.HookSpecificOutput = &GeminiHookSpecificOutput{
				HookEventName:     hookCtx.EventName(),
				AdditionalContext: additionalContext,
			}
		}
	// AfterAgent: deny rejects the response and sends reason back to the
	// agent as a correction prompt. AfterAgent has no additionalContext.
	case hook.CanonicalEventTurnStop:
		if len(blocking) > 0 {
			resp.Decision = decisionDeny
			resp.Reason = formatCompletionReason(blocking)
		}
	// Every other event is advisory: Gemini ignores flow control on
	// SessionEnd, Notification, and PreCompress, and an unmapped event has no
	// contract that would make a deny safe to promise.
	default:
	}

	return resp
}

// BuildOpenCode constructs an opencode bridge-plugin response.
func BuildOpenCode(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *OpenCodeCommandResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, warnings, bypassed := categorize(errs)
	additionalContext := formatAdditionalContext(blocking, warnings, bypassed, patternWarnings)

	resp := &OpenCodeCommandResponse{
		Continue:      true,
		SystemMessage: FormatSystemMessage(errs),
	}

	switch hookCtx.Event {
	case hook.CanonicalEventBeforeTool:
		if len(blocking) > 0 {
			resp.Decision = decisionDeny
			resp.Reason = formatDecisionReason(blocking)

			return resp
		}
	// A submitted prompt cannot be refused: opencode's chat.message hook
	// returns void and exposes no decision channel, so findings here are
	// reported to the user rather than enforced. session.idle is a bus event
	// the bridge only reports, so it cannot keep the agent working either.
	case hook.CanonicalEventUserPromptSubmit, hook.CanonicalEventAfterTool,
		hook.CanonicalEventTurnStop, hook.CanonicalEventSubagentStop,
		hook.CanonicalEventSessionEnd, hook.CanonicalEventStopFailure,
		hook.CanonicalEventSessionStart, hook.CanonicalEventNotification,
		hook.CanonicalEventPreCompress, hook.CanonicalEventPostCompact:
	default:
		if len(blocking) > 0 {
			resp.Decision = decisionDeny
			resp.Reason = formatDecisionReason(blocking)

			return resp
		}
	}

	if additionalContext != "" && openCodeConsumesContext(hookCtx.Event) {
		resp.HookSpecificOutput = &OpenCodeHookSpecificOutput{
			HookEventName:     hookCtx.EventName(),
			AdditionalContext: additionalContext,
		}
	}

	return resp
}

// openCodeConsumesContext reports whether opencode gives the bridge plugin
// anywhere to put model-facing text for an event.
//
// Only two hooks do: tool.execute.after can append to the tool result, and the
// compaction hook can push onto the compaction prompt. The pre-execution and
// lifecycle hooks expose no such field, so emitting additionalContext for them
// would promise the model context it can never receive. Those findings still
// reach the user through systemMessage.
func openCodeConsumesContext(event hook.CanonicalEvent) bool {
	return event == hook.CanonicalEventAfterTool || event == hook.CanonicalEventPreCompress
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
