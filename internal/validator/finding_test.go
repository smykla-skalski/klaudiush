package validator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
)

var _ = Describe("Finding", func() {
	It("reports its code", func() {
		Expect(validator.Finding{Reference: validator.RefGitBadTitle}.Code()).To(Equal("GIT004"))
		Expect(validator.Finding{}.Code()).To(BeEmpty())
	})

	It("adds findings and marks unavailable results", func() {
		result := validator.Fail("plugin failed").
			AddFinding(validator.Finding{Message: "a"}, validator.Finding{Message: "b"}).
			MarkUnavailable()

		Expect(result.Findings).To(HaveLen(2))
		Expect(result.Unavailable).To(BeTrue())
	})

	Describe("SortFindings", func() {
		It("returns nil for no findings", func() {
			Expect(validator.SortFindings(nil, nil)).To(BeNil())
		})

		It("orders by priority, code, natural location and message, dropping duplicates", func() {
			body10 := validator.Finding{
				Reference: validator.RefGitBadBody, Location: "message line 10", Message: "long",
			}
			body9 := validator.Finding{
				Reference: validator.RefGitBadBody, Location: "message line 9", Message: "long",
			}
			body9b := validator.Finding{
				Reference: validator.RefGitBadBody, Location: "message line 09", Message: "z",
			}
			title := validator.Finding{Reference: validator.RefGitBadTitle, Location: "title"}
			prRef := validator.Finding{Reference: validator.RefGitPRRef, Location: "message"}
			plain := validator.Finding{Location: "x"}

			sorted := validator.SortFindings(
				[]validator.Finding{plain, prRef, body10, body9b, title, body9, body10},
				[]validator.Reference{validator.RefGitBadTitle, validator.RefGitBadBody},
			)

			Expect(sorted).To(Equal([]validator.Finding{
				title, body9, body9b, body10, plain, prRef,
			}))
		})

		It("compares locations of different lengths", func() {
			a := validator.Finding{Location: "line"}
			b := validator.Finding{Location: "line 2"}
			c := validator.Finding{Location: "lime 2"}

			Expect(validator.SortFindings([]validator.Finding{b, a, c}, nil)).
				To(Equal([]validator.Finding{c, a, b}))
		})
	})
})

var _ = Describe("GetSuggestionWithLimits", func() {
	It("quotes the configured limits", func() {
		limits := validator.MessageLimits{TitleMaxLength: 72, BodyMaxLineLength: 100}

		Expect(validator.GetSuggestionWithLimits(validator.RefGitBadTitle, limits)).
			To(ContainSubstring("max 72 chars"))
		Expect(validator.GetSuggestionWithLimits(validator.RefGitBadBody, limits)).
			To(Equal("Wrap body lines at 100 characters"))
		Expect(validator.GetSuggestionWithLimits(validator.RefGitConventionalCommit, limits)).
			To(ContainSubstring("at most 72 chars"))
	})

	It("falls back to the defaults for unset limits", func() {
		Expect(
			validator.GetSuggestionWithLimits(validator.RefGitBadTitle, validator.MessageLimits{}),
		).
			To(Equal(validator.GetSuggestion(validator.RefGitBadTitle)))
		Expect(
			validator.GetSuggestionWithLimits(validator.RefGitBadBody, validator.MessageLimits{}),
		).
			To(Equal("Wrap body lines at 72 characters"))
	})

	It("returns the registry hint for other references", func() {
		Expect(
			validator.GetSuggestionWithLimits(validator.RefGitNoSignoff, validator.MessageLimits{}),
		).
			To(Equal(validator.GetSuggestion(validator.RefGitNoSignoff)))
	})
})
