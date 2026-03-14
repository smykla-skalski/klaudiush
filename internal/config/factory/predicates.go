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
			validator.Or(
				validator.ProviderIs(hook.ProviderCodex),
				validator.ProviderIs(hook.ProviderGemini),
			),
			validator.EventIs(hook.CanonicalEventAfterTool),
		),
	)
}

func elicitationEventPredicate() validator.Predicate {
	return validator.Or(
		validator.EventIs(hook.CanonicalEventElicitation),
		validator.EventIs(hook.CanonicalEventElicitationResult),
	)
}

func lifecycleEventPredicate() validator.Predicate {
	return validator.Or(
		validator.EventIs(hook.CanonicalEventSessionStart),
		validator.EventIs(hook.CanonicalEventTurnStop),
		validator.EventIs(hook.CanonicalEventPreCompress),
		validator.EventIs(hook.CanonicalEventPostCompact),
	)
}
