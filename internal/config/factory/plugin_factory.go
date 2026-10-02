package factory

import (
	"context"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/plugin"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
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

	if isValidatorOverridden(cfg.Overrides, "plugins") {
		return nil
	}

	// Load all plugins
	// Plugins that fail to load stay registered as failures, so the contexts
	// they would have checked report validation unavailable.
	if err := f.registry.LoadPlugins(cfg.Plugins); err != nil {
		f.logger.Error("failed to load plugins", "error", err)
	}

	// Create a single catch-all validator that delegates to the registry
	// The registry will match plugins based on their predicates at runtime
	pluginValidator := &PluginRegistryValidator{
		BaseValidator: validator.NewBaseValidator("plugin-registry", f.logger),
		registry:      f.registry,
	}

	// Register with a catch-all predicate and let per-plugin predicates
	// decide which lifecycle/tool/provider combinations should run.
	return []ValidatorWithPredicate{
		{
			Validator: pluginValidator,
			Predicate: validator.Always(),
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

	var (
		warnings    []string
		blocking    *validator.Result
		unavailable []*validator.Result
	)

	for _, p := range plugins {
		result := p.Validate(ctx, hookCtx)

		switch {
		case result.Passed:
		case result.Unavailable:
			unavailable = append(unavailable, result)
		case result.ShouldBlock:
			if blocking == nil {
				blocking = result
			}
		default:
			warnings = append(warnings, result.Message)
		}
	}

	if blocking != nil {
		merged := *blocking

		for _, u := range unavailable {
			warnings = append(warnings, u.Message)
		}

		if len(warnings) > 0 {
			merged.Message += "\n\nWarnings from other plugins:\n- " +
				strings.Join(warnings, "\n- ")
		}

		return &merged
	}

	if len(unavailable) > 0 {
		return mergeUnavailable(unavailable, warnings)
	}

	if len(warnings) > 0 {
		return validator.Warn(strings.Join(warnings, "\n"))
	}

	return validator.Pass()
}

// mergeUnavailable combines the plugins that gave no verdict into one
// unavailable result that blocks when any of them asked to.
func mergeUnavailable(unavailable []*validator.Result, warnings []string) *validator.Result {
	merged := *unavailable[0]

	messages := make([]string, 0, len(unavailable)+len(warnings))
	for _, u := range unavailable {
		messages = append(messages, u.Message)
		merged.ShouldBlock = merged.ShouldBlock || u.ShouldBlock
	}

	merged.Message = strings.Join(append(messages, warnings...), "\n")

	return &merged
}

// Category returns the validator's workload category.
func (*PluginRegistryValidator) Category() validator.ValidatorCategory {
	// Plugins handle their own categorization via the adapter
	return validator.CategoryCPU
}
