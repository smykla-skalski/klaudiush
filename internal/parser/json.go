// Package parser provides JSON input parsing for Claude Code hooks.
package parser

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var (
	// ErrEmptyInput is returned when the input is empty.
	ErrEmptyInput = errors.New("empty input")

	// ErrInvalidJSON is returned when the input is not valid JSON.
	ErrInvalidJSON = errors.New("invalid JSON")
)

var patchPathPattern = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$`)

const (
	patchPathSubmatchCount = 2
	patchPathSubmatchIndex = 1
)

// ParseOptions controls provider-aware JSON parsing.
type ParseOptions struct {
	Provider  hook.Provider
	EventType hook.EventType
	EventName string
}

// JSONInput represents the raw JSON input structure.
type JSONInput struct {
	ToolName         string          `json:"tool_name,omitempty"`
	Tool             string          `json:"tool,omitempty"`
	ToolInput        json.RawMessage `json:"tool_input,omitempty"`
	Command          string          `json:"command,omitempty"`
	HookEventName    string          `json:"hook_event_name,omitempty"`
	NotificationType string          `json:"notification_type,omitempty"`
	Cwd              string          `json:"cwd,omitempty"`
	PermissionMode   string          `json:"permission_mode,omitempty"`
	Model            string          `json:"model,omitempty"`
	Source           string          `json:"source,omitempty"`
	SessionID        string          `json:"session_id,omitempty"`
	ToolUseID        string          `json:"tool_use_id,omitempty"`
	TranscriptPath   string          `json:"transcript_path,omitempty"`
	LastAssistant    *string         `json:"last_assistant_message,omitempty"`
	StopHookActive   bool            `json:"stop_hook_active,omitempty"`
	HookEvent        json.RawMessage `json:"hook_event,omitempty"`
}

// CodexAfterToolEvent represents the nested Codex AfterToolUse payload.
type CodexAfterToolEvent struct {
	EventType     string          `json:"event_type,omitempty"`
	TurnID        string          `json:"turn_id,omitempty"`
	CallID        string          `json:"call_id,omitempty"`
	ToolName      string          `json:"tool_name,omitempty"`
	ToolKind      string          `json:"tool_kind,omitempty"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
	ToolExecuted  bool            `json:"tool_executed,omitempty"`
	ToolSucceeded bool            `json:"tool_succeeded,omitempty"`
	ToolMutating  bool            `json:"tool_mutating,omitempty"`
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
	return p.ParseWithOptions(ParseOptions{
		Provider:  hook.ProviderClaude,
		EventType: eventType,
		EventName: eventType.String(),
	})
}

// ParseWithOptions parses provider-aware hook input and extracts the hook context.
func (p *JSONParser) ParseWithOptions(opts ParseOptions) (*hook.Context, error) {
	jsonBytes, input, err := p.readInput(opts)
	if err != nil {
		return nil, err
	}

	provider, rawEventName, eventType, afterTool := resolveEventMetadata(opts, input)
	canonicalEvent := hook.NormalizeEventName(rawEventName)
	toolName, toolInputRaw, toolUseID := extractToolInvocation(input, afterTool)
	toolInput := parseToolInput(toolName, toolInputRaw, input.Command)
	parsedToolType, toolFamily := hook.ResolveToolMetadata(toolName)

	ctx := &hook.Context{
		Provider:         provider,
		Event:            canonicalEvent,
		RawEventName:     hook.DisplayEventName(provider, canonicalEvent, eventType),
		EventType:        opts.EventType,
		RawToolName:      toolName,
		ToolFamily:       toolFamily,
		ToolName:         parsedToolType,
		ToolInput:        toolInput,
		NotificationType: input.NotificationType,
		RawJSON:          string(jsonBytes),
		WorkingDir:       input.Cwd,
		PermissionMode:   input.PermissionMode,
		Model:            input.Model,
		Source:           input.Source,
		SessionID:        input.SessionID,
		ToolUseID:        toolUseID,
		TranscriptPath:   input.TranscriptPath,
		AffectedPaths:    deriveAffectedPaths(toolName, toolInput),
	}

	if ctx.EventType == hook.EventTypeUnknown {
		ctx.EventType = eventType
	}

	if input.LastAssistant != nil {
		ctx.LastAssistantMessage = *input.LastAssistant
	}

	if afterTool != nil {
		ctx.TurnID = afterTool.TurnID
		ctx.ToolExecuted = afterTool.ToolExecuted
		ctx.ToolSucceeded = afterTool.ToolSucceeded
		ctx.ToolMutating = afterTool.ToolMutating
	}

	ctx.StopHookActive = input.StopHookActive

	return ctx, nil
}

func (p *JSONParser) readInput(opts ParseOptions) ([]byte, JSONInput, error) {
	jsonBytes, err := io.ReadAll(p.reader)
	if err != nil {
		return nil, JSONInput{}, errors.Wrap(err, "failed to read input")
	}

	if len(jsonBytes) == 0 {
		envInput := os.Getenv("CLAUDE_TOOL_INPUT")
		if envInput == "" && opts.Provider == hook.ProviderCodex {
			envInput = os.Getenv("CODEX_HOOK_INPUT")
		}

		if envInput == "" {
			return nil, JSONInput{}, ErrEmptyInput
		}

		jsonBytes = []byte(envInput)
	}

	var input JSONInput
	if unmarshalErr := json.Unmarshal(jsonBytes, &input); unmarshalErr != nil {
		return nil, JSONInput{}, errors.CombineErrors(ErrInvalidJSON, unmarshalErr)
	}

	return jsonBytes, input, nil
}

func resolveEventMetadata(
	opts ParseOptions,
	input JSONInput,
) (hook.Provider, string, hook.EventType, *CodexAfterToolEvent) {
	provider := opts.Provider
	if provider == hook.ProviderUnknown {
		provider = inferProvider(opts.EventName, input)
	}

	rawEventName := strings.TrimSpace(opts.EventName)
	if rawEventName == "" {
		rawEventName = input.HookEventName
	}

	codexAfterToolEvent := decodeCodexAfterToolEvent(input.HookEvent)
	if codexAfterToolEvent != nil && rawEventName == "" {
		rawEventName = codexAfterToolEvent.EventType
	}

	if provider == hook.ProviderUnknown {
		provider = inferProvider(rawEventName, input)
	}

	if provider == hook.ProviderUnknown {
		provider = hook.ProviderClaude
	}

	if rawEventName == "" {
		rawEventName = hook.DefaultEventName(provider)
	}

	legacyEventType := hook.ResolveLegacyEventType(provider, rawEventName, opts.EventType)

	return provider, rawEventName, legacyEventType, codexAfterToolEvent
}

func decodeCodexAfterToolEvent(raw json.RawMessage) *CodexAfterToolEvent {
	if len(raw) == 0 {
		return nil
	}

	var event CodexAfterToolEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil
	}

	return &event
}

func extractToolInvocation(
	input JSONInput,
	codexAfterToolEvent *CodexAfterToolEvent,
) (string, json.RawMessage, string) {
	toolName := input.ToolName
	if toolName == "" {
		toolName = input.Tool
	}

	toolInputRaw := input.ToolInput
	toolUseID := input.ToolUseID

	if codexAfterToolEvent == nil {
		return toolName, toolInputRaw, toolUseID
	}

	toolName = codexAfterToolEvent.ToolName

	toolInputRaw = codexAfterToolEvent.ToolInput

	if toolUseID == "" {
		toolUseID = codexAfterToolEvent.CallID
	}

	return toolName, toolInputRaw, toolUseID
}

func inferProvider(eventName string, input JSONInput) hook.Provider {
	if len(input.HookEvent) > 0 {
		return hook.ProviderCodex
	}

	switch hook.NormalizeEventName(eventName) {
	case hook.CanonicalEventSessionStart, hook.CanonicalEventTurnStop, hook.CanonicalEventAfterTool:
		return hook.ProviderCodex
	default:
		return hook.ProviderClaude
	}
}

func parseToolInput(
	rawToolName string,
	inputRaw json.RawMessage,
	fallbackCommand string,
) hook.ToolInput {
	toolInput := hook.ToolInput{
		Command: fallbackCommand,
	}

	if len(inputRaw) == 0 {
		return toolInput
	}

	var values map[string]json.RawMessage
	if err := json.Unmarshal(inputRaw, &values); err != nil {
		return toolInput
	}

	toolInput.Additional = make(map[string]json.RawMessage)

	for key, value := range values {
		switch key {
		case "command":
			_ = json.Unmarshal(value, &toolInput.Command)
		case "file_path":
			_ = json.Unmarshal(value, &toolInput.FilePath)
		case "path":
			_ = json.Unmarshal(value, &toolInput.Path)
		case "content":
			_ = json.Unmarshal(value, &toolInput.Content)
		case "old_string":
			_ = json.Unmarshal(value, &toolInput.OldString)
		case "new_string":
			_ = json.Unmarshal(value, &toolInput.NewString)
		case "pattern":
			_ = json.Unmarshal(value, &toolInput.Pattern)
		case "input":
			assignProviderSpecificInput(&toolInput, rawToolName, value)
			toolInput.Additional[key] = value
		default:
			toolInput.Additional[key] = value
		}
	}

	if len(toolInput.Additional) == 0 {
		toolInput.Additional = nil
	}

	return toolInput
}

func assignProviderSpecificInput(
	toolInput *hook.ToolInput,
	rawToolName string,
	value json.RawMessage,
) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return
	}

	switch normalizeToolName(rawToolName) {
	case "execcommand", "runusershellcommand", "bash", "shell":
		if toolInput.Command == "" {
			toolInput.Command = text
		}
	case "write", "writefile":
		if toolInput.Content == "" {
			toolInput.Content = text
		}
	}
}

func deriveAffectedPaths(rawToolName string, toolInput hook.ToolInput) []string {
	var paths []string

	if toolInput.FilePath != "" {
		paths = append(paths, toolInput.FilePath)
	}

	if toolInput.Path != "" && !strings.EqualFold(toolInput.Path, toolInput.FilePath) {
		paths = append(paths, toolInput.Path)
	}

	paths = append(paths, patchAffectedPaths(rawToolName, toolInput.Additional)...)

	return dedupePaths(paths)
}

func patchAffectedPaths(rawToolName string, additional map[string]json.RawMessage) []string {
	if normalizeToolName(rawToolName) != "applypatch" || additional == nil {
		return nil
	}

	rawInput, ok := additional["input"]
	if !ok {
		return nil
	}

	var patchText string
	if err := json.Unmarshal(rawInput, &patchText); err != nil {
		return nil
	}

	matches := patchPathPattern.FindAllStringSubmatch(patchText, -1)
	paths := make([]string, 0, len(matches))

	for _, match := range matches {
		if len(match) < patchPathSubmatchCount {
			continue
		}

		paths = append(paths, match[patchPathSubmatchIndex])
	}

	return paths
}

func dedupePaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	result := make([]string, 0, len(paths))

	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}

		if !slices.Contains(result, path) {
			result = append(result, path)
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func normalizeToolName(rawToolName string) string {
	rawToolName = strings.ToLower(strings.TrimSpace(rawToolName))
	rawToolName = strings.ReplaceAll(rawToolName, "_", "")
	rawToolName = strings.ReplaceAll(rawToolName, "-", "")

	return rawToolName
}
