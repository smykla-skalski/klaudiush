// Package hookresponse builds structured JSON responses for Claude Code hooks.
package hookresponse

// HookResponse is the top-level JSON structure written to stdout.
type HookResponse struct {
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
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
type CodexCommandResponse struct {
	Continue           bool                     `json:"continue"`
	HookSpecificOutput *CodexHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string                   `json:"decision,omitempty"`
	Reason             string                   `json:"reason,omitempty"`
	StopReason         string                   `json:"stopReason,omitempty"`
	SuppressOutput     bool                     `json:"suppressOutput,omitempty"`
	SystemMessage      string                   `json:"systemMessage,omitempty"`
}

// CodexHookSpecificOutput carries model-facing additional context for Codex hooks.
type CodexHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
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
