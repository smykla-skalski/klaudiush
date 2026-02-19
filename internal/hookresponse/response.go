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
