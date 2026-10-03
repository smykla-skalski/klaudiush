package hook

import "testing"

func TestConfigChangeEvent(t *testing.T) {
	for _, name := range []string{"ConfigChange", "config_change", "configchange"} {
		if got := NormalizeEventName(name); got != CanonicalEventConfigChange {
			t.Errorf("NormalizeEventName(%q) = %q, want config_change", name, got)
		}
	}

	if got := ResolveLegacyEventType(
		ProviderClaude,
		"ConfigChange",
		EventTypeUnknown,
	); got != EventTypeUnknown {
		t.Errorf("ConfigChange legacy type = %v, want unknown", got)
	}

	capability, ok := ProviderEventCapability(ProviderClaude, CanonicalEventConfigChange)
	if !ok || capability.Enforcement != EnforcementBlockDecision ||
		!capability.Supports(
			ResponseFieldDecision,
		) || capability.Supports(ResponseFieldSystemMessage) {
		t.Fatalf("Claude ConfigChange capability = %+v, %v", capability, ok)
	}

	for _, provider := range []Provider{ProviderCodex, ProviderGemini, ProviderOpenCode} {
		if _, ok := ProviderEventCapability(provider, CanonicalEventConfigChange); ok {
			t.Errorf("%s has no ConfigChange event", provider)
		}
	}

	ctx := &Context{Provider: ProviderClaude, Event: CanonicalEventConfigChange}
	if !ctx.MatchesEventName("ConfigChange") {
		t.Error("ConfigChange context should match its event name")
	}
}

func TestIsMCPTool(t *testing.T) {
	for _, tc := range []struct {
		ctx  *Context
		want bool
	}{
		{&Context{Provider: ProviderClaude, RawToolName: "mcp__fs__read"}, true},
		{&Context{Provider: ProviderClaude, RawToolName: "Bash"}, false},
		{&Context{Provider: ProviderClaude, RawToolName: "mcp_fs_read"}, false},
		{&Context{Provider: ProviderGemini, RawToolName: "mcp_fs_read"}, true},
		{&Context{Provider: ProviderClaude, RawToolName: "x", MCPServer: &MCPProvenance{Name: "x"}}, true},
	} {
		if got := tc.ctx.IsMCPTool(); got != tc.want {
			t.Errorf(
				"IsMCPTool(%s %q) = %v, want %v",
				tc.ctx.Provider,
				tc.ctx.RawToolName,
				got,
				tc.want,
			)
		}
	}
}
