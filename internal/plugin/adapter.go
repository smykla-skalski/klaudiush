package plugin

import (
	"context"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/plugin"
)

// ValidatorAdapter adapts a Plugin to the Validator interface.
// This allows plugins to be used seamlessly alongside built-in validators
// in the dispatcher's validation pipeline.
type ValidatorAdapter struct {
	*validator.BaseValidator
	plugin   Plugin
	category validator.ValidatorCategory
}

// NewValidatorAdapter creates a new validator adapter for a plugin.
func NewValidatorAdapter(
	p Plugin,
	category validator.ValidatorCategory,
	log logger.Logger,
) *ValidatorAdapter {
	info := p.Info()

	return &ValidatorAdapter{
		BaseValidator: validator.NewBaseValidator("plugin:"+info.Name, log),
		plugin:        p,
		category:      category,
	}
}

// Validate performs validation using the plugin.
func (a *ValidatorAdapter) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	// Convert hook context to plugin request
	req := &plugin.ValidateRequest{
		EventType: hookCtx.EventType.String(),
		ToolName:  hookCtx.ToolName.String(),
		Command:   hookCtx.GetCommand(),
		FilePath:  hookCtx.GetFilePath(),
		Content:   hookCtx.GetContent(),
		OldString: hookCtx.ToolInput.OldString,
		NewString: hookCtx.ToolInput.NewString,
		Pattern:   hookCtx.ToolInput.Pattern,
	}

	// Call the plugin
	resp, err := a.plugin.Validate(ctx, req)
	if err != nil {
		a.Logger().Error("plugin validation error",
			"plugin", a.plugin.Info().Name,
			"error", err,
		)

		return validator.Fail("Plugin error: " + err.Error())
	}

	// Convert plugin response to validator result
	result := &validator.Result{
		Passed:      resp.Passed,
		Message:     resp.Message,
		ShouldBlock: resp.ShouldBlock,
		Details:     resp.Details,
	}

	// Use plugin's own error metadata if provided
	// Plugins manage their own error codes and documentation URLs
	if resp.DocLink != "" {
		result.Reference = validator.Reference(resp.DocLink)
	}

	result.FixHint = resp.FixHint

	return result
}

// Category returns the validator's workload category.
func (a *ValidatorAdapter) Category() validator.ValidatorCategory {
	return a.category
}

// Close releases plugin resources.
func (a *ValidatorAdapter) Close() error {
	return a.plugin.Close()
}
