package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Opacity explanations", func() {
	resolver := fakeResolver{
		files: map[string]string{
			"ok.sh":     "git status\n",
			"broken.sh": "git commit -m x && (\n",
		},
		opaque: map[string]bool{"huge.sh": true},
		programs: map[string]parser.Program{
			"git-zz": parser.ProgramMissing,
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	only := func(command string) parser.Opacity {
		result := parse(command)
		Expect(result.Truncated).To(BeTrue(), "not truncated: %q", command)
		Expect(result.Opacities).To(HaveLen(1), "opacities of %q", command)

		return result.Opacities[0]
	}

	It("has no opacity for an inspectable command", func() {
		result := parse("sudo bash -c 'git status' && bash ./ok.sh")

		Expect(result.Truncated).To(BeFalse())
		Expect(result.Opacities).To(BeEmpty())
	})

	DescribeTable("names the cause, operation and origin",
		func(command string, want parser.Opacity) {
			Expect(only(command)).To(Equal(want))
		},
		Entry("nesting past the limit",
			strings.Repeat("env ", 9)+"git commit -m x",
			parser.Opacity{
				Cause:     parser.OpacityDepthLimit,
				Operation: "env",
				Origin:    strings.Fields(strings.Repeat("env ", 8)),
			},
		),
		Entry("an unreadable script under a launcher",
			"sudo bash ./huge.sh",
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "huge.sh",
				Origin:    []string{"sudo", "bash"},
				Detail:    parser.DetailScriptRead,
			},
		),
		Entry("a script path from a variable",
			`bash "$DIR/run.sh"`,
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "run.sh",
				Origin:    []string{"bash"},
				Detail:    parser.DetailScriptVariable,
			},
		),
		Entry("a relative script after an unknown cd",
			`cd "$X" && bash run.sh`,
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "run.sh",
				Origin:    []string{"bash"},
				Detail:    parser.DetailScriptDirectory,
			},
		),
		Entry("a script written with unknown content",
			`echo "$BODY" > s.sh && bash s.sh`,
			parser.Opacity{
				Cause:     parser.OpacityUnreadableScript,
				Operation: "s.sh",
				Origin:    []string{"bash"},
				Detail:    parser.DetailScriptWritten,
			},
		),
		Entry("a script that does not parse",
			"bash ./broken.sh",
			parser.Opacity{
				Cause:     parser.OpacityScriptSyntax,
				Operation: "broken.sh",
				Origin:    []string{"bash"},
			},
		),
		Entry("an inline script that does not parse",
			`env bash -c 'git commit -m x && ('`,
			parser.Opacity{
				Cause:     parser.OpacityScriptSyntax,
				Operation: "inline script",
				Origin:    []string{"env", "bash"},
			},
		),
		Entry("an unknown git subcommand under a shell",
			"bash -c 'git zz'",
			parser.Opacity{
				Cause:     parser.OpacityUnresolvedProgram,
				Operation: "git zz",
				Origin:    []string{"bash"},
			},
		),
		Entry("a function forwarding arguments it cannot follow",
			`f() { git "${@:1}"; }; f commit`,
			parser.Opacity{
				Cause:     parser.OpacityUnresolvedArgs,
				Operation: "f",
			},
		),
	)

	It("explains an exhausted budget once", func() {
		calls := func(name string) string {
			return strings.Repeat(name+"; ", 60)
		}
		command := "f() { " + calls("g") + "}; g() { " + calls("git status") + "}; f"
		result := parse(command)

		Expect(result.Truncated).To(BeTrue())

		var budget []parser.Opacity

		for _, o := range result.Opacities {
			if o.Cause == parser.OpacityWorkBudget {
				budget = append(budget, o)
			}
		}

		Expect(budget).To(HaveLen(1))
		Expect(budget[0].Operation).NotTo(BeEmpty())
	})

	DescribeTable("hides names that are not plain or short",
		func(command, operation string) {
			Expect(only(command).Operation).To(Equal(operation))
		},
		Entry("a long subcommand", "git "+strings.Repeat("z", 40), "git <hidden>"),
		Entry("a script named by a variable", `bash "$X"`, "<hidden>"),
		Entry("a long script name", `bash "$D/`+strings.Repeat("a", 40)+`.sh"`, "<hidden>"),
		Entry("a script under a directory", `bash "$D/x/run.sh"`, "run.sh"),
	)

	It("keeps a bounded number of distinct opacities", func() {
		parts := make([]string, 0, 20)
		for i := range 20 {
			parts = append(parts, "git zz"+strings.Repeat("z", i))
		}

		result := parse(strings.Join(parts, "; ") + "; git zz")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(HaveLen(8))
	})

	It("reports a repeated opacity once", func() {
		result := parse("git zz; git zz")

		Expect(result.Opacities).To(HaveLen(1))
	})
})
