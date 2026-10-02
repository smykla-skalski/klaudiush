package hook

import "testing"

func TestCodexPreToolUseRejectsLifecycleFields(t *testing.T) {
	capability, ok := ProviderEventCapability(ProviderCodex, CanonicalEventBeforeTool)
	if !ok {
		t.Fatal("expected a Codex PreToolUse contract")
	}

	if capability.NativeName != "PreToolUse" {
		t.Fatalf("NativeName = %q, want PreToolUse", capability.NativeName)
	}

	for _, field := range []ResponseField{
		ResponseFieldContinue,
		ResponseFieldStopReason,
		ResponseFieldSuppressOutput,
		ResponseFieldDecision,
	} {
		if capability.Supports(field) {
			t.Errorf("Codex PreToolUse must not accept %q", field)
		}
	}

	for _, field := range []ResponseField{
		ResponseFieldPermissionDecision,
		ResponseFieldAdditionalContext,
		ResponseFieldSystemMessage,
	} {
		if !capability.Supports(field) {
			t.Errorf("Codex PreToolUse should accept %q", field)
		}
	}
}

func TestProviderEventCapabilityUnknownPairs(t *testing.T) {
	for _, tc := range []struct {
		provider Provider
		event    CanonicalEvent
	}{
		{ProviderCodex, CanonicalEventNotification},
		{ProviderCodex, CanonicalEventElicitation},
		{ProviderCodex, CanonicalEventUnknown},
		{ProviderClaude, CanonicalEventBeforeTool},
		{Provider("bogus"), CanonicalEventBeforeTool},
	} {
		if _, ok := ProviderEventCapability(tc.provider, tc.event); ok {
			t.Errorf("unexpected contract for %s/%s", tc.provider, tc.event)
		}
	}
}
