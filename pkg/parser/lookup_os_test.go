package parser_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("OSResolver argument lookups", func() {
	var (
		repo     string
		resolver *parser.OSResolver
	)

	BeforeEach(func() {
		var err error

		repo, err = filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		init := exec.Command("git", "init", "-q", "--initial-branch=trunk", repo)

		init.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		Expect(init.Run()).To(Succeed())

		resolver = &parser.OSResolver{}
	})

	It("prints the current branch", func() {
		out, ok := resolver.CommandOutput(repo, []string{"git", "branch", "--show-current"})
		Expect(ok).To(BeTrue())
		Expect(out).To(Equal("trunk"))
	})

	It("fails when HEAD has no commit to abbreviate", func() {
		_, ok := resolver.CommandOutput(repo, []string{"git", "rev-parse", "--abbrev-ref", "HEAD"})
		Expect(ok).To(BeFalse())
	})

	It("prints the directory pwd would", func() {
		out, ok := resolver.CommandOutput(repo, []string{"pwd"})
		Expect(ok).To(BeTrue())
		Expect(out).To(Equal(repo))

		wd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		out, ok = resolver.CommandOutput("", []string{"pwd"})
		Expect(ok).To(BeTrue())
		Expect(out).To(Equal(wd))
	})

	It("runs nothing else", func() {
		_, ok := resolver.CommandOutput(repo, []string{"git", "branch", "-a"})
		Expect(ok).To(BeFalse())
	})
})
