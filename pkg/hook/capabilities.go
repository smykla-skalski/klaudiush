package hook

import "slices"

// ResponseField names a response field a provider may accept, either at the
// top level or inside hookSpecificOutput.
type ResponseField string

const (
	ResponseFieldContinue           ResponseField = "continue"
	ResponseFieldStopReason         ResponseField = "stopReason"
	ResponseFieldSuppressOutput     ResponseField = "suppressOutput"
	ResponseFieldSystemMessage      ResponseField = "systemMessage"
	ResponseFieldDecision           ResponseField = "decision"
	ResponseFieldPermissionDecision ResponseField = "permissionDecision"
	ResponseFieldAdditionalContext  ResponseField = "additionalContext"
)

// Enforcement is the strongest effect a blocking finding can have on an event.
type Enforcement string

const (
	EnforcementNone          Enforcement = "none"
	EnforcementDenyTool      Enforcement = "deny_tool"
	EnforcementBlockDecision Enforcement = "block_decision"
	EnforcementStop          Enforcement = "stop"
)

// EventCapability lists what a provider accepts in a response to one event.
// Fields outside the list must be omitted: Codex treats an unsupported
// PreToolUse field as a failed hook and then runs the tool anyway.
type EventCapability struct {
	NativeName  string
	Fields      []ResponseField
	Enforcement Enforcement
}

// Supports reports whether the provider accepts the field on this event.
func (c EventCapability) Supports(field ResponseField) bool {
	return slices.Contains(c.Fields, field)
}

// Source: https://developers.openai.com/codex/hooks. Codex has no
// Notification or Elicitation events.
var codexCapabilities = map[CanonicalEvent]EventCapability{
	CanonicalEventBeforeTool: {
		NativeName: codexEventPreToolUse,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldPermissionDecision,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementDenyTool,
	},
	CanonicalEventAfterTool: {
		NativeName: codexEventPostToolUse,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
			ResponseFieldContinue,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementBlockDecision,
	},
	CanonicalEventSessionStart: {
		NativeName: eventNameSessionStart,
		Fields: []ResponseField{
			ResponseFieldContinue,
			ResponseFieldStopReason,
			ResponseFieldSystemMessage,
			ResponseFieldSuppressOutput,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementStop,
	},
	CanonicalEventUserPromptSubmit: {
		NativeName: eventNameUserPromptSubmit,
		Fields: []ResponseField{
			ResponseFieldDecision,
			ResponseFieldSystemMessage,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementBlockDecision,
	},
	CanonicalEventTurnStop: {
		NativeName: eventNameStop,
		Fields: []ResponseField{
			ResponseFieldDecision,
			ResponseFieldContinue,
			ResponseFieldSystemMessage,
		},
		Enforcement: EnforcementBlockDecision,
	},
	CanonicalEventPostCompact: {
		NativeName: displayPostCompact,
		Fields: []ResponseField{
			ResponseFieldContinue,
			ResponseFieldStopReason,
			ResponseFieldSystemMessage,
		},
		Enforcement: EnforcementStop,
	},
}

// ProviderEventCapability returns the documented response contract for a
// provider/event pair. It returns false when no contract is recorded; callers
// should then emit nothing beyond a systemMessage.
func ProviderEventCapability(provider Provider, event CanonicalEvent) (EventCapability, bool) {
	switch provider {
	case ProviderCodex:
		capability, ok := codexCapabilities[event]

		return capability, ok
	case ProviderUnknown, ProviderClaude, ProviderGemini, ProviderOpenCode:
		return EventCapability{}, false
	default:
		return EventCapability{}, false
	}
}

// ToolCoverage names a tool family and the tool names (or matcher aliases)
// that select it, so a hook matcher can be tested against the family.
type ToolCoverage struct {
	Label      string
	ProbeNames []string
}

// CodexPreToolCoverage lists the tool families Codex routes through PreToolUse.
func CodexPreToolCoverage() []ToolCoverage {
	return []ToolCoverage{
		{Label: "shell (Bash)", ProbeNames: []string{"Bash"}},
		{Label: "apply_patch", ProbeNames: []string{"apply_patch", "Edit", "Write"}},
		{Label: "MCP tools", ProbeNames: []string{"mcp__server__tool"}},
		{Label: "local function tools", ProbeNames: []string{"update_plan"}},
	}
}

// CodexUncoveredTools lists Codex tools that never reach command hooks.
func CodexUncoveredTools() []string {
	return []string{"hosted tools (web search)"}
}
