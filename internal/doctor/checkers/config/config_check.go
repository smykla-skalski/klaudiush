// Package config provides checkers for configuration file validation.
package config

import (
	"context"
	"errors"
	"fmt"

	"github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/doctor"
)

// GlobalChecker checks the validity of the global configuration
type GlobalChecker struct {
	loader *config.Loader
}

// NewGlobalChecker creates a new global config checker
func NewGlobalChecker() *GlobalChecker {
	return &GlobalChecker{
		loader: config.NewLoader(),
	}
}

// Name returns the name of the check
func (*GlobalChecker) Name() string {
	return "Global config valid"
}

// Category returns the category of the check
func (*GlobalChecker) Category() doctor.Category {
	return doctor.CategoryConfig
}

// Check performs the global config validity check
func (c *GlobalChecker) Check(_ context.Context) doctor.CheckResult {
	cfg, err := c.loader.LoadGlobal()
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			return doctor.FailWarning("Global config valid", "Config file not found (optional)").
				WithDetails(
					"Expected at: "+c.loader.GlobalConfigPath(),
					"Create with: klaudiush init --global",
				).
				WithFixID("create_global_config")
		}

		if errors.Is(err, config.ErrInvalidTOML) {
			return doctor.FailError("Global config valid", "Invalid TOML syntax").
				WithDetails(
					"File: "+c.loader.GlobalConfigPath(),
					fmt.Sprintf("Error: %v", err),
				)
		}

		if errors.Is(err, config.ErrInvalidPermissions) {
			return doctor.FailError("Global config valid", "Insecure file permissions").
				WithDetails(
					"File: "+c.loader.GlobalConfigPath(),
					"Config file should not be world-writable",
					"Fix with: chmod 600 <config-file>",
				).
				WithFixID("fix_config_permissions")
		}

		return doctor.FailError("Global config valid", fmt.Sprintf("Failed to load: %v", err))
	}

	// Validate config semantics
	validator := config.NewValidator()
	if err := validator.Validate(cfg); err != nil {
		return doctor.FailError("Global config valid", "Configuration validation failed").
			WithDetails(
				"File: "+c.loader.GlobalConfigPath(),
				fmt.Sprintf("Error: %v", err),
			)
	}

	return doctor.Pass("Global config valid", "Valid")
}

// ProjectChecker checks the validity of the project configuration
type ProjectChecker struct {
	loader *config.Loader
}

// NewProjectChecker creates a new project config checker
func NewProjectChecker() *ProjectChecker {
	return &ProjectChecker{
		loader: config.NewLoader(),
	}
}

// Name returns the name of the check
func (*ProjectChecker) Name() string {
	return "Project config valid"
}

// Category returns the category of the check
func (*ProjectChecker) Category() doctor.Category {
	return doctor.CategoryConfig
}

// Check performs the project config validity check
func (c *ProjectChecker) Check(_ context.Context) doctor.CheckResult {
	cfg, err := c.loader.LoadProject()
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			paths := c.loader.ProjectConfigPaths()

			return doctor.FailWarning("Project config valid", "Config file not found (optional)").
				WithDetails(
					fmt.Sprintf("Checked paths: %v", paths),
					"Create with: klaudiush init",
				).
				WithFixID("create_project_config")
		}

		if errors.Is(err, config.ErrInvalidTOML) {
			return doctor.FailError("Project config valid", "Invalid TOML syntax").
				WithDetails(fmt.Sprintf("Error: %v", err))
		}

		if errors.Is(err, config.ErrInvalidPermissions) {
			return doctor.FailError("Project config valid", "Insecure file permissions").
				WithDetails(
					"Config file should not be world-writable",
					"Fix with: chmod 600 <config-file>",
				).
				WithFixID("fix_config_permissions")
		}

		return doctor.FailError("Project config valid", fmt.Sprintf("Failed to load: %v", err))
	}

	// Validate config semantics
	validator := config.NewValidator()
	if err := validator.Validate(cfg); err != nil {
		return doctor.FailError("Project config valid", "Configuration validation failed").
			WithDetails(fmt.Sprintf("Error: %v", err))
	}

	return doctor.Pass("Project config valid", "Valid")
}

// PermissionsChecker checks if config files have secure permissions
type PermissionsChecker struct {
	loader *config.Loader
}

// NewPermissionsChecker creates a new permissions checker
func NewPermissionsChecker() *PermissionsChecker {
	return &PermissionsChecker{
		loader: config.NewLoader(),
	}
}

// Name returns the name of the check
func (*PermissionsChecker) Name() string {
	return "Config file permissions secure"
}

// Category returns the category of the check
func (*PermissionsChecker) Category() doctor.Category {
	return doctor.CategoryConfig
}

// Check performs the permissions check
func (c *PermissionsChecker) Check(_ context.Context) doctor.CheckResult {
	// Check both global and project configs
	hasGlobal := c.loader.HasGlobalConfig()
	hasProject := c.loader.HasProjectConfig()

	if !hasGlobal && !hasProject {
		return doctor.Skip("Config file permissions secure", "No config files found")
	}

	// Try loading both - if they have permission issues, they'll fail
	globalErr := error(nil)
	if hasGlobal {
		_, globalErr = c.loader.LoadGlobal()
	}

	projectErr := error(nil)
	if hasProject {
		_, projectErr = c.loader.LoadProject()
	}

	// Check for permission errors
	hasPermissionError := false
	details := []string{}

	if globalErr != nil && errors.Is(globalErr, config.ErrInvalidPermissions) {
		hasPermissionError = true

		details = append(details, fmt.Sprintf("Global config: %v", globalErr))
	}

	if projectErr != nil && errors.Is(projectErr, config.ErrInvalidPermissions) {
		hasPermissionError = true

		details = append(details, fmt.Sprintf("Project config: %v", projectErr))
	}

	if hasPermissionError {
		return doctor.FailError("Config file permissions secure", "Config files have insecure permissions").
			WithDetails(append(details, "Fix with: chmod 600 <config-file>")...).
			WithFixID("fix_config_permissions")
	}

	return doctor.Pass("Config file permissions secure", "Secure")
}
