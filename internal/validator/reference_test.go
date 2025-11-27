package validator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validator"
)

var _ = Describe("Reference", func() {
	Describe("String", func() {
		It("returns the URL string", func() {
			Expect(validator.RefGitNoSignoff.String()).To(Equal("https://klaudiu.sh/GIT001"))
			Expect(validator.RefShellcheck.String()).To(Equal("https://klaudiu.sh/FILE001"))
			Expect(validator.RefSecretsAPIKey.String()).To(Equal("https://klaudiu.sh/SEC001"))
		})
	})

	Describe("Code", func() {
		It("extracts the error code from the URL", func() {
			Expect(validator.RefGitNoSignoff.Code()).To(Equal("GIT001"))
			Expect(validator.RefGitMissingFlags.Code()).To(Equal("GIT010"))
			Expect(validator.RefShellcheck.Code()).To(Equal("FILE001"))
			Expect(validator.RefSecretsAPIKey.Code()).To(Equal("SEC001"))
		})

		It("returns the whole string if no slash found", func() {
			ref := validator.Reference("GIT001")
			Expect(ref.Code()).To(Equal("GIT001"))
		})
	})

	Describe("Category", func() {
		It("returns GIT for git references", func() {
			Expect(validator.RefGitNoSignoff.Category()).To(Equal("GIT"))
			Expect(validator.RefGitNoGPGSign.Category()).To(Equal("GIT"))
			Expect(validator.RefGitMissingFlags.Category()).To(Equal("GIT"))
		})

		It("returns FILE for file references", func() {
			Expect(validator.RefShellcheck.Category()).To(Equal("FILE"))
			Expect(validator.RefTerraformFmt.Category()).To(Equal("FILE"))
			Expect(validator.RefActionlint.Category()).To(Equal("FILE"))
		})

		It("returns SEC for security references", func() {
			Expect(validator.RefSecretsAPIKey.Category()).To(Equal("SEC"))
			Expect(validator.RefSecretsPassword.Category()).To(Equal("SEC"))
			Expect(validator.RefSecretsToken.Category()).To(Equal("SEC"))
		})

		It("returns empty string for invalid references", func() {
			Expect(validator.Reference("").Category()).To(Equal(""))
			Expect(validator.Reference("AB").Category()).To(Equal(""))
		})
	})
})

var _ = Describe("Suggestions", func() {
	Describe("GetSuggestion", func() {
		It("returns suggestion for known references", func() {
			suggestion := validator.GetSuggestion(validator.RefGitMissingFlags)
			Expect(suggestion).To(ContainSubstring("-sS"))
		})

		It("returns empty string for unknown references", func() {
			unknownRef := validator.Reference("https://klaudiu.sh/UNKNOWN999")
			suggestion := validator.GetSuggestion(unknownRef)
			Expect(suggestion).To(BeEmpty())
		})
	})
})

var _ = Describe("FailWithRef", func() {
	It("creates a failing result with reference", func() {
		result := validator.FailWithRef(validator.RefGitMissingFlags, "test message")

		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Message).To(Equal("test message"))
		Expect(result.Reference).To(Equal(validator.RefGitMissingFlags))
	})

	It("populates FixHint from suggestions registry", func() {
		result := validator.FailWithRef(validator.RefGitMissingFlags, "test")

		Expect(result.FixHint).To(ContainSubstring("-sS"))
	})
})

var _ = Describe("WarnWithRef", func() {
	It("creates a warning result with reference", func() {
		result := validator.WarnWithRef(validator.RefShellcheck, "test warning")

		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeFalse())
		Expect(result.Message).To(Equal("test warning"))
		Expect(result.Reference).To(Equal(validator.RefShellcheck))
	})

	It("populates FixHint from suggestions registry", func() {
		result := validator.WarnWithRef(validator.RefShellcheck, "test")

		Expect(result.FixHint).To(ContainSubstring("shellcheck"))
	})
})
