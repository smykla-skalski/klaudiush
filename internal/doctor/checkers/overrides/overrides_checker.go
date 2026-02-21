// Package overrides provides health checkers for the overrides configuration.
package overrides

import (
	"context"
	"fmt"
	"sort"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// ConfigLoader defines the interface for configuration loading operations.
type ConfigLoader interface {
	Load(flags map[string]any) (*config.Config, error)
}

// ExpiredChecker warns about expired overrides that should be cleaned up.
type ExpiredChecker struct {
	loader    ConfigLoader
	loaderErr error
}

// NewExpiredChecker creates a new expired overrides checker.
func NewExpiredChecker() *ExpiredChecker {
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return &ExpiredChecker{loaderErr: err}
	}

	return &ExpiredChecker{loader: loader}
}

// NewExpiredCheckerWithLoader creates an ExpiredChecker with a custom loader (for testing).
func NewExpiredCheckerWithLoader(loader ConfigLoader) *ExpiredChecker {
	return &ExpiredChecker{loader: loader}
}

// Name returns the name of the check.
func (*ExpiredChecker) Name() string {
	return "Expired Overrides"
}

// Category returns the category of the check.
func (*ExpiredChecker) Category() doctor.Category {
	return doctor.CategoryOverrides
}

// Check looks for expired override entries.
func (c *ExpiredChecker) Check(_ context.Context) doctor.CheckResult {
	if c.loaderErr != nil {
		return doctor.FailError("Expired Overrides",
			fmt.Sprintf("config loader initialization failed: %v", c.loaderErr))
	}

	cfg, err := c.loader.Load(nil)
	if err != nil {
		return doctor.FailError("Expired Overrides",
			fmt.Sprintf("failed to load config: %v", err))
	}

	if cfg.Overrides == nil || len(cfg.Overrides.Entries) == 0 {
		return doctor.Pass("Expired Overrides", "No overrides configured")
	}

	expired := cfg.Overrides.ExpiredEntries()
	if len(expired) == 0 {
		return doctor.Pass("Expired Overrides", "No expired overrides found")
	}

	// Build sorted details for deterministic output
	keys := make([]string, 0, len(expired))
	for k := range expired {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	details := make([]string, 0, len(expired))

	for _, key := range keys {
		entry := expired[key]

		detail := fmt.Sprintf("%s: expired at %s", key, entry.ExpiresAt)
		if entry.Reason != "" {
			detail += fmt.Sprintf(" (reason: %s)", entry.Reason)
		}

		details = append(details, detail)
	}

	return doctor.FailWarning("Expired Overrides",
		fmt.Sprintf("%d expired override(s) should be removed", len(expired))).
		WithDetails(details...).
		WithFixID("remove_expired_overrides")
}

// UnknownTargetChecker errors on unknown error codes or validator names in overrides.
type UnknownTargetChecker struct {
	loader    ConfigLoader
	loaderErr error
}

// NewUnknownTargetChecker creates a new unknown target checker.
func NewUnknownTargetChecker() *UnknownTargetChecker {
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return &UnknownTargetChecker{loaderErr: err}
	}

	return &UnknownTargetChecker{loader: loader}
}

// NewUnknownTargetCheckerWithLoader creates an UnknownTargetChecker with a custom loader (for testing).
func NewUnknownTargetCheckerWithLoader(loader ConfigLoader) *UnknownTargetChecker {
	return &UnknownTargetChecker{loader: loader}
}

// Name returns the name of the check.
func (*UnknownTargetChecker) Name() string {
	return "Override Targets"
}

// Category returns the category of the check.
func (*UnknownTargetChecker) Category() doctor.Category {
	return doctor.CategoryOverrides
}

// Check validates that all override keys are known error codes or validator names.
func (c *UnknownTargetChecker) Check(_ context.Context) doctor.CheckResult {
	if c.loaderErr != nil {
		return doctor.FailError("Override Targets",
			fmt.Sprintf("config loader initialization failed: %v", c.loaderErr))
	}

	cfg, err := c.loader.Load(nil)
	if err != nil {
		return doctor.FailError("Override Targets",
			fmt.Sprintf("failed to load config: %v", err))
	}

	if cfg.Overrides == nil || len(cfg.Overrides.Entries) == 0 {
		return doctor.Pass("Override Targets", "No overrides configured")
	}

	// Collect unknown targets
	var unknown []string

	for key := range cfg.Overrides.Entries {
		if !config.IsKnownTarget(key) {
			unknown = append(unknown, key)
		}
	}

	if len(unknown) == 0 {
		return doctor.Pass("Override Targets",
			fmt.Sprintf("All %d override target(s) are valid", len(cfg.Overrides.Entries)))
	}

	sort.Strings(unknown)

	details := make([]string, 0, len(unknown)+1)

	for _, key := range unknown {
		details = append(details, fmt.Sprintf(
			"%q is not a known error code or validator name", key,
		))
	}

	details = append(details, "Check spelling or remove invalid entries")

	return doctor.FailError("Override Targets",
		fmt.Sprintf("%d unknown override target(s)", len(unknown))).
		WithDetails(details...)
}

// RedundantChecker warns when a code-level override exists but the parent validator
// is already overridden with the same effect.
type RedundantChecker struct {
	loader    ConfigLoader
	loaderErr error
}

// NewRedundantChecker creates a new redundant overrides checker.
func NewRedundantChecker() *RedundantChecker {
	loader, err := internalconfig.NewKoanfLoader()
	if err != nil {
		return &RedundantChecker{loaderErr: err}
	}

	return &RedundantChecker{loader: loader}
}

// NewRedundantCheckerWithLoader creates a RedundantChecker with a custom loader (for testing).
func NewRedundantCheckerWithLoader(loader ConfigLoader) *RedundantChecker {
	return &RedundantChecker{loader: loader}
}

// Name returns the name of the check.
func (*RedundantChecker) Name() string {
	return "Redundant Overrides"
}

// Category returns the category of the check.
func (*RedundantChecker) Category() doctor.Category {
	return doctor.CategoryOverrides
}

// Check looks for code-level overrides that are redundant because the parent
// validator is already overridden with the same disabled state.
func (c *RedundantChecker) Check(_ context.Context) doctor.CheckResult {
	if c.loaderErr != nil {
		return doctor.FailError("Redundant Overrides",
			fmt.Sprintf("config loader initialization failed: %v", c.loaderErr))
	}

	cfg, err := c.loader.Load(nil)
	if err != nil {
		return doctor.FailError("Redundant Overrides",
			fmt.Sprintf("failed to load config: %v", err))
	}

	if cfg.Overrides == nil || len(cfg.Overrides.Entries) == 0 {
		return doctor.Pass("Redundant Overrides", "No overrides configured")
	}

	// For each code-level entry, check if the parent validator is also overridden
	var redundant []string

	for key, entry := range cfg.Overrides.Entries {
		parent, isCode := config.CodeToValidator[key]
		if !isCode {
			continue // not a code-level entry
		}

		parentEntry, parentExists := cfg.Overrides.Entries[parent]
		if !parentExists {
			continue
		}

		// Both must be active to be considered redundant
		if !entry.IsActive() || !parentEntry.IsActive() {
			continue
		}

		// Check if they have the same disabled state (both already confirmed active above)
		codeDisabled := entryIsDisabled(entry)
		parentDisabled := entryIsDisabled(parentEntry)

		if codeDisabled == parentDisabled {
			state := "disabled"
			if !codeDisabled {
				state = "enabled"
			}

			redundant = append(redundant, fmt.Sprintf(
				"%s is redundant: parent %s is already %s", key, parent, state))
		}
	}

	if len(redundant) == 0 {
		return doctor.Pass("Redundant Overrides", "No redundant overrides found")
	}

	sort.Strings(redundant)

	return doctor.FailWarning("Redundant Overrides",
		fmt.Sprintf("%d redundant override(s) can be removed", len(redundant))).
		WithDetails(redundant...).
		WithFixID("remove_redundant_overrides")
}

// entryIsDisabled returns whether this entry represents a "disable" override.
// Default is true (disabled) when the Disabled field is nil.
func entryIsDisabled(e *config.OverrideEntry) bool {
	if e == nil || e.Disabled == nil {
		return true
	}

	return *e.Disabled
}
