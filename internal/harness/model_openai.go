package harness

import (
	"encoding/json"
	"net/http"
)

type responsesItem struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// responsesState finds the script in the first user message item and counts
// the tool calls made after it.
func responsesState(items []responsesItem) (Script, bool, int) {
	var (
		script Script
		found  bool
		calls  int
	)

	for _, item := range items {
		if !found {
			if item.Type == jsonMessage && item.Role == roleUser {
				script, found = findScript(textOf(item.Content))
			}

			continue
		}

		switch item.Type {
		case "function_call", "custom_tool_call", "local_shell_call":
			calls++
		}
	}

	return script, found, calls
}

func (m *ScriptedModel) serveResponses(w http.ResponseWriter, body []byte) {
	var req struct {
		Input []responsesItem `json:"input"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	script, found, made := responsesState(req.Input)
	calls, text := plan(script, found, made)

	output := make([]map[string]any, 0, len(calls)+1)

	for _, call := range calls {
		item := map[string]any{
			jsonID: m.nextID("fc"), "call_id": m.nextID("call"),
			jsonName: call.Tool, jsonStatus: statusCompleted,
		}

		if call.Input != "" {
			item["type"] = "custom_tool_call"
			item["input"] = call.Input
		} else {
			item["type"] = "function_call"
			item["arguments"] = encodeArgs(call)
		}

		output = append(output, item)
	}

	if len(calls) == 0 {
		output = append(output, map[string]any{
			jsonType:   jsonMessage,
			jsonID:     m.nextID("msg"),
			jsonRole:   roleAssistant,
			jsonStatus: statusCompleted,
			jsonContent: []any{
				map[string]any{jsonType: "output_text", jsonText: text, "annotations": []any{}},
			},
		})
	}

	response := map[string]any{
		jsonID: m.nextID("resp"), jsonObject: jsonResponse, jsonStatus: "in_progress",
		jsonModel: scriptedModelName, "output": []any{},
	}

	startSSE(w)
	writeSSE(
		w,
		"response.created",
		map[string]any{jsonType: "response.created", jsonResponse: response},
	)

	for index, item := range output {
		writeSSE(w, "response.output_item.added", map[string]any{
			jsonType: "response.output_item.added", "output_index": index, "item": item,
		})
		writeSSE(w, "response.output_item.done", map[string]any{
			jsonType: "response.output_item.done", "output_index": index, "item": item,
		})
	}

	response["status"] = statusCompleted
	response["output"] = output
	response["usage"] = map[string]any{
		"input_tokens": 1, jsonOutputTokens: 1, "total_tokens": usageTotal,
		"input_tokens_details":  map[string]any{"cached_tokens": 0},
		"output_tokens_details": map[string]any{"reasoning_tokens": 0},
	}
	writeSSE(
		w,
		"response.completed",
		map[string]any{jsonType: "response.completed", jsonResponse: response},
	)
}

type chatMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls []any           `json:"tool_calls"`
}

// chatState finds the script in the first user message and counts the tool
// calls the assistant made after it.
func chatState(messages []chatMessage) (Script, bool, int) {
	var (
		script Script
		found  bool
		calls  int
	)

	for _, message := range messages {
		if !found {
			if message.Role == roleUser {
				script, found = findScript(textOf(message.Content))
			}

			continue
		}

		if message.Role == roleAssistant {
			calls += len(message.ToolCalls)
		}
	}

	return script, found, calls
}

func (m *ScriptedModel) serveChat(w http.ResponseWriter, body []byte) {
	var req struct {
		Stream   bool          `json:"stream"`
		Messages []chatMessage `json:"messages"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}

	script, found, made := chatState(req.Messages)
	calls, text := plan(script, found, made)

	toolCalls := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		toolCalls = append(toolCalls, map[string]any{
			jsonIndex: index, jsonID: m.nextID("call"), jsonType: "function",
			"function": map[string]any{jsonName: call.Tool, "arguments": encodeArgs(call)},
		})
	}

	finish := "stop"
	message := map[string]any{jsonRole: roleAssistant, jsonContent: text}

	if len(toolCalls) > 0 {
		finish = "tool_calls"
		message = map[string]any{jsonRole: roleAssistant, jsonContent: nil, "tool_calls": toolCalls}
	}

	id := m.nextID("chatcmpl")
	usage := map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": usageTotal}

	if !req.Stream {
		writeJSON(w, map[string]any{
			jsonID:     id,
			jsonObject: "chat.completion",
			"created":  1,
			jsonModel:  scriptedModelName,
			"choices": []any{
				map[string]any{jsonIndex: 0, jsonMessage: message, "finish_reason": finish},
			},
			jsonUsage: usage,
		})

		return
	}

	chunk := func(choice map[string]any, withUsage bool) map[string]any {
		out := map[string]any{
			jsonID: id, jsonObject: "chat.completion.chunk", "created": 1,
			jsonModel: scriptedModelName, "choices": []any{choice},
		}
		if withUsage {
			out["usage"] = usage
		}

		return out
	}

	startSSE(w)
	writeSSE(w, "", chunk(map[string]any{jsonIndex: 0, jsonDelta: message}, false))
	writeSSE(
		w,
		"",
		chunk(
			map[string]any{jsonIndex: 0, jsonDelta: map[string]any{}, "finish_reason": finish},
			true,
		),
	)
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}
