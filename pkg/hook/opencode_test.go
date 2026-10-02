package hook

import "testing"

func TestParseProvider_OpenCode(t *testing.T) {
	for _, input := range []string{"opencode", "OpenCode", " opencode "} {
		provider, err := ParseProvider(input)
		if err != nil {
			t.Fatalf("ParseProvider(%q) returned error: %v", input, err)
		}

		if provider != ProviderOpenCode {
			t.Errorf("ParseProvider(%q) = %q, want %q", input, provider, ProviderOpenCode)
		}
	}
}

func TestNormalizeEventName_OpenCodeHookIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CanonicalEvent
	}{
		{"dotted before tool", "tool.execute.before", CanonicalEventBeforeTool},
		{"dotted after tool", "tool.execute.after", CanonicalEventAfterTool},
		{
			name:     "permission ask gates before execution",
			input:    "permission.ask",
			expected: CanonicalEventBeforeTool,
		},
		{"session created", "session.created", CanonicalEventSessionStart},
		{"session idle", "session.idle", CanonicalEventTurnStop},
		{"session error is observational", "session.error", CanonicalEventStopFailure},
		{"session compacting", "session.compacting", CanonicalEventPreCompress},
		{"session compacted", "session.compacted", CanonicalEventPostCompact},
		{"chat message", "chat.message", CanonicalEventUserPromptSubmit},
		{"permission asked", "permission.asked", CanonicalEventNotification},
		{"permission updated legacy spelling", "permission.updated", CanonicalEventNotification},
		{"claude user prompt alias", "UserPromptSubmit", CanonicalEventUserPromptSubmit},
		{"subagent start folds onto session start", "SubagentStart", CanonicalEventSessionStart},
		{"subagent stop gates the subagent", "SubagentStop", CanonicalEventSubagentStop},
		{
			"post tool use failure folds onto after tool",
			"PostToolUseFailure",
			CanonicalEventAfterTool,
		},
		{"stop failure is observational", "StopFailure", CanonicalEventStopFailure},
		{"session end is observational", "SessionEnd", CanonicalEventSessionEnd},
		{"gemini after agent is the turn stop", "AfterAgent", CanonicalEventTurnStop},
		{"claude stop", "Stop", CanonicalEventTurnStop},
		{"permission request gates before the tool", "PermissionRequest", CanonicalEventBeforeTool},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeEventName(tt.input); got != tt.expected {
				t.Errorf("NormalizeEventName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Dotted opencode ids must not leak into the other providers' inference paths.
func TestNormalizeEventName_DottedNamesDoNotBreakExistingProviders(t *testing.T) {
	if got := NormalizeEventName("PreToolUse"); got != CanonicalEventBeforeTool {
		t.Errorf("NormalizeEventName(PreToolUse) = %q, want %q", got, CanonicalEventBeforeTool)
	}

	if got := NormalizeEventName("BeforeTool"); got != CanonicalEventBeforeTool {
		t.Errorf("NormalizeEventName(BeforeTool) = %q, want %q", got, CanonicalEventBeforeTool)
	}
}

func TestDisplayEventName_OpenCodeRoundTrip(t *testing.T) {
	tests := []struct {
		canonical CanonicalEvent
		expected  string
	}{
		{CanonicalEventBeforeTool, "tool.execute.before"},
		{CanonicalEventAfterTool, "tool.execute.after"},
		{CanonicalEventUserPromptSubmit, "chat.message"},
		{CanonicalEventSessionStart, "session.created"},
		{CanonicalEventTurnStop, "session.idle"},
		{CanonicalEventNotification, "permission.asked"},
		{CanonicalEventPreCompress, "session.compacting"},
		{CanonicalEventPostCompact, "session.compacted"},
	}

	for _, tt := range tests {
		t.Run(string(tt.canonical), func(t *testing.T) {
			got := DisplayEventName(ProviderOpenCode, tt.canonical, EventTypeUnknown)
			if got != tt.expected {
				t.Errorf(
					"DisplayEventName(opencode, %q) = %q, want %q",
					tt.canonical,
					got,
					tt.expected,
				)
			}
		})
	}
}

func TestDisplayEventName_OpenCodeElicitationAndFallback(t *testing.T) {
	tests := []struct {
		canonical CanonicalEvent
		fallback  EventType
		expected  string
	}{
		{CanonicalEventElicitation, EventTypeUnknown, "Elicitation"},
		{CanonicalEventElicitationResult, EventTypeUnknown, "ElicitationResult"},
		// No opencode name for the event: fall back to the legacy enum.
		{CanonicalEventUnknown, EventTypePreToolUse, "PreToolUse"},
		{CanonicalEventUnknown, EventTypeUnknown, ""},
	}

	for _, tt := range tests {
		got := DisplayEventName(ProviderOpenCode, tt.canonical, tt.fallback)
		if got != tt.expected {
			t.Errorf(
				"DisplayEventName(opencode, %q, %v) = %q, want %q",
				tt.canonical,
				tt.fallback,
				got,
				tt.expected,
			)
		}
	}
}

func TestDefaultEventName_OpenCode(t *testing.T) {
	if got := DefaultEventName(ProviderOpenCode); got != "tool.execute.before" {
		t.Errorf("DefaultEventName(opencode) = %q, want tool.execute.before", got)
	}
}

func TestResolveToolMetadata_OpenCodeTools(t *testing.T) {
	tests := []struct {
		raw          string
		expectedType ToolType
		expectedFam  ToolFamily
	}{
		{"bash", ToolTypeBash, ToolFamilyShell},
		{"write", ToolTypeWrite, ToolFamilyWrite},
		{"edit", ToolTypeEdit, ToolFamilyEdit},
		{"patch", ToolTypeEdit, ToolFamilyEdit},
		{"read", ToolTypeRead, ToolFamilyRead},
		{"grep", ToolTypeGrep, ToolFamilyGrep},
		{"glob", ToolTypeGlob, ToolFamilyGlob},
		{"list", ToolTypeGlob, ToolFamilyGlob},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			toolType, family := ResolveToolMetadata(tt.raw)
			if toolType != tt.expectedType {
				t.Errorf(
					"ResolveToolMetadata(%q) type = %v, want %v",
					tt.raw,
					toolType,
					tt.expectedType,
				)
			}

			if family != tt.expectedFam {
				t.Errorf(
					"ResolveToolMetadata(%q) family = %q, want %q",
					tt.raw,
					family,
					tt.expectedFam,
				)
			}
		})
	}
}

func TestOpenCodeEventNames_CoverForwardedHooks(t *testing.T) {
	names := OpenCodeEventNames()
	if len(names) == 0 {
		t.Fatal("OpenCodeEventNames() returned no events")
	}

	// Every forwarded hook id must normalize to a known canonical event,
	// otherwise the bridge plugin would send events klaudiush drops.
	for _, name := range names {
		if NormalizeEventName(name) == CanonicalEventUnknown {
			t.Errorf("forwarded opencode event %q does not normalize to a canonical event", name)
		}
	}
}
