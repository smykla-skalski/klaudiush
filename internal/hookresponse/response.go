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
	PermissionDecision       string `json:"permissionDecision,omitempty"`       // "deny" or unset
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

	// ToolConfig narrows the tools offered to the model (BeforeToolSelection).
	ToolConfig *GeminiToolConfig `json:"toolConfig,omitempty"`
}

// GeminiToolConfig is the BeforeToolSelection tool configuration. Gemini
// unions AllowedFunctionNames across hooks.
type GeminiToolConfig struct {
	Mode                 string   `json:"mode"`
	AllowedFunctionNames []string `json:"allowedFunctionNames"`
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

// ElicitationHookResponse is the response for Elicitation/ElicitationResult
// events. Claude declines on either a top-level decision:block or
// hookSpecificOutput.action:decline, and discards systemMessage and continue
// for these events, so neither is modelled.
type ElicitationHookResponse struct {
	HookSpecificOutput *ElicitationOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string             `json:"decision,omitempty"`
	Reason             string             `json:"reason,omitempty"`
}

// ElicitationOutput answers an MCP elicitation on the user's behalf.
type ElicitationOutput struct {
	HookEventName string         `json:"hookEventName"`
	Action        string         `json:"action"`
	Content       map[string]any `json:"content,omitempty"`
}

// PermissionRequestResponse is the Claude and Codex PermissionRequest
// response: a decision object instead of a PreToolUse permissionDecision.
type PermissionRequestResponse struct {
	HookSpecificOutput *PermissionRequestOutput `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string                   `json:"systemMessage,omitempty"`
}

// PermissionRequestOutput carries the PermissionRequest decision.
type PermissionRequestOutput struct {
	HookEventName string                     `json:"hookEventName"`
	Decision      *PermissionRequestDecision `json:"decision,omitempty"`
}

// PermissionRequestDecision denies (or allows) the approval prompt. Message is
// read only for a deny and tells the agent why.
type PermissionRequestDecision struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}
