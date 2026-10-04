package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Lookups in git push arguments", func() {
	resolver := fakeResolver{
		env: map[string]string{
			"PASS":   "hunter",
			"REMOTE": "https://user:hunter@host/r",
		},
		outputs: map[string]string{
			"git branch --show-current":       "main",
			"git rev-parse --abbrev-ref HEAD": "feature/x",
			"git rev-parse --show-toplevel":   "/repo",
			"pwd":                             "/repo/sub",
		},
	}

	odd := fakeResolver{outputs: map[string]string{
		"git branch --show-current":       "-main",
		"git rev-parse --abbrev-ref HEAD": "a..b",
		"pwd":                             "relative",
	}}

	parseWith := func(r fakeResolver, command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(r).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	gitArgs := func(result *parser.ParseResult) []string {
		for _, cmd := range result.Commands {
			if cmd.Name == "git" {
				return cmd.Args
			}
		}

		return nil
	}

	DescribeTable("resolves an allowed lookup",
		func(command string, want ...string) {
			result := parseWith(resolver, command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(gitArgs(result)).To(Equal(want), command)
		},
		Entry("current branch", `git push origin "$(git branch --show-current)"`,
			"push", "origin", "main"),
		Entry("unquoted current branch", `git push -u origin $(git branch --show-current)`,
			"push", "-u", "origin", "main"),
		Entry("abbreviated HEAD", `git push origin "$(git rev-parse --abbrev-ref HEAD)"`,
			"push", "origin", "feature/x"),
		Entry("-C from pwd", `git -C "$(pwd)" push origin x`,
			"-C", "/repo/sub", "push", "origin", "x"),
		Entry("-C from the top level", `git -C "$(git rev-parse --show-toplevel)" push`,
			"-C", "/repo", "push"),
		Entry("after a setup command", `cd /tmp && git push origin "$(git branch --show-current)"`,
			"push", "origin", "main"),
	)

	DescribeTable("keeps a lookup it cannot trust opaque",
		func(r fakeResolver, command, detail string) {
			result := parseWith(r, command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Operation", "git push argument"),
				HaveField("Detail", detail),
			)), command)
		},
		Entry("after a command that may switch branch", resolver,
			`git checkout -b x && git push origin "$(git branch --show-current)"`,
			parser.DetailWordOutput),
		Entry("after a cd to an unknown directory", resolver,
			`cd "$D" && git push origin "$(git branch --show-current)"`, parser.DetailWordOutput),
		Entry("inside a loop", resolver,
			`for r in o; do git push "$r" "$(git branch --show-current)"; done`,
			parser.DetailWordLoop),
		Entry("with extra arguments", resolver,
			`git push origin "$(git branch --show-current --x)"`, parser.DetailWordOutput),
		Entry("as part of a word", resolver,
			`git push origin "HEAD:$(git branch --show-current)"`, parser.DetailWordOutput),
		Entry("with no output (detached HEAD)", fakeResolver{},
			`git push origin "$(git branch --show-current)"`, parser.DetailWordOutput),
		Entry("output that looks like an option", odd,
			`git push origin "$(git branch --show-current)"`, parser.DetailWordOutput),
		Entry("output with a range", odd,
			`git push origin "$(git rev-parse --abbrev-ref HEAD)"`, parser.DetailWordOutput),
		Entry("relative directory", odd, `git -C "$(pwd)" push`, parser.DetailWordOutput),
		Entry("secret behind a default", resolver,
			`git push "https://u:${PASS:-x}@h/r" main`, parser.DetailWordSecret),
		Entry("environment URL with credentials", resolver,
			`git push "$REMOTE" main`, parser.DetailWordSecret),
	)

	It("leaves lookups in other git commands alone", func() {
		result := parseWith(resolver, `git log "$(git branch --show-current)"`)

		Expect(result.Truncated).To(BeFalse())
	})
})
