package factory

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func TestBeforeToolOrProviderAfterToolPredicateMatchesGeminiAfterTool(t *testing.T) {
	predicate := beforeToolOrCodexAfterToolPredicate()

	if !predicate(&hook.Context{
		Provider: hook.ProviderGemini,
		Event:    hook.CanonicalEventAfterTool,
	}) {
		t.Fatal("expected Gemini AfterTool to match post-action predicate")
	}
}

func TestBeforeToolOrProviderAfterToolPredicateDoesNotMatchClaudePostTool(t *testing.T) {
	predicate := beforeToolOrCodexAfterToolPredicate()

	if predicate(&hook.Context{
		Provider: hook.ProviderClaude,
		Event:    hook.CanonicalEventAfterTool,
	}) {
		t.Fatal("did not expect Claude post-tool events to match post-action predicate")
	}
}

func TestLifecycleEventPredicateMatchesPreCompress(t *testing.T) {
	predicate := lifecycleEventPredicate()

	if !predicate(&hook.Context{
		Provider: hook.ProviderGemini,
		Event:    hook.CanonicalEventPreCompress,
	}) {
		t.Fatal("expected PreCompress to match lifecycle predicate")
	}
}
