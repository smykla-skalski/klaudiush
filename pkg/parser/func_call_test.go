package parser_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Calls to a function defined on the line", func() {
	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(fakeResolver{}).Parse(command)
		Expect(err).NotTo(HaveOccurred(), command)

		return result
	}

	argsOf := func(result *parser.ParseResult, name string) [][]string {
		var found [][]string

		for _, cmd := range result.Commands {
			if cmd.Name == name {
				found = append(found, cmd.Args)
			}
		}

		return found
	}

	gitLines := func(result *parser.ParseResult) []string {
		found := argsOf(result, "git")
		lines := make([]string, 0, len(found))

		for _, args := range found {
			lines = append(lines, strings.Join(args, " "))
		}

		return lines
	}

	It("reads the reported test helper without a parse error or a commit", func() {
		result := parse(strings.Join([]string{
			`S=/tmp/s; R=$S/repo; cd $R`,
			`t(){ printf '%s' "$1" > $S/c.txt; out=$(python3 -I $S/mk.py $S/c.txt $R | ` +
				`klaudiush --hook-type PreToolUse | grep -o 'GIT0[0-9]*' | sort -u | ` +
				`tr '\n' ' '); echo "[$out] <= $2"; }`,
			`t "git commit -s -S -q -m \"fix(git): keep merge body\"" "same"`,
			`t "git add a.go && git commit -s -S -m \"fix(git): x\"" "two"`,
		}, "\n"))

		Expect(result.Truncated).To(BeFalse())
		Expect(result.Opacities).To(BeEmpty())
		Expect(gitLines(result)).To(BeEmpty())
	})

	DescribeTable("treats text passed to the function as data",
		func(command string) {
			result := parse(command)

			Expect(gitLines(result)).To(BeEmpty(), command)
			Expect(result.Truncated).To(BeFalse(), command)
		},
		Entry("an empty body", `t(){ :; }; t "git commit -m x"`),
		Entry("on the next line", "t(){ :; }\nt \"git push --force\""),
		Entry("printed", `t(){ printf '%s\n' "$1"; }; t "git push --force"`),
		Entry("written to a file", `t(){ printf '%s' "$1" > /tmp/c; }; t "git commit -m x"`),
		Entry("in a longer string", `t(){ echo "[$1] <= $2"; }; t "git push --force" y`),
		Entry("with the function keyword", `function t { echo "$1"; }; t "git push --force"`),
		Entry("inside a group", `{ t(){ :; }; }; t "git push --force"`),
		Entry("after a definition chained with &&", `t(){ :; } && t "git push --force"`),
		Entry("from a command substitution", `t(){ :; }; x=$(t "git push --force")`),
		Entry("after a sure redefinition", `t(){ eval "$1"; }; t(){ :; }; t "git push --force"`),
		Entry("in an expansion operand inside quotes",
			`t(){ echo "${x:-$1}"; }; t '$(git push --force)'`),
	)

	DescribeTable("follows what the body runs with the arguments",
		func(command, want string) {
			Expect(gitLines(parse(command))).To(ConsistOf(want), command)
		},
		Entry("a quoted message", `t(){ git commit -m "$1"; }; t "fix: x"`, "commit -m fix: x"),
		Entry("a message in a longer string",
			`t(){ git commit -m "fix(parser): $1"; }; t "keep quotes"`,
			"commit -m fix(parser): keep quotes"),
		Entry("an unquoted argument run as a command", `t(){ $1; }; t "git push --force"`,
			"push --force"),
		Entry("an argument handed to eval", `t(){ eval "$1"; }; t "git push --force"`,
			"push --force"),
		Entry("an argument handed to bash -c", `t(){ bash -c "$1"; }; t "git push --force"`,
			"push --force"),
		Entry("all arguments", `t(){ git push "$@"; }; t origin main`, "push origin main"),
		Entry("a heredoc script", "t(){ bash <<EOF\ngit $1\nEOF\n}; t push", "push"),
	)

	It("fails closed on a heredoc script given a substitution as text", func() {
		result := parse("t(){ bash <<EOF\necho $1\nEOF\n}; t '$(git push)'")

		Expect(result.Truncated).To(BeTrue())
	})

	DescribeTable(
		"still scans the arguments when a program may run instead",
		func(command string) {
			Expect(gitLines(parse(command))).To(ContainElement("push --force"), command)
		},
		Entry("a definition behind &&", `false && t(){ :; }; t "git push --force"`),
		Entry("a definition behind ||", `true || t(){ :; }; t "git push --force"`),
		Entry("a definition in an if", `if x; then t(){ :; }; fi; t "git push --force"`),
		Entry("a definition in a loop", `for i in 1; do t(){ :; }; done; t "git push --force"`),
		Entry("a definition in a pipeline", `t(){ :; } | cat; t "git push --force"`),
		Entry("a definition in a subshell", `( t(){ :; } ); t "git push --force"`),
		Entry("a definition in the background", `t(){ :; } & t "git push --force"`),
		Entry("a definition in a command substitution",
			`x=$(t(){ :; }); t "git push --force"`),
		Entry("after unset -f", `t(){ :; }; unset -f t; t "git push --force"`),
		Entry("after unset", `t(){ :; }; unset t; t "git push --force"`),
		Entry("after an unset of an unknown name", `t(){ :; }; unset "$n"; t "git push --force"`),
		Entry("after an unset in eval", `t(){ :; }; eval 'unset -f t'; t "git push --force"`),
		Entry("after zsh unfunction", `t(){ :; }; unfunction t; t "git push --force"`),
		Entry(
			"a redefinition behind &&",
			`t(){ eval "$1"; }; false && t(){ :; }; t "git push --force"`,
		),
		Entry(
			"a redefinition in a pipeline",
			`t(){ eval "$1"; }; echo | t(){ :; }; t "git push --force"`,
		),
		Entry(
			"a redefinition in the background",
			`t(){ eval "$1"; }; t(){ :; } & t "git push --force"`,
		),
		Entry("a redefinition in a loop",
			`t(){ eval "$1"; }; for i in 1; do t(){ :; }; done; t "git push --force"`),
		Entry("a redefinition in a piped group",
			`t(){ eval "$1"; }; { t(){ :; }; } | cat; t "git push --force"`),
		Entry("a redefinition in a case",
			`t(){ eval "$1"; }; case $x in a) t(){ :; };; esac; t "git push --force"`),
		Entry("a redefinition in a conditional eval",
			`t(){ eval "$1"; }; false && eval 't(){ :; }'; t "git push --force"`),
		Entry("through command", `t(){ :; }; command t "git push --force"`),
		Entry("through env", `t(){ :; }; env t "git push --force"`),
		Entry("inside bash -c", `t(){ :; }; bash -c 't "git push --force"'`),
	)

	DescribeTable("keeps the quoting of the function body",
		func(command, name string, want []string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(argsOf(result, name)).To(ContainElement(want), command)
		},
		Entry("a parameter in a longer double-quoted string",
			`t(){ echo "[$1]"; }; t "a b"`, "echo", []string{"[a b]"}),
		Entry("a closing quote after the parameter",
			`t(){ echo "x <= $2"; }; t a b`, "echo", []string{"x <= b"}),
		Entry("a quote inside the value",
			`t(){ echo "x $1 y"; }; t "it's"`, "echo", []string{"x it's y"}),
		Entry("all arguments in a longer string",
			`t(){ echo "a $@ b"; }; t x y`, "echo", []string{"a x", "y b"}),
		Entry("a missing argument in a longer string",
			`t(){ echo "a $3 b"; }; t x`, "echo", []string{"a  b"}),
		Entry("a positional word in single quotes",
			`t(){ awk '{print $1}' "$1"; }; t f.txt`, "awk", []string{"{print $1}", "f.txt"}),
		Entry("a braced parameter in a longer string",
			`t(){ echo "${1}x"; }; t a`, "echo", []string{"ax"}),
		Entry("a parameter in a substitution inside quotes",
			`t(){ echo "$(echo $1)"; }; t "a b"`, "echo", []string{"a", "b"}),
	)
})
