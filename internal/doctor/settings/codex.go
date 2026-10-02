package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/pelletier/go-toml/v2"
)

// CodexHooksParser parses Codex hooks.json files.
type CodexHooksParser struct {
	hooksPath string
}

// CodexHooksFile represents the structure of a Codex hooks.json file.
type CodexHooksFile struct {
	Hooks CodexHookEvents `json:"hooks"`
}

// CodexHookEvents groups the Codex hook events klaudiush registers or reads.
// AfterToolUse is a legacy key current Codex ignores; it is parsed only so
// doctor can report and migrate stale registrations.
type CodexHookEvents struct {
	SessionStart []CodexMatcherGroup `json:"SessionStart,omitempty"`
	PreToolUse   []CodexMatcherGroup `json:"PreToolUse,omitempty"`
	PostToolUse  []CodexMatcherGroup `json:"PostToolUse,omitempty"`
	Stop         []CodexMatcherGroup `json:"Stop,omitempty"`
	AfterToolUse []CodexMatcherGroup `json:"AfterToolUse,omitempty"`
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
	hooksFile := &CodexHooksFile{}
	if err := readJSONSettingsFile(
		p.hooksPath,
		hooksFile,
		"failed to read hooks file",
	); err != nil {
		return nil, err
	}

	return hooksFile, nil
}

// IsDispatcherRegistered checks whether any supported Codex event is configured for klaudiush.
func (p *CodexHooksParser) IsDispatcherRegistered(dispatcherPath string) (bool, error) {
	for _, eventName := range []string{
		eventSessionStart,
		CodexEventPreToolUse,
		CodexEventPostToolUse,
		CodexEventStop,
		CodexLegacyEventAfterToolUse,
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
	case "pretooluse", "before_tool":
		return hooksFile.Hooks.PreToolUse
	case "posttooluse", "after_tool":
		return hooksFile.Hooks.PostToolUse
	case "aftertooluse":
		return hooksFile.Hooks.AfterToolUse
	case "stop", "turn_stop":
		return hooksFile.Hooks.Stop
	default:
		return nil
	}
}

func hasCodexDispatcherCommand(groups []CodexMatcherGroup, dispatcherPath string) bool {
	for _, group := range groups {
		for _, hook := range group.Hooks {
			if isCodexDispatcherHook(hook, dispatcherPath) {
				return true
			}
		}
	}

	return false
}

// isCodexDispatcherHook matches a command whose program is the dispatcher
// binary, by full path or by name, skipping leading env assignments. A command
// that only mentions klaudiush (a wrapper script, an argument) does not match.
func isCodexDispatcherHook(hook CodexHookCommandConfig, dispatcherPath string) bool {
	if hook.Type != commandHookType {
		return false
	}

	program := commandProgram(hook.Command)

	return program == dispatcherPath || filepath.Base(program) == filepath.Base(dispatcherPath)
}

func commandProgram(command string) string {
	for token := range strings.FieldsSeq(command) {
		token = strings.Trim(token, `"'`)
		if token == "env" || strings.Contains(token, "=") {
			continue
		}

		return token
	}

	return ""
}

// CodexPreToolEnforcement describes how a klaudiush PreToolUse registration
// actually gates Codex tool calls.
type CodexPreToolEnforcement struct {
	Registered       bool
	LegacyOnly       bool
	AsyncOnly        bool
	EffectiveMatcher []string
}

// SelectsEveryTool reports whether a synchronous klaudiush handler has no
// matcher (or "*"), the only shape that gates every hook-visible tool call.
func (e CodexPreToolEnforcement) SelectsEveryTool() bool {
	return slices.ContainsFunc(e.EffectiveMatcher, isMatchAllMatcher)
}

func isMatchAllMatcher(matcher string) bool {
	matcher = strings.TrimSpace(matcher)

	return matcher == "" || matcher == "*"
}

// PreToolEnforcement inspects the klaudiush PreToolUse handlers. Async
// handlers cannot block, so only synchronous ones count as enforcing.
func (p *CodexHooksParser) PreToolEnforcement(
	dispatcherPath string,
) (CodexPreToolEnforcement, error) {
	var result CodexPreToolEnforcement

	hooksFile, err := p.Parse()
	if err != nil {
		if errors.Is(err, ErrSettingsNotFound) {
			return result, nil
		}

		return result, err
	}

	for _, group := range hooksFile.Hooks.PreToolUse {
		for _, hook := range group.Hooks {
			if !isCodexDispatcherHook(hook, dispatcherPath) {
				continue
			}

			result.Registered = true

			if !hook.Async {
				result.EffectiveMatcher = append(result.EffectiveMatcher, group.Matcher)
			}
		}
	}

	result.AsyncOnly = result.Registered && len(result.EffectiveMatcher) == 0
	result.LegacyOnly = !result.Registered &&
		hasCodexDispatcherCommand(hooksFile.Hooks.AfterToolUse, dispatcherPath)

	return result, nil
}

// CodexMatcherSelects reports whether a Codex matcher selects any of the tool
// names. An empty matcher or "*" selects every tool; anything else is a regex.
// An invalid regex selects nothing, since Codex cannot match with it either.
func CodexMatcherSelects(matcher string, toolNames []string) bool {
	if isMatchAllMatcher(matcher) {
		return true
	}

	re, err := regexp.Compile("^(?:" + strings.TrimSpace(matcher) + ")$")
	if err != nil {
		return false
	}

	return slices.ContainsFunc(toolNames, re.MatchString)
}

type codexFeatureConfig struct {
	Features struct {
		Hooks *bool `toml:"hooks"`
	} `toml:"features"`
}

// CodexHooksFeatureDisabled reports whether the config.toml next to the hooks
// file turns the Codex hooks feature off. A missing file or key means the
// Codex default (enabled) applies.
func CodexHooksFeatureDisabled(hooksPath string) (bool, string, error) {
	resolvedHooksPath, err := resolveSettingsPath(hooksPath)
	if err != nil {
		return false, "", err
	}

	configPath := filepath.Clean(filepath.Join(filepath.Dir(resolvedHooksPath), "config.toml"))

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, configPath, nil
		}

		return false, configPath, errors.Wrap(err, "failed to read Codex config")
	}

	var cfg codexFeatureConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return false, configPath, errors.Wrap(err, "failed to parse Codex config")
	}

	return cfg.Features.Hooks != nil && !*cfg.Features.Hooks, configPath, nil
}
