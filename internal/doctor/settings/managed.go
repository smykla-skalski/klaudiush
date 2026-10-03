package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// maxManagedBytes bounds how much of a managed settings file is read.
const maxManagedBytes = 1 << 20

// hookCommandKey is the key holding a hook command in settings files.
const hookCommandKey = "command"

// ManagedHookFiles lists where administrators register hooks users cannot
// remove: Claude Code managed settings and Codex requirements.toml.
// Sources: https://code.claude.com/docs/en/settings#settings-files and
// https://developers.openai.com/codex/hooks.
func ManagedHookFiles() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Library/Application Support/ClaudeCode/managed-settings.json",
			"/etc/codex/requirements.toml",
		}
	case "linux":
		return []string{"/etc/claude-code/managed-settings.json", "/etc/codex/requirements.toml"}
	case "windows":
		return []string{
			`C:\Program Files\ClaudeCode\managed-settings.json`,
			filepath.Join(os.Getenv("ProgramData"), "OpenAI", "Codex", "requirements.toml"),
		}
	default:
		return nil
	}
}

// HookCommandsInFile returns every hook command registered in a JSON or
// TOML settings file under its "hooks" key.
func HookCommandsInFile(path string) []string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxManagedBytes {
		return nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}

	var root map[string]any

	if strings.EqualFold(filepath.Ext(path), ".toml") {
		err = toml.Unmarshal(data, &root)
	} else {
		err = json.Unmarshal(data, &root)
	}

	if err != nil {
		return nil
	}

	var commands []string

	collectHookCommands(root["hooks"], &commands)

	return commands
}

func collectHookCommands(node any, commands *[]string) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if text, ok := child.(string); ok && key == hookCommandKey {
				*commands = append(*commands, text)

				continue
			}

			collectHookCommands(child, commands)
		}
	case []any:
		for _, child := range value {
			collectHookCommands(child, commands)
		}
	case []map[string]any:
		for _, child := range value {
			collectHookCommands(child, commands)
		}
	}
}
