// Package hook provides checkers for Claude settings and hook registration.
package hook

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	binaryName = "klaudiush"

	// Settings types
	settingsTypeUser         = "user"
	settingsTypeProject      = "project"
	settingsTypeProjectLocal = "project-local"
)

// RegistrationChecker checks if the dispatcher is registered in Claude settings
type RegistrationChecker struct {
	settingsPath string
	settingsType string
}

// NewUserRegistrationChecker creates a checker for user settings
func NewUserRegistrationChecker() *RegistrationChecker {
	return &RegistrationChecker{
		settingsPath: settings.GetUserSettingsPath(),
		settingsType: settingsTypeUser,
	}
}

// NewProjectRegistrationChecker creates a checker for project settings
func NewProjectRegistrationChecker() *RegistrationChecker {
	return &RegistrationChecker{
		settingsPath: settings.GetProjectSettingsPath(),
		settingsType: settingsTypeProject,
	}
}

// NewProjectLocalRegistrationChecker creates a checker for project-local settings
func NewProjectLocalRegistrationChecker() *RegistrationChecker {
	return &RegistrationChecker{
		settingsPath: settings.GetProjectLocalSettingsPath(),
		settingsType: settingsTypeProjectLocal,
	}
}

// Name returns the name of the check
func (c *RegistrationChecker) Name() string {
	return fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType)
}

// Category returns the category of the check
func (*RegistrationChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the registration check
func (c *RegistrationChecker) Check(_ context.Context) doctor.CheckResult {
	parser := settings.NewSettingsParser(c.settingsPath)

	// Get binary path for checking registration
	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip(
			fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
			"Binary not found in PATH",
		)
	}

	registered, err := parser.IsDispatcherRegistered(binaryPath)
	if err != nil {
		if errors.Is(err, settings.ErrSettingsNotFound) {
			// For project settings, this is just informational since it's optional
			if c.settingsType == settingsTypeProject || c.settingsType == settingsTypeProjectLocal {
				return doctor.Skip(
					fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
					"Settings file not found (optional)",
				)
			}

			// For user settings, this is an error
			return doctor.FailError(
				fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
				"Settings file not found",
			).
				WithDetails(
					"Expected at: "+c.settingsPath,
					"Create settings file with: klaudiush doctor --fix",
				).
				WithFixID("install_hook")
		}

		if errors.Is(err, settings.ErrInvalidJSON) {
			return doctor.FailError(
				fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
				"Settings file has invalid JSON syntax",
			).
				WithDetails(
					"File: "+c.settingsPath,
					fmt.Sprintf("Error: %v", err),
				)
		}

		return doctor.FailError(
			fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
			fmt.Sprintf("Failed to parse settings: %v", err),
		)
	}

	if !registered {
		// For project settings, not registered is just informational
		if c.settingsType == settingsTypeProject || c.settingsType == settingsTypeProjectLocal {
			return doctor.Pass(
				fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
				"Not registered (optional, using user settings)",
			)
		}

		// For user settings, not registered is an error
		return doctor.FailError(
			fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
			"Dispatcher not registered",
		).
			WithDetails(
				"File: "+c.settingsPath,
				"Register with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.Pass(
		fmt.Sprintf("Dispatcher registered in %s settings", c.settingsType),
		"Registered",
	)
}

// PreToolUseChecker checks if PreToolUse hooks are configured
type PreToolUseChecker struct {
	settingsPath string
	settingsType string
}

// NewUserPreToolUseChecker creates a PreToolUse checker for user settings
func NewUserPreToolUseChecker() *PreToolUseChecker {
	return &PreToolUseChecker{
		settingsPath: settings.GetUserSettingsPath(),
		settingsType: settingsTypeUser,
	}
}

// NewProjectPreToolUseChecker creates a PreToolUse checker for project settings
func NewProjectPreToolUseChecker() *PreToolUseChecker {
	return &PreToolUseChecker{
		settingsPath: settings.GetProjectSettingsPath(),
		settingsType: settingsTypeProject,
	}
}

// NewProjectLocalPreToolUseChecker creates a PreToolUse checker for project-local settings.
func NewProjectLocalPreToolUseChecker() *PreToolUseChecker {
	return &PreToolUseChecker{
		settingsPath: settings.GetProjectLocalSettingsPath(),
		settingsType: settingsTypeProjectLocal,
	}
}

// Name returns the name of the check
func (c *PreToolUseChecker) Name() string {
	return fmt.Sprintf("PreToolUse hook in %s settings", c.settingsType)
}

// Category returns the category of the check
func (*PreToolUseChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the PreToolUse hook check
func (c *PreToolUseChecker) Check(_ context.Context) doctor.CheckResult {
	return checkClaudeToolHook(
		c.settingsPath,
		c.settingsType,
		"PreToolUse",
		"The dispatcher requires PreToolUse hooks to function",
		func(parser *settings.SettingsParser) (bool, error) {
			return parser.HasPreToolUseHook()
		},
	)
}

// PostToolUseChecker checks if PostToolUse hooks are configured.
type PostToolUseChecker struct {
	settingsPath string
	settingsType string
}

// NewUserPostToolUseChecker creates a PostToolUse checker for user settings.
func NewUserPostToolUseChecker() *PostToolUseChecker {
	return &PostToolUseChecker{
		settingsPath: settings.GetUserSettingsPath(),
		settingsType: settingsTypeUser,
	}
}

// NewProjectPostToolUseChecker creates a PostToolUse checker for project settings.
func NewProjectPostToolUseChecker() *PostToolUseChecker {
	return &PostToolUseChecker{
		settingsPath: settings.GetProjectSettingsPath(),
		settingsType: settingsTypeProject,
	}
}

// NewProjectLocalPostToolUseChecker creates a PostToolUse checker for project-local settings.
func NewProjectLocalPostToolUseChecker() *PostToolUseChecker {
	return &PostToolUseChecker{
		settingsPath: settings.GetProjectLocalSettingsPath(),
		settingsType: settingsTypeProjectLocal,
	}
}

// Name returns the name of the check.
func (c *PostToolUseChecker) Name() string {
	return fmt.Sprintf("PostToolUse hook in %s settings", c.settingsType)
}

// Category returns the category of the check.
func (*PostToolUseChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the PostToolUse hook check.
func (c *PostToolUseChecker) Check(_ context.Context) doctor.CheckResult {
	return checkClaudeToolHook(
		c.settingsPath,
		c.settingsType,
		"PostToolUse",
		"The dispatcher requires PostToolUse hooks to validate completed edits and writes",
		func(parser *settings.SettingsParser) (bool, error) {
			return parser.HasPostToolUseHook()
		},
	)
}

// PostToolUseFailureChecker checks if PostToolUseFailure hooks are configured.
type PostToolUseFailureChecker struct {
	settingsPath string
	settingsType string
}

// NewUserPostToolUseFailureChecker creates a PostToolUseFailure checker for
// user settings.
func NewUserPostToolUseFailureChecker() *PostToolUseFailureChecker {
	return &PostToolUseFailureChecker{
		settingsPath: settings.GetUserSettingsPath(),
		settingsType: settingsTypeUser,
	}
}

// NewProjectPostToolUseFailureChecker creates a PostToolUseFailure checker for
// project settings.
func NewProjectPostToolUseFailureChecker() *PostToolUseFailureChecker {
	return &PostToolUseFailureChecker{
		settingsPath: settings.GetProjectSettingsPath(),
		settingsType: settingsTypeProject,
	}
}

// NewProjectLocalPostToolUseFailureChecker creates a PostToolUseFailure
// checker for project-local settings.
func NewProjectLocalPostToolUseFailureChecker() *PostToolUseFailureChecker {
	return &PostToolUseFailureChecker{
		settingsPath: settings.GetProjectLocalSettingsPath(),
		settingsType: settingsTypeProjectLocal,
	}
}

// Name returns the name of the check.
func (c *PostToolUseFailureChecker) Name() string {
	return fmt.Sprintf("PostToolUseFailure hook in %s settings", c.settingsType)
}

// Category returns the category of the check.
func (*PostToolUseFailureChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the PostToolUseFailure hook check.
func (c *PostToolUseFailureChecker) Check(_ context.Context) doctor.CheckResult {
	return checkClaudeToolHook(
		c.settingsPath,
		c.settingsType,
		"PostToolUseFailure",
		"The dispatcher requires PostToolUseFailure hooks to audit files a failed tool changed",
		func(parser *settings.SettingsParser) (bool, error) {
			return parser.HasPostToolUseFailureHook()
		},
	)
}

func checkClaudeToolHook(
	settingsPath string,
	settingsType string,
	eventName string,
	requiredMessage string,
	checkFn func(*settings.SettingsParser) (bool, error),
) doctor.CheckResult {
	parser := settings.NewSettingsParser(settingsPath)

	hasHook, err := checkFn(parser)
	if err != nil {
		if errors.Is(err, settings.ErrSettingsNotFound) {
			return doctor.Skip(
				fmt.Sprintf("%s hook in %s settings", eventName, settingsType),
				"Settings file not found",
			)
		}

		return doctor.FailWarning(
			fmt.Sprintf("%s hook in %s settings", eventName, settingsType),
			fmt.Sprintf("Failed to check: %v", err),
		)
	}

	if !hasHook {
		if settingsType == settingsTypeProject || settingsType == settingsTypeProjectLocal {
			return doctor.Pass(
				fmt.Sprintf("%s hook in %s settings", eventName, settingsType),
				"Not configured (optional, using user settings)",
			)
		}

		return doctor.FailError(
			fmt.Sprintf("%s hook in %s settings", eventName, settingsType),
			eventName+" hook not configured",
		).
			WithDetails(
				requiredMessage,
				"Configure with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.Pass(
		fmt.Sprintf("%s hook in %s settings", eventName, settingsType),
		"Configured",
	)
}

// CodexConfigChecker checks whether experimental Codex hooks automation is configured.
type CodexConfigChecker struct {
	cfg *pkgConfig.CodexProviderConfig
}

// NewCodexConfigChecker creates a checker for Codex hooks configuration.
func NewCodexConfigChecker(cfg *pkgConfig.CodexProviderConfig) *CodexConfigChecker {
	return &CodexConfigChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*CodexConfigChecker) Name() string {
	return "Codex hooks configuration"
}

// Category returns the category of the check.
func (*CodexConfigChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check validates Codex hooks configuration readiness.
func (c *CodexConfigChecker) Check(_ context.Context) doctor.CheckResult {
	if c.cfg == nil || !c.cfg.IsEnabled() {
		return doctor.Skip("Codex hooks configuration", "Codex provider disabled")
	}

	if !c.cfg.IsExperimentalEnabled() {
		return doctor.FailWarning(
			"Codex hooks configuration",
			"Experimental Codex hooks support is not enabled",
		).WithDetails(
			"Set [providers.codex] experimental = true",
			"Enable the Codex CLI feature flag separately if needed",
		)
	}

	if !c.cfg.HasHooksConfigPath() {
		return doctor.FailWarning(
			"Codex hooks configuration",
			"hooks_config_path is not configured",
		).WithDetails(
			"Set [providers.codex] hooks_config_path to the exact hooks.json path",
		)
	}

	return doctor.Pass("Codex hooks configuration", c.cfg.HooksConfigPath)
}

// CodexRegistrationChecker checks if the dispatcher is registered in Codex hooks.json.
type CodexRegistrationChecker struct {
	cfg *pkgConfig.CodexProviderConfig
}

// NewCodexRegistrationChecker creates a checker for Codex dispatcher registration.
func NewCodexRegistrationChecker(cfg *pkgConfig.CodexProviderConfig) *CodexRegistrationChecker {
	return &CodexRegistrationChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*CodexRegistrationChecker) Name() string {
	return "Dispatcher registered in Codex hooks"
}

// Category returns the category of the check.
func (*CodexRegistrationChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the Codex dispatcher registration check.
func (c *CodexRegistrationChecker) Check(_ context.Context) doctor.CheckResult {
	if result, ready := c.preflight("Dispatcher registered in Codex hooks"); !ready {
		return result
	}

	return checkProviderRegistration(
		"Dispatcher registered in Codex hooks",
		c.cfg.HooksConfigPath,
		func(dispatcherPath string) (bool, error) {
			return settings.NewCodexHooksParser(c.cfg.HooksConfigPath).IsDispatcherRegistered(
				dispatcherPath,
			)
		},
		c.failForParseError,
	)
}

// CodexEventChecker checks that a specific Codex event hook is configured.
type CodexEventChecker struct {
	cfg       *pkgConfig.CodexProviderConfig
	eventName string
}

// NewCodexEventChecker creates a checker for a specific Codex event hook.
func NewCodexEventChecker(
	cfg *pkgConfig.CodexProviderConfig,
	eventName string,
) *CodexEventChecker {
	return &CodexEventChecker{
		cfg:       cfg,
		eventName: eventName,
	}
}

// Name returns the name of the check.
func (c *CodexEventChecker) Name() string {
	return c.eventName + " hook in Codex hooks"
}

// Category returns the category of the check.
func (*CodexEventChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the configured event coverage check.
func (c *CodexEventChecker) Check(_ context.Context) doctor.CheckResult {
	checkName := c.eventName + " hook in Codex hooks"
	if result, ready := c.preflight(checkName); !ready {
		return result
	}

	registrationChecker := &CodexRegistrationChecker{cfg: c.cfg}

	return checkProviderEventHook(
		checkName,
		c.cfg.HooksConfigPath,
		c.eventName,
		func(eventName, dispatcherPath string) (bool, error) {
			return settings.NewCodexHooksParser(c.cfg.HooksConfigPath).HasEventHook(
				eventName,
				dispatcherPath,
			)
		},
		registrationChecker.failForParseError,
	)
}

func (c *CodexRegistrationChecker) preflight(checkName string) (doctor.CheckResult, bool) {
	if c.cfg == nil || !c.cfg.IsEnabled() {
		return doctor.Skip(checkName, "Codex provider disabled"), false
	}

	if !c.cfg.IsExperimentalEnabled() {
		return doctor.Skip(checkName, "Codex hooks automation not enabled"), false
	}

	if !c.cfg.HasHooksConfigPath() {
		return doctor.Skip(checkName, "hooks_config_path not configured"), false
	}

	return doctor.CheckResult{}, true
}

func (c *CodexEventChecker) preflight(checkName string) (doctor.CheckResult, bool) {
	registrationChecker := &CodexRegistrationChecker{cfg: c.cfg}

	return registrationChecker.preflight(checkName)
}

func (c *CodexRegistrationChecker) failForParseError(
	checkName string,
	err error,
) doctor.CheckResult {
	if errors.Is(err, settings.ErrSettingsNotFound) {
		return doctor.FailError(checkName, "Hooks file not found").
			WithDetails(
				"Expected at: "+c.cfg.HooksConfigPath,
				"Register with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	if errors.Is(err, settings.ErrInvalidJSON) {
		return doctor.FailError(checkName, "Hooks file has invalid JSON syntax").
			WithDetails(
				"File: "+c.cfg.HooksConfigPath,
				fmt.Sprintf("Error: %v", err),
			)
	}

	return doctor.FailError(
		checkName,
		fmt.Sprintf("Failed to parse hooks file: %v", err),
	)
}

// GeminiConfigChecker checks whether Gemini hooks automation is configured.
type GeminiConfigChecker struct {
	cfg *pkgConfig.GeminiProviderConfig
}

// NewGeminiConfigChecker creates a checker for Gemini hooks configuration.
func NewGeminiConfigChecker(cfg *pkgConfig.GeminiProviderConfig) *GeminiConfigChecker {
	return &GeminiConfigChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*GeminiConfigChecker) Name() string {
	return "Gemini hooks configuration"
}

// Category returns the category of the check.
func (*GeminiConfigChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check validates Gemini hooks configuration readiness.
func (c *GeminiConfigChecker) Check(_ context.Context) doctor.CheckResult {
	if c.cfg == nil || !c.cfg.IsEnabled() {
		return doctor.Skip("Gemini hooks configuration", "Gemini provider disabled")
	}

	if !c.cfg.HasSettingsPath() {
		return doctor.FailWarning(
			"Gemini hooks configuration",
			"settings_path is not configured",
		).WithDetails(
			"Set [providers.gemini] settings_path to the exact settings.json path",
		)
	}

	return doctor.Pass("Gemini hooks configuration", c.cfg.SettingsPath)
}

// GeminiRegistrationChecker checks if the dispatcher is registered in Gemini settings.json.
type GeminiRegistrationChecker struct {
	cfg *pkgConfig.GeminiProviderConfig
}

// NewGeminiRegistrationChecker creates a checker for Gemini dispatcher registration.
func NewGeminiRegistrationChecker(cfg *pkgConfig.GeminiProviderConfig) *GeminiRegistrationChecker {
	return &GeminiRegistrationChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*GeminiRegistrationChecker) Name() string {
	return "Dispatcher registered in Gemini settings"
}

// Category returns the category of the check.
func (*GeminiRegistrationChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the Gemini dispatcher registration check.
func (c *GeminiRegistrationChecker) Check(_ context.Context) doctor.CheckResult {
	if result, ready := c.preflight("Dispatcher registered in Gemini settings"); !ready {
		return result
	}

	return checkProviderRegistration(
		"Dispatcher registered in Gemini settings",
		c.cfg.SettingsPath,
		func(dispatcherPath string) (bool, error) {
			return settings.NewGeminiSettingsParser(c.cfg.SettingsPath).IsDispatcherRegistered(
				dispatcherPath,
			)
		},
		c.failForParseError,
	)
}

// GeminiEventChecker checks that a specific Gemini event hook is configured.
type GeminiEventChecker struct {
	cfg       *pkgConfig.GeminiProviderConfig
	eventName string
}

// NewGeminiEventChecker creates a checker for a specific Gemini event hook.
func NewGeminiEventChecker(
	cfg *pkgConfig.GeminiProviderConfig,
	eventName string,
) *GeminiEventChecker {
	return &GeminiEventChecker{
		cfg:       cfg,
		eventName: eventName,
	}
}

// Name returns the name of the check.
func (c *GeminiEventChecker) Name() string {
	return c.eventName + " hook in Gemini settings"
}

// Category returns the category of the check.
func (*GeminiEventChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the configured event coverage check.
func (c *GeminiEventChecker) Check(_ context.Context) doctor.CheckResult {
	checkName := c.eventName + " hook in Gemini settings"
	if result, ready := c.preflight(checkName); !ready {
		return result
	}

	registrationChecker := &GeminiRegistrationChecker{cfg: c.cfg}

	return checkProviderEventHook(
		checkName,
		c.cfg.SettingsPath,
		c.eventName,
		func(eventName, dispatcherPath string) (bool, error) {
			return settings.NewGeminiSettingsParser(c.cfg.SettingsPath).HasEventHook(
				eventName,
				dispatcherPath,
			)
		},
		registrationChecker.failForParseError,
	)
}

func (c *GeminiRegistrationChecker) preflight(checkName string) (doctor.CheckResult, bool) {
	if c.cfg == nil || !c.cfg.IsEnabled() {
		return doctor.Skip(checkName, "Gemini provider disabled"), false
	}

	if !c.cfg.HasSettingsPath() {
		return doctor.Skip(checkName, "settings_path not configured"), false
	}

	return doctor.CheckResult{}, true
}

func (c *GeminiEventChecker) preflight(checkName string) (doctor.CheckResult, bool) {
	registrationChecker := &GeminiRegistrationChecker{cfg: c.cfg}

	return registrationChecker.preflight(checkName)
}

func (c *GeminiRegistrationChecker) failForParseError(
	checkName string,
	err error,
) doctor.CheckResult {
	if errors.Is(err, settings.ErrSettingsNotFound) {
		return doctor.FailError(checkName, "Settings file not found").
			WithDetails(
				"Expected at: "+c.cfg.SettingsPath,
				"Register with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	if errors.Is(err, settings.ErrInvalidJSON) {
		return doctor.FailError(checkName, "Settings file has invalid JSON syntax").
			WithDetails(
				"File: "+c.cfg.SettingsPath,
				fmt.Sprintf("Error: %v", err),
			)
	}

	return doctor.FailError(
		checkName,
		fmt.Sprintf("Failed to parse settings file: %v", err),
	)
}

// OpenCodeConfigChecker checks whether the opencode bridge plugin is configured.
type OpenCodeConfigChecker struct {
	cfg *pkgConfig.OpenCodeProviderConfig
}

// NewOpenCodeConfigChecker creates a checker for opencode plugin configuration.
func NewOpenCodeConfigChecker(cfg *pkgConfig.OpenCodeProviderConfig) *OpenCodeConfigChecker {
	return &OpenCodeConfigChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*OpenCodeConfigChecker) Name() string {
	return "opencode plugin configuration"
}

// Category returns the category of the check.
func (*OpenCodeConfigChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check validates opencode bridge plugin configuration readiness.
func (c *OpenCodeConfigChecker) Check(_ context.Context) doctor.CheckResult {
	if c.cfg == nil || !c.cfg.IsEnabled() {
		return doctor.Skip("opencode plugin configuration", "opencode provider disabled")
	}

	// plugin_path is optional: enabling the provider is enough, and the
	// installer falls back to the default location.
	return doctor.Pass(
		"opencode plugin configuration",
		settings.ResolveOpenCodePluginPath(c.cfg.PluginPath),
	)
}

// OpenCodeRegistrationChecker checks if the bridge plugin invokes the dispatcher.
type OpenCodeRegistrationChecker struct {
	cfg *pkgConfig.OpenCodeProviderConfig
}

// NewOpenCodeRegistrationChecker creates a checker for opencode registration.
func NewOpenCodeRegistrationChecker(
	cfg *pkgConfig.OpenCodeProviderConfig,
) *OpenCodeRegistrationChecker {
	return &OpenCodeRegistrationChecker{cfg: cfg}
}

// Name returns the name of the check.
func (*OpenCodeRegistrationChecker) Name() string {
	return "Dispatcher registered in opencode plugin"
}

// Category returns the category of the check.
func (*OpenCodeRegistrationChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the opencode dispatcher registration check.
func (c *OpenCodeRegistrationChecker) Check(_ context.Context) doctor.CheckResult {
	checkName := "Dispatcher registered in opencode plugin"
	if result, ready := c.preflight(checkName); !ready {
		return result
	}

	pluginPath := c.pluginPath()

	return checkProviderRegistration(
		checkName,
		pluginPath,
		func(dispatcherPath string) (bool, error) {
			return settings.NewOpenCodePluginParser(pluginPath).IsDispatcherRegistered(
				dispatcherPath,
			)
		},
		c.failForParseError,
	)
}

// OpenCodeFreshnessChecker checks the installed plugin matches this binary.
//
// The registration and event checks only look for the dispatcher path and the
// subscribed events, both of which survive a klaudiush upgrade that changed the
// plugin body. Without this check a stale plugin keeps reporting healthy, and
// the fix a release shipped never reaches the session.
type OpenCodeFreshnessChecker struct {
	cfg      *pkgConfig.OpenCodeProviderConfig
	openCode settings.OpenCodeVersionDetector
}

// NewOpenCodeFreshnessChecker creates a checker for plugin staleness.
func NewOpenCodeFreshnessChecker(
	cfg *pkgConfig.OpenCodeProviderConfig,
) *OpenCodeFreshnessChecker {
	return &OpenCodeFreshnessChecker{cfg: cfg, openCode: settings.NewOpenCodeVersionDetector()}
}

// WithOpenCodeVersionDetector replaces how the installed opencode version is
// found, which picks the plugin API the installed bridge is compared against.
func (c *OpenCodeFreshnessChecker) WithOpenCodeVersionDetector(
	detector settings.OpenCodeVersionDetector,
) *OpenCodeFreshnessChecker {
	c.openCode = detector

	return c
}

// Name returns the name of the check.
func (*OpenCodeFreshnessChecker) Name() string {
	return "opencode plugin is up to date"
}

// Category returns the category of the check.
func (*OpenCodeFreshnessChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check compares the installed plugin against the current template.
func (c *OpenCodeFreshnessChecker) Check(ctx context.Context) doctor.CheckResult {
	checkName := "opencode plugin is up to date"

	registrationChecker := &OpenCodeRegistrationChecker{cfg: c.cfg}
	if result, ready := registrationChecker.preflight(checkName); !ready {
		return result
	}

	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip(checkName, "Binary not found in PATH")
	}

	pluginPath := registrationChecker.pluginPath()

	current, err := settings.NewOpenCodePluginParser(pluginPath).Read()
	if err != nil {
		if errors.Is(err, settings.ErrPluginNotFound) {
			// The registration check already reports the missing plugin.
			return doctor.Skip(checkName, "Bridge plugin not installed")
		}

		return registrationChecker.failForParseError(checkName, err)
	}

	target := settings.ResolveOpenCodeTarget(ctx, c.openCode, pluginPath)

	rendered, err := settings.RenderOpenCodePlugin(binaryPath, target.API)
	if err != nil {
		return doctor.FailError(
			checkName,
			fmt.Sprintf("Failed to render bridge plugin: %v", err),
		)
	}

	// An error rather than a warning: doctor --fix only heals errors, and a
	// plugin that no longer matches this binary is silently validating with
	// whatever an older release shipped.
	if current != string(rendered) {
		return doctor.FailError(checkName, "Bridge plugin is out of date").
			WithDetails(
				"File: "+pluginPath,
				"Expected the bridge for: "+target.Describe(),
				"Regenerate with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.Pass(checkName, "Current")
}

// OpenCodeEventChecker checks that the plugin forwards a specific opencode event.
type OpenCodeEventChecker struct {
	cfg       *pkgConfig.OpenCodeProviderConfig
	eventName string
}

// NewOpenCodeEventChecker creates a checker for a specific opencode event.
func NewOpenCodeEventChecker(
	cfg *pkgConfig.OpenCodeProviderConfig,
	eventName string,
) *OpenCodeEventChecker {
	return &OpenCodeEventChecker{
		cfg:       cfg,
		eventName: eventName,
	}
}

// Name returns the name of the check.
func (c *OpenCodeEventChecker) Name() string {
	return c.eventName + " hook in opencode plugin"
}

// Category returns the category of the check.
func (*OpenCodeEventChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the configured event coverage check.
func (c *OpenCodeEventChecker) Check(_ context.Context) doctor.CheckResult {
	checkName := c.eventName + " hook in opencode plugin"
	if result, ready := c.preflight(checkName); !ready {
		return result
	}

	registrationChecker := &OpenCodeRegistrationChecker{cfg: c.cfg}
	pluginPath := registrationChecker.pluginPath()

	return checkProviderEventHook(
		checkName,
		pluginPath,
		c.eventName,
		func(eventName, dispatcherPath string) (bool, error) {
			return settings.NewOpenCodePluginParser(pluginPath).HasEventHook(
				eventName,
				dispatcherPath,
			)
		},
		registrationChecker.failForParseError,
	)
}

func (c *OpenCodeRegistrationChecker) preflight(checkName string) (doctor.CheckResult, bool) {
	if c.cfg == nil || !c.cfg.IsEnabled() {
		return doctor.Skip(checkName, "opencode provider disabled"), false
	}

	return doctor.CheckResult{}, true
}

// pluginPath is the configured location, or the default when unset.
func (c *OpenCodeRegistrationChecker) pluginPath() string {
	if c.cfg == nil {
		return settings.DefaultOpenCodePluginPath()
	}

	return settings.ResolveOpenCodePluginPath(c.cfg.PluginPath)
}

func (c *OpenCodeEventChecker) preflight(checkName string) (doctor.CheckResult, bool) {
	registrationChecker := &OpenCodeRegistrationChecker{cfg: c.cfg}

	return registrationChecker.preflight(checkName)
}

func (c *OpenCodeRegistrationChecker) failForParseError(
	checkName string,
	err error,
) doctor.CheckResult {
	if errors.Is(err, settings.ErrPluginNotFound) {
		return doctor.FailError(checkName, "Bridge plugin not found").
			WithDetails(
				"Expected at: "+c.pluginPath(),
				"Generate with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.FailError(
		checkName,
		fmt.Sprintf("Failed to read bridge plugin: %v", err),
	)
}

type (
	providerEventLookup         func(eventName, dispatcherPath string) (bool, error)
	providerParseErrorFormatter func(checkName string, err error) doctor.CheckResult
	providerRegistrationLookup  func(dispatcherPath string) (bool, error)
)

func checkProviderEventHook(
	checkName string,
	settingsPath string,
	eventName string,
	lookup providerEventLookup,
	failForParseError providerParseErrorFormatter,
) doctor.CheckResult {
	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip(checkName, "Binary not found in PATH")
	}

	hasHook, err := lookup(eventName, binaryPath)
	if err != nil {
		return failForParseError(checkName, err)
	}

	if !hasHook {
		return doctor.FailError(
			checkName,
			eventName+" hook not configured",
		).
			WithDetails(
				"File: "+settingsPath,
				"Register with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.Pass(checkName, "Configured")
}

func checkProviderRegistration(
	checkName string,
	settingsPath string,
	lookup providerRegistrationLookup,
	failForParseError providerParseErrorFormatter,
) doctor.CheckResult {
	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip(checkName, "Binary not found in PATH")
	}

	registered, err := lookup(binaryPath)
	if err != nil {
		return failForParseError(checkName, err)
	}

	if !registered {
		return doctor.FailError(checkName, "Dispatcher not registered").
			WithDetails(
				"File: "+settingsPath,
				"Register with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.Pass(checkName, "Registered")
}

// PathValidationChecker checks if the registered dispatcher path is valid
type PathValidationChecker struct{}

// NewPathValidationChecker creates a new path validation checker
func NewPathValidationChecker() *PathValidationChecker {
	return &PathValidationChecker{}
}

// Name returns the name of the check
func (*PathValidationChecker) Name() string {
	return "Dispatcher path is valid"
}

// Category returns the category of the check
func (*PathValidationChecker) Category() doctor.Category {
	return doctor.CategoryHook
}

// Check performs the path validation check
func (*PathValidationChecker) Check(_ context.Context) doctor.CheckResult {
	binaryPath, err := exec.LookPath(binaryName)
	if err != nil {
		return doctor.Skip("Dispatcher path is valid", "Binary not found")
	}

	// Ensure it's an absolute path
	absPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return doctor.FailWarning(
			"Dispatcher path is valid",
			fmt.Sprintf("Cannot resolve absolute path: %v", err),
		)
	}

	return doctor.Pass("Dispatcher path is valid", absPath)
}
