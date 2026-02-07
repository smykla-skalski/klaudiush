package tools_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/doctor/checkers/tools"
)

func TestTools(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tools Checker Suite")
}

var _ = Describe("ToolChecker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewShellcheckChecker", func() {
		var checker *tools.ToolChecker

		BeforeEach(func() {
			checker = tools.NewShellcheckChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("shellcheck available"))
		})

		It("should have tools category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryTools))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("shellcheck available"))
			Expect(result.Status).To(BeElementOf(doctor.StatusPass, doctor.StatusFail))
		})

		Context("when shellcheck is not available", func() {
			It("should return warning with install hint", func() {
				result := checker.Check(ctx)
				if result.Status == doctor.StatusFail {
					Expect(result.Severity).To(Equal(doctor.SeverityWarning))
					Expect(result.Details).NotTo(BeEmpty())
				}
			})
		})
	})

	Describe("NewTerraformChecker", func() {
		var checker *tools.ToolChecker

		BeforeEach(func() {
			checker = tools.NewTerraformChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("terraform available"))
		})

		It("should have tools category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryTools))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("terraform available"))
			Expect(result.Status).To(BeElementOf(doctor.StatusPass, doctor.StatusFail))
		})

		Context("when neither tofu nor terraform is available", func() {
			It("should return warning", func() {
				result := checker.Check(ctx)
				if result.Status == doctor.StatusFail {
					Expect(result.Severity).To(Equal(doctor.SeverityWarning))
					Expect(result.Message).To(ContainSubstring("not found"))
				}
			})
		})
	})

	Describe("NewTflintChecker", func() {
		var checker *tools.ToolChecker

		BeforeEach(func() {
			checker = tools.NewTflintChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("tflint available"))
		})

		It("should have tools category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryTools))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("tflint available"))
			Expect(result.Status).To(BeElementOf(doctor.StatusPass, doctor.StatusFail))
		})

		Context("when tflint is not available", func() {
			It("should return info severity", func() {
				result := checker.Check(ctx)
				if result.Status == doctor.StatusFail {
					Expect(result.Severity).To(Equal(doctor.SeverityInfo))
				}
			})
		})
	})

	Describe("NewActionlintChecker", func() {
		var checker *tools.ToolChecker

		BeforeEach(func() {
			checker = tools.NewActionlintChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("actionlint available"))
		})

		It("should have tools category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryTools))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("actionlint available"))
			Expect(result.Status).To(BeElementOf(doctor.StatusPass, doctor.StatusFail))
		})

		Context("when actionlint is not available", func() {
			It("should return info severity", func() {
				result := checker.Check(ctx)
				if result.Status == doctor.StatusFail {
					Expect(result.Severity).To(Equal(doctor.SeverityInfo))
				}
			})
		})
	})

	Describe("NewMarkdownlintChecker", func() {
		var checker *tools.ToolChecker

		BeforeEach(func() {
			checker = tools.NewMarkdownlintChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("markdownlint available"))
		})

		It("should have tools category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryTools))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("markdownlint available"))
			Expect(result.Status).To(BeElementOf(doctor.StatusPass, doctor.StatusFail))
		})

		Context("when markdownlint is not available", func() {
			It("should return warning", func() {
				result := checker.Check(ctx)
				if result.Status == doctor.StatusFail {
					Expect(result.Severity).To(Equal(doctor.SeverityWarning))
					Expect(result.Details).NotTo(BeEmpty())
				}
			})
		})
	})
})
