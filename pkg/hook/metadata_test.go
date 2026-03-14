package hook

import "testing"

func TestNormalizeEventName_NewEvents(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected CanonicalEvent
	}{
		{
			name:     "elicitation lowercase",
			input:    "elicitation",
			expected: CanonicalEventElicitation,
		},
		{
			name:     "elicitation mixed case",
			input:    "Elicitation",
			expected: CanonicalEventElicitation,
		},
		{
			name:     "elicitation with underscores",
			input:    "elicitation",
			expected: CanonicalEventElicitation,
		},
		{
			name:     "elicitationresult lowercase",
			input:    "elicitationresult",
			expected: CanonicalEventElicitationResult,
		},
		{
			name:     "elicitationresult with underscore",
			input:    "elicitation_result",
			expected: CanonicalEventElicitationResult,
		},
		{
			name:     "elicitationresult mixed case",
			input:    "ElicitationResult",
			expected: CanonicalEventElicitationResult,
		},
		{
			name:     "postcompact lowercase",
			input:    "postcompact",
			expected: CanonicalEventPostCompact,
		},
		{
			name:     "postcompact with underscore",
			input:    "post_compact",
			expected: CanonicalEventPostCompact,
		},
		{
			name:     "postcompact mixed case",
			input:    "PostCompact",
			expected: CanonicalEventPostCompact,
		},
		{
			name:     "postcompress maps to postcompact",
			input:    "postcompress",
			expected: CanonicalEventPostCompact,
		},
		{
			name:     "postcompress with underscore",
			input:    "post_compress",
			expected: CanonicalEventPostCompact,
		},
		{
			name:     "postcompress mixed case",
			input:    "PostCompress",
			expected: CanonicalEventPostCompact,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeEventName(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeEventName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveLegacyEventType_NewEventsReturnUnknown(t *testing.T) {
	tests := []struct {
		name         string
		rawEventName string
	}{
		{
			name:         "elicitation",
			rawEventName: "elicitation",
		},
		{
			name:         "elicitation result",
			rawEventName: "elicitation_result",
		},
		{
			name:         "post compact",
			rawEventName: "post_compact",
		},
		{
			name:         "postcompress alias",
			rawEventName: "postcompress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, provider := range []Provider{ProviderClaude, ProviderCodex, ProviderGemini} {
				got := ResolveLegacyEventType(provider, tt.rawEventName, EventTypeUnknown)
				if got != EventTypeUnknown {
					t.Errorf("ResolveLegacyEventType(%q, %q, Unknown) = %v, want EventTypeUnknown",
						provider, tt.rawEventName, got)
				}
			}
		})
	}
}

func TestDisplayEventName_NewEvents(t *testing.T) {
	tests := []struct {
		name      string
		provider  Provider
		canonical CanonicalEvent
		expected  string
	}{
		// Claude
		{
			name:      "claude elicitation",
			provider:  ProviderClaude,
			canonical: CanonicalEventElicitation,
			expected:  "Elicitation",
		},
		{
			name:      "claude elicitation result",
			provider:  ProviderClaude,
			canonical: CanonicalEventElicitationResult,
			expected:  "ElicitationResult",
		},
		{
			name:      "claude post compact returns empty",
			provider:  ProviderClaude,
			canonical: CanonicalEventPostCompact,
			expected:  "",
		},
		// Codex
		{
			name:      "codex elicitation",
			provider:  ProviderCodex,
			canonical: CanonicalEventElicitation,
			expected:  "Elicitation",
		},
		{
			name:      "codex elicitation result",
			provider:  ProviderCodex,
			canonical: CanonicalEventElicitationResult,
			expected:  "ElicitationResult",
		},
		{
			name:      "codex post compact returns empty",
			provider:  ProviderCodex,
			canonical: CanonicalEventPostCompact,
			expected:  "",
		},
		// Gemini
		{
			name:      "gemini elicitation",
			provider:  ProviderGemini,
			canonical: CanonicalEventElicitation,
			expected:  "Elicitation",
		},
		{
			name:      "gemini elicitation result",
			provider:  ProviderGemini,
			canonical: CanonicalEventElicitationResult,
			expected:  "ElicitationResult",
		},
		{
			name:      "gemini post compact",
			provider:  ProviderGemini,
			canonical: CanonicalEventPostCompact,
			expected:  "PostCompact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayEventName(tt.provider, tt.canonical, EventTypeUnknown)
			if got != tt.expected {
				t.Errorf("DisplayEventName(%q, %q, Unknown) = %q, want %q",
					tt.provider, tt.canonical, got, tt.expected)
			}
		})
	}
}
