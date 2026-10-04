package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Commands chained after a function definition", func() {
	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(fakeResolver{}).Parse(command)
		Expect(err).NotTo(HaveOccurred(), command)

		return result
	}

	gitLines := func(result *parser.ParseResult) []string {
		var lines []string

		for _, cmd := range result.Commands {
			if cmd.Name == "git" {
				lines = append(lines, strings.Join(cmd.Args, " "))
			}
		}

		return lines
	}

	DescribeTable("walks what runs when the function is defined",
		func(command string) {
			result := parse(command)

			Expect(gitLines(result)).To(ConsistOf("push --force"), command)
			Expect(result.Truncated).To(BeFalse(), command)
		},
		Entry("after &&", "f() { :; } && git push --force"),
		Entry("after ||", "f() { :; } || git push --force"),
		Entry("after a pipe", "f() { :; } | git push --force"),
		Entry("after |&", "f() { :; } |& git push --force"),
		Entry("after ;, as before", "f() { :; }; git push --force"),
		Entry("with the function keyword", "function f { :; } && git push --force"),
		Entry("with the keyword and parens", "function f() { :; } && git push --force"),
		Entry("with a subshell body", "f() ( : ) && git push --force"),
		Entry("with an if body", "f() if true; then :; fi && git push --force"),
		Entry("with a test body", "f() [[ -n x ]] && git push --force"),
		Entry("with a redirect on the body", "f() { :; } > /dev/null && git push --force"),
		Entry("with a redirect on both", "f() { :; } 2>&1 >/dev/null || git push --force >x"),
		Entry("after a mixed list", "f() { :; } || true && git push --force"),
		Entry("after a second definition", "f() { :; } && g() { :; } && git push --force"),
		Entry("after a definition mid-list", "true && f() { :; } && git push --force"),
		Entry("in the background", "f() { :; } && git push --force &"),
		Entry("inside a group", "{ f() { :; } && git push --force; }"),
		Entry("inside a subshell", "( f() { :; } && git push --force )"),
		Entry("inside a command substitution", "echo $(f() { :; } && git push --force)"),
		Entry("inside bash -c", "bash -c 'f() { :; } && git push --force'"),
		Entry("inside eval", "eval 'f() { :; } && git push --force'"),
		Entry("inside a called function", "g() { f() { :; } && git push --force; }; g"),
		Entry("inside a loop", "for i in 1; do f() { :; } && git push --force; done"),
		Entry("after a zsh nested definition", "f() g() { :; } && git push --force"),
		Entry("after a zsh nested definition in a pipe", "f() g() { :; } | git push --force"),
		Entry("after three nested definitions", "f() g() h() { :; } && git push --force"),
		Entry("after a nested keyword definition", "f() function g { :; } && git push --force"),
		Entry("after a nested simple body", "f() g() echo b && git push --force"),
		Entry("inside zsh -c", "zsh -c 'f() g() { :; } && git push --force'"),
	)

	It("keeps a nested definition as the outer body", func() {
		result := parse("f() g() { git status; } && git push --force; f")

		Expect(gitLines(result)).To(Equal([]string{"push --force"}))
	})

	It("blocks a startup file chained after a definition", func() {
		result := parse("f() { :; } && BASH_ENV=$(mktemp) bash -c true")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(ContainElement(
			HaveField("Cause", parser.OpacityStartupFile),
		))
	})

	It("does not walk the list of a function that is never called", func() {
		result := parse("g() { f() { :; } && git push --force; }")

		Expect(gitLines(result)).To(BeEmpty())
	})

	It("keeps only the first command as the body for later calls", func() {
		result := parse("f() { git status; } && git push --force; f")

		Expect(gitLines(result)).To(Equal([]string{"push --force", "status"}))
	})

	It("keeps the body redirect for later calls", func() {
		result := parse("f() { git status; } > /dev/null && :; f; f")

		Expect(gitLines(result)).To(Equal([]string{"status", "status"}))
	})

	It("follows a call made later in the same list", func() {
		result := parse("f() { git push --force; } && f")

		Expect(gitLines(result)).To(ConsistOf("push --force"))
	})

	DescribeTable("fails closed on a body no shell accepts",
		func(command, name string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityFunctionChain),
				HaveField("Operation", name),
			)), command)
		},
		Entry("a negated body", "f() ! { :; } && git push --force", "f"),
		Entry("a negated body in a pipe", "f() ! { :; } | git push --force", "f"),
		Entry("a negated body alone", "f() ! { :; }", "f"),
	)
})
