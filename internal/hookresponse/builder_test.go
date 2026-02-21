package hookresponse_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
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

	It("returns deny for session poisoned (SESS001)", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "session-poisoned",
				Message:     "Blocked: session poisoned by GIT001 at 2026-01-01 12:00:00",
				ShouldBlock: true,
				Reference:   validator.RefSessionPoisoned,
				FixHint:     "Acknowledge violations to unpoison: KLACK=\"SESS:GIT001\" your_command",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecision).To(Equal("deny"))
		Expect(resp.HookSpecificOutput.AdditionalContext).To(ContainSubstring("session check"))
		Expect(
			resp.HookSpecificOutput.AdditionalContext,
		).To(ContainSubstring("Acknowledge the error codes"))
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
})
