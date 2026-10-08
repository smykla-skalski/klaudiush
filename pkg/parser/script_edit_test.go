package parser_test

import (
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
		result, err := parser.NewBashParserWithResolver(fakeResolver{}).Parse(command)
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
		Entry("a tool given the directory", "sometool .\n"),
		Entry("a tool with no operands", "make\n"),
		Entry("git checking out the tree", "git checkout .\n"),
		Entry("git applying a patch", "git apply p.diff\n"),
		Entry("an in-place edit over a glob", "sed -i s/a/b/ *.py\n"),
		Entry("a tool given a variable", "F=w.py; sometool $F\n"),
		Entry("a loop over a glob", "for f in *.py; do sometool $f; done\n"),
		Entry("find running a tool", "find . -name '*.py' -exec sometool {} +\n"),
		Entry("xargs running a tool", "ls | xargs sometool\n"),
		Entry("a flag value naming it", "sometool --file=w.py\n"),
	)

	DescribeTable("reads the captured content when nothing may edit it",
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
	)

	It("does not count an interpreter run before the write", func() {
		result := parse("python3 -c 'print(1)'\n" + writtenScript(""))
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})

	It("trusts a later full rewrite it captures", func() {
		result := parse(writtenScript("sometool w.py\ncat > w.py <<'EOF'\nprint('again')\nEOF\n"))
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})
})
