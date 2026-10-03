package evidence_test

import (
	"time"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

func compileOne(item *config.EvidenceCheckConfig) *evidence.Check {
	checks, err := evidence.Compile(&config.EvidenceConfig{
		Checks: []*config.EvidenceCheckConfig{item},
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(checks).To(HaveLen(1))

	return checks[0]
}

var _ = Describe("Compile", func() {
	It("compiles nothing from a nil configuration", func() {
		checks, err := evidence.Compile(nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(checks).To(BeEmpty())
	})

	It("applies defaults", func() {
		check := compileOne(&config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"go  test   './...'"},
		})

		Expect(check.Kind).To(Equal(config.EvidenceKindTest))
		Expect(check.IsReview()).To(BeFalse())
		Expect(check.Base).To(BeEmpty())
		Expect(check.Timeout).To(Equal(config.DefaultEvidenceTimeout))
		Expect(check.Commands).To(Equal([][]string{{"go", "test", "./..."}}))
		Expect(check.RunCommand()).To(Equal("go test ./..."))
		Expect(check.ID()).To(HavePrefix("tests@"))
		Expect(check.Covers("any/file.txt")).To(BeTrue())
		Expect(check.Covers(".klaudiush/patterns.json")).To(BeFalse())
	})

	It("compiles a review check with a base and timeout", func() {
		check := compileOne(&config.EvidenceCheckConfig{
			Name:     "review",
			Kind:     config.EvidenceKindReview,
			Commands: []string{"scripts/review.sh"},
			Base:     "origin/main",
			Timeout:  config.Duration(time.Minute),
		})

		Expect(check.IsReview()).To(BeTrue())
		Expect(check.Base).To(Equal("origin/main"))
		Expect(check.Timeout).To(Equal(time.Minute))
	})

	It("requires a base for a review", func() {
		_, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{{
				Name:     "review",
				Kind:     config.EvidenceKindReview,
				Commands: []string{"review"},
				Base:     " ",
			}},
		})

		Expect(err).To(MatchError(ContainSubstring("a review needs a base branch")))
	})

	It("matches covered paths with globs and exclusions", func() {
		check := compileOne(&config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"make test"},
			Paths:    []string{"**/*.go", "go.mod"},
			Exclude:  []string{"vendor/**"},
		})

		Expect(check.Covers("main.go")).To(BeTrue())
		Expect(check.Covers("pkg/a/b.go")).To(BeTrue())
		Expect(check.Covers("go.mod")).To(BeTrue())
		Expect(check.Covers("README.md")).To(BeFalse())
		Expect(check.Covers("vendor/x/y.go")).To(BeFalse())
	})

	It("changes the ID when the definition changes", func() {
		base := &config.EvidenceCheckConfig{Name: "tests", Commands: []string{"make test"}}
		other := &config.EvidenceCheckConfig{Name: "tests", Commands: []string{"make check"}}
		paths := &config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"make test"},
			Paths:    []string{"*.go"},
		}

		Expect(compileOne(base).ID()).To(Equal(compileOne(base).ID()))
		Expect(compileOne(base).ID()).NotTo(Equal(compileOne(other).ID()))
		Expect(compileOne(base).ID()).NotTo(Equal(compileOne(paths).ID()))
	})

	It("finds checks by name", func() {
		checks, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{
				{Name: "a", Commands: []string{"a"}},
				{Name: "b", Commands: []string{"b"}},
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(evidence.Find(checks, "b").Name).To(Equal("b"))
		Expect(evidence.Find(checks, "c")).To(BeNil())
	})

	DescribeTable("rejects invalid checks",
		func(item *config.EvidenceCheckConfig, message string) {
			_, err := evidence.Compile(&config.EvidenceConfig{
				Checks: []*config.EvidenceCheckConfig{item},
			})
			Expect(err).To(MatchError(ContainSubstring(message)))
			Expect(errors.Is(err, evidence.ErrInvalidCheck) ||
				errors.Is(err, evidence.ErrNotLiteral)).To(BeTrue())
		},
		Entry("nil", nil, "empty check"),
		Entry("no name", &config.EvidenceCheckConfig{Commands: []string{"x"}}, "name"),
		Entry("name with space",
			&config.EvidenceCheckConfig{Name: "a b", Commands: []string{"x"}}, "name"),
		Entry("bad kind",
			&config.EvidenceCheckConfig{Name: "a", Kind: "lint", Commands: []string{"x"}}, "kind"),
		Entry("no commands", &config.EvidenceCheckConfig{Name: "a"}, "commands must not be empty"),
		Entry(
			"pipe",
			&config.EvidenceCheckConfig{
				Name:     "a",
				Commands: []string{"go test | tee x"},
			},
			"command",
		),
		Entry("chain",
			&config.EvidenceCheckConfig{Name: "a", Commands: []string{"cd x && go test"}},
			"expected one command"),
		Entry("expansion",
			&config.EvidenceCheckConfig{Name: "a", Commands: []string{"go test $PKG"}}, "command"),
		Entry("unparsable",
			&config.EvidenceCheckConfig{Name: "a", Commands: []string{"go test 'x"}}, "command"),
		Entry("bad glob",
			&config.EvidenceCheckConfig{Name: "a", Commands: []string{"x"}, Paths: []string{"[a"}},
			"invalid path pattern"),
		Entry("empty glob",
			&config.EvidenceCheckConfig{Name: "a", Commands: []string{"x"}, Exclude: []string{""}},
			"invalid path pattern"),
	)

	It("rejects duplicate names", func() {
		_, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{
				{Name: "a", Commands: []string{"x"}},
				{Name: "a", Commands: []string{"y"}},
			},
		})
		Expect(err).To(MatchError(ContainSubstring("duplicate name")))
	})
})
