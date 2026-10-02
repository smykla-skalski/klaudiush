package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
)

const (
	// DefaultCommandHookTimeout is the default timeout in seconds for provider command hooks.
	DefaultCommandHookTimeout = 30
	millisecondsPerSecond     = 1000

	defaultDirPermissions  = 0o750
	defaultFilePermissions = 0o600
)

// Hook event names shared across providers.
const (
	eventSessionStart = "SessionStart"
)

// Codex hook event names. AfterToolUse is the legacy post-tool name that
// current Codex no longer fires.
const (
	CodexEventPreToolUse         = "PreToolUse"
	CodexEventPostToolUse        = "PostToolUse"
	CodexEventStop               = "Stop"
	CodexLegacyEventAfterToolUse = "AfterToolUse"
)

// Gemini hook event names.
const (
	geminiEventBeforeTool   = "BeforeTool"
	geminiEventAfterTool    = "AfterTool"
	geminiEventSessionStart = eventSessionStart
	geminiEventSessionEnd   = "SessionEnd"
	geminiEventNotification = "Notification"
	geminiEventPreCompress  = "PreCompress"
)

// LoadRawJSONFile reads and parses a JSON file into a raw map.
func LoadRawJSONFile(path string) (map[string]any, error) {
	resolvedPath, err := resolveSettingsPath(path)
	if err != nil {
		return nil, err
	}

	//nolint:gosec // Path comes from validated config or known settings helpers.
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}

		return nil, errors.Wrap(err, "failed to read settings")
	}

	if len(data) == 0 {
		return make(map[string]any), nil
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.Wrap(err, "failed to parse settings")
	}

	return raw, nil
}

// InstallClaudeDispatcher registers klaudiush in a Claude settings.json file.
// Returns true when all supported Claude hooks were already present.
func InstallClaudeDispatcher(settingsPath, binaryPath string) (bool, error) {
	parser := NewSettingsParser(settingsPath)

	hasPreToolUse, err := parser.HasEventHookCommand("PreToolUse", binaryPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to check settings")
	}

	hasPostToolUse, err := parser.HasEventHookCommand("PostToolUse", binaryPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to check settings")
	}

	if hasPreToolUse && hasPostToolUse {
		return true, nil
	}

	raw, err := LoadRawJSONFile(settingsPath)
	if err != nil {
		return false, err
	}

	AddClaudeDispatcherHooks(raw, binaryPath, !hasPreToolUse, !hasPostToolUse)

	if err := writeRawJSONFile(settingsPath, raw); err != nil {
		return false, errors.Wrap(err, "failed to write settings")
	}

	return false, nil
}

// InstallCodexDispatcher registers klaudiush in a Codex hooks.json file and
// removes its own legacy AfterToolUse entries. Unrelated hooks are kept.
// PreToolUse counts as installed only with a synchronous matcherless handler;
// an async or narrowed one cannot block every tool, so another is added.
// Returns true when the file already matched and nothing was written.
func InstallCodexDispatcher(hooksPath, binaryPath string) (bool, error) {
	parser := NewCodexHooksParser(hooksPath)
	missing := make(map[string]bool, len(CodexDispatcherEvents()))

	for _, eventName := range CodexDispatcherEvents() {
		installed, err := codexEventInstalled(parser, eventName, binaryPath)
		if err != nil {
			return false, errors.Wrapf(err, "failed to check %s hook", eventName)
		}

		missing[eventName] = !installed
	}

	hasLegacy, err := parser.HasEventHook(CodexLegacyEventAfterToolUse, binaryPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to check legacy AfterToolUse hook")
	}

	anyMissing := slices.ContainsFunc(CodexDispatcherEvents(), func(eventName string) bool {
		return missing[eventName]
	})

	if !hasLegacy && !anyMissing {
		return true, nil
	}

	raw, err := LoadRawJSONFile(hooksPath)
	if err != nil {
		return false, err
	}

	RemoveCodexLegacyDispatcherHooks(raw, binaryPath)
	AddCodexDispatcherHooks(raw, binaryPath, missing)

	if err := writeRawJSONFile(hooksPath, raw); err != nil {
		return false, errors.Wrap(err, "failed to write hooks config")
	}

	return false, nil
}

func codexEventInstalled(parser *CodexHooksParser, eventName, binaryPath string) (bool, error) {
	if eventName != CodexEventPreToolUse {
		return parser.HasEventHook(eventName, binaryPath)
	}

	enforcement, err := parser.PreToolEnforcement(binaryPath)
	if err != nil {
		return false, err
	}

	return enforcement.SelectsEveryTool(), nil
}

// CodexDispatcherEvents returns the Codex events klaudiush registers. PreToolUse
// has no matcher so it gates shell, apply_patch, MCP, and local function tools.
// PostToolUse is not registered: pre-tool denial already covers those calls,
// and post-tool validation would re-report findings an exception allowed.
func CodexDispatcherEvents() []string {
	return []string{eventSessionStart, CodexEventPreToolUse, CodexEventStop}
}

// InstallGeminiDispatcher registers klaudiush in a Gemini settings.json file.
// Returns true when all supported Gemini hooks were already present.
func InstallGeminiDispatcher(settingsPath, binaryPath string) (bool, error) {
	parser := NewGeminiSettingsParser(settingsPath)

	allEvents := []string{
		geminiEventBeforeTool,
		geminiEventAfterTool,
		geminiEventSessionStart,
		geminiEventSessionEnd,
		geminiEventNotification,
		geminiEventPreCompress,
	}

	missing := make(map[string]bool, len(allEvents))
	allPresent := true

	for _, eventName := range allEvents {
		hasHook, err := parser.HasEventHook(eventName, binaryPath)
		if err != nil {
			return false, errors.Wrapf(err, "failed to check %s hook", eventName)
		}

		missing[eventName] = !hasHook
		allPresent = allPresent && hasHook
	}

	if allPresent {
		return true, nil
	}

	raw, err := LoadRawJSONFile(settingsPath)
	if err != nil {
		return false, err
	}

	AddGeminiDispatcherHooks(raw, binaryPath, missing)

	if err := writeRawJSONFile(settingsPath, raw); err != nil {
		return false, errors.Wrap(err, "failed to write Gemini settings")
	}

	return false, nil
}

// AddClaudeDispatcherHooks appends missing Claude command hooks.
func AddClaudeDispatcherHooks(
	raw map[string]any,
	binaryPath string,
	addPreToolUse bool,
	addPostToolUse bool,
) {
	hooks := ensureHooksMap(raw)

	if addPreToolUse {
		hooks["PreToolUse"] = appendEventHookWithMatcher(
			hooks["PreToolUse"],
			ClaudeDispatcherCommand(binaryPath, "PreToolUse"),
			claudeDispatcherMatcher(),
			DefaultCommandHookTimeout,
		)
	}

	if addPostToolUse {
		hooks["PostToolUse"] = appendEventHookWithMatcher(
			hooks["PostToolUse"],
			ClaudeDispatcherCommand(binaryPath, "PostToolUse"),
			claudeDispatcherMatcher(),
			DefaultCommandHookTimeout,
		)
	}
}

// AddCodexDispatcherHooks appends Codex command hooks for the missing events.
func AddCodexDispatcherHooks(raw map[string]any, binaryPath string, missing map[string]bool) {
	hooks := ensureHooksMap(raw)

	for _, eventName := range CodexDispatcherEvents() {
		if !missing[eventName] {
			continue
		}

		hooks[eventName] = appendEventHook(
			hooks[eventName],
			CodexDispatcherCommand(binaryPath, eventName),
		)
	}
}

// RemoveCodexLegacyDispatcherHooks drops klaudiush handlers from the legacy
// AfterToolUse event, keeping any other handlers and matcher groups there.
func RemoveCodexLegacyDispatcherHooks(raw map[string]any, binaryPath string) {
	hooks, ok := raw["hooks"].(map[string]any)
	if !ok {
		return
	}

	groups, ok := hooks[CodexLegacyEventAfterToolUse].([]any)
	if !ok {
		return
	}

	kept := make([]any, 0, len(groups))

	for _, rawGroup := range groups {
		group, isMap := rawGroup.(map[string]any)
		if !isMap {
			kept = append(kept, rawGroup)

			continue
		}

		handlers, hasHandlers := group["hooks"].([]any)
		if !hasHandlers {
			kept = append(kept, rawGroup)

			continue
		}

		remaining := slices.DeleteFunc(slices.Clone(handlers), func(handler any) bool {
			return isRawCodexDispatcherHandler(handler, binaryPath)
		})

		if len(remaining) == 0 {
			continue
		}

		group["hooks"] = remaining
		kept = append(kept, group)
	}

	if len(kept) == 0 {
		delete(hooks, CodexLegacyEventAfterToolUse)

		return
	}

	hooks[CodexLegacyEventAfterToolUse] = kept
}

func isRawCodexDispatcherHandler(handler any, binaryPath string) bool {
	values, ok := handler.(map[string]any)
	if !ok {
		return false
	}

	hookType, _ := values["type"].(string)
	command, _ := values["command"].(string)

	return isCodexDispatcherHook(
		CodexHookCommandConfig{Type: hookType, Command: command},
		binaryPath,
	)
}

// AddGeminiDispatcherHooks appends missing Gemini command hooks.
func AddGeminiDispatcherHooks(raw map[string]any, binaryPath string, missing map[string]bool) {
	hooks := ensureHooksMap(raw)

	for _, eventName := range []string{
		geminiEventBeforeTool,
		geminiEventAfterTool,
		geminiEventSessionStart,
		geminiEventSessionEnd,
		geminiEventNotification,
		geminiEventPreCompress,
	} {
		if !missing[eventName] {
			continue
		}

		matcher := ""
		if eventName == geminiEventBeforeTool || eventName == geminiEventAfterTool {
			matcher = geminiDispatcherMatcher()
		}

		hooks[eventName] = appendEventHookWithMatcher(
			hooks[eventName],
			geminiDispatcherEventCommand(binaryPath, eventName),
			matcher,
			DefaultCommandHookTimeout*millisecondsPerSecond,
		)
	}
}

// ClaudeDispatcherCommand returns the Claude hook command string.
func ClaudeDispatcherCommand(binaryPath, eventName string) string {
	return binaryPath + " --hook-type " + eventName
}

// CodexDispatcherCommand returns the Codex command string for an event.
func CodexDispatcherCommand(binaryPath, eventName string) string {
	return binaryPath + " --provider codex --event " + eventName
}

// GeminiBeforeToolCommand returns the Gemini BeforeTool command string.
func GeminiBeforeToolCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, geminiEventBeforeTool)
}

// GeminiAfterToolCommand returns the Gemini AfterTool command string.
func GeminiAfterToolCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, geminiEventAfterTool)
}

// GeminiSessionStartCommand returns the Gemini SessionStart command string.
func GeminiSessionStartCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, geminiEventSessionStart)
}

// GeminiSessionEndCommand returns the Gemini SessionEnd command string.
func GeminiSessionEndCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, geminiEventSessionEnd)
}

// GeminiNotificationCommand returns the Gemini Notification command string.
func GeminiNotificationCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, geminiEventNotification)
}

// GeminiPreCompressCommand returns the Gemini PreCompress command string.
func GeminiPreCompressCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, geminiEventPreCompress)
}

func claudeDispatcherMatcher() string {
	return "Bash|Write|Edit|MultiEdit"
}

func ensureHooksMap(raw map[string]any) map[string]any {
	hooks, ok := raw["hooks"].(map[string]any)
	if ok {
		return hooks
	}

	hooks = make(map[string]any)
	raw["hooks"] = hooks

	return hooks
}

func appendEventHook(existingValue any, command string) []any {
	return appendEventHookWithMatcher(existingValue, command, "", DefaultCommandHookTimeout)
}

func appendEventHookWithMatcher(
	existingValue any,
	command string,
	matcher string,
	timeout int,
) []any {
	existing, ok := existingValue.([]any)
	if !ok {
		existing = nil
	}

	entry := map[string]any{}
	if matcher != "" {
		entry["matcher"] = matcher
	}

	entry["hooks"] = []any{
		map[string]any{
			"type":    "command",
			"command": command,
			"timeout": timeout,
		},
	}

	return append(existing, entry)
}

func writeRawJSONFile(path string, raw map[string]any) error {
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal settings")
	}

	data = append(data, '\n')

	return AtomicWriteFile(path, data, true)
}

// AtomicWriteFile writes data to a file atomically using a temp file and rename.
// It creates a backup of the original file if it exists.
func AtomicWriteFile(path string, data []byte, createBackup bool) error {
	resolvedPath, err := resolveSettingsPath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, defaultDirPermissions); err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	perm := os.FileMode(defaultFilePermissions)
	if info, err := os.Stat(resolvedPath); err == nil {
		perm = info.Mode().Perm()
	}

	if createBackup {
		if _, err := os.Stat(resolvedPath); err == nil {
			backupPath := fmt.Sprintf("%s.backup.%d", resolvedPath, time.Now().Unix())
			if err := copyFile(resolvedPath, backupPath); err != nil {
				return errors.Wrap(err, "failed to create backup")
			}
		}
	}

	tmpFile := resolvedPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, perm); err != nil {
		return errors.Wrap(err, "failed to write temp file")
	}

	if err := os.Rename(tmpFile, resolvedPath); err != nil {
		_ = os.Remove(tmpFile)
		return errors.Wrap(err, "failed to rename temp file")
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src) // #nosec G304 -- source is from resolved settings path
	if err != nil {
		return errors.Wrap(err, "failed to read source file")
	}

	info, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, "failed to stat source file")
	}

	// #nosec G703 -- destination path is derived from validated settings path
	if err := os.WriteFile(dst, data, info.Mode()); err != nil {
		return errors.Wrap(err, "failed to write destination file")
	}

	return nil
}
