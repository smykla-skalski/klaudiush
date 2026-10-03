package harness

import (
	"encoding/json"
	"net/http"
)

type anthropicRequest struct {
	Stream   bool `json:"stream"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// anthropicState finds the script in the first user message that carries one
// and counts the tool_use blocks the assistant made after it.
func anthropicState(req anthropicRequest) (Script, bool, int) {
	var (
		script Script
		found  bool
		calls  int
	)

	for _, message := range req.Messages {
		if !found {
			if message.Role == roleUser {
				script, found = findScript(textOf(message.Content))
			}

			continue
		}

		if message.Role != roleAssistant {
			continue
		}

		var blocks []struct {
			Type string `json:"type"`
		}

		if err := json.Unmarshal(message.Content, &blocks); err != nil {
			continue
		}

		for _, block := range blocks {
			if block.Type == blockToolUse {
				calls++
			}
		}
	}

	return script, found, calls
}

type anthropicBlock struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	Text  *string         `json:"text,omitempty"`
}

func (m *ScriptedModel) serveAnthropic(w http.ResponseWriter, body []byte) {
	var req anthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	script, found, made := anthropicState(req)
	calls, text := plan(script, found, made)

	content := make([]anthropicBlock, 0, len(calls)+1)
	stopReason := "end_turn"

	if len(calls) == 0 {
		content = append(content, anthropicBlock{Type: "text", Text: &text})
	}

	for _, call := range calls {
		content = append(content, anthropicBlock{
			Type:  blockToolUse,
			ID:    m.nextID("toolu"),
			Name:  call.Tool,
			Input: json.RawMessage(encodeArgs(call)),
		})
		stopReason = blockToolUse
	}

	id := m.nextID("msg")
	message := func(blocks []anthropicBlock, stop any) map[string]any {
		return map[string]any{
			jsonID: id, jsonType: jsonMessage, jsonRole: roleAssistant,
			jsonModel: scriptedModelName, jsonContent: blocks,
			"stop_reason": stop, "stop_sequence": nil,
			jsonUsage: map[string]any{"input_tokens": 1, jsonOutputTokens: 1},
		}
	}

	if !req.Stream {
		writeJSON(w, message(content, stopReason))

		return
	}

	startSSE(w)
	writeSSE(w, "message_start", map[string]any{
		jsonType: "message_start", jsonMessage: message([]anthropicBlock{}, nil),
	})

	for index, block := range content {
		writeAnthropicBlock(w, index, block)
	}

	writeSSE(w, "message_delta", map[string]any{
		jsonType:  "message_delta",
		jsonDelta: map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		jsonUsage: map[string]any{jsonOutputTokens: 1},
	})
	writeSSE(w, "message_stop", map[string]any{jsonType: "message_stop"})
}

func writeAnthropicBlock(w http.ResponseWriter, index int, block anthropicBlock) {
	start := block

	var delta map[string]any

	if block.Type == blockToolUse {
		start.Input = json.RawMessage("{}")
		delta = map[string]any{jsonType: "input_json_delta", "partial_json": string(block.Input)}
	} else {
		empty := ""
		start.Text = &empty
		delta = map[string]any{jsonType: "text_delta", jsonText: *block.Text}
	}

	writeSSE(w, "content_block_start", map[string]any{
		jsonType: "content_block_start", jsonIndex: index, "content_block": start,
	})
	writeSSE(w, "content_block_delta", map[string]any{
		jsonType: "content_block_delta", jsonIndex: index, jsonDelta: delta,
	})
	writeSSE(
		w,
		"content_block_stop",
		map[string]any{jsonType: "content_block_stop", jsonIndex: index},
	)
}
