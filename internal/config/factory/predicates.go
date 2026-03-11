package factory

import (
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func beforeToolOnlyPredicate() validator.Predicate {
	return validator.EventIs(hook.CanonicalEventBeforeTool)
}

func beforeToolOrCodexAfterToolPredicate() validator.Predicate {
	return validator.Or(
		validator.EventIs(hook.CanonicalEventBeforeTool),
		validator.And(
			validator.ProviderIs(hook.ProviderCodex),
			validator.EventIs(hook.CanonicalEventAfterTool),
		),
	)
}

func lifecycleEventPredicate() validator.Predicate {
	return validator.Or(
		validator.EventIs(hook.CanonicalEventSessionStart),
		validator.EventIs(hook.CanonicalEventTurnStop),
	)
}
