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

	// ResponseFieldPermissionBehavior is hookSpecificOutput.decision, the
	// PermissionRequest object carrying behavior and message.
	ResponseFieldPermissionBehavior ResponseField = "decision.behavior"

	// ResponseFieldElicitationAction is hookSpecificOutput.action, the
	// accept/decline/cancel answer to an MCP elicitation.
	ResponseFieldElicitationAction ResponseField = "action"
)

// Enforcement is the strongest effect a blocking finding can have on an event.
type Enforcement string

const (
	EnforcementNone          Enforcement = "none"
	EnforcementDenyTool      Enforcement = "deny_tool"
	EnforcementBlockDecision Enforcement = "block_decision"
	EnforcementStop          Enforcement = "stop"

	// EnforcementContinueTurn marks a completion gate: a block keeps the agent
	// working and hands it the reason as its next instruction.
	EnforcementContinueTurn Enforcement = "continue_turn"

	// EnforcementDenyPermission denies an approval request on the user's behalf.
	EnforcementDenyPermission Enforcement = "deny_permission"

	// EnforcementDeclineElicitation declines an MCP elicitation.
	EnforcementDeclineElicitation Enforcement = "decline_elicitation"
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
		Enforcement: EnforcementContinueTurn,
	},
	CanonicalEventSubagentStop: {
		NativeName: eventNameSubagentStop,
		Fields: []ResponseField{
			ResponseFieldDecision,
			ResponseFieldContinue,
			ResponseFieldStopReason,
			ResponseFieldSystemMessage,
		},
		Enforcement: EnforcementContinueTurn,
	},
	// SessionEnd is advisory and its accepted output is not documented
	// consistently, so nothing is emitted for it.
	CanonicalEventSessionEnd: {
		NativeName:  eventNameSessionEnd,
		Enforcement: EnforcementNone,
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

// codexPermissionRequest is keyed by raw name: PermissionRequest shares
// before_tool with PreToolUse so the pre-tool validators run for it, but its
// response is a decision object, not a permissionDecision.
var codexPermissionRequest = EventCapability{
	NativeName: eventNamePermissionRequest,
	Fields: []ResponseField{
		ResponseFieldSystemMessage,
		ResponseFieldPermissionBehavior,
	},
	Enforcement: EnforcementDenyPermission,
}

// Source: https://code.claude.com/docs/en/hooks.
var claudeCapabilities = map[CanonicalEvent]EventCapability{
	CanonicalEventBeforeTool: {
		NativeName: codexEventPreToolUse,
		Fields: []ResponseField{
			ResponseFieldContinue,
			ResponseFieldStopReason,
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
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementBlockDecision,
	},
	CanonicalEventUserPromptSubmit: {
		NativeName: eventNameUserPromptSubmit,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementBlockDecision,
	},
	CanonicalEventTurnStop: {
		NativeName: eventNameStop,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementContinueTurn,
	},
	CanonicalEventSubagentStop: {
		NativeName: eventNameSubagentStop,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementContinueTurn,
	},
	CanonicalEventSessionStart: {
		NativeName: eventNameSessionStart,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementNone,
	},
	CanonicalEventNotification: {
		NativeName:  EventTypeNotification.String(),
		Fields:      []ResponseField{ResponseFieldSystemMessage},
		Enforcement: EnforcementNone,
	},
	CanonicalEventPostCompact: {
		NativeName:  displayPostCompact,
		Fields:      []ResponseField{ResponseFieldSystemMessage},
		Enforcement: EnforcementNone,
	},
	// Claude discards every output field of these two events.
	CanonicalEventSessionEnd: {
		NativeName:  eventNameSessionEnd,
		Enforcement: EnforcementNone,
	},
	CanonicalEventStopFailure: {
		NativeName:  eventNameStopFailure,
		Enforcement: EnforcementNone,
	},
	// Claude acts only on hookSpecificOutput here and drops systemMessage.
	CanonicalEventElicitation: {
		NativeName:  displayElicitation,
		Fields:      []ResponseField{ResponseFieldElicitationAction},
		Enforcement: EnforcementDeclineElicitation,
	},
	CanonicalEventElicitationResult: {
		NativeName:  displayElicitationResult,
		Fields:      []ResponseField{ResponseFieldElicitationAction},
		Enforcement: EnforcementDeclineElicitation,
	},
}

var claudePermissionRequest = EventCapability{
	NativeName: eventNamePermissionRequest,
	Fields: []ResponseField{
		ResponseFieldSystemMessage,
		ResponseFieldPermissionBehavior,
	},
	Enforcement: EnforcementDenyPermission,
}

// Source: https://geminicli.com/docs/hooks/reference/. AfterAgent is the
// completion gate; SessionEnd, Notification, and PreCompress ignore every
// flow-control field.
var geminiCapabilities = map[CanonicalEvent]EventCapability{
	CanonicalEventBeforeTool: {
		NativeName: geminiEventBeforeTool,
		Fields: []ResponseField{
			ResponseFieldContinue,
			ResponseFieldStopReason,
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
		},
		Enforcement: EnforcementDenyTool,
	},
	CanonicalEventAfterTool: {
		NativeName: geminiEventAfterTool,
		Fields: []ResponseField{
			ResponseFieldContinue,
			ResponseFieldStopReason,
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementBlockDecision,
	},
	CanonicalEventTurnStop: {
		NativeName: geminiEventAfterAgent,
		Fields: []ResponseField{
			ResponseFieldContinue,
			ResponseFieldStopReason,
			ResponseFieldSystemMessage,
			ResponseFieldDecision,
		},
		Enforcement: EnforcementContinueTurn,
	},
	CanonicalEventSessionStart: {
		NativeName: eventNameSessionStart,
		Fields: []ResponseField{
			ResponseFieldSystemMessage,
			ResponseFieldAdditionalContext,
		},
		Enforcement: EnforcementNone,
	},
	CanonicalEventSessionEnd: {
		NativeName:  eventNameSessionEnd,
		Fields:      []ResponseField{ResponseFieldSystemMessage},
		Enforcement: EnforcementNone,
	},
	CanonicalEventNotification: {
		NativeName:  EventTypeNotification.String(),
		Fields:      []ResponseField{ResponseFieldSystemMessage},
		Enforcement: EnforcementNone,
	},
	CanonicalEventPreCompress: {
		NativeName:  geminiEventPreCompress,
		Fields:      []ResponseField{ResponseFieldSystemMessage},
		Enforcement: EnforcementNone,
	},
}

// ProviderEventCapability returns the documented response contract for a
// provider/event pair. It returns false when no contract is recorded; callers
// should then emit nothing beyond a systemMessage.
func ProviderEventCapability(provider Provider, event CanonicalEvent) (EventCapability, bool) {
	var table map[CanonicalEvent]EventCapability

	switch provider {
	case ProviderCodex:
		table = codexCapabilities
	case ProviderClaude:
		table = claudeCapabilities
	case ProviderGemini:
		table = geminiCapabilities
	case ProviderUnknown, ProviderOpenCode:
		return EventCapability{}, false
	default:
		return EventCapability{}, false
	}

	capability, ok := table[event]

	return capability, ok
}

// ResolveEventCapability returns the response contract for the native event a
// hook actually received. Raw names that share a canonical event with another
// native event (PermissionRequest, Codex SubagentStart) resolve to their own
// contract, or to none, instead of the canonical event's.
func ResolveEventCapability(
	provider Provider,
	event CanonicalEvent,
	rawEventName string,
) (EventCapability, bool) {
	if IsPermissionRequestEvent(rawEventName) {
		switch provider {
		case ProviderClaude:
			return claudePermissionRequest, true
		case ProviderCodex:
			return codexPermissionRequest, true
		case ProviderUnknown, ProviderGemini, ProviderOpenCode:
			return EventCapability{}, false
		default:
			return EventCapability{}, false
		}
	}

	if provider == ProviderCodex && IsCodexAliasedEvent(rawEventName) {
		return EventCapability{}, false
	}

	return ProviderEventCapability(provider, event)
}

// IsCompletionGate reports whether a blocking finding on this event keeps the
// agent working instead of letting the turn end.
func IsCompletionGate(provider Provider, event CanonicalEvent, rawEventName string) bool {
	capability, ok := ResolveEventCapability(provider, event, rawEventName)

	return ok && capability.Enforcement == EnforcementContinueTurn
}

// IsPermissionRequestEvent reports whether a raw event name is a Claude or
// Codex PermissionRequest (an approval prompt, not a tool gate).
func IsPermissionRequestEvent(rawEventName string) bool {
	return normalizeToken(rawEventName) == tokenPermissionRequest
}

// KeepsRawEventName reports whether a raw event name must survive parsing
// because its canonical event's display name belongs to a different native
// event with a different response contract.
func KeepsRawEventName(provider Provider, rawEventName string) bool {
	switch provider {
	case ProviderCodex:
		return IsCodexAliasedEvent(rawEventName)
	case ProviderClaude:
		return IsPermissionRequestEvent(rawEventName)
	case ProviderUnknown, ProviderGemini, ProviderOpenCode:
		return false
	default:
		return false
	}
}

// ToolCoverage names a tool family and the tool names (or matcher aliases)
// that select it, so a hook matcher can be tested against the family.
// FamilyProbes is set for open-ended families (MCP, local functions): made-up
// names that only a pattern spanning the whole family selects, so a matcher
// listing a few known names is not mistaken for full coverage.
type ToolCoverage struct {
	Label        string
	ProbeNames   []string
	FamilyProbes []string
}

// CodexPreToolCoverage lists the tool families Codex routes through PreToolUse.
func CodexPreToolCoverage() []ToolCoverage {
	return []ToolCoverage{
		{Label: "shell (Bash)", ProbeNames: []string{"Bash"}},
		{Label: "apply_patch", ProbeNames: []string{"apply_patch", "Edit", "Write"}},
		{
			Label:        "MCP tools",
			ProbeNames:   []string{"mcp__server__tool"},
			FamilyProbes: []string{"mcp__klaudiush_probe__q7zx_tool", "mcp__zz9__probe"},
		},
		{
			Label:        "local function tools",
			ProbeNames:   []string{"update_plan"},
			FamilyProbes: []string{"klaudiush_probe_q7zx", "zz9_probe_fn"},
		},
	}
}

// CodexUncoveredTools lists Codex tools that never reach command hooks.
func CodexUncoveredTools() []string {
	return []string{"hosted tools (web search)"}
}

// IsCodexAliasedEvent reports whether a raw Codex event name normalizes onto a
// canonical event whose Codex contract belongs to a different event, so a
// response shaped for the canonical event would be invalid for it.
func IsCodexAliasedEvent(rawEventName string) bool {
	switch normalizeToken(rawEventName) {
	case tokenPermissionRequest, "subagentstart":
		return true
	default:
		return false
	}
}
