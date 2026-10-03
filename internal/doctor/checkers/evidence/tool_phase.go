package evidence

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	toolPhaseCheckName = "Evidence tool phase reaches Gemini"
	installHookFixID   = "install_hook"
	dispatcherName     = "klaudiush"
)

// phaseProbeTools are Gemini tools that change files or state. The ones the
// phase governs itself must reach a klaudiush BeforeTool hook, or a model
// that calls them past the tool selection is not stopped.
var phaseProbeTools = []string{
	evidence.GeminiWriteTool,
	evidence.GeminiEditTool,
	evidence.GeminiShellTool,
}

// phaseOpenTools are tools outside the default BeforeTool matcher that the
// phase withholds only through the tool selection.
var phaseOpenTools = []string{"save_memory", "write_todos", "mcp_server_tool"}

// ToolPhaseChecker verifies the evidence tool phase compiles, that Gemini
// runs klaudiush on BeforeToolSelection, and that the tools the phase
// governs reach a klaudiush BeforeTool hook. It reports which providers the
// phase restricts at all.
type ToolPhaseChecker struct {
	cfg *config.Config
}

// NewToolPhaseChecker creates a ToolPhaseChecker for cfg.
func NewToolPhaseChecker(cfg *config.Config) *ToolPhaseChecker {
	return &ToolPhaseChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*ToolPhaseChecker) Name() string {
	return toolPhaseCheckName
}

// Category returns the category of the check.
func (*ToolPhaseChecker) Category() doctor.Category {
	return doctor.CategoryEvidence
}

// Check reports an invalid phase, a Gemini setup that does not reach it,
// and per-provider coverage.
func (c *ToolPhaseChecker) Check(context.Context) doctor.CheckResult {
	var evidenceCfg *config.EvidenceConfig
	if c.cfg != nil {
		evidenceCfg = c.cfg.Evidence
	}

	if !evidenceCfg.GetToolPhase().IsEnabled() {
		return doctor.Skip(toolPhaseCheckName, "Evidence tool phase disabled")
	}

	phase, err := compilePhase(evidenceCfg)
	if err != nil {
		return doctor.FailError(toolPhaseCheckName,
			"Evidence tool phase is invalid, so Gemini only gets read-only tools").
			WithDetails(err.Error())
	}

	coverage := evidence.PhaseCoverageLines()

	gemini := c.cfg.GetProviders().GetGemini()
	if !gemini.IsEnabled() || !gemini.HasSettingsPath() {
		return doctor.FailWarning(toolPhaseCheckName,
			"Gemini provider is not configured, and only Gemini can have tools withheld").
			WithDetails(append([]string{
				"Enable [providers.gemini] with settings_path, then run klaudiush init --install-hooks",
			}, coverage...)...)
	}

	return checkGeminiSettings(gemini.SettingsPath, phase, coverage)
}

func compilePhase(cfg *config.EvidenceConfig) (*evidence.Phase, error) {
	checks, err := evidence.Compile(cfg)
	if err != nil {
		return nil, err
	}

	return evidence.CompilePhase(cfg, checks)
}

func checkGeminiSettings(
	settingsPath string,
	phase *evidence.Phase,
	coverage []string,
) doctor.CheckResult {
	parser := settings.NewGeminiSettingsParser(settingsPath)

	registered, err := parser.HasEventHook(settings.GeminiEventToolSelection, dispatcherName)
	if err != nil {
		return doctor.FailError(toolPhaseCheckName, "Gemini settings cannot be read").
			WithDetails(settingsPath, err.Error())
	}

	if !registered {
		return doctor.FailWarning(toolPhaseCheckName, fmt.Sprintf(
			"%s does not run klaudiush on %s, so Gemini offers every tool; "+
				"BeforeTool still denies withheld calls",
			settingsPath, settings.GeminiEventToolSelection,
		)).
			WithDetails(append([]string{"Register with: klaudiush doctor --fix"}, coverage...)...).
			WithFixID(installHookFixID)
	}

	matchers, err := parser.GeminiBeforeToolMatchers(dispatcherName)
	if err != nil {
		return doctor.FailError(toolPhaseCheckName, "Gemini settings cannot be read").
			WithDetails(settingsPath, err.Error())
	}

	unchecked := uncheckedTools(matchers, phaseProbeTools)
	if len(unchecked) > 0 {
		return doctor.FailWarning(toolPhaseCheckName, fmt.Sprintf(
			"No klaudiush BeforeTool hook in %s matches %s, so a call the model makes "+
				"past the tool selection is not denied",
			settingsPath, strings.Join(unchecked, ", "),
		)).
			WithDetails(append([]string{
				"Add the missing tools to the matcher of the klaudiush BeforeTool hook",
			}, coverage...)...)
	}

	details := slices.Clone(coverage)
	if open := uncheckedTools(matchers, phaseOpenTools); len(open) > 0 {
		details = append(details,
			"Withheld only by the tool selection, no klaudiush BeforeTool matcher selects: "+
				strings.Join(open, ", "))
	}

	return doctor.Pass(toolPhaseCheckName,
		"Gemini mutation tools wait for "+strings.Join(phase.RequiredNames(), ", "),
	).WithDetails(details...)
}

func uncheckedTools(matchers, tools []string) []string {
	var unchecked []string

	for _, tool := range tools {
		selected := slices.ContainsFunc(matchers, func(matcher string) bool {
			return settings.GeminiMatcherSelects(matcher, tool)
		})
		if !selected {
			unchecked = append(unchecked, tool)
		}
	}

	return unchecked
}
