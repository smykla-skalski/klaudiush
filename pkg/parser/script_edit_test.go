package parser_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// writtenScript writes w.py with a heredoc, runs between, then runs w.py.
func writtenScript(between string) string {
	return "cat > w.py <<'EOF'\nprint('hello')\nEOF\n" + between + "python3 w.py"
}

var _ = Describe("A script captured from a write and edited before it runs", func() {
	parse := func(command string) *parser.ParseResult {
		resolver := fakeResolver{files: map[string]string{
			"build.py": "import sys\ndef f(a: int) -> int:\n    return a\nif len(sys.argv) > 1:\n    print(f(1))\n",
		}}
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	DescribeTable("treats the script as unreadable",
		func(between string) {
			result := parse(writtenScript(between))
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityUnreadableScript),
				HaveField("Detail", parser.DetailScriptEdited),
			)), "opacities: %v", result.Opacities)
		},
		Entry("an unknown tool naming it", "sometool --edit w.py\n"),
		Entry("python code writing it", "python3 -c 'open(\"w.py\", \"w\").write(\"x\")'\n"),
		Entry("a script on disk", "python3 mut.py\n"),
		Entry("ruby editing in place", "ruby -i -pe 'x' w.py\n"),
		Entry("patch naming it", "patch w.py < p.diff\n"),
		Entry("git restoring it", "git checkout -- w.py\n"),
		Entry("after a pipe", "echo x | sometool w.py\n"),
		Entry("behind a launcher", "sudo sometool w.py\n"),
		Entry("inside a shell", "bash -c 'sometool w.py'\n"),
		Entry("a formatter given the directory", "black .\n"),
		Entry("a formatter writing it", "gofmt -w w.py\n"),
		Entry("a linter fixing it", "ruff check --fix w.py\n"),
		Entry("sed with a combined in-place flag", "sed -Ei 's/a/b/' *.py\n"),
		Entry("perl editing in place", "perl -pi -e 's/a/b/' *.py\n"),
		Entry("sort with a combined output flag", "sort -ro w.py\n"),
		Entry("a formatter over a package pattern", "gofmt -w ./...\n"),
		Entry("a fix-only linter", "ruff check --fix-only .\n"),
		Entry("a linter fixing the directory", "ruff check --fix .\n"),
		Entry("a formatter module given the directory", "python3 -m black .\n"),
		Entry("inline code importing a local module", "python3 -c 'import mut'\n"),
		Entry("a tool given a matching glob", "sometool w.p*\n"),
		Entry("a make target that formats", "make fmt\n"),
		Entry("a tarball extracted with flags", "tar -xzf a.tgz\n"),
		Entry("tar extracting with other letters", "tar -xmf a.tar\ntar -xzC d -f a.tgz\n"),
		Entry("tar extracting after -C", "tar -C d -xf a.tar\n"),
		Entry("a formatter given a line length", "black -l 100 w.py\n"),
		Entry("a lister told to write", "gofmt -l -w .\n"),
		Entry("a linter fixing the current directory", "ruff check --fix\n"),
		Entry("a formatter on the current directory", "go fmt\nruff format\n"),
		Entry("git checking out the tree", "git checkout .\n"),
		Entry("git applying a patch", "git apply p.diff\n"),
		Entry("an in-place edit over a glob", "sed -i s/a/b/ *.py\n"),
		Entry("a tool given a variable", "F=w.py; sometool $F\n"),
		Entry("a loop over a glob", "for f in *.py; do sometool $f; done\n"),
		Entry("find running a tool", "find . -name '*.py' -exec sometool {} +\n"),
		Entry("xargs running a tool", "ls | xargs sometool\n"),
		Entry("a flag value naming it", "sometool --file=w.py\n"),
		Entry("python reading its program from a heredoc",
			"python3 - <<'E'\nopen('w.py', 'w').write('x')\nE\n"),
		Entry("python reading its program from a pipe",
			"echo \"open('w.py', 'w').write('x')\" | python3\n"),
		Entry("python reading its program from a here-string",
			"python3 <<< \"open('w.py', 'w').write('x')\"\n"),
		Entry("sort writing its output over it", "sort -o w.py\n"),
		Entry("an archive extracted", "unzip -o a.zip\n"),
		Entry("a tarball extracted", "tar xf a.tar\n"),
		Entry("a sync", "rsync -a src/x dst\n"),
		Entry("patch reading a file", "patch -p1 -i p.diff\n"),
	)

	DescribeTable(
		"reads the captured content when nothing may edit it",
		func(between string) {
			result := parse(writtenScript(between))
			Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		},
		Entry("nothing in between", ""),
		Entry("chmod", "chmod +x w.py\n"),
		Entry("readers", "cat w.py\nwc -l w.py\nls -la\n"),
		Entry("git add", "git add w.py\n"),
		Entry("a tool naming another file", "sometool other.txt\n"),
		Entry("a tool naming a similar file", "sometool w.pyc\n"),
		Entry("an earlier run of the same script", "python3 w.py\n"),
		Entry("a python module", "python3 -m pytest\n"),
		Entry("read-only inline code", "python3 -c 'print(1)'\n"),
		Entry("awk printing a file", "awk '{print}' data.txt\n"),
		Entry("another script written on the line",
			"cat > v.py <<'EOF'\nprint('v')\nEOF\npython3 v.py\n"),
		Entry("a test run", "pytest\n"),
		Entry("system status", "df -h\nps\n"),
		Entry("a download", "curl -s https://example.com/\n"),
		Entry("find listing files", "find src -name '*.go'\n"),
		Entry("inline code dumping JSON", "python3 -c 'import json; print(json.dumps({}))'\n"),
		Entry("a new branch", "git checkout -b feat\n"),
		Entry("a build given the directory", "go build .\n"),
		Entry("linters naming it", "ruff check w.py\nflake8 w.py\nmypy w.py\npylint w.py\n"),
		Entry("a compile check", "python3 -m py_compile w.py\n"),
		Entry("tests naming it", "pytest w.py\npython3 -m pytest w.py\n"),
		Entry("a formatter checking it", "black --check w.py\n"),
		Entry("filters reading it", "sed -n p w.py\nsort w.py\nawk 1 w.py\n"),
		Entry(
			"formatters in check mode",
			"black --check .\nprettier --check .\ngofmt -l .\nruff format --check .\ntofu fmt -check .\n",
		),
		Entry("a linter checking the directory", "ruff check .\n"),
		Entry("an image build", "docker build -t x .\n"),
		Entry("tests given a directory", "pytest tests/\n"),
		Entry("a tool given a glob for other files", "sometool *.txt\n"),
		Entry("a script comparing values", "python3 build.py\n"),
		Entry("make targets that build and test", "make test\nmake build\nmake\n"),
		Entry("import order checks", "isort -c .\ngofmt -l .\n"),
		Entry("an archive listing", "tar tf x.tar\nunzip -l a.zip\n"),
		Entry(
			"archive listings with more letters",
			"unzip -lq a.zip\nunzip -p a.zip f\ntar cf x w\ntar -tvf a.tar\n",
		),
		Entry("sed editing another file", "sed -i s/a/b/ other.txt\n"),
		Entry("a shell command line not naming it", "sh -c 'echo hi'\n"),
		Entry("interactive containers and keys", "docker exec -i c ls\nssh -i key host ls\n"),
		Entry("inline code comparing values", "python3 -c 'print(1 > 0)'\n"),
	)

	It("parses many scripts written and run in turn quickly", func() {
		var command strings.Builder

		for i := range 24 {
			fmt.Fprintf(&command, "cat > v%d.py <<'EOF'\nprint(%d)\nEOF\n", i, i)
		}

		for i := range 24 {
			fmt.Fprintf(&command, "python3 v%d.py\n", i)
		}

		start := time.Now()
		result := parse(command.String())

		Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})

	It("counts a formatter given the parent directory by path", func() {
		result := parse("cat > d/w.py <<'EOF'\nprint(1)\nEOF\nblack d\npython3 d/w.py")
		Expect(result.Opacities).To(ContainElement(HaveField("Detail", parser.DetailScriptEdited)),
			"opacities: %v", result.Opacities)
	})

	It("does not count an interpreter run before the write", func() {
		result := parse("python3 -c 'print(1)'\n" + writtenScript(""))
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})

	It("trusts a later full rewrite it captures", func() {
		result := parse(writtenScript("sometool w.py\ncat > w.py <<'EOF'\nprint('again')\nEOF\n"))
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})
})
