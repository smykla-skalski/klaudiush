package hookresponse

const (
	geminiToolModeAuto = "AUTO"
	geminiToolModeNone = "NONE"
)

// BuildGeminiToolSelection answers Gemini BeforeToolSelection with the only
// tools the model may be offered. Mode AUTO keeps the model free to answer
// without a tool; an empty list disables every tool with mode NONE, since
// AUTO with no names would not restrict anything.
func BuildGeminiToolSelection(eventName string, allowed []string) *GeminiCommandResponse {
	toolConfig := &GeminiToolConfig{Mode: geminiToolModeAuto, AllowedFunctionNames: allowed}
	if len(allowed) == 0 {
		toolConfig = &GeminiToolConfig{Mode: geminiToolModeNone, AllowedFunctionNames: []string{}}
	}

	return &GeminiCommandResponse{
		HookSpecificOutput: &GeminiHookSpecificOutput{
			HookEventName: eventName,
			ToolConfig:    toolConfig,
		},
	}
}
