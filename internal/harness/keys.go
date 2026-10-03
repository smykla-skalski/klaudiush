package harness

// JSON keys and values shared by the scripted model APIs and harness configs.
const (
	jsonType         = "type"
	jsonID           = "id"
	jsonIndex        = "index"
	jsonContent      = "content"
	jsonDelta        = "delta"
	jsonText         = "text"
	jsonUsage        = "usage"
	jsonOutputTokens = "output_tokens"
	jsonObject       = "object"
	jsonMessage      = "message"
	jsonModel        = "model"
	jsonName         = "name"
	jsonCommand      = "command"
	jsonDescription  = "description"
	jsonHooks        = "hooks"
	jsonDecision     = "decision"
	jsonMethod       = "method"
	jsonStatus       = "status"
	jsonRole         = "role"
	jsonResponse     = "response"

	hookTypeCommand = "command"

	roleUser      = "user"
	roleAssistant = "assistant"

	blockToolUse    = "tool_use"
	statusCompleted = "completed"
	decisionDeny    = "deny"

	scriptedModelName = "klaudiush-scripted"
	harnessIdentity   = "klaudiush-harness"
	stepDescription   = "harness step"

	toolBash  = "Bash"
	toolWrite = "Write"
)
