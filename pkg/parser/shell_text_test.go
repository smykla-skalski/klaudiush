package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("ShellText", func() {
	resolver := fakeResolver{env: map[string]string{"HOME": "/home/u", "GIT_EDITOR": "vim"}}

	lastGit := func(command string) parser.Command {
		GinkgoHelper()

		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.GitOperations).NotTo(BeEmpty())

		return result.GitOperations[len(result.GitOperations)-1]
	}

	messageOf := func(command string) (string, string) {
		GinkgoHelper()

		cmd := lastGit(command)
		gitCmd, err := parser.ParseGitCommand(cmd)
		Expect(err).NotTo(HaveOccurred())

		values := gitCmd.ValuesOf("-m", "--message")
		Expect(values).NotTo(BeEmpty())

		fv := values[len(values)-1]
		text, ok := cmd.ArgText(fv.Arg)
		Expect(ok).To(BeTrue())

		if fv.Arg != fv.Value {
			text, ok = text.TrimPrefix(fv.Arg[:len(fv.Arg)-len(fv.Value)])
			Expect(ok).To(BeTrue())
		}

		return text.Value()
	}

	DescribeTable(
		"builds -m values",
		func(command, want string) {
			value, gap := messageOf(command)
			Expect(gap).To(BeEmpty())
			Expect(value).To(Equal(want))
		},
		Entry("literal", `git commit -m 'feat(x): a'`, "feat(x): a"),
		Entry("single-quoted reference stays literal",
			`git commit -m 'use ${HOME}'`, "use ${HOME}"),
		Entry("variable set on the line", `T='feat(x): a b'; git commit -m "$T"`, "feat(x): a b"),
		Entry("exported variable", `export T=x; git commit -m "a $T"`, "a x"),
		Entry("environment variable", `git commit -m "at $HOME"`, "at /home/u"),
		Entry("unquoted variable without spaces", `T=word; git commit -m $T`, "word"),
		Entry("escapes in double quotes", `git commit -m "a \$T \"q\""`, `a $T "q"`),
		Entry("ANSI-C quoting", `git commit -m $'a\nb'`, "a\nb"),
		Entry("quoted cat heredoc stays literal",
			"git commit -m \"$(cat <<'EOF'\nfeat(x): ${HOME}\n\nEOF\n)\"", "feat(x): ${HOME}"),
		Entry("unquoted cat heredoc expands",
			"T=v; git commit -m \"$(cat <<EOF\nfeat(x): $T \\$HOME\nEOF\n)\"", "feat(x): v $HOME"),
		Entry("cat - heredoc", "git commit -m \"$(cat - <<'EOF'\na\nEOF\n)\"", "a"),
		Entry(
			"tab-stripped heredoc",
			"git commit -m \"$(cat <<-EOF\n\ta\n\t\tb\n\tEOF\n)\"",
			"a\nb",
		),
		Entry("here-string through cat", `T=x; git commit -m "$(cat <<< "a $T")"`, "a x"),
		Entry("glued long flag", `T=x; git commit --message="a $T"`, "a x"),
		Entry("glued short flag", `T=x; git commit -ma"$T"`, "ax"),
		Entry("combined short flags", `git commit -sSm 'a b'`, "a b"),
		Entry("value expanded before a prefix assignment",
			`T=old; T=new git commit -m "$T"`, "old"),
	)

	DescribeTable("reports what it cannot build",
		func(command, gap string) {
			_, got := messageOf(command)
			Expect(got).To(ContainSubstring(gap))
		},
		Entry("unset variable", `git commit -m "$NOPE"`, "$NOPE, which is not set"),
		Entry("positional parameter", `git commit -m "$1"`, "$1, which is not set"),
		Entry("prefix assignment", `T=x git commit -m "$T"`, "$T, which is not set"),
		Entry("command output variable", `T=$(date); git commit -m "$T"`, "cannot know"),
		Entry("variable read from input", `T=a; read T; git commit -m "$T"`, "cannot know"),
		Entry("variable in a loop", `for i in 1; do T=a; git commit -m "$T"; done`, "cannot know"),
		Entry("variable holding a reference", `A='${B}'; git commit -m "$A"`, "refers to other"),
		Entry("unquoted variable that splits", `T='a b'; git commit -m $T`, "splits"),
		Entry("expansion with an operator", `git commit -m "${T:-x}"`, "${T:-x}"),
		Entry("command substitution", `git commit -m "a $(date)"`, "command output"),
		Entry("backticks", "git commit -m \"a `date`\"", "command output"),
		Entry("transforming heredoc",
			"git commit -m \"$(sed s/a/b/ <<'EOF'\na\nEOF\n)\"", "command output"),
		Entry("cat heredoc with a second command",
			"git commit -m \"$(cat <<'EOF'\na\nEOF\necho b)\"", "command output"),
		Entry("cat defined on the line",
			"cat() { echo b; }; git commit -m \"$(cat <<'EOF'\na\nEOF\n)\"", "cat is an alias"),
		Entry("arithmetic", `git commit -m "n $((1+1))"`, "arithmetic"),
		Entry("unquoted glob", `git commit -m wip*`, "glob"),
		Entry("unquoted brace", `git commit -m {a,b}`, "glob"),
		Entry("leading tilde", `git commit -m ~/x`, "glob"),
		Entry("unset variable in a heredoc",
			"git commit -m \"$(cat <<EOF\na $NOPE\nEOF\n)\"", "$NOPE"),
	)

	It("marks two words that render alike but build differently", func() {
		cmd := lastGit(`T=x; git commit -m '${T}' -m "${T}"`)
		text, ok := cmd.ArgText("${T}")
		Expect(ok).To(BeTrue())

		_, gap := text.Value()
		Expect(gap).To(Equal(parser.GapAmbiguous))
	})

	It("builds heredoc stdin with its variables", func() {
		cmd := lastGit("T=x; git commit -F - <<EOF\na $T\nEOF")
		text, ok := cmd.StdinText()
		Expect(ok).To(BeTrue())
		Expect(text.Value()).To(Equal("a x\n"))

		cmd = lastGit("git commit -F - <<'EOF'\na ${T}\nEOF")
		text, _ = cmd.StdinText()
		Expect(text.Value()).To(Equal("a ${T}\n"))

		cmd = lastGit("cat <<EOF | git commit -F -\na $NOPE\nEOF")
		text, ok = cmd.StdinText()
		Expect(ok).To(BeTrue())

		_, gap := text.Value()
		Expect(gap).To(ContainSubstring("$NOPE"))

		cmd = lastGit("echo 'a ${T}' | git commit -F -")
		text, _ = cmd.StdinText()
		Expect(text.Value()).To(HavePrefix("a ${T}"))

		bare := lastGit("git commit -F -")
		_, ok = bare.StdinText()
		Expect(ok).To(BeFalse())
	})

	It("does not trim a prefix that is not literal", func() {
		cmd := lastGit(`T=x; git commit -m "$T"`)
		text, _ := cmd.ArgText("${T}")

		_, ok := text.TrimPrefix("-m")
		Expect(ok).To(BeFalse())

		_, ok = parser.ShellText{Parts: []parser.TextPart{{Text: "ab"}}}.TrimPrefix("ax")
		Expect(ok).To(BeFalse())

		trimmed, ok := parser.ShellText{
			Parts: []parser.TextPart{{Text: "-"}, {Text: "mx"}},
		}.TrimPrefix("-m")
		Expect(ok).To(BeTrue())
		Expect(trimmed.Value()).To(Equal("x"))
	})

	DescribeTable("records the git environment of a command",
		func(command string, want parser.EnvValue) {
			cmd := lastGit(command)
			Expect(cmd.Env("GIT_EDITOR")).To(Equal(want))
		},
		Entry("from the environment", "git commit",
			parser.EnvValue{Value: "vim", Set: true, Known: true}),
		Entry("prefix assignment", "GIT_EDITOR=true git commit",
			parser.EnvValue{Value: "true", Set: true, Known: true}),
		Entry("prefix assignment from a variable", "E=true; GIT_EDITOR=$E git commit",
			parser.EnvValue{Value: "true", Set: true, Known: true}),
		Entry("prefix assignment from command output", "GIT_EDITOR=$(echo x) git commit",
			parser.EnvValue{Set: true}),
		Entry("prefix append", "GIT_EDITOR+=x git commit", parser.EnvValue{}),
		Entry("export earlier", "export GIT_EDITOR=:; git commit",
			parser.EnvValue{Value: ":", Set: true, Known: true}),
		Entry("read earlier", "read GIT_EDITOR; git commit", parser.EnvValue{}),
		Entry("through a launcher", "env GIT_EDITOR=true git commit", parser.EnvValue{}),
	)

	DescribeTable("passes a line assignment to git only when exported",
		func(command string, want parser.EnvValue) {
			result, err := parser.NewBashParserWithResolver(fakeResolver{}).Parse(command)
			Expect(err).NotTo(HaveOccurred())

			cmd := result.GitOperations[len(result.GitOperations)-1]
			Expect(cmd.Env("GIT_EDITOR")).To(Equal(want))
		},
		Entry("plain assignment", "GIT_EDITOR=true; git commit", parser.EnvValue{Known: true}),
		Entry("export", "GIT_EDITOR=true; export GIT_EDITOR; git commit",
			parser.EnvValue{Value: "true", Set: true, Known: true}),
		Entry("declare -x", "declare -x GIT_EDITOR=true; git commit",
			parser.EnvValue{Value: "true", Set: true, Known: true}),
		Entry("set -a", "set -a; GIT_EDITOR=true; git commit",
			parser.EnvValue{Value: "true", Set: true, Known: true}),
		Entry("unset", "git commit", parser.EnvValue{Known: true}),
	)

	It("marks PATH unknown once the line changes it", func() {
		cmd := lastGit("PATH=/x:$PATH; git commit")
		Expect(cmd.Env("PATH").Known).To(BeFalse())
	})

	It("records every flag value with the argument it came from", func() {
		gitCmd, err := parser.ParseGitCommand(lastGit(
			`git commit -m a --message=b -sSmc -t tpl --fixup=reword:HEAD --squash x -C HEAD`,
		))
		Expect(err).NotTo(HaveOccurred())
		Expect(gitCmd.ValuesOf("-m", "--message")).To(Equal([]parser.FlagValue{
			{Flag: "-m", Value: "a", Arg: "a"},
			{Flag: "--message", Value: "b", Arg: "--message=b"},
			{Flag: "-m", Value: "c", Arg: "-sSmc"},
		}))
		Expect(
			gitCmd.ValuesOf("-t"),
		).To(Equal([]parser.FlagValue{{Flag: "-t", Value: "tpl", Arg: "tpl"}}))
		Expect(gitCmd.ValuesOf("--fixup")).To(HaveLen(1))
		Expect(gitCmd.ValuesOf("--squash")[0].Value).To(Equal("x"))
		Expect(gitCmd.ValuesOf("-C")[0].Value).To(Equal("HEAD"))
		Expect(gitCmd.Args).To(BeEmpty())
	})
})
