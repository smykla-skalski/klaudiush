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

// ParseProvider parses a provider string.
func ParseProvider(s string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", string(ProviderClaude):
		return ProviderClaude, nil
	case string(ProviderCodex):
		return ProviderCodex, nil
	default:
		return ProviderUnknown, errors.Newf("unknown provider %q", s)
	}
}

// NormalizeEventName converts provider-specific event names to canonical names.
func NormalizeEventName(name string) CanonicalEvent {
	switch normalizeToken(name) {
	case "beforetool", "pretooluse":
		return CanonicalEventBeforeTool
	case "aftertool", "posttooluse", "aftertooluse":
		return CanonicalEventAfterTool
	case "sessionstart":
		return CanonicalEventSessionStart
	case "turnstop", "stop":
		return CanonicalEventTurnStop
	case "notification":
		return CanonicalEventNotification
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
	case CanonicalEventUnknown, CanonicalEventSessionStart, CanonicalEventTurnStop:
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
	default:
		return ""
	}
}

// DisplayEventName returns the provider-specific event name to emit back.
func DisplayEventName(provider Provider, canonical CanonicalEvent, fallback EventType) string {
	switch provider {
	case ProviderUnknown:
	case ProviderCodex:
		switch canonical {
		case CanonicalEventUnknown:
		case CanonicalEventSessionStart:
			return "SessionStart"
		case CanonicalEventTurnStop:
			return "Stop"
		case CanonicalEventAfterTool:
			return "AfterToolUse"
		case CanonicalEventNotification:
			return "Notification"
		case CanonicalEventBeforeTool:
			return "BeforeToolUse"
		}
	case ProviderClaude:
		switch canonical {
		case CanonicalEventUnknown, CanonicalEventSessionStart, CanonicalEventTurnStop:
		case CanonicalEventBeforeTool:
			return EventTypePreToolUse.String()
		case CanonicalEventAfterTool:
			return EventTypePostToolUse.String()
		case CanonicalEventNotification:
			return EventTypeNotification.String()
		}
	}

	if fallback != EventTypeUnknown {
		return fallback.String()
	}

	return ""
}

// ResolveToolMetadata maps a raw tool name onto the legacy enum and canonical family.
func ResolveToolMetadata(rawToolName string) (ToolType, ToolFamily) {
	switch normalizeToken(rawToolName) {
	case "bash", "execcommand", "runusershellcommand", "shell":
		return ToolTypeBash, ToolFamilyShell
	case "write", "writefile":
		return ToolTypeWrite, ToolFamilyWrite
	case "edit", "applypatch":
		return ToolTypeEdit, ToolFamilyEdit
	case "multiedit", "multifileedit":
		return ToolTypeMultiEdit, ToolFamilyMultiEdit
	case "grep", "search":
		return ToolTypeGrep, ToolFamilyGrep
	case "read", "viewimage":
		return ToolTypeRead, ToolFamilyRead
	case "glob", "listfiles":
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

func normalizeToken(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")

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
