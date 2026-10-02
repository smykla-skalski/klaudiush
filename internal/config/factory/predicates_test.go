package factory

import (
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func TestBeforeToolOrProviderAfterToolPredicateMatchesGeminiAfterTool(t *testing.T) {
	predicate := beforeToolOrProviderAfterToolPredicate()

	if !predicate(&hook.Context{
		Provider: hook.ProviderGemini,
		Event:    hook.CanonicalEventAfterTool,
	}) {
		t.Fatal("expected Gemini AfterTool to match post-action predicate")
	}
}

func TestBeforeToolOrProviderAfterToolPredicateDoesNotMatchClaudePostTool(t *testing.T) {
	predicate := beforeToolOrProviderAfterToolPredicate()

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

// opencode reports the tool arguments on its after-tool event, so file
// validators can still inspect what was written. Excluding it would leave a
// rule scoped to tool.execute.after silently never running.
func TestBeforeToolOrProviderAfterToolPredicateMatchesOpenCodeAfterTool(t *testing.T) {
	predicate := beforeToolOrProviderAfterToolPredicate()

	if !predicate(&hook.Context{
		Provider: hook.ProviderOpenCode,
		Event:    hook.CanonicalEventAfterTool,
	}) {
		t.Fatal("expected opencode AfterTool to match post-action predicate")
	}
}

func TestBeforeToolOrProviderAfterToolPredicateMatchesOpenCodeBeforeTool(t *testing.T) {
	predicate := beforeToolOrProviderAfterToolPredicate()

	if !predicate(&hook.Context{
		Provider: hook.ProviderOpenCode,
		Event:    hook.CanonicalEventBeforeTool,
	}) {
		t.Fatal("expected opencode BeforeTool to match post-action predicate")
	}
}

// chat.message is offered as a rule event filter, so the rule engine has to be
// dispatched for it.
func TestLifecycleEventPredicateMatchesUserPromptSubmit(t *testing.T) {
	predicate := lifecycleEventPredicate()

	if !predicate(&hook.Context{
		Provider: hook.ProviderOpenCode,
		Event:    hook.CanonicalEventUserPromptSubmit,
	}) {
		t.Fatal("expected UserPromptSubmit to match lifecycle predicate")
	}
}

func TestFileResultPredicateClaudeAfterTool(t *testing.T) {
	predicate := fileResultPredicate()

	cases := []struct {
		name string
		ctx  *hook.Context
		want bool
	}{
		{
			name: "pre-tool write",
			ctx: &hook.Context{
				Provider: hook.ProviderClaude,
				Event:    hook.CanonicalEventBeforeTool,
			},
			want: true,
		},
		{
			name: "write that succeeded repeats pre-tool validation",
			ctx: &hook.Context{
				Provider:      hook.ProviderClaude,
				Event:         hook.CanonicalEventAfterTool,
				ToolExecuted:  true,
				ToolSucceeded: true,
			},
			want: false,
		},
		{
			name: "failed tool may leave partial changes",
			ctx: &hook.Context{
				Provider:     hook.ProviderClaude,
				Event:        hook.CanonicalEventAfterTool,
				ToolExecuted: true,
			},
			want: true,
		},
		{
			name: "file a shell command changed",
			ctx: &hook.Context{
				Provider:      hook.ProviderClaude,
				Event:         hook.CanonicalEventAfterTool,
				ToolExecuted:  true,
				ToolSucceeded: true,
				Derived:       true,
			},
			want: true,
		},
		{
			name: "codex after-tool keeps its coverage",
			ctx:  &hook.Context{Provider: hook.ProviderCodex, Event: hook.CanonicalEventAfterTool},
			want: true,
		},
	}

	for _, tc := range cases {
		if got := predicate(tc.ctx); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
