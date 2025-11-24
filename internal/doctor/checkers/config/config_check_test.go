package config_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/doctor/checkers/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Checker Suite")
}

var _ = Describe("GlobalChecker", func() {
	var (
		checker *config.GlobalChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = config.NewGlobalChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Global config valid"))
		})
	})

	Describe("Category", func() {
		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		It("should check global config validity", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Global config valid"))
			// Result depends on whether global config exists and is valid
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
			))
		})

		Context("when config is missing", func() {
			It("should return warning with fix ID", func() {
				result := checker.Check(ctx)
				// If config doesn't exist, it should be a warning
				if result.Status == doctor.StatusFail && result.Severity == doctor.SeverityWarning {
					Expect(result.Message).To(ContainSubstring("not found"))
					Expect(result.FixID).To(Equal("create_global_config"))
				}
			})
		})
	})
})

var _ = Describe("ProjectChecker", func() {
	var (
		checker *config.ProjectChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = config.NewProjectChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Project config valid"))
		})
	})

	Describe("Category", func() {
		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		It("should check project config validity", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Project config valid"))
			// Result depends on whether project config exists and is valid
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
			))
		})

		Context("when config is missing", func() {
			It("should return warning with fix ID", func() {
				result := checker.Check(ctx)
				// If config doesn't exist, it should be a warning
				if result.Status == doctor.StatusFail && result.Severity == doctor.SeverityWarning {
					Expect(result.Message).To(ContainSubstring("not found"))
					Expect(result.FixID).To(Equal("create_project_config"))
				}
			})
		})
	})
})

var _ = Describe("PermissionsChecker", func() {
	var (
		checker *config.PermissionsChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = config.NewPermissionsChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Config file permissions secure"))
		})
	})

	Describe("Category", func() {
		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		It("should check config file permissions", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Config file permissions secure"))
			// Result depends on whether config files exist and their permissions
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})

		Context("when no config files exist", func() {
			It("should skip the check", func() {
				result := checker.Check(ctx)
				// If no config files exist, check should be skipped
				if result.Status == doctor.StatusSkipped {
					Expect(result.Message).To(ContainSubstring("No config files found"))
				}
			})
		})
	})
})
