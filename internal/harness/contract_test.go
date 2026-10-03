package harness_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("CheckEvent", func() {
	DescribeTable("accepts events the provider fires",
		func(provider hook.Provider, event string) {
			Expect(harness.CheckEvent(provider, event)).To(Succeed())
		},
		Entry("claude PreToolUse", hook.ProviderClaude, "PreToolUse"),
		Entry("claude PostToolUseFailure", hook.ProviderClaude, "PostToolUseFailure"),
		Entry("codex Stop", hook.ProviderCodex, "Stop"),
		Entry("codex PermissionRequest", hook.ProviderCodex, "PermissionRequest"),
		Entry("gemini AfterAgent", hook.ProviderGemini, "AfterAgent"),
		Entry("opencode tool.execute.before", hook.ProviderOpenCode, "tool.execute.before"),
	)

	DescribeTable(
		"rejects stale and foreign event names",
		func(provider hook.Provider, event, message string) {
			err := harness.CheckEvent(provider, event)
			Expect(err).To(MatchError(harness.ErrContract))
			Expect(err.Error()).To(ContainSubstring(message))
		},
		Entry(
			"codex legacy AfterToolUse",
			hook.ProviderCodex,
			"AfterToolUse",
			`fires "PostToolUse" instead`,
		),
		Entry("opencode permission.ask", hook.ProviderOpenCode, "permission.ask", "stale"),
		Entry("gemini name sent to claude", hook.ProviderClaude, "BeforeTool", "does not fire"),
		Entry("claude name sent to gemini", hook.ProviderGemini, "PreToolUse", "does not fire"),
		Entry("unknown provider", hook.Provider("bogus"), "PreToolUse", "does not fire"),
	)
})

var _ = Describe("CheckPayload", func() {
	It("accepts a tool payload with the event name, tool and input", func() {
		payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`
		Expect(
			harness.CheckPayload(hook.ProviderClaude, "PreToolUse", []byte(payload)),
		).To(Succeed())
	})

	It("accepts a lifecycle payload without tool fields", func() {
		payload := `{"hook_event_name":"Stop","stop_hook_active":false}`
		Expect(harness.CheckPayload(hook.ProviderCodex, "Stop", []byte(payload))).To(Succeed())
	})

	DescribeTable("rejects payloads a provider does not send",
		func(event, payload, message string) {
			err := harness.CheckPayload(hook.ProviderClaude, event, []byte(payload))
			Expect(err).To(MatchError(harness.ErrContract))
			Expect(err.Error()).To(ContainSubstring(message))
		},
		Entry("not an object", "PreToolUse", `[]`, "not a JSON object"),
		Entry("event name mismatch", "PreToolUse", `{"hook_event_name":"PostToolUse"}`, "want"),
		Entry("missing tool input", "PreToolUse",
			`{"hook_event_name":"PreToolUse","tool_name":"Bash"}`, "tool_input"),
		Entry("stale event", "AfterToolUse", `{"hook_event_name":"AfterToolUse"}`, "does not fire"),
	)
})

var _ = Describe("CheckResponse", func() {
	DescribeTable("accepts responses within the capability table",
		func(provider hook.Provider, event, response string) {
			Expect(harness.CheckResponse(provider, event, []byte(response))).To(Succeed())
		},
		Entry("empty pass", hook.ProviderCodex, "PreToolUse", ""),
		Entry("claude deny", hook.ProviderClaude, "PreToolUse",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",`+
				`"permissionDecisionReason":"no","additionalContext":"ctx"},"systemMessage":"m"}`),
		Entry("codex stop block", hook.ProviderCodex, "Stop",
			`{"decision":"block","reason":"r","systemMessage":"m"}`),
		Entry("claude permission request", hook.ProviderClaude, "PermissionRequest",
			`{"hookSpecificOutput":{"hookEventName":"PermissionRequest",`+
				`"decision":{"behavior":"deny","message":"no"}}}`),
		Entry("gemini tool selection", hook.ProviderGemini, "BeforeToolSelection",
			`{"hookSpecificOutput":{"hookEventName":"BeforeToolSelection",`+
				`"toolConfig":{"mode":"ANY","allowedFunctionNames":["read_file"]}}}`),
		Entry("opencode advisory", hook.ProviderOpenCode, "tool.execute.after",
			`{"continue":true,"hookSpecificOutput":{"hookEventName":"tool.execute.after",`+
				`"additionalContext":"fix"}}`),
	)

	DescribeTable(
		"rejects fields and values the provider does not accept",
		func(provider hook.Provider, event, response, message string) {
			err := harness.CheckResponse(provider, event, []byte(response))
			Expect(err).To(MatchError(harness.ErrContract))
			Expect(err.Error()).To(ContainSubstring(message))
		},
		Entry("codex PreToolUse continue", hook.ProviderCodex, "PreToolUse",
			`{"continue":false}`, "unsupported field continue"),
		Entry("unknown top-level field", hook.ProviderClaude, "PreToolUse",
			`{"verdict":"deny"}`, "unsupported field verdict"),
		Entry("unknown specific field", hook.ProviderClaude, "PreToolUse",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{}}}`,
			"unsupported field hookSpecificOutput.updatedInput"),
		Entry("wrong hookEventName", hook.ProviderClaude, "PreToolUse",
			`{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"x"}}`,
			"does not match"),
		Entry("codex ask decision", hook.ProviderCodex, "PreToolUse",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}`,
			"is not read by codex"),
		Entry("reason without decision", hook.ProviderCodex, "Stop",
			`{"reason":"r"}`, "reason without decision"),
		Entry("permission reason without decision", hook.ProviderClaude, "PreToolUse",
			`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecisionReason":"r"}}`,
			"without permissionDecision"),
		Entry("output on an event without a contract", hook.ProviderCodex, "SubagentStart",
			`{"systemMessage":"m"}`, "no recorded contract"),
		Entry("not JSON", hook.ProviderClaude, "PreToolUse", `deny`, "not a JSON object"),
		Entry("specific output not an object", hook.ProviderClaude, "PreToolUse",
			`{"hookSpecificOutput":"deny"}`, "is not an object"),
		Entry("stale event", hook.ProviderCodex, "AfterToolUse", `{}`, "stale"),
		Entry("opencode unknown field", hook.ProviderOpenCode, "tool.execute.before",
			`{"permissionDecision":"deny"}`, "unsupported field permissionDecision"),
		Entry("opencode unknown specific field", hook.ProviderOpenCode, "tool.execute.after",
			`{"hookSpecificOutput":{"hookEventName":"x","toolConfig":{}}}`,
			"unsupported field hookSpecificOutput.toolConfig"),
		Entry("opencode specific not an object", hook.ProviderOpenCode, "tool.execute.after",
			`{"hookSpecificOutput":[]}`, "is not an object"),
		Entry(
			"opencode not JSON",
			hook.ProviderOpenCode,
			"tool.execute.after",
			`x`,
			"not a JSON object",
		),
	)
})

var _ = Describe("ClassifyResponse", func() {
	DescribeTable(
		"names what a response asks for",
		func(response string, want harness.Outcome) {
			Expect(harness.ClassifyResponse([]byte(response))).To(Equal(want))
		},
		Entry("empty", "  ", harness.OutcomePass),
		Entry("notice only", `{"systemMessage":"note"}`, harness.OutcomePass),
		Entry(
			"pre-tool deny",
			`{"hookSpecificOutput":{"permissionDecision":"deny"}}`,
			harness.OutcomeDeny,
		),
		Entry("bridge deny", `{"decision":"deny"}`, harness.OutcomeDeny),
		Entry("permission request deny",
			`{"hookSpecificOutput":{"decision":{"behavior":"deny"}}}`, harness.OutcomeDeny),
		Entry("decision block", `{"decision":"block"}`, harness.OutcomeBlock),
		Entry("continue false", `{"continue":false}`, harness.OutcomeBlock),
		Entry(
			"context only",
			`{"hookSpecificOutput":{"additionalContext":"x"}}`,
			harness.OutcomeAdvise,
		),
	)

	It("rejects output that is not JSON", func() {
		_, err := harness.ClassifyResponse([]byte("nope"))
		Expect(err).To(MatchError(harness.ErrContract))
	})
})

var _ = Describe("committed fixtures", func() {
	fixtures, err := harness.LoadFixtures("testdata/fixtures")

	It("load and validate against the capability table", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures).NotTo(BeEmpty())

		for _, fixture := range fixtures {
			Expect(fixture.Validate()).To(Succeed(), fixture.Path())
		}
	})

	It("include a live pre-tool denial for every live-checked harness", func() {
		denials := map[hook.Provider]bool{}

		for _, fixture := range fixtures {
			if fixture.Source == harness.SourceLive && fixture.Expect == harness.OutcomeDeny &&
				hook.NormalizeEventName(fixture.Event) == hook.CanonicalEventBeforeTool {
				denials[fixture.Provider] = true
			}
		}

		Expect(denials).To(HaveKey(hook.ProviderClaude))
		Expect(denials).To(HaveKey(hook.ProviderCodex))
	})

	It("fail validation once a response gains an unsupported field", func() {
		for _, fixture := range fixtures {
			if fixture.Response == nil || string(fixture.Response) == "null" {
				continue
			}

			var response map[string]any
			Expect(json.Unmarshal(fixture.Response, &response)).To(Succeed())

			response["klaudiushExtra"] = true
			mutated, err := json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())

			fixture.Response = mutated
			Expect(
				fixture.Validate(),
			).To(MatchError(ContainSubstring("unsupported field")), fixture.Path())
		}
	})

	It("fail validation once the event name goes stale", func() {
		for _, fixture := range fixtures {
			if fixture.Provider != hook.ProviderCodex || fixture.Event != "PreToolUse" {
				continue
			}

			fixture.Event = "AfterToolUse"
			Expect(fixture.Validate()).To(MatchError(ContainSubstring("stale")), fixture.Path())
		}
	})
})

var _ = Describe("events klaudiush init registers", func() {
	DescribeTable("are all events the provider fires today",
		func(provider hook.Provider) {
			registered := harness.RegisteredEvents(provider)
			Expect(registered).NotTo(BeEmpty())

			for _, event := range registered {
				Expect(harness.CheckEvent(provider, event)).To(Succeed(), event)
			}
		},
		Entry("claude", hook.ProviderClaude),
		Entry("codex", hook.ProviderCodex),
		Entry("gemini", hook.ProviderGemini),
		Entry("opencode", hook.ProviderOpenCode),
	)
})
