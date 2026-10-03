// Package harness checks klaudiush against real coding-agent harnesses:
// contract checks of captured hook payloads and responses against the
// provider capability table (run in CI), and an opt-in live suite that drives
// installed Claude Code, Codex and opencode binaries against a scripted local
// model in a disposable home directory.
package harness

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// ErrContract marks a payload or response a provider would not accept.
var ErrContract = errors.New("harness contract violation")

const (
	keyHookSpecificOutput = "hookSpecificOutput"
	keyHookEventName      = "hookEventName"
	keyReason             = "reason"
	keyPermissionReason   = "permissionDecisionReason"
	keyToolName           = "tool_name"
	keyToolInput          = "tool_input"
	keyEventName          = "hook_event_name"
)

// staleEvents maps event names a provider no longer fires to their
// replacement, so a check can say what to use instead.
var staleEvents = map[hook.Provider]map[string]string{
	hook.ProviderCodex: {"AfterToolUse": "PostToolUse"},
	hook.ProviderOpenCode: {
		"permission.ask":     "tool.execute.before",
		"permission.updated": "permission.asked",
	},
}

// topLevelFields maps top-level response keys onto capability fields.
var topLevelFields = map[string]hook.ResponseField{
	"continue":       hook.ResponseFieldContinue,
	"stopReason":     hook.ResponseFieldStopReason,
	"suppressOutput": hook.ResponseFieldSuppressOutput,
	"systemMessage":  hook.ResponseFieldSystemMessage,
	jsonDecision:     hook.ResponseFieldDecision,
}

// specificFields maps hookSpecificOutput keys onto capability fields.
var specificFields = map[string]hook.ResponseField{
	"permissionDecision": hook.ResponseFieldPermissionDecision,
	"additionalContext":  hook.ResponseFieldAdditionalContext,
	jsonDecision:         hook.ResponseFieldPermissionBehavior,
	"action":             hook.ResponseFieldElicitationAction,
	"toolConfig":         hook.ResponseFieldToolConfig,
}

// permissionDecisions lists the permissionDecision values each provider reads.
var permissionDecisions = map[hook.Provider][]string{
	hook.ProviderClaude: {"allow", decisionDeny, "ask"},
	hook.ProviderCodex:  {"allow", decisionDeny},
}

// openCodeFields are the keys the generated bridge plugin reads.
var openCodeFields = []string{
	"continue", jsonDecision, keyReason, "stopReason", "systemMessage", keyHookSpecificOutput,
}

// CheckEvent fails when a provider does not currently fire the event name.
func CheckEvent(provider hook.Provider, event string) error {
	if slices.Contains(hook.NativeEventNames(provider), event) {
		return nil
	}

	if replacement, ok := staleEvents[provider][event]; ok {
		return errors.Wrapf(ErrContract, "%s event %q is stale, %s fires %q instead",
			provider, event, provider, replacement)
	}

	return errors.Wrapf(ErrContract, "%s does not fire event %q (known: %s)",
		provider, event, strings.Join(hook.NativeEventNames(provider), ", "))
}

// CheckPayload fails when a captured hook input does not look like what the
// provider sends for the event.
func CheckPayload(provider hook.Provider, event string, payload []byte) error {
	if err := CheckEvent(provider, event); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return errors.Wrapf(ErrContract, "payload is not a JSON object: %v", err)
	}

	var name string
	if err := json.Unmarshal(fields[keyEventName], &name); err != nil || name != event {
		return errors.Wrapf(ErrContract, "payload %s = %q, want %q", keyEventName, name, event)
	}

	canonical := hook.NormalizeEventName(event)
	if canonical != hook.CanonicalEventBeforeTool && canonical != hook.CanonicalEventAfterTool {
		return nil
	}

	for _, key := range []string{keyToolName, keyToolInput} {
		if _, ok := fields[key]; !ok {
			return errors.Wrapf(ErrContract, "%s payload for %s has no %s", provider, event, key)
		}
	}

	return nil
}

// CheckResponse fails when a hook response carries a field the provider does
// not accept for the event, names the wrong event, or uses a decision value
// the provider does not read. An empty response (a clean pass) is valid.
func CheckResponse(provider hook.Provider, event string, response []byte) error {
	if err := CheckEvent(provider, event); err != nil {
		return err
	}

	trimmed := strings.TrimSpace(string(response))
	if trimmed == "" {
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return errors.Wrapf(ErrContract, "response is not a JSON object: %v", err)
	}

	if provider == hook.ProviderOpenCode {
		return checkOpenCodeResponse(event, fields)
	}

	capability, ok := hook.ResolveEventCapability(provider, hook.NormalizeEventName(event), event)
	if !ok {
		return errors.Wrapf(
			ErrContract,
			"%s %s has no recorded contract, so nothing may be emitted",
			provider,
			event,
		)
	}

	var problems []string

	for key := range fields {
		problems = append(problems, checkTopLevelKey(capability, key, fields)...)
	}

	if raw, ok := fields[keyHookSpecificOutput]; ok {
		problems = append(problems, checkSpecific(provider, event, capability, raw)...)
	}

	if len(problems) == 0 {
		return nil
	}

	slices.Sort(problems)

	return errors.Wrapf(ErrContract, "%s %s response: %s",
		provider, event, strings.Join(problems, "; "))
}

func checkTopLevelKey(
	capability hook.EventCapability,
	key string,
	fields map[string]json.RawMessage,
) []string {
	switch key {
	case keyHookSpecificOutput:
		return nil
	case keyReason:
		if _, ok := fields[jsonDecision]; !ok {
			return []string{"reason without decision"}
		}

		return nil
	}

	field, known := topLevelFields[key]
	if !known || !capability.Supports(field) {
		return []string{"unsupported field " + key}
	}

	return nil
}

func checkSpecific(
	provider hook.Provider,
	event string,
	capability hook.EventCapability,
	raw json.RawMessage,
) []string {
	var specific map[string]json.RawMessage
	if err := json.Unmarshal(raw, &specific); err != nil {
		return []string{keyHookSpecificOutput + " is not an object"}
	}

	var problems []string

	var name string

	_ = json.Unmarshal(specific[keyHookEventName], &name)

	if name != capability.NativeName && name != event {
		problems = append(
			problems,
			keyHookEventName+" "+quote(name)+" does not match "+quote(event),
		)
	}

	for key, value := range specific {
		switch key {
		case keyHookEventName:
			continue
		case keyPermissionReason:
			if _, ok := specific["permissionDecision"]; !ok {
				problems = append(problems, keyPermissionReason+" without permissionDecision")
			}

			continue
		}

		field, known := specificFields[key]
		if !known || !capability.Supports(field) {
			problems = append(problems, "unsupported field "+keyHookSpecificOutput+"."+key)

			continue
		}

		if field == hook.ResponseFieldPermissionDecision {
			problems = append(problems, checkPermissionDecision(provider, value)...)
		}
	}

	return problems
}

func checkPermissionDecision(provider hook.Provider, value json.RawMessage) []string {
	var decision string
	if err := json.Unmarshal(value, &decision); err != nil ||
		!slices.Contains(permissionDecisions[provider], decision) {
		return []string{
			"permissionDecision " + string(value) + " is not read by " + string(provider),
		}
	}

	return nil
}

func checkOpenCodeResponse(event string, fields map[string]json.RawMessage) error {
	var problems []string

	for key := range fields {
		if !slices.Contains(openCodeFields, key) {
			problems = append(problems, "unsupported field "+key)
		}
	}

	if raw, ok := fields[keyHookSpecificOutput]; ok {
		problems = append(problems, checkOpenCodeSpecific(event, raw)...)
	}

	if len(problems) == 0 {
		return nil
	}

	slices.Sort(problems)

	return errors.Wrapf(
		ErrContract,
		"opencode %s response: %s",
		event,
		strings.Join(problems, "; "),
	)
}

func checkOpenCodeSpecific(event string, raw json.RawMessage) []string {
	var specific map[string]json.RawMessage
	if err := json.Unmarshal(raw, &specific); err != nil {
		return []string{keyHookSpecificOutput + " is not an object"}
	}

	var problems []string

	for key := range specific {
		if key != keyHookEventName && key != "additionalContext" {
			problems = append(problems, "unsupported field "+keyHookSpecificOutput+"."+key)
		}
	}

	if _, ok := specific[keyHookEventName]; !ok {
		return problems
	}

	var name string

	_ = json.Unmarshal(specific[keyHookEventName], &name)

	native := hook.DisplayEventName(
		hook.ProviderOpenCode,
		hook.NormalizeEventName(event),
		hook.EventTypeUnknown,
	)
	if name != event && name != native {
		problems = append(
			problems,
			keyHookEventName+" "+quote(name)+" does not match "+quote(event),
		)
	}

	return problems
}

func quote(s string) string {
	return `"` + s + `"`
}

// Outcome is what a hook response asks the harness to do: deny a tool call,
// block a decision event, advise the agent (model-facing context only), or
// pass (nothing for the agent; a user-only systemMessage notice is a pass).
type Outcome string

const (
	OutcomePass   Outcome = "pass"
	OutcomeDeny   Outcome = decisionDeny
	OutcomeBlock  Outcome = "block"
	OutcomeAdvise Outcome = "advise"
)

// ClassifyResponse reports what a response asks the harness to do.
func ClassifyResponse(response []byte) (Outcome, error) {
	trimmed := strings.TrimSpace(string(response))
	if trimmed == "" {
		return OutcomePass, nil
	}

	var parsed struct {
		Continue           *bool  `json:"continue"`
		Decision           string `json:"decision"`
		HookSpecificOutput struct {
			PermissionDecision string `json:"permissionDecision"`
			AdditionalContext  string `json:"additionalContext"`
			Decision           *struct {
				Behavior string `json:"behavior"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}

	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return "", errors.Wrapf(ErrContract, "response is not a JSON object: %v", err)
	}

	specific := parsed.HookSpecificOutput

	switch {
	case specific.PermissionDecision == decisionDeny, parsed.Decision == decisionDeny,
		specific.Decision != nil && specific.Decision.Behavior == decisionDeny:
		return OutcomeDeny, nil
	case parsed.Decision == "block", parsed.Continue != nil && !*parsed.Continue:
		return OutcomeBlock, nil
	case specific.AdditionalContext != "":
		return OutcomeAdvise, nil
	default:
		return OutcomePass, nil
	}
}
