package hook

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// Provider identifies the hook source provider.
type Provider string

const (
	// ProviderUnknown represents an unknown provider.
	ProviderUnknown Provider = ""

	// ProviderClaude represents Claude Code hook payloads.
	ProviderClaude Provider = "claude"

	// ProviderCodex represents Codex hook payloads.
	ProviderCodex Provider = "codex"

	// ProviderGemini represents Gemini hook payloads.
	ProviderGemini Provider = "gemini"

	// ProviderOpenCode represents opencode hook payloads.
	ProviderOpenCode Provider = "opencode"
)

// CanonicalEvent represents the normalized cross-provider hook event name.
type CanonicalEvent string

const (
	// CanonicalEventUnknown represents an unknown event.
	CanonicalEventUnknown CanonicalEvent = ""

	// CanonicalEventBeforeTool is a pre-tool event.
	CanonicalEventBeforeTool CanonicalEvent = "before_tool"

	// CanonicalEventAfterTool is a post-tool event.
	CanonicalEventAfterTool CanonicalEvent = "after_tool"

	// CanonicalEventSessionStart is a session-start event.
	CanonicalEventSessionStart CanonicalEvent = "session_start"

	// CanonicalEventTurnStop is a turn-stop event.
	CanonicalEventTurnStop CanonicalEvent = "turn_stop"

	// CanonicalEventNotification is a notification event.
	CanonicalEventNotification CanonicalEvent = "notification"

	// CanonicalEventPreCompress is a pre-compress lifecycle event.
	CanonicalEventPreCompress CanonicalEvent = "pre_compress"

	// CanonicalEventElicitation is an MCP elicitation request event.
	CanonicalEventElicitation CanonicalEvent = "elicitation"

	// CanonicalEventElicitationResult is an MCP elicitation result event.
	CanonicalEventElicitationResult CanonicalEvent = "elicitation_result"

	// CanonicalEventPostCompact is a post-compaction lifecycle event.
	CanonicalEventPostCompact CanonicalEvent = "post_compact"

	// CanonicalEventUserPromptSubmit is a user-prompt submission event.
	CanonicalEventUserPromptSubmit CanonicalEvent = "user_prompt_submit"

	// CanonicalEventSubagentStop is a subagent completion gate.
	CanonicalEventSubagentStop CanonicalEvent = "subagent_stop"

	// CanonicalEventSessionEnd is an observational session-end event.
	CanonicalEventSessionEnd CanonicalEvent = "session_end"

	// CanonicalEventStopFailure is an observational turn-failure event (the
	// turn ended on an API or runtime error).
	CanonicalEventStopFailure CanonicalEvent = "stop_failure"

	// CanonicalEventConfigChange reports that a harness settings file changed
	// during the session (Claude ConfigChange).
	CanonicalEventConfigChange CanonicalEvent = "config_change"

	// CanonicalEventToolSelection fires before the model picks tools (Gemini
	// BeforeToolSelection); a response can narrow the tools it is offered.
	CanonicalEventToolSelection CanonicalEvent = "tool_selection"
)

// ToolFamily represents the normalized cross-provider tool family.
type ToolFamily string

const (
	// ToolFamilyUnknown represents an unknown tool family.
	ToolFamilyUnknown ToolFamily = ""

	// ToolFamilyShell represents shell/command execution tools.
	ToolFamilyShell ToolFamily = "shell"

	// ToolFamilyWrite represents file-write tools.
	ToolFamilyWrite ToolFamily = "write"

	// ToolFamilyEdit represents file-edit/patch tools.
	ToolFamilyEdit ToolFamily = "edit"

	// ToolFamilyMultiEdit represents batched file-edit tools.
	ToolFamilyMultiEdit ToolFamily = "multiedit"

	// ToolFamilyGrep represents search tools.
	ToolFamilyGrep ToolFamily = "grep"

	// ToolFamilyRead represents read/view tools.
	ToolFamilyRead ToolFamily = "read"

	// ToolFamilyGlob represents glob/list-files tools.
	ToolFamilyGlob ToolFamily = "glob"
)

// Display name constants for event names used across multiple providers.
const (
	displayElicitation       = "Elicitation"
	displayElicitationResult = "ElicitationResult"
	displayPostCompact       = "PostCompact"

	eventNameSessionStart      = "SessionStart"
	eventNameSessionEnd        = "SessionEnd"
	eventNameStop              = "Stop"
	eventNameSubagentStop      = "SubagentStop"
	eventNameStopFailure       = "StopFailure"
	eventNameConfigChange      = "ConfigChange"
	eventNamePermissionRequest = "PermissionRequest"
	geminiEventBeforeTool      = "BeforeTool"
	geminiEventAfterTool       = "AfterTool"
	geminiEventAfterAgent      = "AfterAgent"
	geminiEventPreCompress     = "PreCompress"
	geminiEventToolSelection   = "BeforeToolSelection"
	eventNameUserPromptSubmit  = "UserPromptSubmit"
	codexEventPreToolUse       = "PreToolUse"
	codexEventPostToolUse      = "PostToolUse"
)

// Normalized event-name tokens accepted by NormalizeEventName.
const (
	tokenElicitation       = "elicitation"
	tokenPostCompress      = "postcompress"
	tokenPermissionRequest = "permissionrequest"
	tokenSubagentStart     = "subagentstart"
)

// opencode hook identifiers. These are the plugin hook names opencode itself
// uses, so the bridge plugin, the doctor checks, and the response echo all
// speak one vocabulary.
const (
	openCodeEventBeforeTool       = "tool.execute.before"
	openCodeEventAfterTool        = "tool.execute.after"
	openCodeEventUserPromptSubmit = "chat.message"
	openCodeEventSessionStart     = "session.created"
	openCodeEventTurnStop         = "session.idle"
	openCodeEventNotification     = "permission.asked"
	openCodeEventPreCompress      = "session.compacting"
	openCodeEventPostCompact      = "session.compacted"
)

// OpenCodeEventNames returns the opencode hook identifiers the bridge plugin
// forwards to klaudiush, in registration order.
//
// permission.ask is deliberately absent. opencode gates every tool call through
// tool.execute.before regardless of approval outcome, so forwarding the
// approval prompt as well would validate each call twice and double-charge the
// exception rate limiter for one operation.
func OpenCodeEventNames() []string {
	return []string{
		openCodeEventBeforeTool,
		openCodeEventAfterTool,
		openCodeEventUserPromptSubmit,
		openCodeEventSessionStart,
		openCodeEventTurnStop,
		openCodeEventNotification,
		openCodeEventPreCompress,
		openCodeEventPostCompact,
	}
}

// ParseProvider parses a provider string.
func ParseProvider(s string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ProviderClaude):
		return ProviderClaude, nil
	case string(ProviderCodex):
		return ProviderCodex, nil
	case string(ProviderGemini):
		return ProviderGemini, nil
	case string(ProviderOpenCode):
		return ProviderOpenCode, nil
	default:
		return ProviderUnknown, errors.Newf("unknown provider %q", s)
	}
}

// NormalizeEventName converts provider-specific event names to canonical names.
func NormalizeEventName(name string) CanonicalEvent {
	switch normalizeToken(name) {
	// An approval request carries the tool and its arguments, so it normalizes
	// onto before_tool: a hand-written plugin that forwards it still gets every
	// pre-execution validator. klaudiush's own bridge does not forward it, to
	// avoid validating the same call twice — see OpenCodeEventNames. Claude and
	// Codex PermissionRequest keep their raw name (KeepsRawEventName) so the
	// response uses the PermissionRequest decision object.
	case "beforetool", "pretooluse", "toolexecutebefore", "permissionask",
		tokenPermissionRequest:
		return CanonicalEventBeforeTool
	case "aftertool", "posttooluse", "aftertooluse", "toolexecuteafter",
		"posttoolusefailure":
		return CanonicalEventAfterTool
	case "sessionstart", "sessioncreated", tokenSubagentStart:
		return CanonicalEventSessionStart
	// Only events that can keep the agent working are completion gates.
	// Session end and turn failure are observational, and a subagent stop
	// gates the subagent, not the session.
	case "turnstop", "stop", "sessionidle", "afteragent":
		return CanonicalEventTurnStop
	case "subagentstop":
		return CanonicalEventSubagentStop
	case "sessionend":
		return CanonicalEventSessionEnd
	case "stopfailure", "sessionerror":
		return CanonicalEventStopFailure
	// A pending approval means the session is waiting on the user, which is what
	// a Claude Notification reports. opencode has renamed this event across
	// versions, so both spellings are accepted.
	case "notification", "permissionasked", "permissionupdated":
		return CanonicalEventNotification
	case "precompress", "sessioncompacting":
		return CanonicalEventPreCompress
	case "userpromptsubmit", "chatmessage":
		return CanonicalEventUserPromptSubmit
	case tokenElicitation:
		return CanonicalEventElicitation
	case "elicitationresult":
		return CanonicalEventElicitationResult
	case "postcompact", tokenPostCompress, "sessioncompacted":
		return CanonicalEventPostCompact
	case "configchange":
		return CanonicalEventConfigChange
	case "beforetoolselection", "toolselection":
		return CanonicalEventToolSelection
	default:
		return CanonicalEventUnknown
	}
}

// ResolveLegacyEventType maps canonical/provider event names onto the legacy enum.
func ResolveLegacyEventType(
	provider Provider,
	rawEventName string,
	fallback EventType,
) EventType {
	canonical := NormalizeEventName(rawEventName)

	switch canonical {
	case CanonicalEventUnknown, CanonicalEventSessionStart, CanonicalEventTurnStop,
		CanonicalEventPreCompress, CanonicalEventElicitation, CanonicalEventElicitationResult,
		CanonicalEventPostCompact, CanonicalEventUserPromptSubmit,
		CanonicalEventSubagentStop, CanonicalEventSessionEnd, CanonicalEventStopFailure,
		CanonicalEventConfigChange, CanonicalEventToolSelection:
	case CanonicalEventBeforeTool:
		return EventTypePreToolUse
	case CanonicalEventAfterTool:
		return EventTypePostToolUse
	case CanonicalEventNotification:
		return EventTypeNotification
	}

	if fallback != EventTypeUnknown {
		return fallback
	}

	if provider == ProviderClaude && rawEventName == "" {
		return EventTypePreToolUse
	}

	return EventTypeUnknown
}

// DefaultEventName returns the provider-specific default event name.
func DefaultEventName(provider Provider) string {
	switch provider {
	case ProviderUnknown:
		return ""
	case ProviderClaude:
		return EventTypePreToolUse.String()
	case ProviderGemini:
		return "BeforeTool"
	case ProviderOpenCode:
		return openCodeEventBeforeTool
	default:
		return ""
	}
}

// DisplayEventName returns the provider-specific event name to emit back.
func DisplayEventName(provider Provider, canonical CanonicalEvent, fallback EventType) string {
	var name string

	switch provider {
	case ProviderUnknown:
	case ProviderCodex:
		name = displayCodexEvent(canonical)
	case ProviderGemini:
		name = displayGeminiEvent(canonical)
	case ProviderOpenCode:
		name = displayOpenCodeEvent(canonical)
	case ProviderClaude:
		name = displayClaudeEvent(canonical)
	}

	if name != "" {
		return name
	}

	if fallback != EventTypeUnknown {
		return fallback.String()
	}

	return ""
}

func displayCodexEvent(canonical CanonicalEvent) string {
	capability, ok := codexCapabilities[canonical]
	if !ok {
		return ""
	}

	return capability.NativeName
}

func displayGeminiEvent(canonical CanonicalEvent) string {
	switch canonical {
	case CanonicalEventElicitation:
		return displayElicitation
	case CanonicalEventElicitationResult:
		return displayElicitationResult
	case CanonicalEventPostCompact:
		return displayPostCompact
	default:
		capability, ok := geminiCapabilities[canonical]
		if !ok {
			return ""
		}

		return capability.NativeName
	}
}

func displayOpenCodeEvent(canonical CanonicalEvent) string {
	switch canonical {
	case CanonicalEventElicitation:
		return displayElicitation
	case CanonicalEventElicitationResult:
		return displayElicitationResult
	case CanonicalEventBeforeTool:
		return openCodeEventBeforeTool
	case CanonicalEventAfterTool:
		return openCodeEventAfterTool
	case CanonicalEventUserPromptSubmit:
		return openCodeEventUserPromptSubmit
	case CanonicalEventSessionStart:
		return openCodeEventSessionStart
	case CanonicalEventTurnStop:
		return openCodeEventTurnStop
	case CanonicalEventNotification:
		return openCodeEventNotification
	case CanonicalEventPreCompress:
		return openCodeEventPreCompress
	case CanonicalEventPostCompact:
		return openCodeEventPostCompact
	default:
		return ""
	}
}

func displayClaudeEvent(canonical CanonicalEvent) string {
	capability, ok := claudeCapabilities[canonical]
	if !ok {
		return ""
	}

	return capability.NativeName
}

// ResolveToolMetadata maps a raw tool name onto the legacy enum and canonical family.
func ResolveToolMetadata(rawToolName string) (ToolType, ToolFamily) {
	switch normalizeToken(rawToolName) {
	case "bash", "execcommand", "runusershellcommand", "runshellcommand", "shell":
		return ToolTypeBash, ToolFamilyShell
	case "write", "writefile":
		return ToolTypeWrite, ToolFamilyWrite
	case "edit", "applypatch", "replace", "patch":
		return ToolTypeEdit, ToolFamilyEdit
	case "multiedit", "multifileedit":
		return ToolTypeMultiEdit, ToolFamilyMultiEdit
	case "grep", "search":
		return ToolTypeGrep, ToolFamilyGrep
	case "read", "readfile", "viewimage":
		return ToolTypeRead, ToolFamilyRead
	case "glob", "listfiles", "ls", "list":
		return ToolTypeGlob, ToolFamilyGlob
	default:
		if toolType, err := ToolTypeString(rawToolName); err == nil {
			return toolType, toolFamilyFromToolType(toolType)
		}

		return ToolTypeUnknown, ToolFamilyUnknown
	}
}

func toolFamilyFromToolType(toolType ToolType) ToolFamily {
	switch toolType {
	case ToolTypeBash:
		return ToolFamilyShell
	case ToolTypeWrite:
		return ToolFamilyWrite
	case ToolTypeEdit:
		return ToolFamilyEdit
	case ToolTypeMultiEdit:
		return ToolFamilyMultiEdit
	case ToolTypeGrep:
		return ToolFamilyGrep
	case ToolTypeRead:
		return ToolFamilyRead
	case ToolTypeGlob:
		return ToolFamilyGlob
	default:
		return ToolFamilyUnknown
	}
}

// normalizeToken folds provider event and tool spellings onto a single token.
// Dots are stripped so opencode's dotted hook ids ("tool.execute.before")
// compare equal to their undotted counterparts.
func normalizeToken(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ".", "")

	return s
}

func appendUniqueFold(values []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}

	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}

	return append(values, value)
}
