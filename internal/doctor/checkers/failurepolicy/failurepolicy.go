// Package failurepolicy checks that the validation failure policy can do
// what it promises: answer before the provider kills the hook, and run the
// checks configured as critical.
package failurepolicy

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// answerMargin is the time klaudiush needs past its deadline to stop the
// remaining checks and write its answer.
const answerMargin = 5 * time.Second

const terraformName = "terraform"

// criticalTools lists the external tools each tool-backed validator needs.
// Each inner list is one requirement any of whose tools satisfies it.
var criticalTools = map[string][][]string{
	"shellscript":     {{"shellcheck"}},
	terraformName:     {{"tofu", terraformName}},
	"github-workflow": {{"actionlint"}},
	"gofumpt":         {{"gofumpt"}},
	"python":          {{"ruff"}},
	"javascript":      {{"oxlint"}},
	"rust":            {{"rustfmt"}},
}

// toolsFor returns what a critical validator needs under cfg: terraform
// also needs tflint unless use_tflint is off.
func toolsFor(name string, cfg *config.Config) ([][]string, bool) {
	tools, ok := criticalTools[name]
	if !ok || name != terraformName {
		return tools, ok
	}

	if cfg != nil && cfg.Validators != nil && cfg.Validators.File != nil &&
		cfg.Validators.File.Terraform != nil && cfg.Validators.File.Terraform.UseTflint != nil &&
		!*cfg.Validators.File.Terraform.UseTflint {
		return tools, true
	}

	return append(slices.Clone(tools), []string{"tflint"}), true
}

// DeadlineChecker verifies the hook deadline leaves room to answer within
// the timeout each Claude settings file gives the klaudiush hooks.
type DeadlineChecker struct {
	cfg       *config.Config
	locations func() []settings.SettingsLocation
}

// NewDeadlineChecker creates a DeadlineChecker for cfg.
func NewDeadlineChecker(cfg *config.Config) *DeadlineChecker {
	return &DeadlineChecker{cfg: cfg, locations: settings.GetAllSettingsPaths}
}

// NewDeadlineCheckerWithLocations creates a DeadlineChecker reading the
// given settings locations.
func NewDeadlineCheckerWithLocations(
	cfg *config.Config,
	locations func() []settings.SettingsLocation,
) *DeadlineChecker {
	return &DeadlineChecker{cfg: cfg, locations: locations}
}

// Name returns the name of the check.
func (*DeadlineChecker) Name() string {
	return "Validation deadline below hook timeout"
}

// Category returns the category of the check.
func (*DeadlineChecker) Category() doctor.Category {
	return doctor.CategoryFailurePolicy
}

// Check compares the deadline with every klaudiush hook timeout.
func (c *DeadlineChecker) Check(context.Context) doctor.CheckResult {
	deadline := c.policyConfig().GetDeadline()

	var tooShort []string

	for _, location := range c.locations() {
		if !location.Exists {
			continue
		}

		parsed, err := settings.NewSettingsParser(location.Path).Parse()
		if err != nil {
			continue
		}

		for _, timeout := range klaudiushTimeouts(parsed) {
			if deadline+answerMargin > timeout {
				tooShort = append(tooShort, fmt.Sprintf(
					"%s settings (%s): hook timeout %s",
					location.Type, location.Path, timeout,
				))
			}
		}
	}

	if len(tooShort) == 0 {
		return doctor.Pass(c.Name(), fmt.Sprintf(
			"Deadline %s leaves time to answer before every hook timeout",
			deadline,
		))
	}

	result := doctor.FailWarning(c.Name(), fmt.Sprintf(
		"Deadline %s plus %s to answer exceeds a hook timeout; "+
			"the provider kills the hook and lets the action through",
		deadline, answerMargin,
	))

	tooShort = append(
		slices.Compact(tooShort),
		"Lower failure_policy.deadline or raise the hook timeout",
	)
	result.Details = tooShort

	return result
}

func (c *DeadlineChecker) policyConfig() *config.FailurePolicyConfig {
	if c.cfg == nil {
		return nil
	}

	return c.cfg.FailurePolicy
}

// klaudiushTimeouts lists the explicit timeouts of klaudiush hook commands.
// Hooks without a timeout get the provider default, which is long enough.
func klaudiushTimeouts(parsed *settings.ClaudeSettings) []time.Duration {
	var timeouts []time.Duration

	for _, groups := range parsed.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if hook.Timeout > 0 && isKlaudiushCommand(hook.Command) {
					timeouts = append(timeouts, time.Duration(hook.Timeout)*time.Second)
				}
			}
		}
	}

	return timeouts
}

func isKlaudiushCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}

	name := filepath.Base(fields[0])

	return strings.Contains(name, "klaudiush") || name == "dispatcher"
}

// CriticalToolsChecker verifies that validators configured as critical have
// the tools they need: a critical check without its tool blocks every action
// it covers.
type CriticalToolsChecker struct {
	cfg   *config.Config
	tools exec.ToolChecker
}

// NewCriticalToolsChecker creates a CriticalToolsChecker for cfg.
func NewCriticalToolsChecker(cfg *config.Config) *CriticalToolsChecker {
	return &CriticalToolsChecker{cfg: cfg, tools: exec.NewToolChecker()}
}

// NewCriticalToolsCheckerWithTools creates a CriticalToolsChecker that looks
// tools up with the given checker.
func NewCriticalToolsCheckerWithTools(
	cfg *config.Config,
	tools exec.ToolChecker,
) *CriticalToolsChecker {
	return &CriticalToolsChecker{cfg: cfg, tools: tools}
}

// Name returns the name of the check.
func (*CriticalToolsChecker) Name() string {
	return "Critical validators have their tools"
}

// Category returns the category of the check.
func (*CriticalToolsChecker) Category() doctor.Category {
	return doctor.CategoryFailurePolicy
}

// Check looks up the tool of every critical tool-backed validator.
func (c *CriticalToolsChecker) Check(context.Context) doctor.CheckResult {
	var policyCfg *config.FailurePolicyConfig
	if c.cfg != nil {
		policyCfg = c.cfg.FailurePolicy
	}

	critical := policyCfg.GetCritical()
	if len(critical) == 0 {
		return doctor.Skip(c.Name(), "No critical validators configured")
	}

	var missing []string

	for _, name := range critical {
		normalized := failpolicy.NormalizeName(name)
		if !failpolicy.IsKnownName(normalized) {
			missing = append(missing, name+" is not a known validator name, so it protects nothing")

			continue
		}

		requirements, ok := toolsFor(normalized, c.cfg)
		if !ok {
			continue
		}

		for _, tools := range requirements {
			if c.tools.FindTool(tools...) == "" {
				missing = append(missing, fmt.Sprintf(
					"%s needs %s, which is not installed",
					name, strings.Join(tools, " or "),
				))
			}
		}
	}

	if len(missing) == 0 {
		return doctor.Pass(c.Name(), "Every critical validator can run")
	}

	result := doctor.FailError(c.Name(), fmt.Sprintf(
		"%d critical validator(s) cannot run and will block every action they check",
		len(missing),
	))
	missing = append(
		missing,
		"Install the tool or remove the validator from failure_policy.critical",
	)
	result.Details = missing

	return result
}
