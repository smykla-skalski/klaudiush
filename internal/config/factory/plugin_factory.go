package factory

import (
	"context"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/plugin"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// PluginValidatorFactory creates validators from plugin configuration.
type PluginValidatorFactory struct {
	logger   logger.Logger
	registry *plugin.Registry
}

// NewPluginValidatorFactory creates a new PluginValidatorFactory.
func NewPluginValidatorFactory(log logger.Logger) *PluginValidatorFactory {
	return &PluginValidatorFactory{
		logger:   log,
		registry: plugin.NewRegistry(log),
	}
}

// CreateValidators creates validators from plugin configuration.
func (f *PluginValidatorFactory) CreateValidators(cfg *config.Config) []ValidatorWithPredicate {
	if cfg == nil || cfg.Plugins == nil || !cfg.Plugins.IsEnabled() {
		return nil
	}

	// Load all plugins
	if err := f.registry.LoadPlugins(cfg.Plugins); err != nil {
		f.logger.Error("failed to load plugins", "error", err)

		return nil
	}

	// Create a single catch-all validator that delegates to the registry
	// The registry will match plugins based on their predicates at runtime
	pluginValidator := &PluginRegistryValidator{
		BaseValidator: validator.NewBaseValidator("plugin-registry", f.logger),
		registry:      f.registry,
	}

	// Register with a predicate that matches all PreToolUse events
	// Individual plugins will do more specific matching
	return []ValidatorWithPredicate{
		{
			Validator: pluginValidator,
			Predicate: validator.EventTypeIs(hook.EventTypePreToolUse),
		},
	}
}

// Close releases plugin resources.
func (f *PluginValidatorFactory) Close() error {
	return f.registry.Close()
}

// PluginRegistryValidator delegates to the plugin registry for validation.
type PluginRegistryValidator struct {
	*validator.BaseValidator
	registry *plugin.Registry
}

// Validate delegates to matching plugins.
func (v *PluginRegistryValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	// Get validators for plugins that match this context
	plugins := v.registry.GetValidators(hookCtx)
	if len(plugins) == 0 {
		return validator.Pass()
	}

	// Run all matching plugins and aggregate results
	var warnings []string

	var blockingResult validator.Result

	var hasBlockingResult bool

	for _, p := range plugins {
		result := p.Validate(ctx, hookCtx)

		// Collect warnings
		if !result.Passed && !result.ShouldBlock {
			warnings = append(warnings, result.Message)
		}

		// Keep first blocking result
		if result.ShouldBlock && !hasBlockingResult {
			blockingResult = *result
			hasBlockingResult = true
		}
	}

	// If any plugin blocked, return aggregated blocking result
	if hasBlockingResult {
		// Append any warnings to the blocking message
		if len(warnings) > 0 {
			blockingResult.Message += "\n\nWarnings from other plugins:\n- " +
				strings.Join(warnings, "\n- ")
		}

		return &blockingResult
	}

	// If only warnings, return warning result with all warnings
	if len(warnings) > 0 {
		return validator.Warn(strings.Join(warnings, "\n"))
	}

	return validator.Pass()
}

// Category returns the validator's workload category.
func (*PluginRegistryValidator) Category() validator.ValidatorCategory {
	// Plugins handle their own categorization via the adapter
	return validator.CategoryCPU
}
