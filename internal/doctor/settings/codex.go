package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// CodexHooksParser parses Codex hooks.json files.
type CodexHooksParser struct {
	hooksPath string
}

// CodexHooksFile represents the structure of a Codex hooks.json file.
type CodexHooksFile struct {
	Hooks CodexHookEvents `json:"hooks"`
}

// CodexHookEvents groups supported Codex hook events.
type CodexHookEvents struct {
	SessionStart []CodexMatcherGroup `json:"SessionStart,omitempty"`
	Stop         []CodexMatcherGroup `json:"Stop,omitempty"`
}

// CodexMatcherGroup represents one matcher group under a Codex hook event.
type CodexMatcherGroup struct {
	Matcher string                   `json:"matcher,omitempty"`
	Hooks   []CodexHookCommandConfig `json:"hooks"`
}

// CodexHookCommandConfig represents one Codex hook handler configuration.
type CodexHookCommandConfig struct {
	Type          string `json:"type"`
	Command       string `json:"command,omitempty"`
	Timeout       int    `json:"timeout,omitempty"`
	TimeoutSec    int    `json:"timeoutSec,omitempty"`
	Async         bool   `json:"async,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

// NewCodexHooksParser creates a new Codex hooks parser for the given file path.
func NewCodexHooksParser(path string) *CodexHooksParser {
	return &CodexHooksParser{hooksPath: path}
}

// Parse reads and parses the Codex hooks file.
func (p *CodexHooksParser) Parse() (*CodexHooksFile, error) {
	data, err := os.ReadFile(p.hooksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.WithMessage(ErrSettingsNotFound, p.hooksPath)
		}

		if os.IsPermission(err) {
			return nil, errors.WithMessage(ErrPermissionDenied, p.hooksPath)
		}

		return nil, errors.Wrap(err, "failed to read hooks file")
	}

	if len(data) == 0 {
		return &CodexHooksFile{}, nil
	}

	var hooksFile CodexHooksFile
	if err := json.Unmarshal(data, &hooksFile); err != nil {
		return nil, errors.WithSecondaryError(
			errors.WithMessage(ErrInvalidJSON, "in "+p.hooksPath),
			err,
		)
	}

	return &hooksFile, nil
}

// IsDispatcherRegistered checks whether any supported Codex event is configured for klaudiush.
func (p *CodexHooksParser) IsDispatcherRegistered(dispatcherPath string) (bool, error) {
	for _, eventName := range []string{"SessionStart", "Stop"} {
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
func (p *CodexHooksParser) HasEventHook(eventName, dispatcherPath string) (bool, error) {
	hooksFile, err := p.Parse()
	if err != nil {
		if errors.Is(err, ErrSettingsNotFound) {
			return false, nil
		}

		return false, err
	}

	return hasCodexDispatcherCommand(codexEventGroups(hooksFile, eventName), dispatcherPath), nil
}

func codexEventGroups(hooksFile *CodexHooksFile, eventName string) []CodexMatcherGroup {
	switch strings.ToLower(eventName) {
	case "sessionstart", "session_start":
		return hooksFile.Hooks.SessionStart
	case "stop", "turn_stop":
		return hooksFile.Hooks.Stop
	default:
		return nil
	}
}

func hasCodexDispatcherCommand(groups []CodexMatcherGroup, dispatcherPath string) bool {
	dispatcherName := filepath.Base(dispatcherPath)

	for _, group := range groups {
		for _, hook := range group.Hooks {
			if hook.Type != "command" {
				continue
			}

			if strings.Contains(hook.Command, dispatcherPath) ||
				strings.Contains(hook.Command, dispatcherName) {
				return true
			}
		}
	}

	return false
}
