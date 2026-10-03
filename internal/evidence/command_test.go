package evidence_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("MatchCommand", func() {
	var (
		checks []*evidence.Check
		repo   string
	)

	BeforeEach(func() {
		repo = GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(repo, "sub"), 0o755)).To(Succeed())

		var err error

		checks, err = evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{
				{Name: "tests", Commands: []string{"mise run test", "go test ./..."}},
				{
					Name:     "review",
					Kind:     config.EvidenceKindReview,
					Commands: []string{"./review.sh"},
					Base:     "main",
				},
			},
		})
		Expect(err).NotTo(HaveOccurred())
	})

	matched := func(command, dir string) string {
		check := evidence.MatchCommand(checks, command, dir, repo)
		if check == nil {
			return ""
		}

		return check.Name
	}

	DescribeTable("from the repository root",
		func(command, expected string) {
			Expect(matched(command, repo)).To(Equal(expected))
		},
		Entry("exact", "mise run test", "tests"),
		Entry("second command", "go test ./...", "tests"),
		Entry("extra spaces", "  go   test   ./...  ", "tests"),
		Entry("quoted words", `"go" 'test' "./..."`, "tests"),
		Entry("review script", "./review.sh", "review"),
		Entry("output redirect", "mise run test > out.log 2>&1", "tests"),
		Entry("cd to the root first", "cd . && mise run test", "tests"),
		Entry("cd chain back to the root", "cd ./sub && cd .. && mise run test", "tests"),
		Entry("cd chain with trailing slashes", "cd ./sub/ && cd ../ && mise run test", "tests"),
		Entry("cd through CDPATH", "cd sub && cd .. && mise run test", ""),
		Entry("cd to a CDPATH lookup that ends at the root", "cd sub/.. && mise run test", ""),
		Entry("cd to the previous directory", "cd - && mise run test", ""),
		Entry("absolute cd", "cd "+"REPO"+" && mise run test", ""),
		Entry("extra argument", "mise run test -- -run X", ""),
		Entry("fewer arguments", "mise run", ""),
		Entry("pipe", "mise run test | tail", ""),
		Entry("or", "mise run test || true", ""),
		Entry("sequence", "mise run test; true", ""),
		Entry("and after", "mise run test && true", ""),
		Entry("and before", "true && mise run test", ""),
		Entry("background", "mise run test &", ""),
		Entry("negated", "! mise run test", ""),
		Entry("env prefix", "GOFLAGS=-count=1 mise run test", ""),
		Entry("command substitution", "mise run $(echo test)", ""),
		Entry("variable", "mise run $T", ""),
		Entry("glob", "go test ./*", ""),
		Entry("tilde", "~/bin/mise run test", ""),
		Entry("escaped", `mise run te\st`, ""),
		Entry("input redirect", "mise run test < /dev/null", ""),
		Entry("redirect with expansion", "mise run test > $LOG", ""),
		Entry("subshell", "(mise run test)", ""),
		Entry("function", "f() { true; }", ""),
		Entry("two statements", "mise run test\nmise run test", ""),
		Entry("unparsable", "mise run 'test", ""),
		Entry("ansi-c quotes", "mise run $'test'", ""),
		Entry("double-quoted expansion", `mise run "$T"`, ""),
		Entry("double-quoted escape", `mise run "te\st"`, ""),
		Entry("dollar double quotes", `mise run $"test"`, ""),
		Entry("other command", "make test", ""),
	)

	It("accepts an absolute cd into the repository root", func() {
		Expect(
			matched("cd "+repo+" && mise run test", filepath.Join(repo, "sub")),
		).To(Equal("tests"))
	})

	It("refuses a relative cd the shell may resolve through CDPATH", func() {
		parent, name := filepath.Dir(repo), filepath.Base(repo)

		Expect(matched("cd "+name+" && mise run test", parent)).To(BeEmpty())
		Expect(matched("cd ./"+name+" && mise run test", parent)).To(Equal("tests"))
	})

	It("refuses commands run from another directory", func() {
		Expect(matched("mise run test", filepath.Join(repo, "sub"))).To(BeEmpty())
		Expect(matched("cd ./sub && mise run test", repo)).To(BeEmpty())
		Expect(matched("cd sub extra && mise run test", repo)).To(BeEmpty())
		Expect(matched("mise run test", "")).To(BeEmpty())
	})
})
