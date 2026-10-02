package hookresponse_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("after-tool responses", func() {
	gofumptFinding := func(block bool) []*dispatcher.ValidationError {
		return []*dispatcher.ValidationError{{
			Validator:   "validate-gofumpt",
			Message:     "Go code formatting issues detected",
			ShouldBlock: block,
			Reference:   validator.RefGofumpt,
		}}
	}

	claudeAfter := func(raw string, succeeded bool) *hook.Context {
		return &hook.Context{
			Provider:      hook.ProviderClaude,
			Event:         hook.CanonicalEventAfterTool,
			RawEventName:  raw,
			ToolExecuted:  true,
			ToolSucceeded: succeeded,
		}
	}

	It("asks for a repair of partial changes after a Claude tool failed", func() {
		resp := hookresponse.BuildForContext(
			claudeAfter("PostToolUseFailure", false),
			gofumptFinding(true),
			nil,
		)

		claudeResp, ok := resp.(*hookresponse.HookResponse)
		Expect(ok).To(BeTrue())
		Expect(claudeResp.Decision).To(Equal("block"))
		Expect(claudeResp.Reason).To(HavePrefix("Repair required, the failed tool may have left"))
		Expect(claudeResp.HookSpecificOutput.HookEventName).To(Equal("PostToolUseFailure"))
		Expect(claudeResp.HookSpecificOutput.AdditionalContext).
			To(ContainSubstring("may have left partial changes"))
		Expect(claudeResp.HookSpecificOutput.PermissionDecision).To(BeEmpty())
		Expect(claudeResp.SystemMessage).To(HavePrefix("klaudiush checked the files a failed tool"))
	})

	It("frames warnings after the tool as repairs of applied changes", func() {
		resp := hookresponse.BuildForContext(
			claudeAfter("PostToolUse", true),
			gofumptFinding(false),
			nil,
		)

		claudeResp, ok := resp.(*hookresponse.HookResponse)
		Expect(ok).To(BeTrue())
		Expect(claudeResp.Decision).To(BeEmpty())
		Expect(claudeResp.SystemMessage).NotTo(ContainSubstring("need repair"))
		Expect(claudeResp.HookSpecificOutput.AdditionalContext).
			To(HavePrefix("klaudiush checked the files after the tool ran."))
		Expect(claudeResp.HookSpecificOutput.AdditionalContext).NotTo(ContainSubstring("retry"))
	})

	It("frames warnings after a failed tool as possible partial changes", func() {
		resp := hookresponse.BuildForContext(
			claudeAfter("PostToolUseFailure", false),
			gofumptFinding(false),
			nil,
		)

		claudeResp, ok := resp.(*hookresponse.HookResponse)
		Expect(ok).To(BeTrue())
		Expect(claudeResp.HookSpecificOutput.AdditionalContext).
			To(HavePrefix("klaudiush checked the files the failed tool may have partly changed."))
	})

	It("asks Codex for a repair instead of a retry after the tool ran", func() {
		resp := hookresponse.BuildForContext(&hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "PostToolUse",
		}, gofumptFinding(true), nil)

		codexResp, ok := resp.(*hookresponse.CodexCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(
			codexResp.HookSpecificOutput.AdditionalContext,
		).To(ContainSubstring("Repair required"))
		Expect(codexResp.HookSpecificOutput.AdditionalContext).NotTo(ContainSubstring("retry"))
		Expect(codexResp.SystemMessage).To(HavePrefix("klaudiush checked the result after"))
	})

	It("keeps the stop-and-retry framing before the tool runs", func() {
		resp := hookresponse.BuildForContext(&hook.Context{
			Provider:     hook.ProviderClaude,
			Event:        hook.CanonicalEventBeforeTool,
			RawEventName: "PreToolUse",
		}, gofumptFinding(true), nil)

		claudeResp, ok := resp.(*hookresponse.HookResponse)
		Expect(ok).To(BeTrue())
		Expect(claudeResp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("retry"))
		Expect(claudeResp.SystemMessage).NotTo(ContainSubstring("need repair"))
	})

	It("asks opencode for a repair after the tool ran", func() {
		resp := hookresponse.BuildForContext(&hook.Context{
			Provider:     hook.ProviderOpenCode,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "tool.execute.after",
		}, gofumptFinding(true), nil)

		openCodeResp, ok := resp.(*hookresponse.OpenCodeCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(openCodeResp.HookSpecificOutput.AdditionalContext).
			To(ContainSubstring("Repair required"))
	})
})
