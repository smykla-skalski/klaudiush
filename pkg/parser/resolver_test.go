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

var _ = Describe("OSResolver", func() {
	var (
		resolver parser.OSResolver
		dir      string
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()

		// Git hooks export GIT_DIR and friends, which would point git at the
		// repository running the tests instead of the temporary one.
		for _, entry := range os.Environ() {
			if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "GIT_") {
				GinkgoT().Setenv(name, "")
				Expect(os.Unsetenv(name)).To(Succeed())
			}
		}
	})

	requireGit := func() string {
		path, err := exec.LookPath("git")
		if err != nil {
			Skip("git is not installed")
		}

		return path
	}

	Describe("Program", func() {
		It("identifies git by name", func() {
			requireGit()
			Expect(resolver.Program("git", "")).To(Equal(parser.ProgramGit))
		})

		It("identifies a symlink to git under another name", func() {
			gitPath := requireGit()
			link := filepath.Join(dir, "g")
			Expect(os.Symlink(gitPath, link)).To(Succeed())

			Expect(resolver.Program("./g", dir)).To(Equal(parser.ProgramGit))
			Expect(resolver.Program(link, "")).To(Equal(parser.ProgramGit))
		})

		It("identifies a copy of git under another name", func() {
			gitPath, err := filepath.EvalSymlinks(requireGit())
			Expect(err).NotTo(HaveOccurred())

			data, err := os.ReadFile(gitPath)
			Expect(err).NotTo(HaveOccurred())

			copyPath := filepath.Join(dir, "vcs")
			Expect(os.WriteFile(copyPath, data, 0o755)).To(Succeed())

			Expect(resolver.Program(copyPath, "")).To(Equal(parser.ProgramGit))
		})

		It("reports another program as other", func() {
			script := filepath.Join(dir, "tool")
			Expect(os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755)).To(Succeed())

			Expect(resolver.Program(script, "")).To(Equal(parser.ProgramOther))
		})

		It("reports a name nothing on disk runs as missing", func() {
			Expect(
				resolver.Program("klaudiush-no-such-program", ""),
			).To(Equal(parser.ProgramMissing))
			Expect(resolver.Program("./absent", dir)).To(Equal(parser.ProgramMissing))
		})
	})

	Describe("ReadScript", func() {
		It("reads a text script", func() {
			path := filepath.Join(dir, "x.sh")
			Expect(os.WriteFile(path, []byte("git status\n"), 0o600)).To(Succeed())

			text, ok := resolver.ReadScript(path)
			Expect(ok).To(BeTrue())
			Expect(text).To(Equal("git status\n"))
		})

		It("refuses a binary file", func() {
			path := filepath.Join(dir, "bin")
			Expect(os.WriteFile(path, []byte("ELF\x00\x01"), 0o600)).To(Succeed())

			_, ok := resolver.ReadScript(path)
			Expect(ok).To(BeFalse())
		})

		It("refuses an oversized file", func() {
			path := filepath.Join(dir, "big.sh")
			Expect(os.WriteFile(path, []byte(strings.Repeat("#", 300<<10)), 0o600)).To(Succeed())

			_, ok := resolver.ReadScript(path)
			Expect(ok).To(BeFalse())
		})

		It("refuses a directory", func() {
			_, ok := resolver.ReadScript(dir)
			Expect(ok).To(BeFalse())
		})
	})

	Describe("GitAlias", func() {
		It("reads an alias from the repository config", func() {
			requireGit()

			for _, args := range [][]string{
				{"init", "-q", dir},
				{"-C", dir, "config", "alias.zz", "commit"},
			} {
				Expect(exec.Command("git", args...).Run()).To(Succeed())
			}

			value, ok := resolver.GitAlias(dir, "zz")
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal("commit"))

			_, ok = resolver.GitAlias(dir, "klaudiush-no-such-alias")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("LookupEnv", func() {
		It("reads the environment", func() {
			GinkgoT().Setenv("KLAUDIUSH_RESOLVER_TEST", "/usr/bin/git")

			value, ok := resolver.LookupEnv("KLAUDIUSH_RESOLVER_TEST")
			Expect(ok).To(BeTrue())
			Expect(value).To(Equal("/usr/bin/git"))
		})
	})
})
