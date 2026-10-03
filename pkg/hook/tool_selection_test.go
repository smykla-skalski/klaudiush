package hook

import (
	"slices"
	"testing"
)

func TestToolSelectionIsGeminiOnly(t *testing.T) {
	for _, name := range []string{"BeforeToolSelection", "before_tool_selection", "tool_selection"} {
		if got := NormalizeEventName(name); got != CanonicalEventToolSelection {
			t.Fatalf("NormalizeEventName(%q) = %q, want tool_selection", name, got)
		}
	}

	capability, ok := ProviderEventCapability(ProviderGemini, CanonicalEventToolSelection)
	if !ok || capability.NativeName != "BeforeToolSelection" ||
		!capability.Supports(ResponseFieldToolConfig) ||
		capability.Supports(
			ResponseFieldDecision,
		) || capability.Supports(ResponseFieldSystemMessage) {
		t.Fatalf("unexpected Gemini BeforeToolSelection contract: %+v", capability)
	}

	if !FiltersTools(ProviderGemini) {
		t.Fatal("Gemini filters tools")
	}

	for _, provider := range []Provider{ProviderClaude, ProviderCodex, ProviderOpenCode, ProviderUnknown} {
		if FiltersTools(provider) {
			t.Fatalf("%s has no tool-selection event", provider)
		}
	}

	if got := DisplayEventName(
		ProviderGemini,
		CanonicalEventToolSelection,
		EventTypeUnknown,
	); got != "BeforeToolSelection" {
		t.Fatalf("DisplayEventName = %q", got)
	}

	ctx := &Context{Provider: ProviderGemini, Event: CanonicalEventToolSelection}
	if !slices.Contains(ctx.EventNames(), "BeforeToolSelection") {
		t.Fatalf("EventNames = %v", ctx.EventNames())
	}

	if got := ResolveLegacyEventType(
		ProviderGemini,
		"BeforeToolSelection",
		EventTypeUnknown,
	); got != EventTypeUnknown {
		t.Fatalf("legacy event type = %v", got)
	}
}
