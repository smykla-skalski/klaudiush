package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Dynamic git arguments", func() {
	resolver := fakeResolver{
		env: map[string]string{
			"ENV_BRANCH": "feature",
			"GH_TOKEN":   "ghp1234abcd5678",
			"EMPTY":      "",
			"PASS":       "hunter",
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	gitArgs := func(result *parser.ParseResult) [][]string {
		var args [][]string

		for _, cmd := range result.Commands {
			if cmd.Name == "git" {
				args = append(args, cmd.Args)
			}
		}

		return args
	}

	DescribeTable(
		"blocks an argument a validator reads but cannot see",
		func(command, operation, detail string) {
			result := parse(command)

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityUnresolvedWord),
				HaveField("Operation", operation),
				HaveField("Detail", detail),
			)), command)
		},
		Entry("push target from command output",
			`git push origin $(echo main)`, "git push argument", parser.DetailWordOutput),
		Entry("quoted push target from command output",
			`git push origin "$(echo main)"`, "git push argument", parser.DetailWordOutput),
		Entry("refspec built on command output",
			`git push origin "HEAD:$(echo main)"`, "git push argument", parser.DetailWordOutput),
		Entry("arithmetic", `git push origin $((1))`, "git push argument", parser.DetailWordOutput),
		Entry("unknown remote", `git push $R main`, "git push argument", parser.DetailWordVariable),
		Entry("quoted unknown remote",
			`git push "$R" main`, "git push argument", parser.DetailWordVariable),
		Entry("variable holding command output",
			`B=$(echo main); git push origin "$B"`, "git push argument", parser.DetailWordVariable),
		Entry("variable glob", `B='mai[n]'; git push origin $B`,
			"git push argument", parser.DetailWordOutput),
		Entry("literal glob", `git push origin mai?`, "git push argument", parser.DetailWordOutput),
		Entry("brace expansion", `git push origin {main,dev}`,
			"git push argument", parser.DetailWordOutput),
		Entry(
			"splitting",
			`B='origin main'; git push $B`,
			"git push argument",
			parser.DetailWordSplit,
		),
		Entry("IFS change", `IFS=,; B=origin,main; git push $B`,
			"git push argument", parser.DetailWordUntrusted),
		Entry("force flag from a variable", `git push $F origin feature`,
			"git push argument", parser.DetailWordVariable),
		Entry("loop variable", `for b in main; do git push origin "$b"; done`,
			"git push argument", parser.DetailWordLoop),
		Entry("new shell", `B=main; bash -c 'git push origin "$B"'`,
			"git push argument", parser.DetailWordNewShell),
		Entry("positional parameter", `git push origin "$1"`, "git push argument",
			parser.DetailWordVariable),
		Entry("indirect reference", `git push origin "${!x}"`, "git push argument",
			parser.DetailWordVariable),
		Entry("array as scalar", `a=(main dev); git push origin "$a"`, "git push argument",
			parser.DetailWordVariable),
		Entry("global option from command output", `git -C "$(pwd)" push origin main`,
			"git push argument", parser.DetailWordOutput),
		Entry("--repo from a variable", `git push --repo "$R" main`,
			"git push argument", parser.DetailWordVariable),
		Entry("unquoted push option from command output",
			`git push -o $(echo x) origin main`, "git push argument", parser.DetailWordOutput),
		Entry("secret from the environment",
			`git push "https://x:$GH_TOKEN@github.com/o/r" main`,
			"git push argument", parser.DetailWordSecret),
		Entry("under a launcher", `sudo git push origin $(echo main)`,
			"git push argument", parser.DetailWordOutput),
		Entry("through a function", `f() { git push origin "$1"; }; f "$(echo main)"`,
			"git push argument", parser.DetailWordOutput),
		Entry("through a git alias", `git -c alias.p=push p origin $(echo main)`,
			"git push argument", parser.DetailWordOutput),
		Entry("commit option from a variable", `git commit $F -m msg`,
			"git commit argument", parser.DetailWordVariable),
		Entry("commit option from command output", `git commit $(echo --no-verify) -m msg`,
			"git commit argument", parser.DetailWordOutput),
		Entry("quoted commit option from command output",
			`git commit "$(echo --no-verify)" -m msg`, "git commit argument",
			parser.DetailWordOutput),
		Entry("commit option cluster from a variable", `git commit -a$X -m msg`,
			"git commit argument", parser.DetailWordVariable),
		Entry("long option name from a variable", `git commit --no-$X -m msg`,
			"git commit argument", parser.DetailWordVariable),
		Entry("commit glob that can match an option", `git commit -m msg *`,
			"git commit argument", parser.DetailWordOutput),
		Entry("unquoted message from command output",
			`git commit -m $(cat msg)`, "git commit argument", parser.DetailWordOutput),
		Entry("unquoted message from an unknown variable",
			`git commit -m $MSG`, "git commit argument", parser.DetailWordVariable),
		Entry(
			"option after a repeated message flag",
			`git commit -m -m "$(echo --no-verify)"`,
			"git commit argument",
			parser.DetailWordOutput,
		),
		Entry("variable after a repeated message flag",
			`git commit -m -m "$NV"`, "git commit argument", parser.DetailWordVariable),
		Entry(
			"option after a reused commit named -m",
			`git commit -C -m "$(echo --no-verify)"`,
			"git commit argument",
			parser.DetailWordOutput,
		),
		Entry("option after a cluster whose message is attached",
			`git commit -mF "$(echo --no-verify)"`, "git commit argument", parser.DetailWordOutput),
		Entry(
			"option after a cluster whose key id is attached",
			`git commit -sSm "$(echo --no-verify)"`,
			"git commit argument",
			parser.DetailWordOutput,
		),
		Entry("push target after a repeated push option",
			`git push origin -o -o "$(echo main)"`, "git push argument", parser.DetailWordOutput),
		Entry("push flag cluster from command output",
			`git push -$(echo f)o x origin main`, "git push argument", parser.DetailWordOutput),
		Entry(
			"quoted array of options",
			`a=(--no-verify -m x); git commit "${a[@]}"`,
			"git commit argument",
			parser.DetailWordSplit,
		),
		Entry(
			"quoted array as a message",
			`a=(x --no-verify); git commit -m "${a[@]}"`,
			"git commit argument",
			parser.DetailWordSplit,
		),
		Entry("quoted positional parameters as a message",
			`git commit -m "$@"`, "git commit argument", parser.DetailWordVariable),
		Entry(
			"quoted array as a push option",
			`a=(x main); git push origin -o "${a[@]}"`,
			"git push argument",
			parser.DetailWordSplit,
		),
		Entry("function forwarding command output to commit",
			`f() { git commit "$@"; }; f -m x "$(echo --no-verify)"`,
			"git commit argument", parser.DetailWordOutput),
		Entry("alias forwarding command output to commit",
			`alias c='git commit'; c -m x $(echo --no-verify)`,
			"git commit argument", parser.DetailWordOutput),
		Entry(
			"function forwarding a brace expansion",
			`f() { git push origin "$@"; }; f {main,dev}`,
			"git push argument",
			parser.DetailWordOutput,
		),
		Entry("alias forwarding a glob", `alias p='git push origin'; p mai?`,
			"git push argument", parser.DetailWordOutput),
		Entry(
			"function forwarding a split value",
			`B='origin main'; f() { git push "$@"; }; f $B`,
			"git push argument",
			parser.DetailWordSplit,
		),
		Entry(
			"function forwarding an unknown variable",
			`f() { git push origin "$@"; }; f "$X"`,
			"git push argument",
			parser.DetailWordVariable,
		),
		Entry("tilde of another user", `git push origin ~x`, "git push argument",
			parser.DetailWordVariable),
		Entry("environment variable in URL credentials",
			`git push "https://user:$PASS@github.com/o/r" main`,
			"git push argument", parser.DetailWordSecret),
		Entry("message splitting into an option",
			`M='x --no-verify'; git commit -m $M`, "git commit argument", parser.DetailWordSplit),
	)

	DescribeTable("checks what it can resolve",
		func(command string, want []string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(gitArgs(result)).To(ContainElement(Equal(want)), command)
		},
		Entry("assigned branch", `B=feature; git push origin $B`,
			[]string{"push", "origin", "feature"}),
		Entry("assigned blocked branch", `B=main; git push origin "$B"`,
			[]string{"push", "origin", "main"}),
		Entry("assigned refspec", `B=main; git push origin "HEAD:refs/heads/${B}"`,
			[]string{"push", "origin", "HEAD:refs/heads/main"}),
		Entry("environment branch", `git push origin "$ENV_BRANCH"`,
			[]string{"push", "origin", "feature"}),
		Entry("default value", `git push origin "${UNSET_X:-main}"`,
			[]string{"push", "origin", "main"}),
		Entry("unquoted empty variable leaves no word", `git push $EMPTY origin main`,
			[]string{"push", "origin", "main"}),
		Entry("assigned force flag", `F=--force; git push $F origin main`,
			[]string{"push", "--force", "origin", "main"}),
		Entry("quoted glob characters", `git push origin 'refs/heads/*:refs/heads/*'`,
			[]string{"push", "origin", "refs/heads/*:refs/heads/*"}),
		Entry("escaped glob character", `git push origin mai\?`,
			[]string{"push", "origin", "mai?"}),
		Entry("quoted value with a blank", `B='a b'; git push origin "$B"`,
			[]string{"push", "origin", "a b"}),
		Entry("assigned commit option", `F=--no-verify; git commit $F -m msg`,
			[]string{"commit", "--no-verify", "-m", "msg"}),
		Entry("assigned remote under a launcher", `R=origin; env git push "$R" main`,
			[]string{"push", "origin", "main"}),
		Entry("assigned branch through a function",
			`B=main; f() { git push origin "$@"; }; f "$B"`, []string{"push", "origin", "main"}),
		Entry("assigned branch through an alias",
			`B=main; alias p='git push origin'; p $B`, []string{"push", "origin", "main"}),
		Entry("assigned option through a function",
			`B=--no-verify; f() { git commit "$@"; }; f -m x $B`,
			[]string{"commit", "-m", "x", "--no-verify"}),
		Entry("tilde from an assigned home", `HOME=main; git push origin ~`,
			[]string{"push", "origin", "main"}),
		Entry("quoted empty part keeps the word", `git push $EMPTY"" origin main`,
			[]string{"push", "", "origin", "main"}),
		Entry("single array element", `a=(main); git push origin "${a[@]}"`,
			[]string{"push", "origin", "main"}),
		Entry("environment variable in a URL path", `git push "https://h/$ENV_BRANCH" main`,
			[]string{"push", "https://h/feature", "main"}),
		Entry("assigned variable in URL credentials", `P=x; git push "https://u:$P@h/r" main`,
			[]string{"push", "https://u:x@h/r", "main"}),
	)

	DescribeTable(
		"leaves values no validator reads alone",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.Opacities).To(BeEmpty(), command)
		},
		Entry("message from command output", `git commit -m "$(cat <<'EOF'
fix: x
EOF
)"`),
		Entry("message from a variable", `git commit -sS -m "$MSG"`),
		Entry("attached message", `git commit -m"$(echo x)"`),
		Entry("attached long message", `git commit --message="$(echo x)"`),
		Entry("combined flags with message", `git commit -am "$(echo x)"`),
		Entry("author", `git commit --author "$(git config user.name) <x@y>" -m x`),
		Entry("reuse message", `git commit -C "$(git rev-parse HEAD)"`),
		Entry("fixup", `git commit --fixup "$(git rev-parse HEAD)"`),
		Entry("path after --", `git commit -m x -- "$f" $(echo a)`),
		Entry("path with literal start", `git commit -m x "src/$f"`),
		Entry("glob path", `git commit -m x src/*.go`),
		Entry("commit global option", `git -C "$(git rev-parse --show-toplevel)" commit -m x`),
		Entry("log range", `git log $(git merge-base HEAD main)..HEAD`),
		Entry("diff", `git diff "$BASE"`),
		Entry("add", `git add $(git ls-files -m)`),
		Entry("fetch", `git fetch origin "$B"`),
		Entry("checkout new branch", `git checkout -b "feat/$(date +%s)"`),
		Entry("quoted literal push option", `git push -o 'a b' origin main`),
		Entry(
			"quoted push option from command output",
			`git push -o "$(echo ci.skip)" origin main`,
		),
		Entry(
			"attached push option",
			`git push -o"$(echo x)" --push-option="$(echo y)" origin main`,
		),
		Entry("message attached after a boolean cluster", `git commit -sSam"$(echo x)"`),
		Entry("forwarded message", `f() { git commit -m "$1"; }; f "msg"`),
	)

	It("keeps the variable name, never a secret value, in the opacity", func() {
		result := parse(`git push "https://x:$GH_TOKEN@github.com/o/r" main`)

		Expect(result.Commands).NotTo(ContainElement(HaveField("Args",
			ContainElement(ContainSubstring("ghp1234abcd5678")))))
	})

	It("marks an argument operation", func() {
		Expect(parser.IsArgumentOperation(parser.ArgumentOperation("push"))).To(BeTrue())
		Expect(parser.IsArgumentOperation("git")).To(BeFalse())
		Expect(parser.IsArgumentOperation(parser.ProgramWordOperation)).To(BeFalse())
	})
})
