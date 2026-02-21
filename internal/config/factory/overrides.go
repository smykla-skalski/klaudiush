package factory

import "github.com/smykla-skalski/klaudiush/pkg/config"

// isValidatorOverridden checks if a validator is disabled via overrides config.
func isValidatorOverridden(overrides *config.OverridesConfig, name string) bool {
	if overrides == nil {
		return false
	}

	return overrides.IsDisabled(name)
}
