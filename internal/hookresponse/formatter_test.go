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

var _ = Describe("Decision reason summarization", func() {
	buildReason := func(msg string) string {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test",
				Message:     msg,
				ShouldBlock: true,
				Reference:   "https://klaudiu.sh/GIT024",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)

		return resp.HookSpecificOutput.PermissionDecisionReason
	}

	It("passes short single-line messages through unchanged", func() {
		reason := buildReason("Missing -s flag")
		Expect(reason).To(Equal("[GIT024] Missing -s flag"))
	})

	It("summarizes rich remote-not-found message", func() {
		msg := "\U0001F6AB Git fetch validation failed:\n\n" +
			"\u274c Remote 'origin' does not exist\n\n" +
			"Available remotes:\n" +
			"  Automaat  git@github.com:Automaat/klaudiush.git\n" +
			"  upstream  git@github.com:smykla-skalski/klaudiush.git\n\n" +
			"Use 'git remote -v' to list all configured remotes."
		reason := buildReason(msg)
		Expect(reason).To(Equal(
			"[GIT024] Git fetch validation failed: Remote 'origin' does not exist"))
	})

	It("strips supplementary 'Use' and 'Example' paragraphs", func() {
		msg := "Branch validation failed:\n\n" +
			"Push to 'production' is restricted\n\n" +
			"Use 'git push origin feature-branch' instead\n\n" +
			"Example:\n  git checkout -b fix/my-fix"
		reason := buildReason(msg)
		Expect(reason).To(Equal(
			"[GIT024] Branch validation failed: Push to 'production' is restricted"))
	})

	It("handles secrets detection concisely", func() {
		msg := "Potential secrets detected (2 finding(s)):"
		reason := buildReason(msg)
		Expect(reason).To(Equal(
			"[GIT024] Potential secrets detected (2 finding(s)):"))
	})

	It("strips emoji characters from messages", func() {
		msg := "\u274c Commit message too long"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Commit message too long"))
	})

	It("falls back to first line when all content is supplementary", func() {
		msg := "Available remotes:\n  origin  git@github.com:user/repo.git"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Available remotes:"))
	})

	It("joins with space when first part ends with colon", func() {
		msg := "Validation failed:\n\nMissing required flag"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Validation failed: Missing required flag"))
	})

	It("joins with period when first part does not end with colon", func() {
		msg := "Commit blocked\n\nNo staged files found"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Commit blocked. No staged files found"))
	})
})
