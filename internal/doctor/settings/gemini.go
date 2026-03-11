package settings

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// GeminiSettingsParser parses Gemini settings.json hook definitions.
type GeminiSettingsParser struct {
	settingsPath string
}

// GeminiSettingsFile represents the subset of Gemini settings relevant to hooks.
type GeminiSettingsFile struct {
	Hooks GeminiHookEvents `json:"hooks"`
}

// GeminiHookEvents groups supported Gemini hook events.
type GeminiHookEvents struct {
	BeforeTool   []CodexMatcherGroup `json:"BeforeTool,omitempty"`
	AfterTool    []CodexMatcherGroup `json:"AfterTool,omitempty"`
	SessionStart []CodexMatcherGroup `json:"SessionStart,omitempty"`
	SessionEnd   []CodexMatcherGroup `json:"SessionEnd,omitempty"`
	Notification []CodexMatcherGroup `json:"Notification,omitempty"`
	PreCompress  []CodexMatcherGroup `json:"PreCompress,omitempty"`
}

// NewGeminiSettingsParser creates a new Gemini settings parser for the given file path.
func NewGeminiSettingsParser(path string) *GeminiSettingsParser {
	return &GeminiSettingsParser{settingsPath: path}
}

// Parse reads and parses the Gemini settings file.
func (p *GeminiSettingsParser) Parse() (*GeminiSettingsFile, error) {
	settingsFile := &GeminiSettingsFile{}
	if err := readJSONSettingsFile(
		p.settingsPath,
		settingsFile,
		"failed to read settings file",
	); err != nil {
		return nil, err
	}

	return settingsFile, nil
}

// IsDispatcherRegistered checks whether any supported Gemini hook is configured for klaudiush.
func (p *GeminiSettingsParser) IsDispatcherRegistered(dispatcherPath string) (bool, error) {
	for _, eventName := range []string{
		"BeforeTool",
		"AfterTool",
		"SessionStart",
		"SessionEnd",
		"Notification",
		"PreCompress",
	} {
		hasHook, err := p.HasEventHook(eventName, dispatcherPath)
		if err != nil {
			return false, err
		}

		if hasHook {
			return true, nil
		}
	}

	return false, nil
}

// HasEventHook checks whether the given event contains a dispatcher command hook.
func (p *GeminiSettingsParser) HasEventHook(eventName, dispatcherPath string) (bool, error) {
	settingsFile, err := p.Parse()
	if err != nil {
		if errors.Is(err, ErrSettingsNotFound) {
			return false, nil
		}

		return false, err
	}

	groups := geminiEventGroups(settingsFile, eventName)

	return hasCodexDispatcherCommand(groups, dispatcherPath), nil
}

func geminiEventGroups(settingsFile *GeminiSettingsFile, eventName string) []CodexMatcherGroup {
	switch strings.ToLower(eventName) {
	case "beforetool", "before_tool":
		return settingsFile.Hooks.BeforeTool
	case "aftertool", "after_tool":
		return settingsFile.Hooks.AfterTool
	case "sessionstart", "session_start":
		return settingsFile.Hooks.SessionStart
	case "sessionend", "turn_stop":
		return settingsFile.Hooks.SessionEnd
	case "notification":
		return settingsFile.Hooks.Notification
	case "precompress", "pre_compress":
		return settingsFile.Hooks.PreCompress
	default:
		return nil
	}
}

func geminiDispatcherMatcher() string {
	return "run_shell_command|write_file|replace|read_file|glob|grep|ls"
}

func geminiDispatcherEventCommand(binaryPath, eventName string) string {
	return binaryPath + " --provider gemini --event " + eventName
}
