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

	toAny := func(names []string) []any {
		out := make([]any, 0, len(names))
		for _, name := range names {
			out = append(out, name)
		}

		return out
	}

	DescribeTable(
		"fails closed on a word it cannot resolve",
		func(command string, want parser.Opacity) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", want.Cause),
				HaveField("Operation", want.Operation),
				HaveField("Detail", want.Detail),
				HaveField("Origin", HaveExactElements(toAny(want.Origin)...)),
			)), command)
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
		Entry("assigned, then set by printf -v", `x=status; printf -v x commit; git $x -m bad`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then set with attached -v", `x=status; printf -vx commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then a loop variable", `x=status; for x in commit; do :; done; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then set in a function", `x=status; f(){ x=commit; }; f; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then set by eval", `x=status; eval x=commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then read", `x=status; IFS= read -r -p p x <<< commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then read into an array", `x=status; read -a x <<< commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("read with no name", `REPLY=status; read <<< commit; git $REPLY`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then readarray", `x=status; readarray -t x <<< commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned, then sourced", `x=status; . <(echo x=commit); git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("environment, then printf -v", `printf -v SUB commit; git $SUB`,
			opacity("git", parser.DetailWordVariable)),
		Entry("gh action set by printf -v", `a=pr; b=view; printf -v b create; gh $a $b`,
			opacity("gh", parser.DetailWordVariable)),
		Entry("unknown subcommand under docker run", `docker run img git $X -m bad`,
			opacity("git", parser.DetailWordVariable, "docker")),
		Entry("substituted subcommand under docker run", `docker run img git $(echo commit)`,
			opacity("git", parser.DetailWordOutput, "docker")),
		Entry("git-name from a variable under a runner", `mise exec -- git-$X`,
			opacity("git", parser.DetailWordVariable, "mise", "exec")),
		Entry("gh command under a runner", `mise exec -- gh $X`,
			opacity("gh", parser.DetailWordVariable, "mise", "exec")),
		Entry("assigned in a subshell", `X=push; (X=status); git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned as a prefix", `X=push; X=status true; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned in a pipeline", `X=push; X=status | true; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned after &&", `X=push; false && X=status; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned in an if", `X=push; if false; then X=status; fi; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned in the background", `X=push; X=status & git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned by a trap", `X=status; trap 'X=push' DEBUG; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned through a name reference", `declare -n X=Y; Y=push; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("assigned later in a loop", `X=status; while :; do git $X; X=push; done`,
			opacity("git", parser.DetailWordVariable)),
		Entry("read later in a loop", `X=status; while :; do git $X; read X; done`,
			opacity("git", parser.DetailWordVariable)),
		Entry(
			"getopts",
			`X=status; getopts a X; git $X`,
			opacity("git", parser.DetailWordVariable),
		),
		Entry("partly substituted subcommand", `git cherry"$(echo -pick)" abc`,
			opacity("git", parser.DetailWordOutput)),
		Entry("partly substituted with backticks", "git cherry`echo -pick` abc",
			opacity("git", parser.DetailWordOutput)),
		Entry("read with an empty delimiter", `x=status; read -d '' x <<< commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("printf -v to a dynamic name", `x=status; v=x; printf -v $v commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("printf -v to an element", `x=status; printf -v x[0] commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("read to a dynamic name", `x=status; v=x; read $v <<< commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("declare of a dynamic name", `x=status; v=x; declare $v=commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("export of a dynamic name", `x=status; v=x; export $v=commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("declare of a quoted assignment", `x=status; v='x=commit'; declare "$v"; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("declare of command output", `x=status; declare -- "$(echo x=commit)"; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("lowercase attribute", `x=status; declare -l x=COMMIT; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("lowercase attribute set first", `declare -l y; y=COMMIT; git $y`,
			opacity("git", parser.DetailWordVariable)),
		Entry("reference to a literal name", `X=status; declare -n R=X; R=push; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("export through builtin", `X=status; builtin export X=push; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("declare through command", `X=status; command declare X=push; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("reference through builtin", `X=status; builtin declare -n R=X; R=push; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("readonly of a dynamic name through builtin",
			`X=status; v=X; builtin readonly $v=push; git $X`,
			opacity("git", parser.DetailWordVariable)),
		Entry("declaration from a variable", `x=status; d=declare; $d x=commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("declaration from command output", `x=status; $(echo declare) x=commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("sourced dynamic text", `x=status; v=x; source /dev/stdin <<< "$v=commit"; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("mapfile callback", `x=status; mapfile -C 'x=commit;:' -c 1 a <<< z; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("attached mapfile callback", `x=status; mapfile -c1 -C'x=commit;:' a <<< z; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("combined mapfile callback", `x=status; mapfile -tC 'x=commit;:' a <<< z; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("sourced descriptor", `x=status; . /dev/fd/0 <<< x=commit; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("sourced redirected stdin", `x=status; echo x=commit > g; . /dev/stdin < g; git $x`,
			opacity("git", parser.DetailWordVariable)),
		Entry("sourced process substitution", `x=status; v=x; source <(echo "$v=commit"); git $x`,
			opacity("git", parser.DetailWordVariable)),
	)

	DescribeTable("keeps a flag with a substituted value from taking the next argument",
		func(command string, args ...string) {
			result := parse(command)

			Expect(result.GitOperations).To(HaveLen(1))
			Expect(result.GitOperations[0].Args).To(Equal(args))
		},
		Entry("git -C", `git -C$(pwd) push --force o main`,
			"-C", "", "push", "--force", "o", "main"),
		Entry("git -c", `git -c$(echo a=b) push`, "-c", "", "push"),
		Entry("commit -m", `git commit -sS -m$(echo x) --no-verify`,
			"commit", "-sS", "-m", "", "--no-verify"),
	)

	It("hides a value substituted into an eval line", func() {
		secret := fakeResolver{env: map[string]string{"SECRET": "hunterpassword"}}
		result, err := parser.NewBashParserWithResolver(secret).Parse(`eval "git $SECRET"`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Opacities).To(ConsistOf(HaveField("Operation", "git <hidden>")))
	})

	It("checks a launcher program from command output as git when it runs a git command", func() {
		for _, command := range []string{
			`sudo $(echo git) push --force`,
			`command $(echo git) push --force`,
			`xargs $(echo git) push --force`,
		} {
			result := parse(command)
			Expect(result.GitOperations).NotTo(BeEmpty(), command)
			Expect(result.GitOperations[0].Args).To(Equal([]string{"push", "--force"}), command)
		}
	})

	It("hides a subcommand that came from a variable's value", func() {
		secret := fakeResolver{env: map[string]string{"SECRET": "hunterpassword"}}
		result, err := parser.NewBashParserWithResolver(secret).Parse(`git $SECRET`)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Opacities).To(ConsistOf(parser.Opacity{
			Cause:     parser.OpacityUnresolvedProgram,
			Operation: "git <hidden>",
		}))
	})

	It("does not take find's {} for a brace expansion", func() {
		Expect(parse(`find . -exec git add {} \;`).Truncated).To(BeFalse())
	})

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
		Entry("substituted global option value", `git -C "$(pwd)" status`, "-C", "", "status"),
		Entry("substituted global option before push", `git -C "$(pwd)" push -f o main`,
			"-C", "", "push", "-f", "o", "main"),
		Entry("substituted -c value", `git -c "$(echo k=v)" push`, "-c", "", "push"),
		Entry("substituted launcher operand", `timeout $(echo 5) git status`, "status"),
		Entry("substituted env words", `env $(cat .env) git commit -m x`, "commit", "-m", "x"),
		Entry("substituted message", `git commit -m "$(date)" --no-verify`,
			"commit", "-m", "", "--no-verify"),
		Entry("partly substituted message", `git commit -m "at $(date)"`, "commit", "-m", "at "),
		Entry("typed placeholder text", `git commit -m '$(...)' --no-verify`,
			"commit", "-m", "$(...)", "--no-verify"),
		Entry("eval of a literal line", `eval "git status"`, "status"),
		Entry("eval of an assigned line", `X="git commit -m 'a b'"; eval "$X"`,
			"commit", "-m", "a b"),
		Entry("eval of an environment line", `eval "$LINE"`, "commit", "-m", "x"),
		Entry("eval of a single-quoted line", `eval 'git $SUB'`, "status"),
		Entry("reassigned literally after a loop",
			`for x in a; do :; done; x=status; git $x`, "status"),
		Entry("a local in a function", `f(){ local y=1; }; x=status; f; git $x`, "status"),
		Entry("printf -v of another variable", `x=status; printf -v y commit; git $x`, "status"),
		Entry("printf without -v", `x=status; printf '%s' x; git $x`, "status"),
		Entry("read of another variable with a prompt", `read -t 5 -p "Name: " y; x=status; git $x`,
			"status"),
		Entry("export attribute", `declare -x x=status; git $x`, "status"),
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
