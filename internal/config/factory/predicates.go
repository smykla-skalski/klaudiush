package factory

import (
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func beforeToolOnlyPredicate() validator.Predicate {
	return validator.EventIs(hook.CanonicalEventBeforeTool)
}

// beforeToolOrProviderAfterToolPredicate selects pre-execution validation for
// every provider, plus post-execution validation for Codex, Gemini, and
// opencode. Claude command checks stay pre-tool only: its PostToolUse carries
// the same command its PreToolUse already validated.
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

// fileResultPredicate selects file validators. On top of the events
// beforeToolOrProviderAfterToolPredicate covers, it inspects files after a
// Claude tool ran when PreToolUse could not have seen the result: files a
// shell command changed, and anything a failed tool may have left half
// written. A Write or Edit that succeeded wrote exactly what PreToolUse
// validated, and a blocking finding there would have denied it, so checking it
// again would only repeat the warnings.
func fileResultPredicate() validator.Predicate {
	return validator.Or(
		beforeToolOrProviderAfterToolPredicate(),
		validator.And(
			validator.ProviderIs(hook.ProviderClaude),
			validator.EventIs(hook.CanonicalEventAfterTool),
			func(ctx *hook.Context) bool {
				return ctx.Derived || ctx.ToolFailed()
			},
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
