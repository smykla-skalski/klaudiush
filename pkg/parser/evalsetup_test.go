package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Eval of a tool's shell setup", func() {
	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(fakeResolver{}).Parse(command)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Truncated).To(BeTrue(), "eval must stay blocked: %q", command)

		return result
	}

	toAny := func(names []string) []any {
		out := make([]any, 0, len(names))
		for _, name := range names {
			out = append(out, name)
		}

		return out
	}

	evalOpacity := func(tool string, origin ...string) types.GomegaMatcher {
		return SatisfyAll(
			HaveField("Cause", parser.OpacityUnresolvedWord),
			HaveField("Operation", "eval"),
			HaveField("Origin", HaveExactElements(toAny(origin)...)),
			HaveField("Detail", parser.DetailWordOutput),
			HaveField("Tool", tool),
		)
	}

	DescribeTable("names the tool whose output eval runs",
		func(command, tool string) {
			Expect(parse(command).Opacities).To(ConsistOf(evalOpacity(tool)))
		},
		Entry("ssh-agent -s", `eval "$(ssh-agent -s)"`, "ssh-agent"),
		Entry("ssh-agent without options", `eval $(ssh-agent)`, "ssh-agent"),
		Entry("ssh-agent in backquotes", "eval `ssh-agent -c`", "ssh-agent"),
		Entry("mise activate", `eval "$(mise activate bash)"`, "mise"),
		Entry("mise env", `eval "$(mise env -s bash)"`, "mise"),
		Entry("mise with a global option", `eval "$(mise --quiet hook-env)"`, "mise"),
		Entry("direnv export", `eval "$(direnv export bash)"`, "direnv"),
		Entry("direnv hook", `eval "$(direnv hook zsh)"`, "direnv"),
		Entry("rbenv init", `eval "$(rbenv init - bash)"`, "rbenv"),
		Entry("pyenv init", `eval "$(pyenv init -)"`, "pyenv"),
		Entry("pyenv virtualenv-init", `eval "$(pyenv virtualenv-init -)"`, "pyenv"),
		Entry("nodenv init", `eval "$(nodenv init -)"`, "nodenv"),
		Entry("conda hook", `eval "$(conda shell.bash hook)"`, "conda"),
		Entry("brew by path", `eval "$(/opt/homebrew/bin/brew shellenv)"`, "brew"),
		Entry("starship init", `eval "$(starship init bash)"`, "starship"),
		Entry("zoxide init", `eval "$(zoxide init bash)"`, "zoxide"),
		Entry("fnm env", `eval "$(fnm env --use-on-cd)"`, "fnm"),
		Entry("quoted literal program", `eval "$('mise' activate bash)"`, "mise"),
		Entry("with a redirect", `eval "$(ssh-agent -s 2>/dev/null)"`, "ssh-agent"),
		Entry("escaped program name", `eval "$(\mise activate bash)"`, "mise"),
		Entry("escaped eval", `\eval "$(direnv export bash)"`, "direnv"),
	)

	DescribeTable("keeps the generic explanation for anything else",
		func(command string) {
			Expect(parse(command).Opacities).To(ContainElement(SatisfyAll(
				HaveField("Operation", "eval"),
				HaveField("Tool", ""),
			)))
		},
		Entry("a name that contains a tool", `eval "$(evil-mise activate bash)"`),
		Entry("a tool as a suffix", `eval "$(mise-evil activate bash)"`),
		Entry("program from a variable", `eval "$($TOOL activate bash)"`),
		Entry("program from command output", `eval "$($(echo mise) activate bash)"`),
		Entry("subcommand from a variable", `eval "$(mise $SUB bash)"`),
		Entry("a subcommand that prints no setup", `eval "$(mise exec -- echo x)"`),
		Entry("ssh-agent killing the agent", `eval "$(ssh-agent -k)"`),
		Entry("ssh-agent running a command", `eval "$(ssh-agent sh)"`),
		Entry("conda without a shell", `eval "$(conda shell.)"`),
		Entry("a pipeline", `eval "$(mise activate bash | cat)"`),
		Entry("two statements", `eval "$(mise activate bash; echo x)"`),
		Entry("a negated statement", `eval "$(! mise activate bash)"`),
		Entry("text around the substitution", `eval "x$(mise activate bash)"`),
		Entry("two substitutions", `eval "$(mise activate bash)$(echo x)"`),
		Entry("a second argument", `eval "$(mise activate bash)" x`),
		Entry("a process substitution", `eval "$(cat <(mise activate bash))"`),
		Entry("an unknown tool", `eval "$(tool init)"`),
		Entry("eval through a launcher", `builtin eval "$(mise activate bash)"`),
		Entry("ssh-agent given a computed argument", `eval "$(ssh-agent "$CMD")"`),
		Entry("a computed argument after the subcommand", `eval "$(mise activate $SH)"`),
		Entry("a function named like the tool",
			`mise() { echo x; }; eval "$(mise activate bash)"`),
		Entry("an alias named like the tool",
			`alias mise='echo x'; eval "$(mise activate bash)"`),
		Entry("a function named eval", `eval() { echo x; }; eval "$(mise activate bash)"`),
	)

	It("keeps the tool through a shell that runs eval", func() {
		Expect(
			parse(`bash -c 'eval "$(direnv export bash)"'`).Opacities,
		).To(ConsistOf(evalOpacity("direnv", "bash")))
	})

	It("names every eval of setup on the line", func() {
		Expect(
			parse(`eval "$(ssh-agent -s)"; eval "$(mise activate bash)"`).Opacities,
		).To(ConsistOf(evalOpacity("ssh-agent"), evalOpacity("mise")))
	})

	DescribeTable("follows the commands the suggested forms run",
		func(command string) {
			result, err := parser.NewBashParserWithResolver(fakeResolver{}).Parse(command)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Truncated).To(BeFalse(), command)
			Expect(result.GitOperations).To(ContainElement(
				HaveField("Args", ContainElement("commit")),
			), command)
		},
		Entry("direnv exec in the current directory", `direnv exec . git commit -m x`),
		Entry("direnv exec in ./", `direnv exec ./ git commit -m x`),
		Entry("direnv exec in a directory", `direnv exec /repo git commit -m x`),
		Entry("mise exec", `mise exec -- git commit -m x`),
		Entry("ssh-agent", `ssh-agent git commit -m x`),
		Entry("ssh-agent with a shell", `ssh-agent bash -c 'ssh-add && git commit -m x'`),
		Entry("rbenv exec", `rbenv exec git commit -m x`),
		Entry("conda run", `conda run -n base git commit -m x`),
		Entry("fnm exec", `fnm exec --using=20 git commit -m x`),
	)

	It("lists the tools it recognizes", func() {
		Expect(parser.EvalSetupTools()).To(ContainElements("ssh-agent", "mise", "direnv"))
	})
})
