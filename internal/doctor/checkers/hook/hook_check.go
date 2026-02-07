// Package hook provides checkers for Claude settings and hook registration.
package hook

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/doctor/settings"
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
