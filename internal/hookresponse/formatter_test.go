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

	It("preserves empty lines in details", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test",
				Message:     "Commit message validation failed",
				ShouldBlock: true,
				Details: map[string]string{
					"errors": "Title exceeds 50 characters\n\n\U0001F4DD Commit message:\n---\nfix(hookresponse): deduplicate hook rejection output\n\npermissionDecisionReason and systemMessage both render\n---",
				},
			},
		}

		result := hookresponse.FormatSystemMessage(errs)
		Expect(result).To(ContainSubstring("output\n\npermissionDecisionReason"))
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
		msg := "\u274c Remote 'origin' does not exist\n\n" +
			"Available remotes:\n" +
			"  Automaat  git@github.com:Automaat/klaudiush.git\n" +
			"  upstream  git@github.com:smykla-skalski/klaudiush.git\n\n" +
			"Use 'git remote -v' to list all configured remotes."
		reason := buildReason(msg)
		Expect(reason).To(Equal(
			"[GIT024] Remote 'origin' does not exist"))
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

	It("skips paragraphs where stripEmoji produces empty string", func() {
		// Paragraph made entirely of emoji - stripEmoji returns ""
		msg := "Header line\n\n\U0001F6AB\n\nActual content"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Header line. Actual content"))
	})

	It("returns raw message when all lines are empty", func() {
		msg := "\n\n\n\n"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] \n\n\n\n"))
	})

	It("strips transport & map emoji", func() {
		msg := "\U0001F680 Rocket launch"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Rocket launch"))
	})

	It("strips misc symbols & pictographs", func() {
		msg := "\U0001F4E6 Package update"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Package update"))
	})

	It("strips supplemental symbols", func() {
		msg := "\U0001F9EA Test tube result"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Test tube result"))
	})

	It("strips misc symbols like warning sign", func() {
		msg := "\u26A0 Warning detected"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Warning detected"))
	})

	It("strips variation selector and zero-width joiner", func() {
		msg := "\u26A0\uFE0F\u200D Warning with selectors"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Warning with selectors"))
	})

	It("handles message with only empty paragraphs after filtering", func() {
		// All paragraphs are supplementary
		msg := "See docs for details\n\nNote: this is expected\n\nTip: try again"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] See docs for details"))
	})

	It("handles single paragraph with leading blank lines", func() {
		msg := "\n\nActual content here"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[GIT024] Actual content here"))
	})
})

var _ = Describe("Emoji stripping edge cases", func() {
	buildReason := func(msg string) string {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "test",
				Message:     msg,
				ShouldBlock: true,
				Reference:   "https://klaudiu.sh/TEST001",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)

		return resp.HookSpecificOutput.PermissionDecisionReason
	}

	It("strips emoticon range emoji", func() {
		// U+1F600 = grinning face (emoticons range 0x1F600-0x1F64F)
		msg := "\U0001F600 Smiling error"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[TEST001] Smiling error"))
	})

	It("strips non-printable control characters", func() {
		// \x01 = SOH, non-printable non-space
		msg := "Error\x01message"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[TEST001] Errormessage"))
	})

	It("preserves plain ASCII text unchanged", func() {
		msg := "just plain text"
		reason := buildReason(msg)
		Expect(reason).To(Equal("[TEST001] just plain text"))
	})
})

var _ = Describe("Additional context formatting", func() {
	It("uses default reason for bypassed error without bypass reason", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:    "git.push",
				Message:      "push blocked [BYPASSED]",
				ShouldBlock:  false,
				Reference:    validator.RefGitKongOrgPush,
				Bypassed:     true,
				BypassReason: "",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp.HookSpecificOutput.AdditionalContext).To(
			ContainSubstring("no reason provided"))
	})

	It("includes table suggestion in additionalContext for blocking errors", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-markdown",
				Message:     "Line 3: Inconsistent spacing in table row",
				ShouldBlock: true,
				Reference:   validator.RefMarkdownLint,
				Details: map[string]string{
					"errors":          "Line 3: Inconsistent spacing in table row",
					"suggested_table": "Line 3 - Use this properly formatted table:\n\n| A | B |\n|:--|:--|\n| x | y |\n",
				},
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp.HookSpecificOutput.AdditionalContext).To(
			ContainSubstring("properly formatted table"))
	})

	It("includes table suggestion in additionalContext for warning errors", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-markdown",
				Message:     "Line 1: Table column widths inconsistent",
				ShouldBlock: false,
				Details: map[string]string{
					"suggested_table": "Line 1 - Use this properly formatted table:\n\n| Name | Age |\n|:-----|:----|\n",
				},
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		Expect(resp.HookSpecificOutput.AdditionalContext).To(
			ContainSubstring("properly formatted table"))
	})

	It("includes cascading failure warning in non-session-poisoned context", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "git.commit",
				Message:     "Missing -s flag",
				ShouldBlock: true,
				Reference:   validator.RefGitNoSignoff,
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		ctx := resp.HookSpecificOutput.AdditionalContext
		Expect(ctx).To(ContainSubstring("type(scope): prefix makes title exceed 50 chars"))
	})

	It("truncates large table suggestions", func() {
		// Build a suggestion with more than 15 lines
		lines := make([]string, 0, 22)

		lines = append(lines, "Line 1 - Use this properly formatted table:")

		lines = append(lines, "")
		for i := range 20 {
			lines = append(lines, "| row"+strings.Repeat("x", i)+" |")
		}

		suggestion := strings.Join(lines, "\n")

		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-markdown",
				Message:     "Table issue",
				ShouldBlock: true,
				Reference:   validator.RefMarkdownLint,
				Details: map[string]string{
					"suggested_table": suggestion,
				},
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		ctx := resp.HookSpecificOutput.AdditionalContext
		// Should be truncated
		Expect(ctx).To(ContainSubstring("..."))
	})

	It("shows specific permissionDecisionReason for markdown errors", func() {
		errs := []*dispatcher.ValidationError{
			{
				Validator:   "validate-markdown",
				Message:     "Line 3: Table column widths inconsistent",
				ShouldBlock: true,
				Reference:   validator.RefMarkdownLint,
				FixHint:     "Fix the formatting issue and retry",
			},
		}

		resp := hookresponse.Build("PreToolUse", errs)
		reason := resp.HookSpecificOutput.PermissionDecisionReason
		// Should be specific, not generic
		Expect(reason).To(ContainSubstring("Table column widths"))
		Expect(reason).NotTo(Equal("Markdown formatting errors"))
	})
})
