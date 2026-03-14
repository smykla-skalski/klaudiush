// Package hook provides core types for Claude Code hook context.
package hook

import (
	"encoding/json"
	"strings"
)

//go:generate enumer -type=EventType -trimprefix=EventType -json -text -yaml -sql
//go:generate go run github.com/smykla-skalski/klaudiush/tools/enumerfix eventtype_enumer.go
//go:generate enumer -type=ToolType -trimprefix=ToolType -json -text -yaml -sql
//go:generate go run github.com/smykla-skalski/klaudiush/tools/enumerfix tooltype_enumer.go

// EventType represents the type of hook event.
type EventType int

const (
	// EventTypeUnknown represents an unknown event type.
	EventTypeUnknown EventType = iota

	// EventTypePreToolUse is triggered before a tool is executed.
	EventTypePreToolUse

	// EventTypePostToolUse is triggered after a tool is executed.
	EventTypePostToolUse

	// EventTypeNotification is triggered for user notifications.
	EventTypeNotification
)

// ToolType represents the type of tool being used.
type ToolType int

const (
	// ToolTypeUnknown represents an unknown tool type.
	ToolTypeUnknown ToolType = iota

	// ToolTypeBash represents the Bash tool for executing shell commands.
	ToolTypeBash

	// ToolTypeWrite represents the Write tool for creating files.
	ToolTypeWrite

	// ToolTypeEdit represents the Edit tool for modifying files.
	ToolTypeEdit

	// ToolTypeMultiEdit represents the MultiEdit tool for modifying multiple files.
	ToolTypeMultiEdit

	// ToolTypeGrep represents the Grep tool for searching files.
	ToolTypeGrep

	// ToolTypeRead represents the Read tool for reading files.
	ToolTypeRead

	// ToolTypeGlob represents the Glob tool for finding files by pattern.
	ToolTypeGlob
)

// ToolInput contains the raw tool input data.
type ToolInput struct {
	// Command is the shell command for Bash tool.
	Command string `json:"command,omitempty"`

	// FilePath is the file path for file operations.
	FilePath string `json:"file_path,omitempty"`

	// Path is an alternative field for file path.
	Path string `json:"path,omitempty"`

	// Content is the file content for Write tool.
	Content string `json:"content,omitempty"`

	// OldString is the string to replace for Edit tool.
	OldString string `json:"old_string,omitempty"`

	// NewString is the replacement string for Edit tool.
	NewString string `json:"new_string,omitempty"`

	// Pattern is the search pattern for Grep/Glob tools.
	Pattern string `json:"pattern,omitempty"`

	// Additional fields stored as raw JSON.
	Additional map[string]json.RawMessage `json:"-"`
}

// ElicitationInput contains MCP elicitation event data.
type ElicitationInput struct {
	// MCPServerName is the MCP server requesting elicitation.
	MCPServerName string `json:"mcp_server_name,omitempty"`

	// Message is the elicitation prompt message.
	Message string `json:"message,omitempty"`

	// Mode is the elicitation mode ("form" or "url").
	Mode string `json:"mode,omitempty"`

	// URL is the target URL for URL-mode elicitations.
	URL string `json:"url,omitempty"`

	// ElicitationID uniquely identifies this elicitation request.
	ElicitationID string `json:"elicitation_id,omitempty"`

	// RequestedSchema is the JSON schema for form-mode elicitations.
	RequestedSchema json.RawMessage `json:"requested_schema,omitempty"`

	// Action is the user's action on the elicitation (for ElicitationResult).
	Action string `json:"action,omitempty"`

	// Content is the user's response content (for ElicitationResult).
	Content json.RawMessage `json:"content,omitempty"`
}

// Context represents the complete hook invocation context.
type Context struct {
	// Provider identifies the source hook system (Claude, Codex).
	Provider Provider

	// Event is the provider-neutral normalized event name.
	Event CanonicalEvent

	// RawEventName is the original provider event name.
	RawEventName string

	// EventType is the type of hook event (PreToolUse, PostToolUse, Notification).
	EventType EventType

	// RawToolName is the original provider tool name.
	RawToolName string

	// ToolFamily is the provider-neutral normalized tool family.
	ToolFamily ToolFamily

	// ToolName is the name of the tool being invoked.
	ToolName ToolType

	// ToolInput contains the tool-specific input parameters.
	ToolInput ToolInput

	// NotificationType is the type of notification (for Notification events).
	NotificationType string

	// RawJSON contains the original JSON input for advanced parsing.
	RawJSON string

	// WorkingDir is the effective working directory reported by the provider.
	WorkingDir string

	// PermissionMode is the provider-specific permission mode.
	PermissionMode string

	// Model is the active model identifier, when present.
	Model string

	// Source is the provider-specific hook source (for example startup/resume).
	Source string

	// SessionID is the unique identifier for the Claude Code session.
	SessionID string

	// ToolUseID is the unique identifier for this tool invocation.
	ToolUseID string

	// TranscriptPath is the path to the session transcript file.
	TranscriptPath string

	// LastAssistantMessage is populated for stop-style events when available.
	LastAssistantMessage string

	// StopHookActive indicates whether a stop hook is already active.
	StopHookActive bool

	// TurnID identifies the provider turn when available.
	TurnID string

	// ToolExecuted reports whether the provider actually ran the tool.
	ToolExecuted bool

	// ToolSucceeded reports whether the tool completed successfully.
	ToolSucceeded bool

	// ToolMutating reports whether the tool changed repository or filesystem state.
	ToolMutating bool

	// AffectedPaths contains provider-derived file paths affected by the tool.
	AffectedPaths []string

	// Elicitation contains MCP elicitation event data (nil for non-elicitation events).
	Elicitation *ElicitationInput

	// CompactSummary is the summary produced by context compaction (PostCompact only).
	CompactSummary string

	// CompactTrigger is what triggered the compaction (PostCompact only).
	CompactTrigger string
}

// GetCommand returns the command from ToolInput.
func (c *Context) GetCommand() string {
	return c.ToolInput.Command
}

// GetFilePath returns the file path from ToolInput, preferring FilePath over Path.
func (c *Context) GetFilePath() string {
	if c.ToolInput.FilePath != "" {
		return c.ToolInput.FilePath
	}

	if len(c.AffectedPaths) > 0 {
		return c.AffectedPaths[0]
	}

	return c.ToolInput.Path
}

// GetContent returns the file content from ToolInput.
func (c *Context) GetContent() string {
	return c.ToolInput.Content
}

// IsBashTool returns true if the tool is Bash.
func (c *Context) IsBashTool() bool {
	return c.ToolName == ToolTypeBash || c.ToolFamily == ToolFamilyShell
}

// IsFileTool returns true if the tool is a file operation (Write, Edit, MultiEdit).
func (c *Context) IsFileTool() bool {
	return c.ToolName == ToolTypeWrite ||
		c.ToolName == ToolTypeEdit ||
		c.ToolName == ToolTypeMultiEdit ||
		c.ToolFamily == ToolFamilyWrite ||
		c.ToolFamily == ToolFamilyEdit ||
		c.ToolFamily == ToolFamilyMultiEdit
}

// IsElicitationEvent returns true if this is an Elicitation or ElicitationResult event.
func (c *Context) IsElicitationEvent() bool {
	return c.Event == CanonicalEventElicitation || c.Event == CanonicalEventElicitationResult
}

// GetMCPServerName returns the MCP server name from elicitation data.
func (c *Context) GetMCPServerName() string {
	if c.Elicitation != nil {
		return c.Elicitation.MCPServerName
	}

	return ""
}

// HasSessionID returns true if a session ID is present.
func (c *Context) HasSessionID() bool {
	return c.SessionID != ""
}

// GetWorkingDir returns the provider-reported working directory.
func (c *Context) GetWorkingDir() string {
	return c.WorkingDir
}

// EventName returns the provider-specific event name.
func (c *Context) EventName() string {
	if c.RawEventName != "" {
		return c.RawEventName
	}

	return DisplayEventName(c.Provider, c.Event, c.EventType)
}

// EventNames returns the provider-specific and normalized event aliases for matching.
func (c *Context) EventNames() []string {
	var names []string

	names = appendUniqueFold(names, c.RawEventName)
	names = appendUniqueFold(names, c.EventName())
	names = appendUniqueFold(names, string(c.Event))

	if c.EventType != EventTypeUnknown {
		names = appendUniqueFold(names, c.EventType.String())
	}

	switch c.Event {
	case CanonicalEventUnknown:
	case CanonicalEventBeforeTool:
		names = appendUniqueFold(names, "PreToolUse")
	case CanonicalEventAfterTool:
		names = appendUniqueFold(names, "PostToolUse")
		names = appendUniqueFold(names, "AfterToolUse")
	case CanonicalEventSessionStart:
		names = appendUniqueFold(names, "SessionStart")
	case CanonicalEventTurnStop:
		names = appendUniqueFold(names, "Stop")
	case CanonicalEventNotification:
		names = appendUniqueFold(names, "Notification")
	case CanonicalEventPreCompress:
		names = appendUniqueFold(names, "PreCompress")
	case CanonicalEventElicitation:
		names = appendUniqueFold(names, "Elicitation")
	case CanonicalEventElicitationResult:
		names = appendUniqueFold(names, "ElicitationResult")
	case CanonicalEventPostCompact:
		names = appendUniqueFold(names, "PostCompact")
		names = appendUniqueFold(names, "PostCompress")
	}

	return names
}

// ToolNameString returns the provider-specific tool name.
func (c *Context) ToolNameString() string {
	if c.RawToolName != "" {
		return c.RawToolName
	}

	if c.ToolName != ToolTypeUnknown {
		return c.ToolName.String()
	}

	return string(c.ToolFamily)
}

// ToolNames returns the provider-specific and normalized tool aliases for matching.
func (c *Context) ToolNames() []string {
	var names []string

	names = appendUniqueFold(names, c.RawToolName)
	names = appendUniqueFold(names, c.ToolNameString())
	names = appendUniqueFold(names, string(c.ToolFamily))

	if c.ToolName != ToolTypeUnknown {
		names = appendUniqueFold(names, c.ToolName.String())
	}

	switch c.ToolFamily {
	case ToolFamilyUnknown:
	case ToolFamilyShell:
		names = appendUniqueFold(names, "Bash")
	case ToolFamilyWrite:
		names = appendUniqueFold(names, "Write")
	case ToolFamilyEdit:
		names = appendUniqueFold(names, "Edit")
	case ToolFamilyMultiEdit:
		names = appendUniqueFold(names, "MultiEdit")
	case ToolFamilyGrep:
		names = appendUniqueFold(names, "Grep")
	case ToolFamilyRead:
		names = appendUniqueFold(names, "Read")
	case ToolFamilyGlob:
		names = appendUniqueFold(names, "Glob")
	}

	return names
}

// ProviderName returns the normalized provider name.
func (c *Context) ProviderName() string {
	if c.Provider != ProviderUnknown {
		return string(c.Provider)
	}

	return ""
}

// MatchesProvider reports whether the context matches a provider filter.
func (c *Context) MatchesProvider(provider string) bool {
	return strings.EqualFold(c.ProviderName(), provider)
}

// MatchesEventName reports whether the context matches an event alias.
func (c *Context) MatchesEventName(eventName string) bool {
	for _, name := range c.EventNames() {
		if strings.EqualFold(name, eventName) {
			return true
		}
	}

	return false
}

// MatchesToolName reports whether the context matches a tool alias.
func (c *Context) MatchesToolName(toolName string) bool {
	for _, name := range c.ToolNames() {
		if strings.EqualFold(name, toolName) {
			return true
		}
	}

	return false
}
