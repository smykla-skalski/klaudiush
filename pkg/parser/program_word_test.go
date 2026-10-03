package parser_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Unresolved program words", func() {
	resolver := fakeResolver{
		env: map[string]string{
			"GIT":    "git",
			"EDITOR": "vim",
			"SHELL":  "/bin/bash",
			"HOME":   "/home/u",
			"G":      "git",
			"SECRET": "hunter2token",
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	program := func(detail string, origin ...string) parser.Opacity {
		return parser.Opacity{
			Cause:     parser.OpacityUnresolvedWord,
			Operation: parser.ProgramWordOperation,
			Origin:    origin,
			Detail:    detail,
		}
	}

	hasCommand := func(result *parser.ParseResult, name string, args ...string) bool {
		for _, cmd := range result.Commands {
			if cmd.Name == name && fmt.Sprint(cmd.Args) == fmt.Sprint(args) {
				return true
			}
		}

		return false
	}

	DescribeTable("fails closed on a program word it cannot resolve",
		func(command string, want parser.Opacity) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ConsistOf(want), command)
		},
		Entry("unknown variable", `$UNSET origin main`, program(parser.DetailWordVariable)),
		Entry("command output", `$(echo git) push`, program(parser.DetailWordOutput)),
		Entry("quoted command output", `"$(cat f)" status`, program(parser.DetailWordOutput)),
		Entry("command without -v", `$(command git) push`, program(parser.DetailWordOutput)),
		Entry("printf output", `$(printf git) push`, program(parser.DetailWordOutput)),
		Entry("lookup of an option", `$(which -a) push`, program(parser.DetailWordOutput)),
		Entry("default of a variable holding output", `X=$(cat f); "${X:-git}" push`,
			program(parser.DetailWordVariable)),
		Entry("default with an expansion", `${X:-$Y} push`, program(parser.DetailWordVariable)),
		Entry("alternate value", `${G:+git} push`, program(parser.DetailWordVariable)),
		Entry("assigned by an expansion", `X=""; : ${X:=git}; $X push`,
			program(parser.DetailWordVariable)),
		Entry("assigned by an expansion in a chain", `X="" && : ${X=git} && $X push`,
			program(parser.DetailWordVariable)),
		Entry("array element assigned", `a=(git); a[1]=push; "${a[@]}"`,
			program(parser.DetailWordVariable)),
		Entry("array with indexes", `a=([1]=push [0]=git); "${a[@]}"`,
			program(parser.DetailWordVariable)),
		Entry("arithmetic", `f1() { git push; }; f$((1))`, program(parser.DetailWordOutput)),
		Entry("lookup of two names", `$(command -v git other) ci`,
			program(parser.DetailWordOutput)),
		Entry("type without -p", `$(type git) status`, program(parser.DetailWordOutput)),
		Entry("found path under find", `find /usr/bin -name git -exec {} ci \;`,
			program(parser.DetailWordOutput, "find")),
		Entry("IFS appended to", `IFS+=,; X=git,push; $X origin main`,
			program(parser.DetailWordVariable)),
		Entry("IFS appended to by export", `X=git,push; export IFS+=,; $X origin main`,
			program(parser.DetailWordVariable)),
		Entry("array redeclared", `a=(ls); declare -a a=(git push); "${a[@]}"`,
			program(parser.DetailWordVariable)),
		Entry("array exported", `a=(ls); export a=(git push); "${a[@]}"`,
			program(parser.DetailWordVariable)),
		Entry("scalar made an array", `a=ls; declare -a a=$v; $a push`,
			program(parser.DetailWordVariable)),
		Entry("first element of an array", `a=(git status); $a commit -m x`,
			program(parser.DetailWordVariable)),
		Entry("array given a scalar", `a=(git status); a=ls; "${a[@]}"`,
			program(parser.DetailWordVariable)),
		Entry("array element from command output", `a=($(echo git) push); "${a[@]}"`,
			program(parser.DetailWordVariable)),
		Entry("quoted array element from output", `a=("$(echo git)" push); ${a[*]}`,
			program(parser.DetailWordVariable)),
		Entry("custom IFS", `IFS=,; X=git,push; $X origin main`,
			program(parser.DetailWordVariable)),
		Entry("IFS set by read", `read IFS <<< ","; X=git,push; $X`,
			program(parser.DetailWordVariable)),
		Entry("indirect reference", `${!x} push`, program(parser.DetailWordVariable)),
		Entry("array element", `${arr[0]} push`, program(parser.DetailWordVariable)),
		Entry("positional parameter", `"$1" push`, program(parser.DetailWordVariable)),
		Entry("all positional parameters", `"$@"`, program(parser.DetailWordVariable)),
		Entry("variable holding output", `X=$(echo git); $X push`,
			program(parser.DetailWordVariable)),
		Entry("loop variable", `for p in git; do $p push; done`,
			program(parser.DetailWordVariable)),
		Entry("environment variable in a loop", `for f in a; do $EDITOR $f; done`,
			program(parser.DetailWordVariable)),
		Entry("environment variable in a new shell", `bash -c '$G push'`,
			program(parser.DetailWordVariable, "bash")),
		Entry("under env", `env $X push`, program(parser.DetailWordVariable, "env")),
		Entry("under sudo", `sudo $(echo git) push`, program(parser.DetailWordOutput, "sudo")),
		Entry("under nohup", `nohup $X &`, program(parser.DetailWordVariable, "nohup")),
		Entry("under command", `command $X`, program(parser.DetailWordVariable, "command")),
		Entry("under builtin", `builtin $X`, program(parser.DetailWordVariable, "builtin")),
		Entry("under exec", `exec $X`, program(parser.DetailWordVariable, "exec")),
		Entry("under xargs", `echo a | xargs $X`, program(parser.DetailWordVariable, "xargs")),
		Entry("under find -exec", `find . -exec $X {} \;`,
			program(parser.DetailWordVariable, "find")),
		Entry("glob", `gi? push`, program(parser.DetailWordOutput)),
		Entry("glob in a path", `/usr/bin/gi* push`, program(parser.DetailWordOutput)),
		Entry("bracket glob", `/usr/bin/gi[t] push`, program(parser.DetailWordOutput)),
		Entry("extended glob", `@(git) push`, program(parser.DetailWordOutput)),
		Entry("glob in a variable's value", `X='gi?'; $X push`, program(parser.DetailWordOutput)),
		Entry("script path from a variable", `$DIR/run.sh`, program(parser.DetailWordVariable)),
		Entry("path under command output", `$(go env GOPATH)/bin/tool run`,
			program(parser.DetailWordOutput)),
		Entry("launcher from a variable", `X=sudo; $X $(echo git) push`,
			program(parser.DetailWordOutput, "<hidden>")),
	)

	DescribeTable("resolves a program word it can know",
		func(command, name string, args ...string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "%s: %+v", command, result.Opacities)
			Expect(hasCommand(result, name, args...)).To(BeTrue(), "%s: %+v", command,
				result.Commands)
		},
		Entry("variable assigned on the line", `G=git; $G status`, "git", "status"),
		Entry("environment variable", `$GIT status`, "git", "status"),
		Entry("editor", `$EDITOR file`, "vim", "file"),
		Entry("shell", `$SHELL -c 'git status'`, "git", "status"),
		Entry("function forwarding its arguments", `f() { "$@"; }; f git push`, "git", "push"),
		Entry("value split into words", `x="git commit"; $x -m y`, "git", "commit", "-m", "y"),
		Entry("empty value", `X=""; $X git push`, "git", "push"),
		Entry("bare empty assignment", `X=; $X git push`, "git", "push"),
		Entry("default of an unset variable", `"${X:-git}" push`, "git", "push"),
		Entry("default of a set variable", `${EDITOR:-vi} file`, "vim", "file"),
		Entry("default of a set empty variable", `X=; ${X-git} ${X:-ls}`, "ls"),
		Entry("default in a path", `${BIN:-/usr/bin}/git status`, "git", "status"),
		Entry("which", `$(which git) push`, "git", "push"),
		Entry("command -v", `$(command -v git) status`, "git", "status"),
		Entry("type -P", `"$(type -P git)" status`, "git", "status"),
		Entry("test builtin", `[ -f x ] && echo y`, "[", "-f", "x", "]"),
		Entry("home path", `"$HOME/bin/tool" --x`, "tool", "--x"),
		Entry("lookup of a variable", `command -v $X`, "command", "-v", "${X}"),
	)

	It("fails closed on eval of every element of an array", func() {
		result := parse(`a=(git push); eval '"${a[@]}"'`)

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).To(ConsistOf(HaveField("Operation", "eval")))
	})

	It("still checks a validated git subcommand behind an opaque program", func() {
		result := parse(`$(echo git) push --force`)

		Expect(result.Truncated).To(BeTrue())
		Expect(hasCommand(result, "git", "push", "--force")).To(BeTrue())
	})

	It("never shows a variable's name or value", func() {
		for _, command := range []string{
			`$SECRET push`, `$UNSET_SECRET_NAME push`, `sudo $SECRET_NAME push`,
			`X=hunter2token; for i in 1; do $X; done`,
		} {
			for _, o := range parse(command).Opacities {
				Expect(fmt.Sprintf("%+v", o)).NotTo(
					Or(ContainSubstring("SECRET"), ContainSubstring("hunter2")), command)
			}
		}
	})
})
