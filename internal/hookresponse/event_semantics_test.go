package hookresponse_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func semanticsBlocking() []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{{
		Validator:   "git.commit",
		Message:     "missing signoff",
		ShouldBlock: true,
		Reference:   validator.RefGitNoSignoff,
		FixHint:     "Add -s",
	}}
}

func semanticsWarning() []*dispatcher.ValidationError {
	return []*dispatcher.ValidationError{{
		Validator: "markdown",
		Message:   "heading style",
	}}
}

func eventCtx(provider hook.Provider, raw string) *hook.Context {
	return &hook.Context{
		Provider:     provider,
		Event:        hook.NormalizeEventName(raw),
		RawEventName: raw,
	}
}

func responseFields(resp any) map[string]any {
	if hookresponse.IsEmpty(resp) {
		return nil
	}

	data, err := json.Marshal(resp)
	Expect(err).NotTo(HaveOccurred())

	var fields map[string]any
	Expect(json.Unmarshal(data, &fields)).To(Succeed())

	return fields
}

func hookSpecific(fields map[string]any) map[string]any {
	out, ok := fields["hookSpecificOutput"].(map[string]any)
	Expect(ok).To(BeTrue(), "hookSpecificOutput missing in %v", fields)

	return out
}

var _ = Describe("event decision semantics", func() {
	Describe("Claude completion gates", func() {
		DescribeTable("block and continue the agent on blocking findings",
			func(raw string) {
				fields := responseFields(hookresponse.BuildForContext(
					eventCtx(hook.ProviderClaude, raw), semanticsBlocking(), nil,
				))

				Expect(fields).To(HaveKeyWithValue("decision", "block"))
				Expect(fields["reason"]).To(ContainSubstring("completion check failed"))
				Expect(fields["reason"]).To(ContainSubstring("[GIT001]"))
				Expect(fields["reason"]).To(ContainSubstring("Add -s"))

				out := hookSpecific(fields)
				Expect(out).To(HaveKeyWithValue("hookEventName", raw))
				Expect(out).NotTo(HaveKey("permissionDecision"))
				Expect(out).To(HaveKey("additionalContext"))
			},
			Entry("Stop", "Stop"),
			Entry("SubagentStop", "SubagentStop"),
		)

		It("lets the turn end on warnings without additionalContext", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderClaude, "Stop"), semanticsWarning(), []string{"pattern"},
			))

			Expect(fields).NotTo(HaveKey("decision"))
			Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
			Expect(fields).To(HaveKey("systemMessage"))
		})
	})

	Describe("Claude observational events", func() {
		DescribeTable("emit nothing, since Claude discards their output",
			func(raw string) {
				Expect(hookresponse.IsEmpty(hookresponse.BuildForContext(
					eventCtx(hook.ProviderClaude, raw), semanticsBlocking(), nil,
				))).To(BeTrue())
			},
			Entry("SessionEnd", "SessionEnd"),
			Entry("StopFailure", "StopFailure"),
		)

		It("reports Notification findings to the user only", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderClaude, "Notification"), semanticsBlocking(), nil,
			))

			Expect(fields).To(HaveLen(1))
			Expect(fields).To(HaveKey("systemMessage"))
		})

		It("gives SessionStart findings to the model as context, never a decision", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderClaude, "SessionStart"), semanticsBlocking(), nil,
			))

			Expect(fields).NotTo(HaveKey("decision"))
			out := hookSpecific(fields)
			Expect(out).To(HaveKeyWithValue("hookEventName", "SessionStart"))
			Expect(out).To(HaveKey("additionalContext"))
			Expect(out).NotTo(HaveKey("permissionDecision"))
		})

		It("returns only systemMessage for an event with no recorded contract", func() {
			ctx := &hook.Context{Provider: hook.ProviderClaude, RawEventName: "TeammateIdle"}
			fields := responseFields(hookresponse.BuildForContext(ctx, semanticsBlocking(), nil))

			Expect(fields).To(HaveLen(1))
			Expect(fields).To(HaveKey("systemMessage"))
		})
	})

	It("blocks Claude UserPromptSubmit with a top-level decision", func() {
		fields := responseFields(hookresponse.BuildForContext(
			eventCtx(hook.ProviderClaude, "UserPromptSubmit"), semanticsBlocking(), nil,
		))

		Expect(fields).To(HaveKeyWithValue("decision", "block"))
		Expect(fields["reason"]).To(ContainSubstring("[GIT001]"))
		Expect(fields["reason"]).NotTo(ContainSubstring("completion check"))
	})

	It("keeps the PreToolUse permissionDecision shape", func() {
		fields := responseFields(hookresponse.BuildForContext(
			eventCtx(hook.ProviderClaude, "PreToolUse"), semanticsBlocking(), nil,
		))

		Expect(hookSpecific(fields)).To(HaveKeyWithValue("permissionDecision", "deny"))
	})

	It("treats an unknown provider as Claude", func() {
		fields := responseFields(hookresponse.BuildForContext(
			eventCtx(hook.ProviderUnknown, "PreToolUse"), semanticsBlocking(), nil,
		))

		Expect(hookSpecific(fields)).To(HaveKeyWithValue("permissionDecision", "deny"))
	})

	Describe("PermissionRequest", func() {
		DescribeTable("denies through the decision object",
			func(provider hook.Provider) {
				fields := responseFields(hookresponse.BuildForContext(
					eventCtx(provider, "PermissionRequest"), semanticsBlocking(), nil,
				))

				out := hookSpecific(fields)
				Expect(out).To(HaveKeyWithValue("hookEventName", "PermissionRequest"))
				Expect(out).NotTo(HaveKey("permissionDecision"))

				decision, ok := out["decision"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(decision).To(HaveKeyWithValue("behavior", "deny"))
				Expect(decision["message"]).To(ContainSubstring("[GIT001]"))
				Expect(decision).NotTo(HaveKey("interrupt"))
				Expect(decision).NotTo(HaveKey("updatedInput"))
				Expect(fields).NotTo(HaveKey("decision"))
				Expect(fields).NotTo(HaveKey("continue"))
			},
			Entry("Claude", hook.ProviderClaude),
			Entry("Codex", hook.ProviderCodex),
		)

		It("leaves the decision to the user on warnings", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderClaude, "PermissionRequest"), semanticsWarning(), nil,
			))

			Expect(fields).To(HaveLen(1))
			Expect(fields).To(HaveKey("systemMessage"))
		})

		It("returns nothing without findings", func() {
			Expect(hookresponse.BuildPermissionRequest(nil)).To(BeNil())
		})
	})

	Describe("Elicitation", func() {
		DescribeTable("declines with top-level block and hookSpecificOutput",
			func(raw string) {
				fields := responseFields(hookresponse.BuildForContext(
					eventCtx(hook.ProviderClaude, raw), semanticsBlocking(), nil,
				))

				Expect(fields).To(HaveLen(3))
				Expect(fields).To(HaveKeyWithValue("decision", "block"))
				Expect(fields).To(HaveKeyWithValue("reason", Not(BeEmpty())))
				Expect(fields).NotTo(HaveKey("systemMessage"))
				out := hookSpecific(fields)
				Expect(out).To(HaveKeyWithValue("hookEventName", raw))
				Expect(out).To(HaveKeyWithValue("action", "decline"))
				Expect(out).NotTo(HaveKey("content"))
			},
			Entry("Elicitation", "Elicitation"),
			Entry("ElicitationResult", "ElicitationResult"),
		)

		It("emits nothing for warnings", func() {
			Expect(hookresponse.IsEmpty(hookresponse.BuildForContext(
				eventCtx(hook.ProviderClaude, "Elicitation"), semanticsWarning(), nil,
			))).To(BeTrue())
		})

		It("tolerates a nil context", func() {
			resp := hookresponse.BuildElicitation(nil, semanticsBlocking(), nil)
			Expect(resp.HookSpecificOutput.Action).To(Equal("decline"))
			Expect(resp.Decision).To(Equal("block"))
		})
	})

	Describe("Gemini", func() {
		It("requests a correction on AfterAgent with deny and reason", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderGemini, "AfterAgent"), semanticsBlocking(), nil,
			))

			Expect(fields).To(HaveKeyWithValue("decision", "deny"))
			Expect(fields["reason"]).To(ContainSubstring("completion check failed"))
			Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
			Expect(fields).NotTo(HaveKey("continue"))
		})

		It("lets AfterAgent pass on warnings", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderGemini, "AfterAgent"), semanticsWarning(), nil,
			))

			Expect(fields).NotTo(HaveKey("decision"))
			Expect(fields).To(HaveKey("systemMessage"))
		})

		DescribeTable("never denies on events that ignore flow control",
			func(raw string) {
				fields := responseFields(hookresponse.BuildForContext(
					eventCtx(hook.ProviderGemini, raw), semanticsBlocking(), nil,
				))

				Expect(fields).To(HaveLen(1))
				Expect(fields).To(HaveKey("systemMessage"))
			},
			Entry("SessionEnd", "SessionEnd"),
			Entry("Notification", "Notification"),
			Entry("PreCompress", "PreCompress"),
			Entry("unmapped BeforeModel", "BeforeModel"),
		)
	})

	Describe("Codex", func() {
		It("continues the subagent on SubagentStop", func() {
			fields := responseFields(hookresponse.BuildForContext(
				eventCtx(hook.ProviderCodex, "SubagentStop"), semanticsBlocking(), nil,
			))

			Expect(fields).To(HaveKeyWithValue("decision", "block"))
			Expect(fields["reason"]).To(ContainSubstring("completion check failed"))
			Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
		})

		It("emits nothing on SessionEnd", func() {
			Expect(hookresponse.IsEmpty(hookresponse.BuildForContext(
				eventCtx(hook.ProviderCodex, "SessionEnd"), semanticsBlocking(), nil,
			))).To(BeTrue())
		})
	})
})
