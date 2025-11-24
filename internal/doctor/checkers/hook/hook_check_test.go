package hook_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/doctor/checkers/hook"
)

func TestHook(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hook Checker Suite")
}

var _ = Describe("RegistrationChecker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewUserRegistrationChecker", func() {
		var checker *hook.RegistrationChecker

		BeforeEach(func() {
			checker = hook.NewUserRegistrationChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher registered in user settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher registered in user settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectRegistrationChecker", func() {
		var checker *hook.RegistrationChecker

		BeforeEach(func() {
			checker = hook.NewProjectRegistrationChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher registered in project settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher registered in project settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectLocalRegistrationChecker", func() {
		var checker *hook.RegistrationChecker

		BeforeEach(func() {
			checker = hook.NewProjectLocalRegistrationChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher registered in project-local settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher registered in project-local settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})
})

var _ = Describe("PreToolUseChecker", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewUserPreToolUseChecker", func() {
		var checker *hook.PreToolUseChecker

		BeforeEach(func() {
			checker = hook.NewUserPreToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PreToolUse hook in user settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("PreToolUse hook in user settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})

	Describe("NewProjectPreToolUseChecker", func() {
		var checker *hook.PreToolUseChecker

		BeforeEach(func() {
			checker = hook.NewProjectPreToolUseChecker()
		})

		It("should have correct name", func() {
			Expect(checker.Name()).To(Equal("PreToolUse hook in project settings"))
		})

		It("should have hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})

		It("should perform check", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("PreToolUse hook in project settings"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})
})

var _ = Describe("PathValidationChecker", func() {
	var (
		checker *hook.PathValidationChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = hook.NewPathValidationChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Dispatcher path is valid"))
		})
	})

	Describe("Category", func() {
		It("should return hook category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryHook))
		})
	})

	Describe("Check", func() {
		It("should check dispatcher path", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Dispatcher path is valid"))
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})
})
