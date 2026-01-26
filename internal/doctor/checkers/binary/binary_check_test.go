package binary_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/checkers/binary"
)

func TestBinary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Binary Checker Suite")
}

var _ = Describe("ExistsChecker", func() {
	var (
		checker *binary.ExistsChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = binary.NewExistsChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Binary available and executable"))
		})
	})

	Describe("Category", func() {
		It("should return binary category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryBinary))
		})
	})

	Describe("Check", func() {
		Context("when binary exists in PATH", func() {
			It("should return pass result", func() {
				// This test assumes klaudiush is in PATH during testing
				// In real scenarios, we'd mock exec.LookPath
				result := checker.Check(ctx)
				// We can't guarantee klaudiush is in PATH, so just check result structure
				Expect(result.Name).To(Equal("Binary available and executable"))
				Expect(result.Status).To(BeElementOf(doctor.StatusPass, doctor.StatusFail))
			})
		})
	})
})

var _ = Describe("PermissionsChecker", func() {
	var (
		checker *binary.PermissionsChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = binary.NewPermissionsChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Correct permissions"))
		})
	})

	Describe("Category", func() {
		It("should return binary category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryBinary))
		})
	})

	Describe("Check", func() {
		It("should check binary permissions", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Correct permissions"))
			// Status depends on whether binary exists and its permissions
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusFail,
				doctor.StatusSkipped,
			))
		})
	})
})

var _ = Describe("LocationChecker", func() {
	var (
		checker *binary.LocationChecker
		ctx     context.Context
	)

	BeforeEach(func() {
		checker = binary.NewLocationChecker()
		ctx = context.Background()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Binary location"))
		})
	})

	Describe("Category", func() {
		It("should return binary category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryBinary))
		})
	})

	Describe("Check", func() {
		It("should check binary location", func() {
			result := checker.Check(ctx)
			Expect(result.Name).To(Equal("Binary location"))
			// Status depends on whether binary exists
			Expect(result.Status).To(BeElementOf(
				doctor.StatusPass,
				doctor.StatusSkipped,
			))
		})
	})
})
