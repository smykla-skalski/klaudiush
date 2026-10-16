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
		Entry("a heredoc before a later parameter",
			"t(){ cat <<EOF | git \"$1\"\n$2\nEOF\n}; t push b", "push"),
		Entry("single-quoted code for eval", `t(){ eval 'git $1'; }; t 'push --force'`,
			"push --force"),
		Entry("a quoted parameter in eval code", `t(){ eval 'git commit -m "$1"'; }; t 'fix: x'`,
			"commit -m fix: x"),
		Entry("single-quoted code for trap", `t(){ trap '$1' EXIT; }; t 'git push'`, "push"),
	)

	It("fails closed on a heredoc script given a substitution as text", func() {
		result := parse("t(){ bash <<EOF\necho $1\nEOF\n}; t '$(git push)'")

		Expect(result.Truncated).To(BeTrue())
	})

	DescribeTable(
		"still scans the arguments of a runner when a program may run instead",
		func(command string) {
			Expect(gitLines(parse(command))).To(ContainElement("push --force"), command)
		},
		Entry("a definition behind &&", `false && tmux(){ :; }; tmux "git push --force"`),
		Entry("a definition behind ||", `true || tmux(){ :; }; tmux "git push --force"`),
		Entry("a definition in an if", `if x; then tmux(){ :; }; fi; tmux "git push --force"`),
		Entry(
			"a definition in a loop",
			`for i in 1; do tmux(){ :; }; done; tmux "git push --force"`,
		),
		Entry("a definition in a pipeline", `tmux(){ :; } | cat; tmux "git push --force"`),
		Entry("a definition in a subshell", `( tmux(){ :; } ); tmux "git push --force"`),
		Entry("a definition in the background", `tmux(){ :; } & tmux "git push --force"`),
		Entry("a definition in a command substitution",
			`x=$(tmux(){ :; }); tmux "git push --force"`),
		Entry("after unset -f", `tmux(){ :; }; unset -f tmux; tmux "git push --force"`),
		Entry("after unset", `tmux(){ :; }; unset tmux; tmux "git push --force"`),
		Entry(
			"after an unset of an unknown name",
			`tmux(){ :; }; unset "$n"; tmux "git push --force"`,
		),
		Entry(
			"after an unset in eval",
			`tmux(){ :; }; eval 'unset -f tmux'; tmux "git push --force"`,
		),
		Entry("after an unset of a brace expansion",
			`tmux(){ :; }; unset -f {tmux,x}; tmux "git push --force"`),
		Entry("after an unset of a partial brace expansion",
			`tmux(){ :; }; unset -f tmux{,}; tmux "git push --force"`),
		Entry("after zsh unfunction", `tmux(){ :; }; unfunction tmux; tmux "git push --force"`),
		Entry(
			"a redefinition behind &&",
			`tmux(){ eval "$1"; }; false && tmux(){ :; }; tmux "git push --force"`,
		),
		Entry(
			"a redefinition in a pipeline",
			`tmux(){ eval "$1"; }; echo | tmux(){ :; }; tmux "git push --force"`,
		),
		Entry(
			"a redefinition in the background",
			`tmux(){ eval "$1"; }; tmux(){ :; } & tmux "git push --force"`,
		),
		Entry("a redefinition in a loop",
			`tmux(){ eval "$1"; }; for i in 1; do tmux(){ :; }; done; tmux "git push --force"`),
		Entry("a redefinition in a piped group",
			`tmux(){ eval "$1"; }; { tmux(){ :; }; } | cat; tmux "git push --force"`),
		Entry("a redefinition in a case",
			`tmux(){ eval "$1"; }; case $x in a) tmux(){ :; };; esac; tmux "git push --force"`),
		Entry("a redefinition in a conditional eval",
			`tmux(){ eval "$1"; }; false && eval 'tmux(){ :; }'; tmux "git push --force"`),
		Entry("in a loop that unsets it",
			`tmux(){ :; }; for i in 1 2; do tmux "git push --force"; unset -f tmux; done`),
		Entry(
			"in a while condition",
			`tmux(){ :; }; while tmux "git push --force"; do unset -f tmux; done`,
		),
		Entry("after sourcing a file", `tmux(){ :; }; . ./x.sh; tmux "git push --force"`),
		Entry("after source", `tmux(){ :; }; source ./x.sh; tmux "git push --force"`),
		Entry("through command", `tmux(){ :; }; command tmux "git push --force"`),
		Entry("through env", `tmux(){ :; }; env tmux "git push --force"`),
		Entry("inside bash -c", `tmux(){ :; }; bash -c 'tmux "git push --force"'`),
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
