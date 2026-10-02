package hook

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
	pkghook "github.com/smykla-skalski/klaudiush/pkg/hook"
)

const codexEnforcementCheckName = "Codex pre-tool enforcement"

// CodexEnforcementChecker reports which Codex tool calls klaudiush can refuse
// before they run, as opposed to merely having a hook registered.
type CodexEnforcementChecker struct {
	cfg *pkgConfig.CodexProviderConfig
}

// NewCodexEnforcementChecker creates a checker for effective Codex enforcement.
func NewCodexEnforcementChecker(cfg *pkgConfig.CodexProviderConfig) *CodexEnforcementChecker {
	return &CodexEnforcementChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*CodexEnforcementChecker) Name() string {
	return codexEnforcementCheckName
}

// Category returns the category of the check.
func (*CodexEnforcementChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check reports the tool families a synchronous klaudiush PreToolUse hook gates.
func (c *CodexEnforcementChecker) Check(_ context.Context) doctor.CheckResult {
	registrationChecker := &CodexRegistrationChecker{cfg: c.cfg}
	if result, ready := registrationChecker.preflight(codexEnforcementCheckName); !ready {
		return result
	}

	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip(codexEnforcementCheckName, "Binary not found in PATH")
	}

	hooksPath := c.cfg.HooksConfigPath

	disabled, configPath, err := settings.CodexHooksFeatureDisabled(hooksPath)
	if err != nil {
		return doctor.FailWarning(
			codexEnforcementCheckName,
			fmt.Sprintf("Cannot read Codex feature flags: %v", err),
		)
	}

	if disabled {
		return doctor.FailError(
			codexEnforcementCheckName,
			"Codex hooks feature is disabled; no tool calls are enforced",
		).WithDetails(
			"File: "+configPath,
			"Remove [features] hooks = false or set it to true",
		)
	}

	enforcement, err := settings.NewCodexHooksParser(hooksPath).PreToolEnforcement(binaryPath)
	if err != nil {
		return registrationChecker.failForParseError(codexEnforcementCheckName, err)
	}

	if result, done := codexRegistrationGap(enforcement, hooksPath); done {
		return result
	}

	return codexCoverageResult(enforcement.EffectiveMatcher, hooksPath)
}

func codexRegistrationGap(
	enforcement settings.CodexPreToolEnforcement,
	hooksPath string,
) (doctor.CheckResult, bool) {
	switch {
	case enforcement.LegacyOnly:
		return doctor.FailError(
			codexEnforcementCheckName,
			"Registered only on legacy AfterToolUse, which Codex never fires; "+
				"no tool calls are enforced",
		).WithDetails(
			"File: "+hooksPath,
			"Migrate with: klaudiush doctor --fix",
		).WithFixID("install_hook"), true
	case !enforcement.Registered:
		return doctor.FailError(
			codexEnforcementCheckName,
			"PreToolUse hook not registered; no tool calls are enforced",
		).WithDetails(
			"File: "+hooksPath,
			"Register with: klaudiush doctor --fix",
		).WithFixID("install_hook"), true
	case enforcement.AsyncOnly:
		return doctor.FailError(
			codexEnforcementCheckName,
			"PreToolUse hook is async; async hooks cannot block tool calls",
		).WithDetails(
			"File: "+hooksPath,
			"Remove \"async\": true from the klaudiush PreToolUse handler",
		), true
	default:
		return doctor.CheckResult{}, false
	}
}

func codexCoverageResult(matchers []string, hooksPath string) doctor.CheckResult {
	var covered, uncovered []string

	for _, family := range pkghook.CodexPreToolCoverage() {
		if codexMatchersSelect(matchers, family.ProbeNames) {
			covered = append(covered, family.Label)
		} else {
			uncovered = append(uncovered, family.Label)
		}
	}

	uncovered = append(uncovered, pkghook.CodexUncoveredTools()...)
	details := []string{
		"Blocked before running: " + joinOrNone(covered),
		"Not enforced: " + joinOrNone(uncovered),
		"Codex runs new or changed hooks only after you trust them in /hooks",
	}

	if len(covered) < len(pkghook.CodexPreToolCoverage()) {
		return doctor.FailWarning(
			codexEnforcementCheckName,
			"PreToolUse matcher skips some tool calls",
		).WithDetails(append(details, "File: "+hooksPath)...)
	}

	return doctor.Pass(codexEnforcementCheckName, "All hook-visible tool calls").
		WithDetails(details...)
}

func codexMatchersSelect(matchers, toolNames []string) bool {
	for _, matcher := range matchers {
		if settings.CodexMatcherSelects(matcher, toolNames) {
			return true
		}
	}

	return false
}

func joinOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}

	return strings.Join(values, ", ")
}
