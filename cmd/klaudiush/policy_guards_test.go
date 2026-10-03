package main

import (
	"slices"
	"testing"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

func TestPolicyGuardsAreCritical(t *testing.T) {
	if guards := policyGuards(nil); len(guards) != 0 {
		t.Fatalf("policyGuards(nil) = %v, want none", guards)
	}

	enabled := true
	cfg := &config.Config{
		Protection: &config.ProtectionConfig{Enabled: &enabled},
		MCPTrust:   &config.MCPTrustConfig{Enabled: &enabled},
	}

	guards := policyGuards(cfg)
	if !slices.Equal(guards, []string{"protection", "mcp-trust"}) {
		t.Fatalf("policyGuards = %v", guards)
	}

	failureMode = ""

	policy, err := buildPolicy(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range guards {
		if !policy.IsCritical(name) {
			t.Errorf("%s should be critical", name)
		}
	}

	plain, err := buildPolicy(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}

	if plain.IsCritical("protection") {
		t.Error("protection is critical only when enabled")
	}
}
