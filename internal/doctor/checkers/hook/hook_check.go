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
	parser := settings.NewSettingsParser(c.settingsPath)

	hasHook, err := parser.HasPreToolUseHook()
	if err != nil {
		if errors.Is(err, settings.ErrSettingsNotFound) {
			return doctor.Skip(
				fmt.Sprintf("PreToolUse hook in %s settings", c.settingsType),
				"Settings file not found",
			)
		}

		return doctor.FailWarning(
			fmt.Sprintf("PreToolUse hook in %s settings", c.settingsType),
			fmt.Sprintf("Failed to check: %v", err),
		)
	}

	if !hasHook {
		// For project settings, not having hook is just informational
		if c.settingsType == settingsTypeProject || c.settingsType == settingsTypeProjectLocal {
			return doctor.Pass(
				fmt.Sprintf("PreToolUse hook in %s settings", c.settingsType),
				"Not configured (optional, using user settings)",
			)
		}

		// For user settings, not having hook is an error
		return doctor.FailError(
			fmt.Sprintf("PreToolUse hook in %s settings", c.settingsType),
			"PreToolUse hook not configured",
		).
			WithDetails(
				"The dispatcher requires PreToolUse hooks to function",
				"Configure with: klaudiush doctor --fix",
			).
			WithFixID("install_hook")
	}

	return doctor.Pass(
		fmt.Sprintf("PreToolUse hook in %s settings", c.settingsType),
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
