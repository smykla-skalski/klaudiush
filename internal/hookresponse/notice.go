package hookresponse

import (
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// BuildNotice creates a minimal response carrying only a user-visible message
// in systemMessage. No permission decision is set, and the AI never sees it.
func BuildNotice(hookCtx *hook.Context, msg string) any {
	if hookCtx != nil && hookCtx.Provider == hook.ProviderCodex {
		return &CodexCommandResponse{
			Continue:      true,
			SystemMessage: msg,
		}
	}

	if hookCtx != nil && hookCtx.Provider == hook.ProviderGemini {
		return &GeminiCommandResponse{
			SystemMessage: msg,
		}
	}

	if hookCtx != nil && hookCtx.Provider == hook.ProviderOpenCode {
		return &OpenCodeCommandResponse{
			Continue:      true,
			SystemMessage: msg,
		}
	}

	return &HookResponse{
		SystemMessage: msg,
	}
}

// AppendNotice appends a user-visible message to an existing response's
// systemMessage field, separated by a blank line.
func AppendNotice(resp any, msg string) {
	if p := systemMessage(resp); p != nil {
		*p = joinNotice(*p, msg)
	}
}

// ClearSystemMessage removes the user-visible message from a built response,
// leaving the decision and model-facing context untouched.
func ClearSystemMessage(resp any) {
	if p := systemMessage(resp); p != nil {
		*p = ""
	}
}

// agentSummaryInstruction asks the agent to explain a denial to the user in plain words.
const agentSummaryInstruction = "In your next message, tell the user in one short, " +
	"plain-language sentence what klaudiush blocked and why. Skip error codes and details."

// AppendAgentSummary asks the agent to explain a denied action to the user.
// Only denials get it: advisory results (after-tool events) let the action
// through, so telling the user it was blocked would be wrong.
func AppendAgentSummary(resp any) {
	r, ok := resp.(*HookResponse)
	if !ok || r.HookSpecificOutput == nil ||
		r.HookSpecificOutput.PermissionDecision != decisionDeny {
		return
	}

	r.HookSpecificOutput.AdditionalContext = strings.TrimSpace(
		r.HookSpecificOutput.AdditionalContext + " " + agentSummaryInstruction,
	)
}

// systemMessage returns a pointer to the response's systemMessage field, or
// nil for unknown types.
func systemMessage(resp any) *string {
	switch r := resp.(type) {
	case *HookResponse:
		return &r.SystemMessage
	case *CodexCommandResponse:
		return &r.SystemMessage
	case *GeminiCommandResponse:
		return &r.SystemMessage
	case *OpenCodeCommandResponse:
		return &r.SystemMessage
	case *ElicitationHookResponse:
		return &r.SystemMessage
	default:
		return nil
	}
}

// IsEmpty reports whether a built response carries nothing to write.
// Builders return typed nil pointers, which never compare equal to a nil
// interface, so callers cannot test the interface value directly.
func IsEmpty(resp any) bool {
	switch r := resp.(type) {
	case nil:
		return true
	case *HookResponse:
		return r == nil
	case *CodexCommandResponse:
		return r == nil
	case *GeminiCommandResponse:
		return r == nil
	case *OpenCodeCommandResponse:
		return r == nil
	case *ElicitationHookResponse:
		return r == nil
	default:
		return false
	}
}

// joinNotice separates messages with a single blank line, skipping empty parts.
func joinNotice(existing, msg string) string {
	if existing == "" {
		return msg
	}

	if msg == "" {
		return existing
	}

	return strings.TrimRight(existing, "\n") + "\n\n" + msg
}
