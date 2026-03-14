package elicitation

import (
	"context"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func TestServerValidator_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *config.ElicitationServerConfig
		hookCtx   *hook.Context
		wantPass  bool
		wantRef   validator.Reference
		wantBlock bool
	}{
		{
			name: "pass with nil config",
			cfg:  nil,
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "some-server",
				},
			},
			wantPass: true,
		},
		{
			name: "pass when server name is empty",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"allowed-server"},
			},
			hookCtx: &hook.Context{
				Event:       hook.CanonicalEventElicitation,
				Elicitation: nil,
			},
			wantPass: true,
		},
		{
			name: "pass when server matches allow list",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"my-server", "other-server"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "my-server",
				},
			},
			wantPass: true,
		},
		{
			name: "fail RefMCPServerNotAllowed when server not on allow list",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"allowed-a", "allowed-b"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "unknown-server",
				},
			},
			wantPass:  false,
			wantRef:   validator.RefMCPServerNotAllowed,
			wantBlock: true,
		},
		{
			name: "fail RefMCPServerBlocked when server on deny list",
			cfg: &config.ElicitationServerConfig{
				DeniedServers: []string{"evil-server"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "evil-server",
				},
			},
			wantPass:  false,
			wantRef:   validator.RefMCPServerBlocked,
			wantBlock: true,
		},
		{
			name: "pass when server does not match deny list",
			cfg: &config.ElicitationServerConfig{
				DeniedServers: []string{"evil-server"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "good-server",
				},
			},
			wantPass: true,
		},
		{
			name: "fail RefMCPURLModeBlocked when URL mode blocked and mode is url",
			cfg: &config.ElicitationServerConfig{
				BlockURLMode: new(true),
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "some-server",
					Mode:          "url",
				},
			},
			wantPass:  false,
			wantRef:   validator.RefMCPURLModeBlocked,
			wantBlock: true,
		},
		{
			name: "pass when URL mode blocked but mode is form",
			cfg: &config.ElicitationServerConfig{
				BlockURLMode: new(true),
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "some-server",
					Mode:          "form",
				},
			},
			wantPass: true,
		},
		{
			name: "glob pattern matching in allow list",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"my-*"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "my-server",
				},
			},
			wantPass: true,
		},
		{
			name: "glob pattern matching in deny list",
			cfg: &config.ElicitationServerConfig{
				DeniedServers: []string{"bad-*"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "bad-actor",
				},
			},
			wantPass:  false,
			wantRef:   validator.RefMCPServerBlocked,
			wantBlock: true,
		},
		{
			name: "glob pattern no match in allow list blocks",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"my-*"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "other-server",
				},
			},
			wantPass:  false,
			wantRef:   validator.RefMCPServerNotAllowed,
			wantBlock: true,
		},
		{
			name: "deny list checked before allow list",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"*"},
				DeniedServers:  []string{"blocked-server"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "blocked-server",
				},
			},
			wantPass:  false,
			wantRef:   validator.RefMCPServerBlocked,
			wantBlock: true,
		},
		{
			name: "case insensitive matching",
			cfg: &config.ElicitationServerConfig{
				AllowedServers: []string{"My-Server"},
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "my-server",
				},
			},
			wantPass: true,
		},
		{
			name: "URL mode not blocked when BlockURLMode is false",
			cfg: &config.ElicitationServerConfig{
				BlockURLMode: new(false),
			},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "some-server",
					Mode:          "url",
				},
			},
			wantPass: true,
		},
		{
			name: "URL mode not blocked when BlockURLMode is nil",
			cfg:  &config.ElicitationServerConfig{},
			hookCtx: &hook.Context{
				Event: hook.CanonicalEventElicitation,
				Elicitation: &hook.ElicitationInput{
					MCPServerName: "some-server",
					Mode:          "url",
				},
			},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewServerValidator(logger.NewNoOpLogger(), tt.cfg, nil)
			result := v.Validate(context.Background(), tt.hookCtx)

			if result.Passed != tt.wantPass {
				t.Errorf(
					"Passed = %v, want %v (message: %s)",
					result.Passed,
					tt.wantPass,
					result.Message,
				)
			}

			if !tt.wantPass {
				if result.Reference != tt.wantRef {
					t.Errorf("Reference = %v, want %v", result.Reference, tt.wantRef)
				}

				if result.ShouldBlock != tt.wantBlock {
					t.Errorf("ShouldBlock = %v, want %v", result.ShouldBlock, tt.wantBlock)
				}
			}
		})
	}
}
