package hookresponse

import (
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Build constructs a HookResponse from validation errors.
// Returns nil when there are no errors (clean pass, no output needed).
func Build(eventName string, errs []*dispatcher.ValidationError) *HookResponse {
	return BuildWithPatterns(eventName, errs, nil)
}

// BuildWithPatterns constructs a HookResponse with optional pattern warnings.
// Pattern warnings are appended to the additionalContext for blocking errors.
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
			PermissionDecision:       "deny",
			PermissionDecisionReason: formatDecisionReason(blocking),
			AdditionalContext: formatAdditionalContext(
				blocking,
				warnings,
				bypassed,
				patternWarnings,
			),
		}
	case len(bypassed) > 0:
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:      eventName,
			PermissionDecision: "allow",
			AdditionalContext:  formatAdditionalContext(nil, warnings, bypassed, nil),
		}
	case len(warnings) > 0:
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:      eventName,
			PermissionDecision: "allow",
			AdditionalContext:  formatAdditionalContext(nil, warnings, nil, nil),
		}
	}

	return resp
}

// BuildForContext constructs a provider-specific hook response.
// Returns nil when there are no errors (clean pass, no output needed).
func BuildForContext(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) any {
	if len(errs) == 0 {
		return nil
	}

	if hookCtx != nil && hookCtx.Provider == hook.ProviderCodex {
		return BuildCodex(hookCtx, errs, patternWarnings)
	}

	if hookCtx != nil && hookCtx.Provider == hook.ProviderGemini {
		return BuildGemini(hookCtx, errs, patternWarnings)
	}

	if hookCtx != nil && hookCtx.IsElicitationEvent() {
		return BuildElicitation(hookCtx, errs, patternWarnings)
	}

	if hookCtx != nil &&
		hookCtx.Provider == hook.ProviderClaude &&
		hookCtx.Event == hook.CanonicalEventAfterTool {
		return BuildClaudeAfterTool(hookCtx, errs, patternWarnings)
	}

	eventName := ""
	if hookCtx != nil {
		eventName = hookCtx.EventName()
	}

	return BuildWithPatterns(eventName, errs, patternWarnings)
}

// BuildClaudeAfterTool constructs a Claude PostToolUse response.
func BuildClaudeAfterTool(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *HookResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, warnings, bypassed := categorize(errs)
	additionalContext := formatAdditionalContext(blocking, warnings, bypassed, patternWarnings)
	resp := &HookResponse{
		SystemMessage: FormatSystemMessage(errs),
	}

	if len(blocking) > 0 {
		resp.Decision = "block"
		resp.Reason = formatDecisionReason(blocking)
	}

	if additionalContext != "" {
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:     hookCtx.EventName(),
			AdditionalContext: additionalContext,
		}
	}

	return resp
}

// BuildCodex constructs a Codex command-hook response.
func BuildCodex(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *CodexCommandResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, warnings, bypassed := categorize(errs)
	additionalContext := formatAdditionalContext(blocking, warnings, bypassed, patternWarnings)

	resp := &CodexCommandResponse{
		Continue:      true,
		SystemMessage: FormatSystemMessage(errs),
	}

	switch hookCtx.Event {
	case hook.CanonicalEventTurnStop:
		if len(blocking) > 0 {
			resp.Decision = "block"
			resp.Reason = formatDecisionReason(blocking)
		}

	case hook.CanonicalEventSessionStart:
		if len(blocking) > 0 {
			resp.Continue = false
			resp.StopReason = formatDecisionReason(blocking)

			return resp
		}

		if additionalContext != "" {
			resp.HookSpecificOutput = &CodexHookSpecificOutput{
				HookEventName:     hookCtx.EventName(),
				AdditionalContext: additionalContext,
			}
		}
	case hook.CanonicalEventAfterTool:
		if additionalContext != "" {
			resp.HookSpecificOutput = &CodexHookSpecificOutput{
				HookEventName:     hookCtx.EventName(),
				AdditionalContext: additionalContext,
			}
		}
	default:
		if len(blocking) > 0 {
			resp.Continue = false
			resp.StopReason = formatDecisionReason(blocking)

			return resp
		}

		if additionalContext != "" {
			resp.HookSpecificOutput = &CodexHookSpecificOutput{
				HookEventName:     hookCtx.EventName(),
				AdditionalContext: additionalContext,
			}
		}
	}

	return resp
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
			resp.Decision = "deny"
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
	case hook.CanonicalEventTurnStop, hook.CanonicalEventNotification,
		hook.CanonicalEventPreCompress, hook.CanonicalEventPostCompact:
	default:
		if len(blocking) > 0 {
			resp.Decision = "deny"
			resp.Reason = formatDecisionReason(blocking)

			return resp
		}

		if additionalContext != "" {
			resp.HookSpecificOutput = &GeminiHookSpecificOutput{
				HookEventName:     hookCtx.EventName(),
				AdditionalContext: additionalContext,
			}
		}
	}

	return resp
}

// BuildElicitation constructs an ElicitationHookResponse.
// Returns nil when there are no blocking errors (warnings are allowed through).
func BuildElicitation(
	_ *hook.Context,
	errs []*dispatcher.ValidationError,
	_ []string,
) *ElicitationHookResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, _, _ := categorize(errs)
	if len(blocking) == 0 {
		return nil
	}

	return &ElicitationHookResponse{
		Action:        "decline",
		SystemMessage: FormatSystemMessage(errs),
	}
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
