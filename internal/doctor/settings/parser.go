// Package settings provides utilities for parsing and analyzing Claude Code settings files.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cockroachdb/errors"
)

var (
	ErrSettingsNotFound   = errors.New("settings file not found")
	ErrInvalidJSON        = errors.New("invalid JSON syntax")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrDispatcherNotFound = errors.New("dispatcher not registered in settings")
)

// SettingsParser parses Claude Code settings.json files.
type SettingsParser struct {
	settingsPath string
}

// ClaudeSettings represents the structure of a Claude Code settings file.
type ClaudeSettings struct {
	Hooks map[string][]HookConfig `json:"hooks"`
}

// HookConfig represents a hook configuration block.
type HookConfig struct {
	Matcher string              `json:"matcher"`
	Hooks   []HookCommandConfig `json:"hooks"`
}

// HookCommandConfig represents an individual hook command configuration.
type HookCommandConfig struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// SettingsLocation represents a Claude settings file location.
type SettingsLocation struct {
	Path   string
	Type   string
	Exists bool
}

// NewSettingsParser creates a new settings parser for the given file path.
func NewSettingsParser(path string) *SettingsParser {
	return &SettingsParser{
		settingsPath: path,
	}
}

// Parse reads and parses the Claude settings file.
func (p *SettingsParser) Parse() (*ClaudeSettings, error) {
	data, err := os.ReadFile(p.settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.WithMessage(ErrSettingsNotFound, p.settingsPath)
		}

		if os.IsPermission(err) {
			return nil, errors.WithMessage(ErrPermissionDenied, p.settingsPath)
		}

		return nil, errors.Wrap(err, "failed to read settings file")
	}

	if len(data) == 0 {
		return &ClaudeSettings{Hooks: make(map[string][]HookConfig)}, nil
	}

	var settings ClaudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, errors.WithSecondaryError(
			errors.WithMessage(ErrInvalidJSON, "in "+p.settingsPath),
			err,
		)
	}

	if settings.Hooks == nil {
		settings.Hooks = make(map[string][]HookConfig)
	}

	return &settings, nil
}

// IsDispatcherRegistered checks if the dispatcher command is registered in the settings.
// It searches for any hook command that contains the dispatcherPath.
func (p *SettingsParser) IsDispatcherRegistered(dispatcherPath string) (bool, error) {
	settings, err := p.Parse()
	if err != nil {
		if errors.Is(err, ErrSettingsNotFound) {
			return false, nil
		}

		return false, err
	}

	if settings.Hooks == nil {
		return false, nil
	}

	dispatcherName := filepath.Base(dispatcherPath)

	for _, hookConfigs := range settings.Hooks {
		for _, hookConfig := range hookConfigs {
			for _, hook := range hookConfig.Hooks {
				if hook.Type == "command" && (strings.Contains(hook.Command, dispatcherPath) ||
					strings.Contains(hook.Command, dispatcherName)) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// HasPreToolUseHook checks if the settings file contains any PreToolUse hooks.
func (p *SettingsParser) HasPreToolUseHook() (bool, error) {
	settings, err := p.Parse()
	if err != nil {
		if errors.Is(err, ErrSettingsNotFound) {
			return false, nil
		}

		return false, err
	}

	_, exists := settings.Hooks["PreToolUse"]

	return exists, nil
}

// GetUserSettingsPath returns the path to the user's global settings file.
func GetUserSettingsPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".claude", "settings.json")
}

// GetProjectSettingsPath returns the path to the project settings file.
func GetProjectSettingsPath() string {
	return filepath.Join(".claude", "settings.json")
}

// GetProjectLocalSettingsPath returns the path to the project-local settings file.
func GetProjectLocalSettingsPath() string {
	return filepath.Join(".claude", "settings.local.json")
}

// GetEnterprisePolicyPaths returns all possible enterprise policy file paths
// based on the current operating system.
func GetEnterprisePolicyPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Library/Application Support/ClaudeCode/policies.json"}
	case "linux":
		return []string{"/etc/claude-code/policies.json"}
	default:
		return nil
	}
}

// GetAllSettingsPaths returns all possible Claude settings file locations
// with their type and existence status.
func GetAllSettingsPaths() []SettingsLocation {
	locations := []SettingsLocation{
		{
			Path: GetUserSettingsPath(),
			Type: "user",
		},
		{
			Path: GetProjectSettingsPath(),
			Type: "project",
		},
		{
			Path: GetProjectLocalSettingsPath(),
			Type: "project-local",
		},
	}

	for _, path := range GetEnterprisePolicyPaths() {
		locations = append(locations, SettingsLocation{
			Path: path,
			Type: "enterprise",
		})
	}

	for i := range locations {
		if locations[i].Path != "" {
			if _, err := os.Stat(locations[i].Path); err == nil {
				locations[i].Exists = true
			}
		}
	}

	return locations
}
