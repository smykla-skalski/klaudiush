package fixers

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// OverridesFixer fixes expired and redundant overrides by removing them from config.
type OverridesFixer struct {
	prompter prompt.Prompter
	loader   *internalconfig.KoanfLoader
}

// NewOverridesFixer creates a new OverridesFixer.
func NewOverridesFixer(prompter prompt.Prompter) *OverridesFixer {
	loader, _ := internalconfig.NewKoanfLoader()

	return &OverridesFixer{
		prompter: prompter,
		loader:   loader,
	}
}

// ID returns the fixer identifier.
func (*OverridesFixer) ID() string {
	return "overrides_fixer"
}

// Description returns a human-readable description.
func (*OverridesFixer) Description() string {
	return "Remove expired and redundant overrides from configuration"
}

// CanFix checks if this fixer can fix the given result.
func (*OverridesFixer) CanFix(result doctor.CheckResult) bool {
	return (result.FixID == "remove_expired_overrides" ||
		result.FixID == "remove_redundant_overrides") &&
		result.Status == doctor.StatusFail
}

// Fix removes expired and/or redundant overrides from the project configuration.
func (f *OverridesFixer) Fix(_ context.Context, interactive bool) error {
	cfg, configPath, err := f.loadProjectConfig()
	if err != nil {
		return err
	}

	if cfg == nil || cfg.Overrides == nil || len(cfg.Overrides.Entries) == 0 {
		return nil
	}

	removals := f.collectRemovals(cfg.Overrides)
	if len(removals) == 0 {
		return nil
	}

	if interactive {
		if !f.confirmFix(removals) {
			return nil
		}
	}

	for _, key := range removals {
		delete(cfg.Overrides.Entries, key)
	}

	writer := internalconfig.NewWriter()
	if err := writer.WriteFile(configPath, cfg); err != nil {
		return errors.Wrapf(err, "failed to write config to %s", configPath)
	}

	return nil
}

// loadProjectConfig loads only the project configuration file.
func (f *OverridesFixer) loadProjectConfig() (*config.Config, string, error) {
	if f.loader == nil {
		loader, err := internalconfig.NewKoanfLoader()
		if err != nil {
			return nil, "", errors.Wrap(err, "failed to create config loader")
		}

		f.loader = loader
	}

	cfg, path, err := f.loader.LoadProjectConfigOnly()
	if err != nil {
		return nil, path, errors.Wrap(err, "failed to load project config")
	}

	return cfg, path, nil
}

// collectRemovals returns override keys that should be removed (expired + redundant).
func (*OverridesFixer) collectRemovals(overrides *config.OverridesConfig) []string {
	var removals []string

	seen := make(map[string]bool)

	// Collect expired entries
	for key, entry := range overrides.Entries {
		if entry.IsExpired() {
			removals = append(removals, key)
			seen[key] = true
		}
	}

	// Collect redundant code-level entries
	for key, entry := range overrides.Entries {
		if seen[key] {
			continue
		}

		parent, isCode := config.CodeToValidator[key]
		if !isCode {
			continue
		}

		parentEntry, parentExists := overrides.Entries[parent]
		if !parentExists || !entry.IsActive() || !parentEntry.IsActive() {
			continue
		}

		codeDisabled := overrideIsDisabled(entry)
		parentDisabled := overrideIsDisabled(parentEntry)

		if codeDisabled == parentDisabled {
			removals = append(removals, key)
		}
	}

	return removals
}

// confirmFix prompts the user for confirmation.
func (f *OverridesFixer) confirmFix(removals []string) bool {
	msg := fmt.Sprintf("Remove %d expired/redundant override(s)?", len(removals))

	confirmed, err := f.prompter.Confirm(msg, true)
	if err != nil {
		return false
	}

	return confirmed
}

// overrideIsDisabled returns whether an entry represents a "disable" override.
func overrideIsDisabled(e *config.OverrideEntry) bool {
	if e == nil || e.Disabled == nil {
		return true
	}

	return *e.Disabled
}
