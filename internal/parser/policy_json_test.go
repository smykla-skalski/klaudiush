package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/parser"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("MCP provenance and ConfigChange", func() {
	parse := func(provider hook.Provider, event, payload string) *hook.Context {
		ctx, err := parser.NewJSONParser(strings.NewReader(payload)).ParseWithOptions(
			parser.ParseOptions{Provider: provider, EventName: event},
		)
		Expect(err).NotTo(HaveOccurred())

		return ctx
	}

	It("reads Claude's mcp_server object", func() {
		ctx := parse(
			hook.ProviderClaude,
			"PreToolUse",
			`{"tool_name":"mcp__db__q","tool_input":{},"mcp_server":{"name":"plugin:p:db","source":"plugin"}}`,
		)

		Expect(ctx.MCPServer).To(Equal(&hook.MCPProvenance{Name: "plugin:p:db", Source: "plugin"}))
	})

	It("reads the older string form as a name without source", func() {
		ctx := parse(hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"mcp__db__q","tool_input":{},"mcp_server":"db"}`)

		Expect(ctx.MCPServer).To(Equal(&hook.MCPProvenance{Name: "db"}))
	})

	It("leaves provenance empty when absent or unusable", func() {
		for _, payload := range []string{
			`{"tool_name":"mcp__db__q","tool_input":{}}`,
			`{"tool_name":"mcp__db__q","tool_input":{},"mcp_server":""}`,
			`{"tool_name":"mcp__db__q","tool_input":{},"mcp_server":{}}`,
			`{"tool_name":"mcp__db__q","tool_input":{},"mcp_server":42}`,
			`{"tool_name":"mcp__db__q","tool_input":{},"mcp_context":"x"}`,
		} {
			Expect(parse(hook.ProviderClaude, "PreToolUse", payload).MCPServer).To(BeNil(), payload)
		}
	})

	It("reads Gemini's mcp_context", func() {
		ctx := parse(
			hook.ProviderGemini,
			"BeforeTool",
			`{"tool_name":"mcp_db_q","tool_input":{},"mcp_context":{"server_name":"db","tool_name":"q",`+
				`"command":"node","args":["s.js"],"cwd":"/w","url":"https://x","tcp":"h:1"}}`,
		)

		Expect(ctx.MCPServer).To(Equal(&hook.MCPProvenance{
			Name: "db", Tool: "q", Command: "node", Args: []string{"s.js"}, Cwd: "/w",
			URL: "https://x", TCP: "h:1",
		}))
	})

	It("reads ConfigChange source and file", func() {
		ctx := parse(
			hook.ProviderClaude,
			"ConfigChange",
			`{"hook_event_name":"ConfigChange","source":"project_settings","file_path":"/p/.claude/settings.json"}`,
		)

		Expect(ctx.Event).To(Equal(hook.CanonicalEventConfigChange))
		Expect(ctx.ConfigChange).To(Equal(&hook.ConfigChangeInput{
			Source: "project_settings", FilePath: "/p/.claude/settings.json",
		}))

		Expect(parse(hook.ProviderClaude, "PreToolUse",
			`{"tool_name":"Bash","tool_input":{"command":"ls"}}`).ConfigChange).To(BeNil())
	})
})
