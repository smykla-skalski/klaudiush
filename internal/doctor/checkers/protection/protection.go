// Package protection checks that policy protection and MCP trust can see
// what they guard, and reports what each provider lets them enforce.
package protection

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	eventPreToolUse   = "PreToolUse"
	eventConfigChange = "ConfigChange"
	mcpProbe          = "mcp__klaudiush_probe__q7zx_tool"
)

// Locations lists the Claude settings files to inspect.
type Locations func() []settings.SettingsLocation

// ProtectionChecker verifies the protection configuration and that Claude
// runs klaudiush on ConfigChange, and reports provider coverage and
// managed hook configuration.
type ProtectionChecker struct {
	cfg       *config.Config
	locations Locations
	managed   func() []string
}

// NewProtectionChecker creates a ProtectionChecker for cfg.
func NewProtectionChecker(cfg *config.Config) *ProtectionChecker {
	return NewProtectionCheckerWith(cfg, settings.GetAllSettingsPaths, ManagedSettingsFiles)
}

// NewProtectionCheckerWith creates a ProtectionChecker reading the given
// Claude settings locations and managed settings files.
func NewProtectionCheckerWith(
	cfg *config.Config,
	locations Locations,
	managed func() []string,
) *ProtectionChecker {
	return &ProtectionChecker{cfg: cfg, locations: locations, managed: managed}
}

// Name returns the name of the check.
func (*ProtectionChecker) Name() string {
	return "Policy files are protected"
}

// Category returns the category of the check.
func (*ProtectionChecker) Category() doctor.Category {
	return doctor.CategoryProtection
}

// Check reports configuration errors, Claude settings without a
// ConfigChange hook, and what each provider lets protection enforce.
func (c *ProtectionChecker) Check(context.Context) doctor.CheckResult {
	var protectionCfg *config.ProtectionConfig
	if c.cfg != nil {
		protectionCfg = c.cfg.Protection
	}

	if !protectionCfg.IsEnabled() {
		return doctor.Skip(c.Name(), "Protection disabled")
	}

	if err := protection.Validate(protectionCfg); err != nil {
		result := doctor.FailError(
			c.Name(),
			"Protection configuration is invalid, so klaudiush cannot load its configuration",
		)
		result.Details = []string{err.Error()}

		return result
	}

	details := append(ProtectionCoverage(), c.managedLines()...)

	missing := missingEvent(c.locations(), eventConfigChange)
	if len(missing) > 0 && len(protectionCfg.GetConfigChangeSources()) > 0 {
		result := doctor.FailWarning(c.Name(), fmt.Sprintf(
			"%d Claude settings file(s) run klaudiush but not on ConfigChange, so settings "+
				"changed mid-session take effect",
			len(missing),
		))
		result.Details = slices.Concat(missing, []string{
			`Add a ConfigChange hook running "klaudiush --provider claude --event ConfigChange"`,
		}, details)

		return result
	}

	result := doctor.Pass(c.Name(), "Policy files are protected")
	result.Details = details

	return result
}

func (c *ProtectionChecker) managedLines() []string {
	var lines []string

	for _, path := range c.managed() {
		commands := settings.HookCommandsInFile(path)
		if len(commands) == 0 {
			continue
		}

		if slices.ContainsFunc(commands, isKlaudiushCommand) {
			lines = append(
				lines,
				"Managed hooks run klaudiush ("+path+"): users cannot remove them",
			)
		} else {
			lines = append(lines, "Managed settings exist but do not run klaudiush ("+path+")")
		}
	}

	if len(lines) == 0 {
		lines = append(lines, "No managed hook configuration runs klaudiush: a user can still "+
			"remove the hooks outside the agent. Register klaudiush in managed settings "+
			"(Claude managed-settings.json, Codex requirements.toml) to prevent that")
	}

	return lines
}

// ProtectionCoverage describes what each provider lets protection enforce.
func ProtectionCoverage() []string {
	return []string{
		"Claude: Write, Edit, MultiEdit, NotebookEdit, Bash and MCP tools before they run " +
			"(as far as the PreToolUse matcher selects them); ConfigChange blocks settings " +
			"changes except policy_settings; PostToolUse reports protected files a command changed",
		"Codex: Bash, apply_patch and MCP tools through PreToolUse; no configuration change " +
			"event, but Codex asks to trust changed hooks again",
		"Gemini: BeforeTool for every tool; no configuration change event, but Gemini warns " +
			"about changed project hooks",
		"opencode: tool.execute.before for every tool; no configuration change event",
		"All providers: a command klaudiush cannot fully inspect is blocked, and a program " +
			"that writes files it does not name (a script, a compiler) is not seen",
	}
}

// MCPTrustChecker verifies the MCP trust configuration, that Claude routes
// MCP tools to klaudiush, and reports which providers report provenance.
type MCPTrustChecker struct {
	cfg       *config.Config
	locations Locations
}

// NewMCPTrustChecker creates an MCPTrustChecker for cfg.
func NewMCPTrustChecker(cfg *config.Config) *MCPTrustChecker {
	return NewMCPTrustCheckerWith(cfg, settings.GetAllSettingsPaths)
}

// NewMCPTrustCheckerWith creates an MCPTrustChecker reading the given
// Claude settings locations.
func NewMCPTrustCheckerWith(cfg *config.Config, locations Locations) *MCPTrustChecker {
	return &MCPTrustChecker{cfg: cfg, locations: locations}
}

// Name returns the name of the check.
func (*MCPTrustChecker) Name() string {
	return "MCP servers are checked by provenance"
}

// Category returns the category of the check.
func (*MCPTrustChecker) Category() doctor.Category {
	return doctor.CategoryProtection
}

// Check reports what keeps MCP trust from working.
func (c *MCPTrustChecker) Check(context.Context) doctor.CheckResult {
	var trustCfg *config.MCPTrustConfig
	if c.cfg != nil {
		trustCfg = c.cfg.MCPTrust
	}

	if !trustCfg.IsEnabled() {
		return doctor.Skip(c.Name(), "MCP trust disabled")
	}

	details := MCPCoverage(trustCfg.GetUnknownProvenance())

	unrouted := c.settingsWithoutMCP()
	if len(unrouted) > 0 {
		result := doctor.FailWarning(c.Name(), fmt.Sprintf(
			"%d Claude settings file(s) run klaudiush on PreToolUse without selecting MCP "+
				"tools, so Claude MCP calls are not checked",
			len(unrouted),
		))
		result.Details = slices.Concat(unrouted, []string{
			`Add "mcp__.*" to the klaudiush PreToolUse matcher`,
		}, details)

		return result
	}

	if len(trustCfg.TrustedSources) == 0 && len(trustCfg.Servers) == 0 {
		result := doctor.FailWarning(c.Name(), "No MCP server is trusted, so every MCP call is "+
			trustCfg.GetUntrusted()+"ed")
		result.Details = details

		return result
	}

	if trustCfg.GetUnknownProvenance() == config.MCPTrustActionAllow {
		result := doctor.FailWarning(c.Name(), "unknown_provenance = \"allow\" lets every Codex "+
			"and opencode MCP call through unchecked")
		result.Details = details

		return result
	}

	result := doctor.Pass(c.Name(), "MCP calls are checked against harness provenance")
	result.Details = details

	return result
}

func (c *MCPTrustChecker) settingsWithoutMCP() []string {
	var unrouted []string

	for _, location := range c.locations() {
		if !location.Exists {
			continue
		}

		parsed, err := settings.NewSettingsParser(location.Path).Parse()
		if err != nil {
			continue
		}

		matchers, runs := klaudiushMatchers(parsed, eventPreToolUse)
		if !runs {
			continue
		}

		if !slices.ContainsFunc(matchers, func(matcher string) bool {
			return settings.CodexMatcherSelects(matcher, []string{mcpProbe})
		}) {
			unrouted = append(
				unrouted,
				fmt.Sprintf("%s settings (%s)", location.Type, location.Path),
			)
		}
	}

	return unrouted
}

// MCPCoverage describes which providers report MCP provenance.
func MCPCoverage(unknown string) []string {
	return []string{
		"Claude: mcp_server reports the server name and source (Claude Code 2.1.274+); " +
			"older releases send no provenance",
		"Gemini: mcp_context reports the server's command or URL, but no configuration source",
		"Codex and opencode: no provenance, so unknown_provenance = \"" + unknown + "\" applies " +
			"to every MCP call",
	}
}

// missingEvent lists Claude settings files that run klaudiush but not on
// event.
func missingEvent(locations []settings.SettingsLocation, event string) []string {
	var missing []string

	for _, location := range locations {
		if !location.Exists {
			continue
		}

		parsed, err := settings.NewSettingsParser(location.Path).Parse()
		if err != nil {
			continue
		}

		if _, runs := klaudiushMatchers(parsed, ""); !runs {
			continue
		}

		if _, onEvent := klaudiushMatchers(parsed, event); !onEvent {
			missing = append(missing, fmt.Sprintf("%s settings (%s)", location.Type, location.Path))
		}
	}

	return missing
}

// klaudiushMatchers returns the matchers of the hook groups that run
// klaudiush on event (any event when empty), and whether there is one.
func klaudiushMatchers(parsed *settings.ClaudeSettings, event string) ([]string, bool) {
	var (
		matchers []string
		runs     bool
	)

	for name, groups := range parsed.Hooks {
		if event != "" && name != event {
			continue
		}

		for _, group := range groups {
			for _, hookCmd := range group.Hooks {
				if isKlaudiushCommand(hookCmd.Command) {
					matchers = append(matchers, group.Matcher)
					runs = true
				}
			}
		}
	}

	return matchers, runs
}

func isKlaudiushCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}

	name := filepath.Base(fields[0])

	return strings.Contains(name, "klaudiush") || name == "dispatcher"
}

// ManagedSettingsFiles lists the managed hook configuration files that
// exist on this machine.
func ManagedSettingsFiles() []string {
	var files []string

	for _, path := range settings.ManagedHookFiles() {
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}

	return files
}
