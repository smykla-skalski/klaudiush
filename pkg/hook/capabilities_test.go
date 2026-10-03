package hook

import (
	"slices"
	"testing"
)

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
		{ProviderClaude, CanonicalEventPreCompress},
		{ProviderGemini, CanonicalEventSubagentStop},
		{ProviderOpenCode, CanonicalEventTurnStop},
		{ProviderUnknown, CanonicalEventBeforeTool},
		{Provider("bogus"), CanonicalEventBeforeTool},
	} {
		if _, ok := ProviderEventCapability(tc.provider, tc.event); ok {
			t.Errorf("unexpected contract for %s/%s", tc.provider, tc.event)
		}
	}
}

func TestCompletionGates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider Provider
		raw      string
		want     bool
	}{
		{"claude stop", ProviderClaude, "Stop", true},
		{"claude subagent stop", ProviderClaude, "SubagentStop", true},
		{"claude session end", ProviderClaude, "SessionEnd", false},
		{"claude stop failure", ProviderClaude, "StopFailure", false},
		{"codex stop", ProviderCodex, "Stop", true},
		{"codex subagent stop", ProviderCodex, "SubagentStop", true},
		{"codex session end", ProviderCodex, "SessionEnd", false},
		{"gemini after agent", ProviderGemini, "AfterAgent", true},
		{"gemini session end", ProviderGemini, "SessionEnd", false},
		{"opencode session idle cannot enforce", ProviderOpenCode, "session.idle", false},
		{"claude pre tool use", ProviderClaude, "PreToolUse", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := IsCompletionGate(tc.provider, NormalizeEventName(tc.raw), tc.raw)
			if got != tc.want {
				t.Errorf(
					"IsCompletionGate(%s, %s) = %v, want %v",
					tc.provider,
					tc.raw,
					got,
					tc.want,
				)
			}
		})
	}
}

func TestObservationalEventsPromiseNoBlock(t *testing.T) {
	blockingFields := []ResponseField{
		ResponseFieldDecision,
		ResponseFieldContinue,
		ResponseFieldPermissionDecision,
		ResponseFieldPermissionBehavior,
		ResponseFieldElicitationAction,
	}

	for _, tc := range []struct {
		provider Provider
		raw      string
	}{
		{ProviderClaude, "SessionEnd"},
		{ProviderClaude, "StopFailure"},
		{ProviderClaude, "Notification"},
		{ProviderCodex, "SessionEnd"},
		{ProviderGemini, "SessionEnd"},
		{ProviderGemini, "Notification"},
		{ProviderGemini, "PreCompress"},
	} {
		capability, ok := ResolveEventCapability(tc.provider, NormalizeEventName(tc.raw), tc.raw)
		if !ok {
			t.Errorf("%s/%s: expected a recorded contract", tc.provider, tc.raw)

			continue
		}

		if capability.Enforcement != EnforcementNone {
			t.Errorf(
				"%s/%s: enforcement = %q, want none",
				tc.provider,
				tc.raw,
				capability.Enforcement,
			)
		}

		for _, field := range blockingFields {
			if capability.Supports(field) {
				t.Errorf("%s/%s must not accept %q", tc.provider, tc.raw, field)
			}
		}
	}
}

func TestResolveEventCapabilityPermissionRequest(t *testing.T) {
	for _, provider := range []Provider{ProviderClaude, ProviderCodex} {
		capability, ok := ResolveEventCapability(
			provider,
			CanonicalEventBeforeTool,
			"PermissionRequest",
		)
		if !ok {
			t.Fatalf("%s: expected a PermissionRequest contract", provider)
		}

		if capability.Enforcement != EnforcementDenyPermission {
			t.Errorf("%s: enforcement = %q", provider, capability.Enforcement)
		}

		if capability.Supports(ResponseFieldPermissionDecision) {
			t.Errorf("%s: PermissionRequest must not accept permissionDecision", provider)
		}
	}

	for _, provider := range []Provider{ProviderGemini, ProviderOpenCode, ProviderUnknown} {
		if _, ok := ResolveEventCapability(
			provider,
			CanonicalEventBeforeTool,
			"PermissionRequest",
		); ok {
			t.Errorf("%s: unexpected PermissionRequest contract", provider)
		}
	}

	if _, ok := ResolveEventCapability(
		ProviderCodex,
		CanonicalEventSessionStart,
		"SubagentStart",
	); ok {
		t.Error("Codex SubagentStart must not inherit the SessionStart contract")
	}
}

func TestKeepsRawEventName(t *testing.T) {
	for _, tc := range []struct {
		provider Provider
		raw      string
		want     bool
	}{
		{ProviderClaude, "PermissionRequest", true},
		{ProviderClaude, "PreToolUse", false},
		{ProviderClaude, "SubagentStart", true},
		{ProviderClaude, "PostToolUseFailure", true},
		{ProviderCodex, "PermissionRequest", true},
		{ProviderCodex, "SubagentStart", true},
		{ProviderCodex, "SubagentStop", false},
		{ProviderGemini, "PermissionRequest", false},
		{ProviderOpenCode, "permission.ask", false},
		{ProviderUnknown, "PermissionRequest", false},
	} {
		if got := KeepsRawEventName(tc.provider, tc.raw); got != tc.want {
			t.Errorf("KeepsRawEventName(%s, %s) = %v, want %v", tc.provider, tc.raw, got, tc.want)
		}
	}
}

func TestClaudeElicitationAcceptsBothDeclineForms(t *testing.T) {
	for _, raw := range []string{"Elicitation", "ElicitationResult"} {
		capability, ok := ResolveEventCapability(ProviderClaude, NormalizeEventName(raw), raw)
		if !ok {
			t.Fatalf("%s: expected a Claude contract", raw)
		}

		if capability.Enforcement != EnforcementDeclineElicitation {
			t.Errorf("%s: enforcement = %q, want decline_elicitation", raw, capability.Enforcement)
		}

		for _, field := range []ResponseField{ResponseFieldDecision, ResponseFieldElicitationAction} {
			if !capability.Supports(field) {
				t.Errorf("%s must accept %q", raw, field)
			}
		}

		if capability.Supports(ResponseFieldSystemMessage) {
			t.Errorf("%s must not accept systemMessage", raw)
		}
	}
}

func TestNativeEventNames(t *testing.T) {
	for _, tc := range []struct {
		provider Provider
		want     []string
		absent   []string
	}{
		{
			ProviderClaude,
			[]string{"PreToolUse", "PostToolUse", "PostToolUseFailure", "Stop", "PermissionRequest"},
			[]string{"AfterToolUse", "BeforeTool", "AfterAgent"},
		},
		{
			ProviderCodex,
			[]string{"PreToolUse", "PostToolUse", "Stop", "SessionStart", "SubagentStart"},
			[]string{"AfterToolUse", "Notification", "PostToolUseFailure"},
		},
		{
			ProviderGemini,
			[]string{"BeforeTool", "AfterTool", "AfterAgent", "BeforeToolSelection"},
			[]string{"PreToolUse", "Stop"},
		},
		{
			ProviderOpenCode,
			[]string{"tool.execute.before", "tool.execute.after", "session.idle"},
			[]string{"permission.ask", "PreToolUse"},
		},
	} {
		got := NativeEventNames(tc.provider)
		if !slices.IsSorted(got) {
			t.Errorf("%s: names not sorted: %v", tc.provider, got)
		}

		for _, name := range tc.want {
			if !slices.Contains(got, name) {
				t.Errorf("%s: missing %q in %v", tc.provider, name, got)
			}
		}

		for _, name := range tc.absent {
			if slices.Contains(got, name) {
				t.Errorf("%s: unexpected %q", tc.provider, name)
			}
		}
	}

	if got := NativeEventNames(ProviderUnknown); got != nil {
		t.Errorf("unknown provider: got %v", got)
	}

	if got := NativeEventNames(Provider("bogus")); got != nil {
		t.Errorf("bogus provider: got %v", got)
	}
}
