package hookresponse_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
)

var _ = Describe("FormatSystemMessage", func() {
	It("returns empty for no errors", func() {
		Expect(hookresponse.FormatSystemMessage(nil)).To(BeEmpty())
		Expect(hookresponse.FormatSystemMessage([]*dispatcher.ValidationError{})).To(BeEmpty())
	})

	It("formats blocking errors with red emoji header", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing -s flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
				FixHint:     "Add -s flag: git commit -sS -m \"message\"",
			},
		}

		result := hookresponse.FormatSystemMessage(errs)
		Expect(result).To(ContainSubstring("\u274c"))
		Expect(result).To(ContainSubstring("Validation Failed:"))
		Expect(result).To(ContainSubstring("Missing -s flag"))
		Expect(result).To(ContainSubstring("Fix: Add -s flag"))
		Expect(result).To(ContainSubstring("Reference: https://klaudiu.sh/GIT001"))
	})

	It("formats warnings with warning emoji header", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "markdown",
				Message:     "line too long",
				ShouldBlock: false,
			},
		}

		result := hookresponse.FormatSystemMessage(errs)
		Expect(result).To(ContainSubstring("\u26a0\ufe0f"))
		Expect(result).To(ContainSubstring("Warnings:"))
		Expect(result).To(ContainSubstring("line too long"))
	})

	It("separates blocking errors and warnings", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "blocker",
				Message:     "Blocking error",
				ShouldBlock: true,
			},
			{
				Validator:   "warner",
				Message:     "Warning message",
				ShouldBlock: false,
			},
		}

		result := hookresponse.FormatSystemMessage(errs)
		Expect(result).To(ContainSubstring("Validation Failed:"))
		Expect(result).To(ContainSubstring("Blocking error"))
		Expect(result).To(ContainSubstring("Warnings:"))
		Expect(result).To(ContainSubstring("Warning message"))
	})

	It("includes error details", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test",
				Message:     "Validation error",
				ShouldBlock: true,
				Details: map[string]string{
					"detail1": "first detail\nsecond line",
				},
			},
		}

		result := hookresponse.FormatSystemMessage(errs)
		Expect(result).To(ContainSubstring("first detail"))
		Expect(result).To(ContainSubstring("second line"))
	})

	It("strips validate- prefix from validator names", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-git-commit",
				Message:     "error",
				ShouldBlock: true,
			},
		}

		result := hookresponse.FormatSystemMessage(errs)
		Expect(result).To(ContainSubstring("git-commit"))
		Expect(result).NotTo(ContainSubstring("validate-git-commit"))
	})
})

var _ = Describe("Decision reason formatting", func() {
	It("truncates long messages to 200 chars per error", func() {
		longMsg := strings.Repeat("x", 250)
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test",
				Message:     longMsg,
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(len(resp.HookSpecificOutput.PermissionDecisionReason)).To(BeNumerically("<=", 200))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(HaveSuffix("..."))
	})

	It("includes fix hint in decision reason", func() {
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
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(
			ContainSubstring("Add -s flag"))
	})

	It("handles error without reference code", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test",
				Message:     "some error",
				ShouldBlock: true,
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp).NotTo(BeNil())
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).To(Equal("some error"))
		Expect(resp.HookSpecificOutput.PermissionDecisionReason).NotTo(ContainSubstring("["))
	})
})
