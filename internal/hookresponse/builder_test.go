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

var _ = Describe("Build", func() {
	It("returns nil for no errors", func() {
		Expect(hookresponse.Build("PreToolUse", nil)).To(BeNil())
		Expect(hookresponse.Build("PreToolUse", []*dispatcher.ValidationError{})).To(BeNil())
	})

	It("returns deny for a single blocking error", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing -s flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
				FixHint:     "Add -s flag: git commit -sS -m \"message\"",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
		Expect(resp.HookSpecificOutput.HookEventName).To(Equal("PreToolUse"))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(ContainSubstring("[GIT001]"))
		Expect(
			resp.HookSpecificOutput.PermissionDecisionReason,
		).To(ContainSubstring("Missing -s flag"))
		Expect(
			resp.HookSpecificOutput.AdditionalContext,
		).To(ContainSubstring("Fix ALL reported errors at once"))

		// systemMessage uses self-contained format: emoji CODE: message
		Expect(resp.SystemMessage).To(ContainSubstring("GIT001: Missing -s flag"))
		Expect(resp.SystemMessage).NotTo(ContainSubstring("Validation Failed:"))
		// Ref: instead of Reference:
		Expect(resp.SystemMessage).To(ContainSubstring("Ref:"))
		Expect(resp.SystemMessage).NotTo(ContainSubstring("Reference:"))
		// Disable hint for blocking errors
		Expect(
			resp.SystemMessage,
		).To(ContainSubstring("Wrong for your workflow? klaudiush disable GIT001"))
	})

	It("returns deny for multiple blocking errors", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing -s flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
				FixHint:     "Add -s flag",
			},
			{
				Validator:   "git.commit",
				Message:     "Missing -S flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoGPGSign,
				FixHint:     "Add -S flag",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(ContainSubstring("[GIT001]"))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(ContainSubstring("[GIT002]"))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(ContainSubstring(" | "))
	})

	It("returns allow for warnings only", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "markdown",
				Message:     "line too long",
				ShouldBlock: false,
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("allow"))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(BeEmpty())
		Expect(resp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("warning"))
		Expect(resp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("Not blocking"))
	})

	It("returns allow for bypassed exception", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:    "git.push",
				Message:      "cannot push to protected branch [BYPASSED: Emergency hotfix]",
				ShouldBlock:  false,
				Reference:    validator.RefGitKongOrgPush,
				Bypassed:     true,
				BypassReason: "Emergency hotfix",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("allow"))
		Expect(
			resp.HookSpecificOutput.AdditionalContext,
		).To(ContainSubstring("Exception EXC:GIT022"))
		Expect(resp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("Emergency hotfix"))
	})

	It("handles mixed blocking and warnings", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing -s flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
				FixHint:     "Add -s flag",
			},
			{
				Validator:   "markdown",
				Message:     "line too long",
				ShouldBlock: false,
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
		// Both messages present without category headers
		Expect(resp.SystemMessage).To(ContainSubstring("GIT001: Missing -s flag"))
		Expect(resp.SystemMessage).To(ContainSubstring("line too long"))
		Expect(resp.SystemMessage).NotTo(ContainSubstring("Validation Failed:"))
		Expect(resp.SystemMessage).NotTo(ContainSubstring("Warnings:"))
		// Disable hint for the blocking code
		Expect(
			resp.SystemMessage,
		).To(ContainSubstring("Wrong for your workflow? klaudiush disable GIT001"))
	})

	It("appends pattern warnings to deny-path additional context", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing conventional format",
				ShouldBlock: true,
				Reference:   validator.RefGitConventionalCommit,
			},
		}

		resp := hookresponse.BuildWithPatterns("PreToolUse", errs, []string{
			"Pattern hint: after fixing GIT013 (conventional format), GIT004 (title too long) often follows.",
		})

		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
		Expect(resp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("Pattern hint:"))
		Expect(resp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("GIT004"))
	})

	It("does not append pattern warnings for warnings-only responses", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "markdown",
				Message:     "line too long",
				ShouldBlock: false,
			},
		}

		resp := hookresponse.BuildWithPatterns("PreToolUse", errs, []string{
			"Pattern hint: after fixing GIT013 (conventional format), GIT004 (title too long) often follows.",
		})

		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("allow"))
		Expect(resp.HookSpecificOutput.AdditionalContext).NotTo(ContainSubstring("Pattern hint:"))
	})

	It("produces valid JSON", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing -s flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
				FixHint:     "Add -s flag",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		data, err := json.Marshal(resp)
		Expect(err).NotTo(HaveOccurred())
		Expect(data).NotTo(BeEmpty())

		// Verify it can be unmarshaled back
		var decoded hookresponse.HookResponse
		Expect(json.Unmarshal(data, &decoded)).To(Succeed())
		Expect(decoded.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
	})

	It("builds SessionStart Codex advisory responses", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "patterns",
				Message:     "session guidance",
				ShouldBlock: false,
			},
		}

		resp := hookresponse.BuildForContext(&hook.Context{
			Provider: hook.ProviderCodex,
			Event:    hook.CanonicalEventSessionStart,
		}, errs, nil)

		codexResp, ok := resp.(*hookresponse.CodexCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(codexResp.Continue).To(BeTrue())
		Expect(codexResp.StopReason).To(BeEmpty())
		Expect(codexResp.HookSpecificOutput).NotTo(BeNil())
		Expect(codexResp.HookSpecificOutput.HookEventName).To(Equal("SessionStart"))
		Expect(codexResp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("warning"))
	})

	It("builds SessionStart Codex blocking responses", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "config",
				Message:     "invalid setup",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
			},
		}

		resp := hookresponse.BuildForContext(&hook.Context{
			Provider: hook.ProviderCodex,
			Event:    hook.CanonicalEventSessionStart,
		}, errs, nil)

		codexResp, ok := resp.(*hookresponse.CodexCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(codexResp.Continue).To(BeFalse())
		Expect(codexResp.StopReason).To(ContainSubstring("[GIT001]"))
		Expect(codexResp.HookSpecificOutput).To(BeNil())
	})

	It("builds Stop Codex blocking responses", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "summary",
				Message:     "must stop",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
			},
		}

		resp := hookresponse.BuildForContext(&hook.Context{
			Provider: hook.ProviderCodex,
			Event:    hook.CanonicalEventTurnStop,
		}, errs, nil)

		codexResp, ok := resp.(*hookresponse.CodexCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(codexResp.Decision).To(Equal("block"))
		Expect(codexResp.Reason).To(ContainSubstring("[GIT001]"))
		Expect(codexResp.Continue).To(BeTrue())
	})

	It("keeps AfterToolUse Codex responses advisory even for blocking findings", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.push",
				Message:     "protected branch",
				ShouldBlock: true,
				Reference:   validator.RefGitKongOrgPush,
			},
		}

		resp := hookresponse.BuildForContext(&hook.Context{
			Provider:     hook.ProviderCodex,
			Event:        hook.CanonicalEventAfterTool,
			RawEventName: "AfterToolUse",
		}, errs, nil)

		codexResp, ok := resp.(*hookresponse.CodexCommandResponse)
		Expect(ok).To(BeTrue())
		Expect(codexResp.Continue).To(BeTrue())
		Expect(codexResp.StopReason).To(BeEmpty())
		Expect(codexResp.HookSpecificOutput).NotTo(BeNil())
		Expect(codexResp.HookSpecificOutput.HookEventName).To(Equal("AfterToolUse"))
		Expect(codexResp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("Fix ALL"))
	})
})
