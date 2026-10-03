package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Unresolved eval and command words", func() {
	resolver := fakeResolver{
		env: map[string]string{"SUB": "status", "LINE": "git commit -m x"},
		programs: map[string]parser.Program{
			"git-zz": parser.ProgramMissing,
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	opacity := func(operation, detail string, origin ...string) parser.Opacity {
		return parser.Opacity{
			Cause:     parser.OpacityUnresolvedWord,
			Operation: operation,
			Origin:    origin,
			Detail:    detail,
		}
	}

	DescribeTable("fails closed on a word it cannot resolve",
		func(command string, want parser.Opacity) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(want), command)
		},
		Entry("git subcommand from an unknown variable",
			`git $SECRET`, opacity("git", parser.DetailWordVariable)),
		Entry("quoted, after a global option",
			`git --no-pager "$SECRET" -m x`, opacity("git", parser.DetailWordVariable)),
		Entry("default value", `git "${X:-status}"`, opacity("git", parser.DetailWordVariable)),
		Entry("indirect reference", `git ${!x}`, opacity("git", parser.DetailWordVariable)),
		Entry("array element", `git "${arr[0]}"`, opacity("git", parser.DetailWordVariable)),
		Entry("positional parameter", `git "$1"`, opacity("git", parser.DetailWordVariable)),
		Entry("variable holding command output",
			`X=$(echo commit); git $X`, opacity("git", parser.DetailWordVariable)),
		Entry("variable reassigned from command output",
			`X=status; X=$(echo commit); git $X`, opacity("git", parser.DetailWordVariable)),
		Entry("loop variable", `for s in status log; do git $s; done`,
			opacity("git", parser.DetailWordVariable)),
		Entry("command substitution", `git $(echo commit) -m x`,
			opacity("git", parser.DetailWordOutput)),
		Entry("quoted command substitution", `git "$(echo commit)"`,
			opacity("git", parser.DetailWordOutput)),
		Entry("global option from command output", `git $(echo -c) alias.x=commit x`,
			opacity("git", parser.DetailWordOutput)),
		Entry("brace expansion", `git {commit,-m,x}`, opacity("git", parser.DetailWordOutput)),
		Entry("glob", `git c?mmit -m x`, opacity("git", parser.DetailWordOutput)),
		Entry("under a launcher", `command git $x`,
			opacity("git", parser.DetailWordVariable, "command")),
		Entry("substitution under sudo", `sudo git $(echo commit) -m x`,
			opacity("git", parser.DetailWordOutput, "sudo")),
		Entry("substitution under a runner", `mise exec -- git $(echo commit)`,
			opacity("git", parser.DetailWordOutput, "mise", "exec")),
		Entry("substitution passed to a function", `f() { git "$1"; }; f $(echo commit)`,
			opacity("git", parser.DetailWordOutput, "f")),
		Entry("substitution through an alias", `alias g=git; g $(echo push)`,
			opacity("git", parser.DetailWordOutput, "g")),
		Entry("git-name from a variable", `git-$X`, opacity("git", parser.DetailWordVariable)),
		Entry("shell script argument", `bash -c 'git $0' commit`,
			opacity("git", parser.DetailWordVariable, "bash")),
		Entry("gh command", `gh $SUB2`, opacity("gh", parser.DetailWordVariable)),
		Entry("gh action", `gh pr $A`, opacity("gh", parser.DetailWordVariable)),
		Entry("gh action after a resolved command", `X=issue; gh $X $Y`,
			opacity("gh", parser.DetailWordVariable)),
		Entry("gh command from output", `gh $(echo pr) create`,
			opacity("gh", parser.DetailWordOutput)),
		Entry("eval of an unknown variable", `eval "$UNSET"`,
			opacity("eval", parser.DetailWordVariable)),
		Entry("eval of command output", `eval "$(echo git commit -m x)"`,
			opacity("eval", parser.DetailWordOutput)),
		Entry("eval of arithmetic", `eval "echo $((1+2))"`,
			opacity("eval", parser.DetailWordOutput)),
		Entry("eval of a variable holding output", `X=$(cat f); eval "$X"`,
			opacity("eval", parser.DetailWordVariable)),
		Entry("eval under a launcher", `builtin eval "$UNSET"`,
			opacity("eval", parser.DetailWordVariable, "builtin")),
	)

	It("names no variable or value in the opacity", func() {
		result := parse(`git $ghp_SECRET1234567890; eval "x $TOKEN_abc123"`)

		for _, o := range result.Opacities {
			Expect(o.Operation).To(BeElementOf("git", "eval"))
			Expect(o.Origin).To(BeEmpty())
			Expect(o.Detail).NotTo(ContainSubstring("SECRET"))
			Expect(o.Detail).NotTo(ContainSubstring("TOKEN"))
		}
	})

	DescribeTable("checks a word it can resolve as the value it holds",
		func(command string, args ...string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "%q: %+v", command, result.Opacities)
			Expect(result.GitOperations).NotTo(BeEmpty())
			Expect(result.GitOperations[len(result.GitOperations)-1].Args).To(Equal(args))
		},
		Entry("same-line assignment", `X=status; git $X`, "status"),
		Entry("exported assignment", `export X=status; git $X`, "status"),
		Entry("chained assignment", `Y=commit; X=$Y; git $X -m bad`, "commit", "-m", "bad"),
		Entry("assignment that splits", `X="commit -m bad"; git $X`, "commit", "-m", "bad"),
		Entry("environment", `git "$SUB"`, "status"),
		Entry("global option kept", `X=status; git -C /tmp $X`, "-C", "/tmp", "status"),
		Entry("empty assignment", `E=""; git $E status`, "status"),
		Entry("assignment to a flag", `X=--no-pager; git $X status`, "--no-pager", "status"),
		Entry("variable in a later argument", `git status "$FILE"`, "status", "${FILE}"),
		Entry("substituted global option value", `git -C "$(pwd)" status`, "-C", "status"),
		Entry("substituted launcher operand", `timeout $(echo 5) git status`, "status"),
		Entry("substituted env words", `env $(cat .env) git commit -m x`, "commit", "-m", "x"),
		Entry("substituted message", `git commit -m "$(date)"`, "commit", "-m"),
		Entry("eval of a literal line", `eval "git status"`, "status"),
		Entry("eval of an assigned line", `X="git commit -m 'a b'"; eval "$X"`,
			"commit", "-m", "a b"),
		Entry("eval of an environment line", `eval "$LINE"`, "commit", "-m", "x"),
		Entry("eval of a single-quoted line", `eval 'git $SUB'`, "status"),
	)

	DescribeTable("leaves gh words it does not dispatch on alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "%q: %+v", command, result.Opacities)
		},
		Entry("api endpoint", `gh api repos/$O/$R`),
		Entry("pr number", `gh pr view $N`),
		Entry("repository flag", `gh -R $REPO pr create`),
		Entry("assigned action", `X=pr; Y=create; gh $X $Y`),
		Entry("eval of echo", `eval "echo hi"`),
		Entry("literal git commit", `git commit -m "$(cat <<'EOF'
msg
EOF
)"`),
	)

	DescribeTable("treats a subcommand git would not accept as an alias as unknown",
		func(command string, want parser.Opacity) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ConsistOf(want))
		},
		Entry("padded far from a builtin", `git 'zz ' x`,
			parser.Opacity{Cause: parser.OpacityUnresolvedProgram, Operation: "git <hidden>"}),
		Entry("ANSI-C newline", `git $'zz\n' x`,
			parser.Opacity{Cause: parser.OpacityUnresolvedProgram, Operation: "git <hidden>"}),
	)

	DescribeTable("corrects a padded subcommand the way git autocorrect does",
		func(command string, args ...string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse())
			Expect(result.GitOperations).To(HaveLen(1))
			Expect(result.GitOperations[0].Args).To(Equal(args))
		},
		Entry("trailing space",
			`git -c help.autocorrect=immediate 'push ' --force origin main`,
			"-c", "help.autocorrect=immediate", "push", "--force", "origin", "main"),
		Entry("trailing newline", `git $'push\n' --force origin main`,
			"push", "--force", "origin", "main"),
	)
})
