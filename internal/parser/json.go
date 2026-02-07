// Package parser provides JSON input parsing for Claude Code hooks.
package parser

import (
	"encoding/json"
	"io"
	"os"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var (
	// ErrEmptyInput is returned when the input is empty.
	ErrEmptyInput = errors.New("empty input")

	// ErrInvalidJSON is returned when the input is not valid JSON.
	ErrInvalidJSON = errors.New("invalid JSON")
)

// JSONInput represents the raw JSON input structure.
type JSONInput struct {
	ToolName         string          `json:"tool_name,omitempty"`
	Tool             string          `json:"tool,omitempty"`
	ToolInput        json.RawMessage `json:"tool_input,omitempty"`
	Command          string          `json:"command,omitempty"`
	NotificationType string          `json:"notification_type,omitempty"`
	SessionID        string          `json:"session_id,omitempty"`
	ToolUseID        string          `json:"tool_use_id,omitempty"`
	TranscriptPath   string          `json:"transcript_path,omitempty"`
}

// JSONParser parses JSON input from stdin or environment variable.
type JSONParser struct {
	reader io.Reader
}

// NewJSONParser creates a new JSONParser that reads from the given reader.
func NewJSONParser(reader io.Reader) *JSONParser {
	return &JSONParser{
		reader: reader,
	}
}

// Parse parses the JSON input and extracts the hook context.
func (p *JSONParser) Parse(eventType hook.EventType) (*hook.Context, error) {
	// Try reading from stdin
	jsonBytes, err := io.ReadAll(p.reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read input")
	}

	// If stdin is empty, try environment variable
	if len(jsonBytes) == 0 {
		envInput := os.Getenv("CLAUDE_TOOL_INPUT")
		if envInput == "" {
			return nil, ErrEmptyInput
		}

		jsonBytes = []byte(envInput)
	}

	// Parse JSON
	var input JSONInput

	if unmarshalErr := json.Unmarshal(jsonBytes, &input); unmarshalErr != nil {
		return nil, errors.CombineErrors(ErrInvalidJSON, unmarshalErr)
	}

	// Extract tool name
	toolName := input.ToolName
	if toolName == "" {
		toolName = input.Tool
	}

	// Parse tool input
	var toolInput hook.ToolInput

	if len(input.ToolInput) > 0 {
		if unmarshalErr := json.Unmarshal(input.ToolInput, &toolInput); unmarshalErr != nil {
			// If tool_input fails to parse, try extracting command directly
			toolInput.Command = input.Command
		}
	} else {
		// No tool_input, use top-level command
		toolInput.Command = input.Command
	}

	// Parse tool type from string using enumer-generated function
	parsedToolType, parseErr := hook.ToolTypeString(toolName)
	if parseErr != nil {
		// Unknown tool type - use ToolTypeUnknown to allow pass-through
		parsedToolType = hook.ToolTypeUnknown
	}

	ctx := &hook.Context{
		EventType:        eventType,
		ToolName:         parsedToolType,
		ToolInput:        toolInput,
		NotificationType: input.NotificationType,
		RawJSON:          string(jsonBytes),
		SessionID:        input.SessionID,
		ToolUseID:        input.ToolUseID,
		TranscriptPath:   input.TranscriptPath,
	}

	return ctx, nil
}
