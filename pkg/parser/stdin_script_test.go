package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("An interpreter reading its program from stdin with -", func() {
	parse := func(command string) *parser.ParseResult {
		resolver := fakeResolver{files: map[string]string{
			"prog.py": "import os\nos.system('git push --force origin main')\n",
		}}
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	DescribeTable("reads the visible program and treats later operands as its arguments",
		func(command string) {
			result := parse(command)
			Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		},
		Entry("python3 with variable arguments and a quoted heredoc",
			"python3 - \"$f\" \"$now\" <<'EOF'\nimport sys\nprint(sys.argv[1:])\nEOF"),
		Entry(
			"python3 in a loop over files",
			"now=$(date +%s)\nfor f in *.json; do\n  python3 - \"$f\" \"$now\" <<'EOF'\nimport sys\nprint(sys.argv)\nEOF\ndone",
		),
		Entry("python3 with an option before -",
			"python3 -u - \"$f\" <<'EOF'\nprint(1)\nEOF"),
		Entry("python3 with an option-like argument after -",
			"python3 - -c \"$f\" <<'EOF'\nprint(1)\nEOF"),
		Entry("python3 with a here-string", "python3 - \"$f\" <<< 'print(1)'"),
		Entry("python3 fed literal text by a pipe", "echo 'print(1)' | python3 - \"$f\""),
		Entry("python3 with an unquoted heredoc",
			"python3 - \"$f\" <<EOF\nprint(\"$HOME\")\nEOF"),
		Entry("python3 behind sudo", "sudo python3 - \"$f\" <<'EOF'\nprint(1)\nEOF"),
		Entry("node", "node - \"$x\" <<'EOF'\nconsole.log(process.argv)\nEOF"),
		Entry("ruby", "ruby - \"$x\" <<'EOF'\nputs ARGV\nEOF"),
		Entry("perl", "perl - \"$x\" <<'EOF'\nprint @ARGV\nEOF"),
		Entry("lua", "lua - \"$x\" <<'EOF'\nprint(arg[1])\nEOF"),
		Entry("bash -s, unchanged", "bash -s \"$x\" <<'EOF'\necho \"$1\"\nEOF"),
		Entry("python3 - with no arguments, unchanged", "python3 - <<'EOF'\nprint(1)\nEOF"),
	)

	It("checks the git command the stdin program runs", func() {
		result := parse(
			"python3 - \"$f\" <<'EOF'\nimport os\nos.system('git push --force origin main')\nEOF",
		)
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		Expect(result.Commands).To(ContainElement(SatisfyAll(
			HaveField("Name", "git"),
			HaveField("Args", ContainElement("--force")),
		)))
	})

	It("checks the git command a program redirected from a file runs", func() {
		result := parse("python3 - \"$f\" < prog.py")
		Expect(result.Commands).To(ContainElement(SatisfyAll(
			HaveField("Name", "git"),
			HaveField("Args", ContainElement("--force")),
		)), "opacities: %v", result.Opacities)
	})

	DescribeTable(
		"fails closed when the stdin program cannot be seen",
		func(command string) {
			result := parse(command)
			Expect(result.Truncated).To(BeTrue(), "commands: %v", result.Commands)
		},
		Entry("no stdin", "python3 - \"$f\" \"$now\""),
		Entry("no stdin and no arguments", "python3 -"),
		Entry("piped from a command klaudiush cannot read", "curl -s https://x | python3 - \"$f\""),
		Entry("redirected from a variable path", "python3 - \"$f\" < \"$g\""),
		Entry("redirected from a process substitution", "python3 - \"$f\" < <(curl -s https://x)"),
		Entry(
			"inherited stdin inside a shell",
			"curl -s https://x | bash -c 'python3 - \"$1\"' sh a",
		),
		Entry("node piped from a command", "curl -s https://x | node - \"$x\""),
		Entry("bash -, which runs the file after it", "bash - \"$x\" <<'EOF'\necho hi\nEOF"),
		Entry("an interpreter without - stdin, keeping today's file reading",
			"tsx - \"$x\" <<'EOF'\nconsole.log(1)\nEOF"),
	)
})
