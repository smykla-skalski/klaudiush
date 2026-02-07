package fixers

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/prompt"
)

// ConfigFixer creates missing configuration files with default values.
type ConfigFixer struct {
	prompter prompt.Prompter
	writer   *config.Writer
}

// NewConfigFixer creates a new ConfigFixer.
func NewConfigFixer(prompter prompt.Prompter) *ConfigFixer {
	return &ConfigFixer{
		prompter: prompter,
		writer:   config.NewWriter(),
	}
}

// ID returns the fixer identifier.
func (*ConfigFixer) ID() string {
	return "config_fixer"
}

// Description returns a human-readable description.
func (*ConfigFixer) Description() string {
	return "Create missing configuration files with default values"
}

// CanFix checks if this fixer can fix the given result.
func (*ConfigFixer) CanFix(result doctor.CheckResult) bool {
	return (result.FixID == "create_global_config" || result.FixID == "create_project_config") &&
		result.Status == doctor.StatusFail
}

// Fix creates the missing config file.
func (f *ConfigFixer) Fix(_ context.Context, interactive bool) error {
	// Determine which configs to create
	needsGlobal := !f.writer.IsGlobalConfigExists()
	needsProject := !f.writer.IsProjectConfigExists()

	if needsGlobal {
		if err := f.createGlobalConfig(interactive); err != nil {
			return errors.Wrap(err, "failed to create global config")
		}
	}

	if needsProject {
		if err := f.createProjectConfig(interactive); err != nil {
			return errors.Wrap(err, "failed to create project config")
		}
	}

	return nil
}

func (f *ConfigFixer) createGlobalConfig(interactive bool) error {
	path := f.writer.GlobalConfigPath()

	if interactive {
		msg := fmt.Sprintf("Create global config at %s?", path)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	// Create default config
	cfg := config.DefaultConfig()

	// Write to global location
	if err := f.writer.WriteGlobal(cfg); err != nil {
		return errors.Wrap(err, "failed to write global config")
	}

	return nil
}

func (f *ConfigFixer) createProjectConfig(interactive bool) error {
	path := f.writer.ProjectConfigPath()

	if interactive {
		msg := fmt.Sprintf("Create project config at %s?", path)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	// Create default config
	cfg := config.DefaultConfig()

	// Write to project location
	if err := f.writer.WriteProject(cfg); err != nil {
		return errors.Wrap(err, "failed to write project config")
	}

	return nil
}
