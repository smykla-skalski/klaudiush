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
			"outer.sh":  "bash ./inner.sh\n",
			"inner.sh":  "git zz\n",
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
		Entry("a subcommand that looks like a password", "git hunter2pw", "git <hidden>"),
		Entry("a function that looks like a key",
			`AKIAIOSFODNN7EXAMPLE() { git "${@:1}"; }; AKIAIOSFODNN7EXAMPLE x`, "<hidden>"),
		Entry("a short mixed subcommand", "git zz9", "git zz9"),
	)

	It("hides origin names that look like secrets but keeps known programs", func() {
		o := only("ab12cd34() { python3 -c 'import os; os.system(\"bash ./huge.sh\")'; }; ab12cd34")

		Expect(o.Origin).To(Equal([]string{"<hidden>", "python3", "bash"}))
	})

	It("keeps a bounded number of distinct opacities", func() {
		result := parse(manyUnknown(20))

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(HaveLen(parser.MaxOpacities - 1))
		Expect(result.MoreOpacities).To(BeTrue())
	})

	It("keeps a slot for an exhausted budget", func() {
		calls := strings.Repeat("g; ", 60)
		result := parse(manyUnknown(20) + "; f() { " + calls + "}; g() { " +
			strings.Repeat("git status; ", 60) + "}; f")

		Expect(result.Opacities).To(HaveLen(parser.MaxOpacities))
		Expect(result.Opacities[parser.MaxOpacities-1].Cause).To(Equal(parser.OpacityWorkBudget))
	})

	DescribeTable("names the scripts and aliases on the way",
		func(command string, origin []string, operation string) {
			o := only(command)

			Expect(o.Origin).To(Equal(origin))
			Expect(o.Operation).To(Equal(operation))
		},
		Entry("a script running a script", "bash ./outer.sh",
			[]string{"bash", "outer.sh", "bash", "inner.sh"}, "git zz"),
		Entry("a git shell alias", `git -c alias.yy='!git zz' yy`,
			[]string{"git", "git alias yy"}, "git zz"),
		Entry("a git shell alias that does not parse", `git -c alias.yy='!git status && (' yy`,
			[]string{"git"}, "git alias yy"),
		Entry("a same-line alias that does not parse", "alias g='git status && ('; g",
			[]string{"g"}, "g"),
		Entry("a script on stdin", "bash /dev/stdin <<< 'git zz'",
			[]string{"bash", "stdin"}, "git zz"),
	)

	It("ignores stdin with nothing on it", func() {
		Expect(parse("bash -").Truncated).To(BeFalse())
	})

	It("reports a repeated opacity once", func() {
		result := parse("git zz; git zz")

		Expect(result.Opacities).To(HaveLen(1))
	})
})

// manyUnknown runs count distinct unknown git subcommands.
func manyUnknown(count int) string {
	parts := make([]string, 0, count)
	for i := range count {
		parts = append(parts, "git zz"+strings.Repeat("z", i))
	}

	return strings.Join(parts, "; ")
}
