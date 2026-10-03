package harness_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

const probeVar = "KLAUDIUSH_HARNESS_PROBE"

// writeScript writes a script and returns its path with symlinks
// resolved, the form ResolveBinary returns.
func writeScript(dir, name, body string) string {
	path := filepath.Join(dir, name)
	Expect(os.MkdirAll(dir, 0o700)).To(Succeed())
	Expect(os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700)).To(Succeed())

	resolved, err := filepath.EvalSymlinks(path)
	Expect(err).NotTo(HaveOccurred())

	return resolved
}

// fakeMise writes a mise stand-in that answers `which <tool>` from answers
// and fails for any other tool, the way mise does for an inactive tool.
func fakeMise(dir string, answers map[string]string) string {
	var cases strings.Builder

	for tool, answer := range answers {
		cases.WriteString(tool + ") echo '" + answer + "'; exit 0 ;;\n")
	}

	return writeScript(dir, "mise", `[ "$1" = which ] || exit 2
case "$2" in
`+cases.String()+`esac
echo "mise ERROR $2 is a mise bin however it is not currently active" >&2
exit 1`)
}

func symlink(target, link string) string {
	Expect(os.MkdirAll(filepath.Dir(link), 0o700)).To(Succeed())
	Expect(os.Symlink(target, link)).To(Succeed())

	return link
}

var _ = Describe("ResolveBinary", func() {
	var root string

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		GinkgoT().Setenv(probeVar, "")
		GinkgoT().Setenv("PATH", "")
	})

	setPath := func(dirs ...string) {
		GinkgoT().Setenv("PATH", strings.Join(dirs, string(os.PathListSeparator)))
	}

	It("returns nothing when the harness is not installed", func() {
		setPath(filepath.Join(root, "empty"), "relative/bin")

		Expect(harness.ResolveBinary(probeVar, "tool")).To(BeEmpty())
	})

	It("follows symlinks of a plain binary on PATH", func() {
		want := writeScript(filepath.Join(root, "opt"), "tool-1.0", "true")
		bin := filepath.Join(root, "bin")
		symlink(want, filepath.Join(bin, "tool"))
		setPath(bin)

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(want))
	})

	It("skips entries that are not executable files", func() {
		first := filepath.Join(root, "first")
		Expect(os.MkdirAll(filepath.Join(first, "tool"), 0o700)).To(Succeed())

		second := filepath.Join(root, "second")
		Expect(os.MkdirAll(second, 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(second, "tool"), []byte("x"), 0o600)).To(Succeed())

		want := writeScript(filepath.Join(root, "third"), "tool", "true")
		setPath("relative", first, second, filepath.Join(root, "third"))

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(want))
	})

	It("prefers the override variable over PATH", func() {
		onPath := filepath.Join(root, "bin")
		writeScript(onPath, "tool", "true")
		setPath(onPath)

		override := writeScript(filepath.Join(root, "override"), "tool", "true")
		GinkgoT().Setenv(probeVar, override)

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(override))
	})

	It("fails clearly when the override does not exist", func() {
		GinkgoT().Setenv(probeVar, filepath.Join(root, "dangling"))

		path, err := harness.ResolveBinary(probeVar, "tool")
		Expect(path).To(BeEmpty())
		Expect(err).To(MatchError(ContainSubstring(probeVar + "=")))

		plain := filepath.Join(root, "plain")
		Expect(os.WriteFile(plain, []byte("x"), 0o600)).To(Succeed())
		GinkgoT().Setenv(probeVar, plain)

		_, err = harness.ResolveBinary(probeVar, "tool")
		Expect(err).To(MatchError(ContainSubstring("not an executable file")))
	})

	It("resolves an active mise shim with mise which", func() {
		want := writeScript(filepath.Join(root, "installs", "tool", "1.0"), "tool", "true")
		mise := fakeMise(filepath.Join(root, "mise-bin"), map[string]string{"tool": want})
		shims := filepath.Join(root, "shims")
		symlink(mise, filepath.Join(shims, "tool"))

		fallback := filepath.Join(root, "fallback")
		writeScript(fallback, "tool", "true")
		setPath(shims, fallback)

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(want))
	})

	It("resolves a mise shim named by the override variable", func() {
		want := writeScript(filepath.Join(root, "installs"), "tool", "true")
		mise := fakeMise(filepath.Join(root, "mise-bin"), map[string]string{"tool": want})
		GinkgoT().Setenv(probeVar, symlink(mise, filepath.Join(root, "shims", "tool")))

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(want))
	})

	It("falls through an inactive mise shim to the next PATH entry", func() {
		mise := fakeMise(filepath.Join(root, "mise-bin"), nil)
		shims := filepath.Join(root, "shims")
		symlink(mise, filepath.Join(shims, "tool"))

		fallback := filepath.Join(root, "fallback")
		want := writeScript(fallback, "tool", "true")
		setPath(shims, fallback)

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(want))
	})

	It("fails clearly when no shim on PATH resolves", func() {
		mise := fakeMise(filepath.Join(root, "mise-bin"), nil)
		shims := filepath.Join(root, "shims")
		symlink(mise, filepath.Join(shims, "tool"))
		setPath(shims)

		path, err := harness.ResolveBinary(probeVar, "tool")
		Expect(path).To(BeEmpty())
		Expect(err).To(MatchError(And(
			ContainSubstring("set "+probeVar),
			ContainSubstring("mise shim"),
			ContainSubstring("not currently active"),
		)))
	})

	It("rejects mise answers that are not a want executable", func() {
		mise := fakeMise(filepath.Join(root, "mise-bin"), map[string]string{
			"relative": "bin/relative",
			"missing":  filepath.Join(root, "missing"),
			"loop":     filepath.Join(root, "shims", "loop"),
		})
		shims := filepath.Join(root, "shims")

		for _, tool := range []string{"relative", "missing", "loop"} {
			symlink(mise, filepath.Join(shims, tool))
		}

		setPath(shims)

		for tool, reason := range map[string]string{
			"relative": "no absolute path",
			"missing":  "returned " + filepath.Join(root, "missing"),
			"loop":     "returned another shim",
		} {
			_, err := harness.ResolveBinary(probeVar, tool)
			Expect(err).To(MatchError(ContainSubstring(reason)), tool)
		}
	})

	It("resolves an asdf shim with asdf which", func() {
		want := writeScript(filepath.Join(root, "installs"), "tool", "true")
		asdfBin := filepath.Join(root, "asdf-bin")
		writeScript(asdfBin, "asdf", `[ "$1" = which ] && [ "$2" = tool ] && echo '`+want+`'`)

		shims := filepath.Join(root, "asdf-shims")
		writeScript(shims, "tool", `# asdf-plugin: tool 1.0
exec asdf exec "tool" "$@"`)
		setPath(shims, asdfBin)

		Expect(harness.ResolveBinary(probeVar, "tool")).To(Equal(want))
	})

	It("reports a resolution error from the drivers", func() {
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CLAUDE", filepath.Join(root, "dangling"))
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_CODEX", filepath.Join(root, "dangling"))
		GinkgoT().Setenv("KLAUDIUSH_HARNESS_OPENCODE", filepath.Join(root, "dangling"))

		for _, driver := range []harness.Driver{
			harness.NewClaudeDriver(), harness.NewCodexDriver(), harness.NewOpenCodeDriver(),
		} {
			Expect(driver.Binary()).To(BeEmpty(), driver.Name())
			Expect(driver.BinaryError()).To(
				MatchError(ContainSubstring("KLAUDIUSH_HARNESS_")), driver.Name())
		}
	})
})
