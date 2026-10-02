// Package hookresponse builds structured JSON responses for Claude Code hooks.
package hookresponse

// HookResponse is the top-level JSON structure written to stdout.
type HookResponse struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

// HookSpecificOutput carries the permission decision and context for Claude.
type HookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`                 // "allow" or "deny"
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"` // shown to Claude
	AdditionalContext        string `json:"additionalContext,omitempty"`        // behavioral framing for Claude
}

// CodexCommandResponse is the top-level JSON structure for Codex command hooks.
// Every field is optional so a response carries only what the event accepts;
// see hook.ProviderEventCapability.
type CodexCommandResponse struct {
	Continue           *bool                    `json:"continue,omitempty"`
	HookSpecificOutput *CodexHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string                   `json:"decision,omitempty"`
	Reason             string                   `json:"reason,omitempty"`
	StopReason         string                   `json:"stopReason,omitempty"`
	SystemMessage      string                   `json:"systemMessage,omitempty"`
}

// CodexHookSpecificOutput carries the PreToolUse permission decision and
// model-facing additional context for Codex hooks.
type CodexHookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

// GeminiCommandResponse is the top-level JSON structure for Gemini command hooks.
type GeminiCommandResponse struct {
	Continue           bool                      `json:"continue,omitempty"`
	HookSpecificOutput *GeminiHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string                    `json:"decision,omitempty"`
	Reason             string                    `json:"reason,omitempty"`
	StopReason         string                    `json:"stopReason,omitempty"`
	SuppressOutput     bool                      `json:"suppressOutput,omitempty"`
	SystemMessage      string                    `json:"systemMessage,omitempty"`
}

// GeminiHookSpecificOutput carries Gemini hook-specific fields.
type GeminiHookSpecificOutput struct {
	HookEventName     string         `json:"hookEventName"`
	AdditionalContext string         `json:"additionalContext,omitempty"`
	ToolInput         map[string]any `json:"tool_input,omitempty"`
}

// OpenCodeCommandResponse is the top-level JSON structure for opencode hooks.
//
// The bridge plugin reads Decision to decide whether to reject a tool call
// (by throwing) or to deny an approval request, and surfaces SystemMessage to
// the user via a toast while AdditionalContext goes back to the model.
type OpenCodeCommandResponse struct {
	Continue           bool                        `json:"continue"`
	HookSpecificOutput *OpenCodeHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string                      `json:"decision,omitempty"`
	Reason             string                      `json:"reason,omitempty"`
	StopReason         string                      `json:"stopReason,omitempty"`
	SystemMessage      string                      `json:"systemMessage,omitempty"`
}

// OpenCodeHookSpecificOutput carries model-facing additional context.
type OpenCodeHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// ElicitationHookResponse is the response for Elicitation/ElicitationResult events.
type ElicitationHookResponse struct {
	Action        string         `json:"action,omitempty"`
	Content       map[string]any `json:"content,omitempty"`
	SystemMessage string         `json:"systemMessage,omitempty"`
}
