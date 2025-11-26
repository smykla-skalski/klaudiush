package validator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validator"
)

var _ = Describe("ErrorCode", func() {
	Describe("String", func() {
		It("returns the string representation", func() {
			Expect(validator.ErrGitNoSignoff.String()).To(Equal("GIT001"))
			Expect(validator.ErrShellcheck.String()).To(Equal("FILE001"))
			Expect(validator.ErrSecretsAPIKey.String()).To(Equal("SEC001"))
		})
	})

	Describe("Category", func() {
		It("returns GIT for git error codes", func() {
			Expect(validator.ErrGitNoSignoff.Category()).To(Equal("GIT"))
			Expect(validator.ErrGitNoGPGSign.Category()).To(Equal("GIT"))
			Expect(validator.ErrGitMissingFlags.Category()).To(Equal("GIT"))
		})

		It("returns FILE for file error codes", func() {
			Expect(validator.ErrShellcheck.Category()).To(Equal("FILE"))
			Expect(validator.ErrTerraformFmt.Category()).To(Equal("FILE"))
			Expect(validator.ErrActionlint.Category()).To(Equal("FILE"))
		})

		It("returns SEC for security error codes", func() {
			Expect(validator.ErrSecretsAPIKey.Category()).To(Equal("SEC"))
			Expect(validator.ErrSecretsPassword.Category()).To(Equal("SEC"))
			Expect(validator.ErrSecretsToken.Category()).To(Equal("SEC"))
		})

		It("returns empty string for invalid codes", func() {
			Expect(validator.ErrorCode("").Category()).To(Equal(""))
			Expect(validator.ErrorCode("AB").Category()).To(Equal(""))
		})
	})
})

var _ = Describe("Suggestions", func() {
	Describe("GetSuggestion", func() {
		It("returns suggestion for known error codes", func() {
			suggestion := validator.GetSuggestion(validator.ErrGitMissingFlags)
			Expect(suggestion).To(ContainSubstring("-sS"))
		})

		It("returns empty string for unknown error codes", func() {
			suggestion := validator.GetSuggestion(validator.ErrorCode("UNKNOWN999"))
			Expect(suggestion).To(BeEmpty())
		})
	})
})

var _ = Describe("DocLinks", func() {
	Describe("GetDocLink", func() {
		It("returns doc link for known error codes", func() {
			link := validator.GetDocLink(validator.ErrGitMissingFlags)
			Expect(link).To(ContainSubstring("GIT010"))
		})

		It("returns empty string for unknown error codes", func() {
			link := validator.GetDocLink(validator.ErrorCode("UNKNOWN999"))
			Expect(link).To(BeEmpty())
		})
	})
})

var _ = Describe("FailWithCode", func() {
	It("creates a failing result with error code", func() {
		result := validator.FailWithCode(validator.ErrGitMissingFlags, "test message")

		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Message).To(Equal("test message"))
		Expect(result.ErrorCode).To(Equal(validator.ErrGitMissingFlags))
	})

	It("populates FixHint from suggestions registry", func() {
		result := validator.FailWithCode(validator.ErrGitMissingFlags, "test")

		Expect(result.FixHint).To(ContainSubstring("-sS"))
	})

	It("populates DocLink from doc links registry", func() {
		result := validator.FailWithCode(validator.ErrGitMissingFlags, "test")

		Expect(result.DocLink).To(ContainSubstring("GIT010"))
	})
})

var _ = Describe("WarnWithCode", func() {
	It("creates a warning result with error code", func() {
		result := validator.WarnWithCode(validator.ErrShellcheck, "test warning")

		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeFalse())
		Expect(result.Message).To(Equal("test warning"))
		Expect(result.ErrorCode).To(Equal(validator.ErrShellcheck))
	})

	It("populates FixHint from suggestions registry", func() {
		result := validator.WarnWithCode(validator.ErrShellcheck, "test")

		Expect(result.FixHint).To(ContainSubstring("shellcheck"))
	})

	It("populates DocLink from doc links registry", func() {
		result := validator.WarnWithCode(validator.ErrShellcheck, "test")

		Expect(result.DocLink).To(ContainSubstring("FILE001"))
	})
})
