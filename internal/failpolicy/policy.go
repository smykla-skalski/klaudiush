// Package failpolicy decides what an action gets when validation cannot run.
//
// Every provider klaudiush supports lets the action through when a hook
// exits non-zero, prints output it cannot read, or is killed at its timeout.
// klaudiush therefore answers every failure it can catch itself, with exit
// code 0 and a response the provider honors: a warning that says validation
// was unavailable, or a deny when the policy asks to block.
package failpolicy

import (
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// Action is what an unavailable check does to the action.
type Action int

// Actions, from most to least permissive.
const (
	ActionIgnore Action = iota
	ActionWarn
	ActionBlock
)

// String returns the config spelling of the action.
func (a Action) String() string {
	switch a {
	case ActionIgnore:
		return config.FailureModeIgnore
	case ActionWarn:
		return config.FailureModeWarn
	case ActionBlock:
		return config.FailureModeBlock
	default:
		return config.FailureModeWarn
	}
}

// ErrInvalidMode is returned for a mode that is not warn or block.
var ErrInvalidMode = errors.New("invalid failure mode")

// ParseMode parses a mode ("warn" or "block").
func ParseMode(mode string) (Action, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case config.FailureModeWarn:
		return ActionWarn, nil
	case config.FailureModeBlock:
		return ActionBlock, nil
	default:
		return ActionWarn, errors.Wrapf(ErrInvalidMode, "%q (use warn or block)", mode)
	}
}

func parseMissingTools(value string) Action {
	switch value {
	case config.FailureModeWarn:
		return ActionWarn
	case config.FailureModeBlock:
		return ActionBlock
	default:
		return ActionIgnore
	}
}

// overrideNames maps override-style validator names onto runtime names, so
// critical accepts either spelling.
var overrideNames = map[string]string{
	"git.add":          "git-add",
	"git.branch":       "branch-name",
	"git.commit":       "commit",
	"git.fetch":        "git-fetch",
	"git.merge":        "merge",
	"git.no_verify":    "no-verify",
	"git.pr":           "pr",
	"git.push":         "git-push",
	"github.api":       "gh-api",
	"github.issue":     "issue",
	"secrets":          "secrets",
	"shell.backtick":   "backticks",
	"shell.nesting":    "nesting",
	"file.shellscript": "shellscript",
	"file.terraform":   "terraform",
	"file.workflow":    "github-workflow",
	"file.gofumpt":     "gofumpt",
	"file.python":      "python",
	"file.javascript":  "javascript",
	"file.rust":        "rust",
	"file.markdown":    "markdown",
	"file.ai_comments": "ai-comments",
	"plugins":          "plugin-registry",
}

// NormalizeName reduces a validator name to the form critical matches on.
func NormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if runtime, ok := overrideNames[name]; ok {
		return runtime
	}

	return strings.TrimPrefix(name, "validate-")
}

// Policy resolves failures to actions. The zero value and a nil Policy keep
// each check's own choice and ignore missing tools.
type Policy struct {
	mode         Action
	modeSet      bool
	missingTools Action
	critical     map[string]bool
	deadline     time.Duration
}

// New builds a policy from configuration. A nil config gives the defaults.
func New(cfg *config.FailurePolicyConfig) *Policy {
	p := &Policy{
		missingTools: parseMissingTools(cfg.GetMissingTools()),
		critical:     make(map[string]bool),
		deadline:     cfg.GetDeadline(),
	}

	if mode := cfg.GetMode(); mode != "" {
		if action, err := ParseMode(mode); err == nil {
			p.mode, p.modeSet = action, true
		}
	}

	for _, name := range cfg.GetCritical() {
		if name != "" {
			p.critical[NormalizeName(name)] = true
		}
	}

	return p
}

// WithMode overrides the mode, as the --failure-mode flag does.
func (p *Policy) WithMode(action Action) *Policy {
	p.mode, p.modeSet = action, true

	return p
}

// Mode returns the effective mode for failures of the hook itself.
func (p *Policy) Mode() Action {
	if p == nil || !p.modeSet {
		return ActionWarn
	}

	return p.mode
}

// ModeSet reports whether a mode was configured.
func (p *Policy) ModeSet() bool {
	return p != nil && p.modeSet
}

// Deadline returns how long one hook run may take.
func (p *Policy) Deadline() time.Duration {
	if p == nil || p.deadline <= 0 {
		return config.DefaultFailureDeadline
	}

	return p.deadline
}

// IsCritical reports whether name blocks whenever it cannot run.
func (p *Policy) IsCritical(name string) bool {
	return p != nil && p.critical[NormalizeName(name)]
}

// Critical lists the normalized critical validator names.
func (p *Policy) Critical() []string {
	if p == nil {
		return nil
	}

	names := make([]string, 0, len(p.critical))
	for name := range p.critical {
		names = append(names, name)
	}

	return names
}

// Resolve decides what an unavailable check of validator does. checkBlocks
// is the check's own choice, kept when no mode is configured.
func (p *Policy) Resolve(
	validatorName string,
	reason validator.UnavailableReason,
	checkBlocks bool,
) Action {
	switch {
	case p.IsCritical(validatorName):
		return ActionBlock
	case reason == validator.ReasonMissingTool:
		if p == nil {
			return ActionIgnore
		}

		return p.missingTools
	case p.ModeSet():
		return p.mode
	case checkBlocks:
		return ActionBlock
	default:
		return ActionWarn
	}
}
