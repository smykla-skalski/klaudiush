// Package hook provides core types for Claude Code hook context.
package hook

import "encoding/json"

//go:generate enumer -type=EventType -trimprefix=EventType -json -text -yaml -sql
//go:generate go run github.com/smykla-labs/klaudiush/tools/enumerfix eventtype_enumer.go
//go:generate enumer -type=ToolType -trimprefix=ToolType -json -text -yaml -sql
//go:generate go run github.com/smykla-labs/klaudiush/tools/enumerfix tooltype_enumer.go

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

// Context represents the complete hook invocation context.
type Context struct {
	// EventType is the type of hook event (PreToolUse, PostToolUse, Notification).
	EventType EventType

	// ToolName is the name of the tool being invoked.
	ToolName ToolType

	// ToolInput contains the tool-specific input parameters.
	ToolInput ToolInput

	// NotificationType is the type of notification (for Notification events).
	NotificationType string

	// RawJSON contains the original JSON input for advanced parsing.
	RawJSON string

	// SessionID is the unique identifier for the Claude Code session.
	SessionID string

	// ToolUseID is the unique identifier for this tool invocation.
	ToolUseID string

	// TranscriptPath is the path to the session transcript file.
	TranscriptPath string
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

	return c.ToolInput.Path
}

// GetContent returns the file content from ToolInput.
func (c *Context) GetContent() string {
	return c.ToolInput.Content
}

// IsBashTool returns true if the tool is Bash.
func (c *Context) IsBashTool() bool {
	return c.ToolName == ToolTypeBash
}

// IsFileTool returns true if the tool is a file operation (Write, Edit, MultiEdit).
func (c *Context) IsFileTool() bool {
	return c.ToolName == ToolTypeWrite ||
		c.ToolName == ToolTypeEdit ||
		c.ToolName == ToolTypeMultiEdit
}

// HasSessionID returns true if a session ID is present.
func (c *Context) HasSessionID() bool {
	return c.SessionID != ""
}
