package hookresponse

import (
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// BuildCodex constructs a Codex command-hook response that carries only the
// fields Codex documents for the event. Codex treats an unsupported field as a
// failed hook, and on PreToolUse a failed hook lets the tool run.
//
// Post-tool findings stay advisory: the tool already ran, and blocking
// findings are recorded for the Stop hook instead.
func BuildCodex(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *CodexCommandResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, warnings, bypassed := categorize(errs)
	reasonSent := hookCtx.Event == hook.CanonicalEventBeforeTool
	additionalContext := formatContextFor(
		hookCtx, blocking, warnings, bypassed, patternWarnings, !reasonSent,
	)
	budget := agentBudgetFor(hookCtx)

	resp := &CodexCommandResponse{
		SystemMessage: formatSystemMessageFor(hookCtx, errs),
	}

	capability, known := hook.ProviderEventCapability(hook.ProviderCodex, hookCtx.Event)
	if !known || hook.IsCodexAliasedEvent(hookCtx.RawEventName) {
		return resp
	}

	eventName := capability.NativeName

	switch hookCtx.Event {
	case hook.CanonicalEventBeforeTool:
		resp.HookSpecificOutput = codexContext(eventName, additionalContext)

		if len(blocking) > 0 {
			if resp.HookSpecificOutput == nil {
				resp.HookSpecificOutput = &CodexHookSpecificOutput{HookEventName: eventName}
			}

			resp.HookSpecificOutput.PermissionDecision = decisionDeny
			resp.HookSpecificOutput.PermissionDecisionReason = formatDecisionReasonWithin(
				blocking,
				budget,
			)
		}
	case hook.CanonicalEventSessionStart, hook.CanonicalEventPostCompact:
		if len(blocking) > 0 {
			stop := false
			resp.Continue = &stop
			resp.StopReason = formatDecisionReasonWithin(blocking, budget)
		} else {
			resp.HookSpecificOutput = codexContext(eventName, additionalContext)
		}
	case hook.CanonicalEventTurnStop, hook.CanonicalEventSubagentStop:
		if len(blocking) > 0 {
			resp.Decision = decisionBlock
			resp.Reason = formatCompletionReason(blocking, budget)
		}
	case hook.CanonicalEventUserPromptSubmit:
		if len(blocking) > 0 {
			resp.Decision = decisionBlock
			resp.Reason = formatDecisionReasonWithin(blocking, budget)
		} else {
			resp.HookSpecificOutput = codexContext(eventName, additionalContext)
		}
	case hook.CanonicalEventAfterTool:
		resp.HookSpecificOutput = codexContext(eventName, additionalContext)
	case hook.CanonicalEventUnknown, hook.CanonicalEventNotification,
		hook.CanonicalEventPreCompress, hook.CanonicalEventElicitation,
		hook.CanonicalEventElicitationResult, hook.CanonicalEventSessionEnd,
		hook.CanonicalEventStopFailure:
	}

	resp = restrictCodexResponse(resp, capability)
	if *resp == (CodexCommandResponse{}) {
		return nil
	}

	return resp
}

func codexContext(eventName, additionalContext string) *CodexHookSpecificOutput {
	if additionalContext == "" {
		return nil
	}

	return &CodexHookSpecificOutput{
		HookEventName:     eventName,
		AdditionalContext: additionalContext,
	}
}

// restrictCodexResponse drops every field the event does not accept, so a
// policy change in BuildCodex cannot emit a field Codex rejects.
func restrictCodexResponse(
	resp *CodexCommandResponse,
	capability hook.EventCapability,
) *CodexCommandResponse {
	if !capability.Supports(hook.ResponseFieldContinue) {
		resp.Continue = nil
	}

	if !capability.Supports(hook.ResponseFieldStopReason) {
		resp.StopReason = ""
	}

	if !capability.Supports(hook.ResponseFieldSystemMessage) {
		resp.SystemMessage = ""
	}

	if !capability.Supports(hook.ResponseFieldDecision) {
		resp.Decision = ""
		resp.Reason = ""
	}

	if out := resp.HookSpecificOutput; out != nil {
		if !capability.Supports(hook.ResponseFieldPermissionDecision) {
			out.PermissionDecision = ""
			out.PermissionDecisionReason = ""
		}

		if !capability.Supports(hook.ResponseFieldAdditionalContext) {
			out.AdditionalContext = ""
		}

		if out.PermissionDecision == "" && out.AdditionalContext == "" {
			resp.HookSpecificOutput = nil
		}
	}

	return resp
}
