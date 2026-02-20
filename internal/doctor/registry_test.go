package doctor_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registry Suite")
}

// stubChecker is a minimal HealthChecker for testing.
type stubChecker struct {
	name     string
	category doctor.Category
}

func (s *stubChecker) Name() string              { return s.name }
func (s *stubChecker) Category() doctor.Category { return s.category }

func (s *stubChecker) Check(_ context.Context) doctor.CheckResult {
	return doctor.Pass(s.name, "ok")
}

var _ = Describe("Registry", func() {
	var registry *doctor.Registry

	BeforeEach(func() {
		registry = doctor.NewRegistry()
		registry.RegisterChecker(&stubChecker{name: "bin-exists", category: doctor.CategoryBinary})
		registry.RegisterChecker(&stubChecker{name: "bin-perms", category: doctor.CategoryBinary})
		registry.RegisterChecker(&stubChecker{name: "hook-user", category: doctor.CategoryHook})
		registry.RegisterChecker(&stubChecker{name: "tool-sc", category: doctor.CategoryTools})
	})

	Describe("Checkers", func() {
		It("returns all registered checkers", func() {
			all := registry.Checkers()
			Expect(all).To(HaveLen(4))
		})
	})

	Describe("CheckersForCategories", func() {
		It("returns checkers for specified categories", func() {
			checkers := registry.CheckersForCategories([]doctor.Category{doctor.CategoryBinary})
			Expect(checkers).To(HaveLen(2))

			for _, c := range checkers {
				Expect(c.Category()).To(Equal(doctor.CategoryBinary))
			}
		})

		It("returns checkers for multiple categories", func() {
			checkers := registry.CheckersForCategories([]doctor.Category{
				doctor.CategoryBinary,
				doctor.CategoryHook,
			})
			Expect(checkers).To(HaveLen(3))
		})

		It("returns all checkers when categories is empty", func() {
			checkers := registry.CheckersForCategories(nil)
			Expect(checkers).To(HaveLen(4))
		})

		It("returns empty slice for unknown category", func() {
			checkers := registry.CheckersForCategories([]doctor.Category{"nonexistent"})
			Expect(checkers).To(BeEmpty())
		})
	})
})
