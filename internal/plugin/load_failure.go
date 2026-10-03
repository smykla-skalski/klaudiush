package plugin

import (
	"context"
	"fmt"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// loadFailure stands in for a plugin that failed to load. Every context the
// plugin would have checked gets an unavailable result instead of silently
// going unchecked. A plugin whose predicate cannot be built stands in for
// every context.
type loadFailure struct {
	*validator.BaseValidator
	name      string
	err       error
	predicate *PredicateMatcher
}

func newLoadFailure(
	cfg *config.PluginInstanceConfig,
	err error,
	log logger.Logger,
) *loadFailure {
	predicate, predicateErr := NewPredicateMatcher(cfg.Predicate)
	if predicateErr != nil {
		predicate = nil
	}

	return &loadFailure{
		BaseValidator: validator.NewBaseValidator("plugin:"+cfg.Name, log),
		name:          cfg.Name,
		err:           err,
		predicate:     predicate,
	}
}

// Validate reports that the plugin could not check the context.
func (f *loadFailure) Validate(context.Context, *hook.Context) *validator.Result {
	return validator.Unavailable(
		validator.ReasonError,
		fmt.Sprintf("Plugin %s failed to load: %v", f.name, f.err),
	)
}

// Category returns the validator's workload category.
func (*loadFailure) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}
