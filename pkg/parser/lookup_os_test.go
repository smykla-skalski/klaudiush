package parser_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// writeRanScript writes an executable script and runs it once. macOS checks
// a new file the first time it runs, and under load that check alone
// outlasts the resolver's lookup timeout.
func writeRanScript(path, text string) {
	Expect(os.WriteFile(path, []byte(text), 0o755)).To(Succeed())
	Expect(exec.Command(path, "--version").Run()).To(Succeed())
}

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

	It("runs the allowed environment path lookups", func() {
		bin := GinkgoT().TempDir()
		tools := map[string]string{
			"brew":   "#!/bin/sh\nif [ \"$#\" -eq 1 ]; then printf '/brew\\n'; else printf '/brew/%s\\n' \"$2\"; fi\n",
			"poetry": "#!/bin/sh\nprintf '/poetry-env\\n'\n",
			"conda":  "#!/bin/sh\nprintf '/conda-base\\n'\n",
		}

		for name, text := range tools {
			writeRanScript(filepath.Join(bin, name), text)
		}

		GinkgoT().Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

		lookups := []struct {
			argv []string
			want string
		}{
			{[]string{"brew", "--prefix"}, "/brew"},
			{[]string{"brew", "--prefix", "jq"}, "/brew/jq"},
			{[]string{"poetry", "env", "info", "--path"}, "/poetry-env"},
			{[]string{"conda", "info", "--base"}, "/conda-base"},
		}

		for _, lookup := range lookups {
			line := strings.Join(lookup.argv, " ")
			out, ok := resolver.CommandOutput(repo, lookup.argv)
			Expect(ok).To(BeTrue(), line)
			Expect(out).To(Equal(lookup.want), line)
		}
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

		_, ok = resolver.CommandOutput(repo, []string{"brew", "--prefix", "--installed"})
		Expect(ok).To(BeFalse())

		_, ok = resolver.CommandOutput(repo, []string{"poetry", "env", "info", "--path", "x"})
		Expect(ok).To(BeFalse())
	})
})
