package hookresponse_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("Notice", func() {
	msg := "Update available: klaudiush 1.0.0 -> 2.0.0. Run 'klaudiush update' to install."

	Describe("BuildNotice", func() {
		It("builds HookResponse for Claude provider", func() {
			ctx := &hook.Context{Provider: hook.ProviderClaude}
			resp := hookresponse.BuildNotice(ctx, msg)

			hr, ok := resp.(*hookresponse.HookResponse)
			Expect(ok).To(BeTrue())
			Expect(hr.SystemMessage).To(Equal(msg))
			Expect(hr.HookSpecificOutput).To(BeNil())
		})

		It("builds CodexCommandResponse for Codex provider", func() {
			ctx := &hook.Context{Provider: hook.ProviderCodex}
			resp := hookresponse.BuildNotice(ctx, msg)

			cr, ok := resp.(*hookresponse.CodexCommandResponse)
			Expect(ok).To(BeTrue())
			Expect(cr.SystemMessage).To(Equal(msg))
			Expect(cr.Continue).To(BeNil())

			data, err := json.Marshal(cr)
			Expect(err).NotTo(HaveOccurred())

			var fields map[string]any
			Expect(json.Unmarshal(data, &fields)).To(Succeed())
			Expect(fields).To(HaveLen(1))
			Expect(fields).To(HaveKeyWithValue("systemMessage", msg))
		})

		It("builds GeminiCommandResponse for Gemini provider", func() {
			ctx := &hook.Context{Provider: hook.ProviderGemini}
			resp := hookresponse.BuildNotice(ctx, msg)

			gr, ok := resp.(*hookresponse.GeminiCommandResponse)
			Expect(ok).To(BeTrue())
			Expect(gr.SystemMessage).To(Equal(msg))
		})

		It("builds HookResponse for nil context", func() {
			resp := hookresponse.BuildNotice(nil, msg)

			hr, ok := resp.(*hookresponse.HookResponse)
			Expect(ok).To(BeTrue())
			Expect(hr.SystemMessage).To(Equal(msg))
		})
	})

	Describe("AppendNotice", func() {
		It("appends to HookResponse", func() {
			resp := &hookresponse.HookResponse{SystemMessage: "existing error"}
			hookresponse.AppendNotice(resp, msg)
			Expect(resp.SystemMessage).To(Equal("existing error\n\n" + msg))
		})

		It("appends to CodexCommandResponse", func() {
			resp := &hookresponse.CodexCommandResponse{SystemMessage: "existing"}
			hookresponse.AppendNotice(resp, msg)
			Expect(resp.SystemMessage).To(Equal("existing\n\n" + msg))
		})

		It("appends to GeminiCommandResponse", func() {
			resp := &hookresponse.GeminiCommandResponse{SystemMessage: "existing"}
			hookresponse.AppendNotice(resp, msg)
			Expect(resp.SystemMessage).To(Equal("existing\n\n" + msg))
		})

		It("appends to PermissionRequestResponse", func() {
			resp := &hookresponse.PermissionRequestResponse{SystemMessage: "existing"}
			hookresponse.AppendNotice(resp, msg)
			Expect(resp.SystemMessage).To(Equal("existing\n\n" + msg))
		})

		It("leaves ElicitationHookResponse untouched, Claude drops systemMessage there", func() {
			resp := &hookresponse.ElicitationHookResponse{
				HookSpecificOutput: &hookresponse.ElicitationOutput{Action: "decline"},
			}
			hookresponse.AppendNotice(resp, msg)

			data, err := json.Marshal(resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("systemMessage"))
		})

		It("skips the separator when there is no existing message", func() {
			resp := &hookresponse.HookResponse{}
			hookresponse.AppendNotice(resp, msg)
			Expect(resp.SystemMessage).To(Equal(msg))
		})
	})

	Describe("ClearSystemMessage", func() {
		It("clears HookResponse and keeps the decision", func() {
			resp := &hookresponse.HookResponse{
				SystemMessage: "details",
				HookSpecificOutput: &hookresponse.HookSpecificOutput{
					PermissionDecision: "deny",
					AdditionalContext:  "context",
				},
			}
			hookresponse.ClearSystemMessage(resp)
			Expect(resp.SystemMessage).To(BeEmpty())
			Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
			Expect(resp.HookSpecificOutput.AdditionalContext).To(Equal("context"))
		})

		It("clears every provider response type", func() {
			codex := &hookresponse.CodexCommandResponse{SystemMessage: "x", Decision: "block"}
			gemini := &hookresponse.GeminiCommandResponse{SystemMessage: "x", Decision: "deny"}
			opencode := &hookresponse.OpenCodeCommandResponse{SystemMessage: "x"}
			permission := &hookresponse.PermissionRequestResponse{SystemMessage: "x"}

			for _, resp := range []any{codex, gemini, opencode, permission} {
				hookresponse.ClearSystemMessage(resp)
			}

			Expect(codex.SystemMessage).To(BeEmpty())
			Expect(codex.Decision).To(Equal("block"))
			Expect(gemini.SystemMessage).To(BeEmpty())
			Expect(gemini.Decision).To(Equal("deny"))
			Expect(opencode.SystemMessage).To(BeEmpty())
			Expect(permission.SystemMessage).To(BeEmpty())
		})

		It("ignores a nil response", func() {
			Expect(func() { hookresponse.ClearSystemMessage(nil) }).NotTo(Panic())
		})
	})

	Describe("AppendAgentSummary", func() {
		instruction := "plain-language sentence what klaudiush blocked"

		It("appends to a deny", func() {
			resp := &hookresponse.HookResponse{
				HookSpecificOutput: &hookresponse.HookSpecificOutput{
					PermissionDecision: "deny",
					AdditionalContext:  "Fix ALL errors.",
				},
			}
			hookresponse.AppendAgentSummary(resp)
			ctx := resp.HookSpecificOutput.AdditionalContext
			Expect(ctx).To(HavePrefix("Fix ALL errors. In your next message"))
			Expect(ctx).To(ContainSubstring(instruction))
		})

		It("skips warnings without a permission decision", func() {
			resp := &hookresponse.HookResponse{
				HookSpecificOutput: &hookresponse.HookSpecificOutput{
					AdditionalContext: "warning",
				},
			}
			hookresponse.AppendAgentSummary(resp)
			Expect(resp.HookSpecificOutput.AdditionalContext).To(Equal("warning"))
		})

		It("skips a PostToolUse block, where the tool already ran", func() {
			resp := &hookresponse.HookResponse{
				Decision:           "block",
				HookSpecificOutput: &hookresponse.HookSpecificOutput{AdditionalContext: "ctx"},
			}
			hookresponse.AppendAgentSummary(resp)
			Expect(resp.HookSpecificOutput.AdditionalContext).To(Equal("ctx"))
		})

		It("skips advisory provider responses", func() {
			resp := &hookresponse.GeminiCommandResponse{
				HookSpecificOutput: &hookresponse.GeminiHookSpecificOutput{
					AdditionalContext: "ctx",
				},
			}
			hookresponse.AppendAgentSummary(resp)
			Expect(resp.HookSpecificOutput.AdditionalContext).To(Equal("ctx"))
		})

		It("ignores a nil response", func() {
			Expect(func() { hookresponse.AppendAgentSummary(nil) }).NotTo(Panic())
		})
	})

	Describe("IsEmpty", func() {
		It("reports an untyped nil as empty", func() {
			Expect(hookresponse.IsEmpty(nil)).To(BeTrue())
		})

		It("reports typed nil pointers as empty", func() {
			var (
				claude      *hookresponse.HookResponse
				codex       *hookresponse.CodexCommandResponse
				gemini      *hookresponse.GeminiCommandResponse
				elicitation *hookresponse.ElicitationHookResponse
				permission  *hookresponse.PermissionRequestResponse
			)

			Expect(hookresponse.IsEmpty(claude)).To(BeTrue())
			Expect(hookresponse.IsEmpty(codex)).To(BeTrue())
			Expect(hookresponse.IsEmpty(gemini)).To(BeTrue())
			Expect(hookresponse.IsEmpty(elicitation)).To(BeTrue())
			Expect(hookresponse.IsEmpty(permission)).To(BeTrue())
		})

		It("reports built responses as non-empty", func() {
			Expect(hookresponse.IsEmpty(&hookresponse.HookResponse{})).To(BeFalse())
		})
	})
})
