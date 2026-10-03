package hookresponse

import (
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	// elicitationDecline is the hookSpecificOutput.action that refuses an
	// MCP elicitation.
	elicitationDecline = "decline"

	permissionRequestEventName = "PermissionRequest"
)

// completionReasonPrefix tells the agent why it is being kept working.
const completionReasonPrefix = "klaudiush completion check failed. " +
	"Fix these findings before finishing: "

// BuildClaude constructs a Claude Code response shaped for the native event
// the hook received, emitting only the fields that event accepts. An event
// without a documented contract gets a user-facing systemMessage only.
func BuildClaude(
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) any {
	if len(errs) == 0 {
		return nil
	}

	capability, ok := hook.ResolveEventCapability(
		hook.ProviderClaude,
		hookCtx.Event,
		hookCtx.RawEventName,
	)
	if !ok {
		return &HookResponse{SystemMessage: formatSystemMessageFor(hookCtx, errs)}
	}

	eventName := hookCtx.EventName()
	if eventName == "" {
		eventName = capability.NativeName
	}

	switch capability.Enforcement {
	case hook.EnforcementDenyTool:
		return BuildWithPatterns(eventName, errs, patternWarnings)
	case hook.EnforcementDenyPermission:
		return permissionRequestWithin(errs, agentBudgetFor(hookCtx))
	case hook.EnforcementDeclineElicitation:
		return BuildElicitation(hookCtx, errs, patternWarnings)
	case hook.EnforcementBlockDecision, hook.EnforcementContinueTurn:
		return buildClaudeDecision(hookCtx, capability, eventName, errs, patternWarnings)
	case hook.EnforcementNone, hook.EnforcementStop, hook.EnforcementFilterTools:
		return buildClaudeAdvisory(hookCtx, capability, eventName, errs, patternWarnings)
	default:
		return buildClaudeAdvisory(hookCtx, capability, eventName, errs, patternWarnings)
	}
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

	capability, _ := hook.ProviderEventCapability(hook.ProviderClaude, hook.CanonicalEventAfterTool)

	return buildClaudeDecision(hookCtx, capability, hookCtx.EventName(), errs, patternWarnings)
}

// buildClaudeDecision handles events with a top-level decision: "block".
//
// On a completion gate (Stop, SubagentStop) additionalContext also keeps the
// agent working, so it is sent only alongside a block. Warnings alone must let
// the turn end.
func buildClaudeDecision(
	hookCtx *hook.Context,
	capability hook.EventCapability,
	eventName string,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *HookResponse {
	blocking, warnings, bypassed := categorize(errs)
	gate := capability.Enforcement == hook.EnforcementContinueTurn

	resp := &HookResponse{SystemMessage: formatSystemMessageFor(hookCtx, errs)}

	if len(blocking) > 0 {
		resp.Decision = decisionBlock
		resp.Reason = formatReasonFor(hookCtx, blocking)

		if gate {
			resp.Reason = formatCompletionReason(blocking, agentBudgetFor(hookCtx))
		}
	}

	if gate && len(blocking) == 0 {
		return resp
	}

	additionalContext := formatContextFor(
		hookCtx, blocking, warnings, bypassed, patternWarnings, false,
	)
	if additionalContext != "" && capability.Supports(hook.ResponseFieldAdditionalContext) {
		resp.HookSpecificOutput = &HookSpecificOutput{
			HookEventName:     eventName,
			AdditionalContext: additionalContext,
		}
	}

	return resp
}

// buildClaudeAdvisory handles events that cannot block. Findings reach the
// user through systemMessage and the model through additionalContext where
// the event accepts them; events that accept neither get no output.
func buildClaudeAdvisory(
	hookCtx *hook.Context,
	capability hook.EventCapability,
	eventName string,
	errs []*dispatcher.ValidationError,
	patternWarnings []string,
) *HookResponse {
	resp := &HookResponse{}

	if capability.Supports(hook.ResponseFieldSystemMessage) {
		resp.SystemMessage = formatSystemMessageFor(hookCtx, errs)
	}

	if capability.Supports(hook.ResponseFieldAdditionalContext) {
		blocking, warnings, bypassed := categorize(errs)

		additionalContext := formatContextFor(
			hookCtx,
			blocking,
			warnings,
			bypassed,
			patternWarnings,
			true,
		)
		if additionalContext != "" {
			resp.HookSpecificOutput = &HookSpecificOutput{
				HookEventName:     eventName,
				AdditionalContext: additionalContext,
			}
		}
	}

	if resp.SystemMessage == "" && resp.HookSpecificOutput == nil {
		return nil
	}

	return resp
}

// BuildPermissionRequest constructs a Claude or Codex PermissionRequest
// response. Blocking findings deny the request with a message for the agent.
// Warnings leave the decision to the user, so no decision object is sent.
func BuildPermissionRequest(errs []*dispatcher.ValidationError) *PermissionRequestResponse {
	return permissionRequestWithin(errs, defaultAgentBudget)
}

func permissionRequestWithin(
	errs []*dispatcher.ValidationError,
	budget int,
) *PermissionRequestResponse {
	if len(errs) == 0 {
		return nil
	}

	blocking, _, _ := categorize(errs)
	resp := &PermissionRequestResponse{SystemMessage: FormatSystemMessage(errs)}

	if len(blocking) == 0 {
		return resp
	}

	resp.HookSpecificOutput = &PermissionRequestOutput{
		HookEventName: permissionRequestEventName,
		Decision: &PermissionRequestDecision{
			Behavior: decisionDeny,
			Message:  formatDecisionReasonWithin(blocking, budget),
		},
	}

	return resp
}

// BuildElicitation constructs an Elicitation or ElicitationResult response.
// A blocking finding sends both decline forms Claude accepts: the top-level
// decision:block with a reason and hookSpecificOutput.action:decline. Claude
// discards systemMessage here, so warnings produce no output.
func BuildElicitation(
	hookCtx *hook.Context,
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

	eventName := ""
	if hookCtx != nil {
		eventName = hookCtx.EventName()
	}

	return &ElicitationHookResponse{
		HookSpecificOutput: &ElicitationOutput{
			HookEventName: eventName,
			Action:        elicitationDecline,
		},
		Decision: decisionBlock,
		Reason:   formatDecisionReason(blocking),
	}
}

// formatCompletionReason builds the instruction a completion gate hands the
// agent when it keeps the turn going.
func formatCompletionReason(blocking []*dispatcher.ValidationError, budget int) string {
	return completionReasonPrefix +
		formatDecisionReasonWithin(blocking, budget-len(completionReasonPrefix))
}
