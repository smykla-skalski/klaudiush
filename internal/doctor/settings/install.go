package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// InstallCodexDispatcher registers klaudiush in a Codex hooks.json file.
// Returns true when all supported Codex hooks were already present.
func InstallCodexDispatcher(hooksPath, binaryPath string) (bool, error) {
	parser := NewCodexHooksParser(hooksPath)

	hasSessionStart, err := parser.HasEventHook("SessionStart", binaryPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to check SessionStart hook")
	}

	hasAfterToolUse, err := parser.HasEventHook("AfterToolUse", binaryPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to check AfterToolUse hook")
	}

	hasStop, err := parser.HasEventHook("Stop", binaryPath)
	if err != nil {
		return false, errors.Wrap(err, "failed to check Stop hook")
	}

	if hasSessionStart && hasAfterToolUse && hasStop {
		return true, nil
	}

	raw, err := LoadRawJSONFile(hooksPath)
	if err != nil {
		return false, err
	}

	AddCodexDispatcherHooks(raw, binaryPath, !hasSessionStart, !hasAfterToolUse, !hasStop)

	if err := writeRawJSONFile(hooksPath, raw); err != nil {
		return false, errors.Wrap(err, "failed to write hooks config")
	}

	return false, nil
}

// InstallGeminiDispatcher registers klaudiush in a Gemini settings.json file.
// Returns true when all supported Gemini hooks were already present.
func InstallGeminiDispatcher(settingsPath, binaryPath string) (bool, error) {
	parser := NewGeminiSettingsParser(settingsPath)

	allEvents := []string{
		"BeforeTool",
		"AfterTool",
		"SessionStart",
		"SessionEnd",
		"Notification",
		"PreCompress",
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

// AddCodexDispatcherHooks appends missing Codex command hooks.
func AddCodexDispatcherHooks(
	raw map[string]any,
	binaryPath string,
	addSessionStart bool,
	addAfterToolUse bool,
	addStop bool,
) {
	hooks := ensureHooksMap(raw)

	if addSessionStart {
		hooks["SessionStart"] = appendEventHook(
			hooks["SessionStart"],
			CodexSessionStartCommand(binaryPath),
		)
	}

	if addAfterToolUse {
		hooks["AfterToolUse"] = appendEventHook(
			hooks["AfterToolUse"],
			CodexAfterToolUseCommand(binaryPath),
		)
	}

	if addStop {
		hooks["Stop"] = appendEventHook(
			hooks["Stop"],
			CodexStopCommand(binaryPath),
		)
	}
}

// AddGeminiDispatcherHooks appends missing Gemini command hooks.
func AddGeminiDispatcherHooks(raw map[string]any, binaryPath string, missing map[string]bool) {
	hooks := ensureHooksMap(raw)

	for _, eventName := range []string{
		"BeforeTool",
		"AfterTool",
		"SessionStart",
		"SessionEnd",
		"Notification",
		"PreCompress",
	} {
		if !missing[eventName] {
			continue
		}

		matcher := ""
		if eventName == "BeforeTool" || eventName == "AfterTool" {
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

// CodexSessionStartCommand returns the Codex SessionStart command string.
func CodexSessionStartCommand(binaryPath string) string {
	return binaryPath + " --provider codex --event SessionStart"
}

// CodexAfterToolUseCommand returns the Codex AfterToolUse command string.
func CodexAfterToolUseCommand(binaryPath string) string {
	return binaryPath + " --provider codex --event AfterToolUse"
}

// CodexStopCommand returns the Codex Stop command string.
func CodexStopCommand(binaryPath string) string {
	return binaryPath + " --provider codex --event Stop"
}

// GeminiBeforeToolCommand returns the Gemini BeforeTool command string.
func GeminiBeforeToolCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, "BeforeTool")
}

// GeminiAfterToolCommand returns the Gemini AfterTool command string.
func GeminiAfterToolCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, "AfterTool")
}

// GeminiSessionStartCommand returns the Gemini SessionStart command string.
func GeminiSessionStartCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, "SessionStart")
}

// GeminiSessionEndCommand returns the Gemini SessionEnd command string.
func GeminiSessionEndCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, "SessionEnd")
}

// GeminiNotificationCommand returns the Gemini Notification command string.
func GeminiNotificationCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, "Notification")
}

// GeminiPreCompressCommand returns the Gemini PreCompress command string.
func GeminiPreCompressCommand(binaryPath string) string {
	return geminiDispatcherEventCommand(binaryPath, "PreCompress")
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
