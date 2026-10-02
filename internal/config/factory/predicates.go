package factory

import (
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func beforeToolOnlyPredicate() validator.Predicate {
	return validator.EventIs(hook.CanonicalEventBeforeTool)
}

// beforeToolOrProviderAfterToolPredicate selects pre-execution validation for
// every provider, plus post-execution validation for the providers that report
// the tool arguments on their after-tool event. Claude's PostToolUse omits
// them, so file validators would have nothing to inspect there.
func beforeToolOrProviderAfterToolPredicate() validator.Predicate {
	return validator.Or(
		validator.EventIs(hook.CanonicalEventBeforeTool),
		validator.And(
			validator.Or(
				validator.ProviderIs(hook.ProviderCodex),
				validator.ProviderIs(hook.ProviderGemini),
				validator.ProviderIs(hook.ProviderOpenCode),
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
		validator.EventIs(hook.CanonicalEventSubagentStop),
		validator.EventIs(hook.CanonicalEventSessionEnd),
		validator.EventIs(hook.CanonicalEventStopFailure),
		validator.EventIs(hook.CanonicalEventPreCompress),
		validator.EventIs(hook.CanonicalEventPostCompact),
		validator.EventIs(hook.CanonicalEventUserPromptSubmit),
	)
}
