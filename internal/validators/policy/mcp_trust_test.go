package policy_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/policy"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("MCPTrustValidator", func() {
	var cfg *config.MCPTrustConfig

	validate := func(provider hook.Provider, event, payload string) *validator.Result {
		v := policy.NewMCPTrustValidator(logger.NewNoOpLogger(), cfg)

		return v.Validate(context.Background(), parseHook(provider, event, payload))
	}

	BeforeEach(func() {
		enabled := true
		cfg = &config.MCPTrustConfig{
			Enabled:        &enabled,
			TrustedSources: []string{"managed"},
			Servers: []*config.MCPTrustedServer{
				{Name: "docs", Source: "project", Tools: []string{"search*"}},
				{Name: "plugin:tools:*", Source: "plugin"},
				{Command: "/opt/mcp/*"},
				{URL: "https://*.example.com/mcp/*"},
				{Command: "npx", Args: []string{"-y", "@acme/server@*"}},
				{Name: "ignored"},
			},
		}
	})

	It("is named mcp-trust", func() {
		v := policy.NewMCPTrustValidator(logger.NewNoOpLogger(), cfg)
		Expect(v.Name()).To(Equal(policy.MCPTrustValidatorName))
		Expect(v.Category()).To(Equal(validator.CategoryCPU))
	})

	It("ignores tools that are not MCP tools", func() {
		Expect(validate(hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"Bash","tool_input":{"command":"ls"}}`).Passed).To(BeTrue())
	})

	DescribeTable("trusts by provenance",
		func(provider hook.Provider, event, payload string) {
			Expect(validate(provider, event, payload).Passed).To(BeTrue())
		},
		Entry(
			"trusted source",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__x__y","tool_input":{},"mcp_server":{"name":"x","source":"managed"}}`,
		),
		Entry(
			"pinned name and source with tool",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__docs__search_all","tool_input":{},"mcp_server":{"name":"docs","source":"project"}}`,
		),
		Entry(
			"plugin server",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__plugin_tools_db__q","tool_input":{},"mcp_server":{"name":"plugin:tools:db","source":"plugin"}}`,
		),
		Entry(
			"gemini command",
			hook.ProviderGemini,
			"BeforeTool",
			`{"tool_name":"mcp_srv_run","tool_input":{},"mcp_context":{"server_name":"srv","command":"/opt/mcp/server"}}`,
		),
	)

	DescribeTable("distrusts what provenance does not support",
		func(provider hook.Provider, event, payload, contains string) {
			result := validate(provider, event, payload)
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefMCPUntrustedSource))
			Expect(result.Message).To(ContainSubstring(contains))
		},
		Entry(
			"spoofed tool prefix",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__docs__search","tool_input":{},"mcp_server":{"name":"evil","source":"project"}}`,
			"tool name claims server",
		),
		Entry(
			"trusted name from another source",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__docs__search","tool_input":{},"mcp_server":{"name":"docs","source":"user"}}`,
			"source",
		),
		Entry(
			"tool outside the trusted list",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__docs__delete","tool_input":{},"mcp_server":{"name":"docs","source":"project"}}`,
			"docs",
		),
		Entry(
			"name-only entry",
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__ignored__x","tool_input":{},"mcp_server":{"name":"ignored","source":"local"}}`,
			"ignored",
		),
		Entry("unknown source string", hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"mcp__a__b","tool_input":{},"mcp_server":{"name":"a","source":"sdk2"}}`,
			"sdk2"),
		Entry(
			"gemini url",
			hook.ProviderGemini,
			"BeforeTool",
			`{"tool_name":"mcp_srv_run","tool_input":{},"mcp_context":{"server_name":"srv","url":"https://evil.example.com"}}`,
			"url",
		),
		Entry(
			"gemini websocket",
			hook.ProviderGemini,
			"BeforeTool",
			`{"tool_name":"mcp_srv_run","tool_input":{},"mcp_context":{"server_name":"srv","tcp":"localhost:1"}}`,
			"tcp",
		),
	)

	It("escapes untrusted names", func() {
		result := validate(hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"mcp__a__b","tool_input":{},"mcp_server":{"name":"bad\u0007\"name`+
				`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","source":"local"}}`)

		Expect(result.Message).NotTo(ContainSubstring("\u0007"))
		Expect(result.Message).To(ContainSubstring("..."))
	})

	It("warns instead of blocking when configured", func() {
		cfg.Untrusted = config.MCPTrustActionWarn

		result := validate(hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"mcp__a__b","tool_input":{},"mcp_server":{"name":"a","source":"local"}}`)
		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeFalse())
	})

	It("says when nothing is trusted", func() {
		cfg.TrustedSources = nil
		cfg.Servers = nil

		result := validate(hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"mcp__a__b","tool_input":{},"mcp_server":{"name":"a","source":"local"}}`)
		Expect(result.Findings[0].Required).To(Equal("no MCP server is trusted"))
	})

	Context("without provenance", func() {
		payloads := []struct {
			provider hook.Provider
			event    string
			payload  string
		}{
			{
				hook.ProviderCodex,
				"PreToolUse",
				`{"hook_event_name":"PreToolUse","tool_name":"mcp__docs__search","tool_input":{},"turn_id":"t"}`,
			},
			{
				hook.ProviderClaude,
				"PreToolUse",
				`{"tool_name":"mcp__docs__search","tool_input":{},"mcp_server":"docs"}`,
			},
			{
				hook.ProviderClaude,
				"PreToolUse",
				`{"tool_name":"mcp__docs__search","tool_input":{}}`,
			},
			{
				hook.ProviderGemini,
				"BeforeTool",
				`{"tool_name":"mcp_docs_search","tool_input":{},"mcp_context":{"server_name":"docs"}}`,
			},
		}

		It("blocks by default", func() {
			for _, p := range payloads {
				result := validate(p.provider, p.event, p.payload)
				Expect(result.ShouldBlock).To(BeTrue(), p.payload)
				Expect(result.Reference).To(Equal(validator.RefMCPUnknownProvenance))
			}
		})

		It("warns or allows when configured", func() {
			cfg.UnknownProvenance = config.MCPTrustActionWarn
			result := validate(payloads[0].provider, payloads[0].event, payloads[0].payload)
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Passed).To(BeFalse())

			cfg.UnknownProvenance = config.MCPTrustActionAllow
			result = validate(payloads[0].provider, payloads[0].event, payloads[0].payload)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
