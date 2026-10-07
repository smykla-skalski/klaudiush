package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Startup file writes", func() {
	parse := func(command string, resolver fakeResolver) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred(), command)

		return result
	}

	pushed := func(result *parser.ParseResult) bool {
		for _, operation := range result.GitOperations {
			if len(operation.Args) > 0 && operation.Args[0] == "push" {
				return true
			}
		}

		return false
	}

	DescribeTable(
		"follows captured shell content",
		func(command string) {
			result := parse(command, fakeResolver{env: map[string]string{"HOME": "/home/u"}})

			Expect(pushed(result)).To(BeTrue(), command)
		},
		Entry("literal append", `echo 'git push --force origin main' >> ~/.zshenv`),
		Entry("literal overwrite", `printf '%s' 'git push --force origin main' > ~/.bashrc`),
		Entry("heredoc overwrite", "cat > ~/.bashrc <<'EOF'\ngit push --force origin main\nEOF"),
		Entry("heredoc append", "cat >> ~/.bashrc <<'EOF'\ngit push --force origin main\nEOF"),
		Entry(
			"resolved heredoc variable",
			"PUSH='git push --force origin main'; cat > ~/.bashrc <<EOF\n$PUSH\nEOF",
		),
		Entry("system startup file", `echo 'git push --force origin main' > /etc/profile`),
		Entry(
			"configured BASH_ENV",
			`BASH_ENV=/tmp/rc; echo 'git push --force origin main' > /tmp/rc`,
		),
	)

	It("follows a readable copied file", func() {
		result := parse(`cp /source ~/.zshenv`, fakeResolver{
			env:   map[string]string{"HOME": "/home/u"},
			files: map[string]string{"/source": "git push --force origin main"},
		})

		Expect(pushed(result)).To(BeTrue())
	})

	It("follows every startup file copied into a directory", func() {
		result := parse(`cp /safe/.bashrc /unsafe/.zshenv ~`, fakeResolver{
			env: map[string]string{"HOME": "/home/u"},
			files: map[string]string{
				"/safe/.bashrc":   "true",
				"/unsafe/.zshenv": "git push --force origin main",
			},
		})

		Expect(pushed(result)).To(BeTrue())
	})

	DescribeTable(
		"follows a readable file copied into a startup directory",
		func(command string) {
			result := parse(command, fakeResolver{
				env:   map[string]string{"HOME": "/home/u"},
				files: map[string]string{"/source/.zshenv": "git push --force origin main"},
			})

			Expect(pushed(result)).To(BeTrue())
		},
		Entry("destination operand", `cp /source/.zshenv ~`),
		Entry("target directory option", `cp -t ~ /source/.zshenv`),
		Entry("move", `mv /source/.zshenv ~`),
		Entry("rsync", `rsync /source/.zshenv ~`),
		Entry("install", `install /source/.zshenv ~`),
	)

	It("follows startup writes planted by startup content", func() {
		result := parse(
			`echo 'echo "git push --force origin main" > ~/.bashrc' > ~/.zshenv`,
			fakeResolver{env: map[string]string{"HOME": "/home/u"}},
		)

		Expect(pushed(result)).To(BeTrue())
	})

	It("uses future-shell variables for planted startup content", func() {
		result := parse(
			`echo 'echo "git push --force origin main" > "$HOME/.bashrc"' > ~/.zshenv; HOME=/tmp`,
			fakeResolver{env: map[string]string{"HOME": "/home/u"}},
		)

		Expect(pushed(result)).To(BeTrue())
	})

	It("resolves literal dynamic redirect targets", func() {
		result := parse(
			`echo 'git push --force origin main' > "$(printf %s ~/.zshenv)"`,
			fakeResolver{env: map[string]string{"HOME": "/home/u"}},
		)

		Expect(pushed(result)).To(BeTrue())
		Expect(result.Truncated).To(BeFalse())
	})

	It("resolves literal dynamic system startup targets", func() {
		result := parse(
			`echo 'git push --force origin main' > "$(printf /etc/profile)"`,
			fakeResolver{env: map[string]string{"HOME": "/home/u"}},
		)

		Expect(pushed(result)).To(BeTrue())
		Expect(result.Truncated).To(BeFalse())
	})

	DescribeTable(
		"combines appended content with prior bytes",
		func(command string, files map[string]string) {
			result := parse(command, fakeResolver{
				env:   map[string]string{"HOME": "/home/u"},
				files: files,
			})

			Expect(pushed(result)).To(BeTrue())
		},
		Entry(
			"file from an earlier command",
			`printf 'push --force origin main' >> ~/.zshenv`,
			map[string]string{"/home/u/.zshenv": "git "},
		),
		Entry(
			"overwrite earlier on the line",
			`printf git > ~/.zshenv; printf ' push --force origin main' >> ~/.zshenv`,
			nil,
		),
		Entry(
			"heredoc appended to an existing file",
			"cat >> ~/.zshenv <<'EOF'\npush --force origin main\nEOF",
			map[string]string{"/home/u/.zshenv": "git "},
		),
		Entry(
			"echo overwrite keeps its trailing newline",
			`echo '# safe' > ~/.zshenv; echo 'git push --force origin main' >> ~/.zshenv`,
			nil,
		),
		Entry(
			"printf overwrite keeps its trailing newline",
			`printf '# safe\n' > ~/.zshenv; printf 'git push --force origin main' >> ~/.zshenv`,
			nil,
		),
	)

	DescribeTable(
		"uses inherited startup locations after temporary overrides",
		func(command string, env map[string]string) {
			result := parse(command, fakeResolver{env: env})

			Expect(pushed(result)).To(BeTrue(), command)
		},
		Entry(
			"HOME",
			`HOME=/tmp; echo 'git push --force origin main' > /home/u/.zshenv`,
			map[string]string{"HOME": "/home/u"},
		),
		Entry(
			"BASH_ENV",
			`BASH_ENV=/tmp/other; echo 'git push --force origin main' > /home/u/bash-env`,
			map[string]string{"HOME": "/home/u", "BASH_ENV": "/home/u/bash-env"},
		),
		Entry(
			"ZDOTDIR",
			`ZDOTDIR=/tmp; echo 'git push --force origin main' > /home/u/zsh/.zshenv`,
			map[string]string{"HOME": "/home/u", "ZDOTDIR": "/home/u/zsh"},
		),
	)

	It("does not invent a newline for echo -n", func() {
		result := parse(
			`echo -n '# safe' > ~/.zshenv; echo 'git push --force origin main' >> ~/.zshenv`,
			fakeResolver{env: map[string]string{"HOME": "/home/u"}},
		)

		Expect(pushed(result)).To(BeFalse())
		Expect(result.Truncated).To(BeFalse())
	})

	DescribeTable("fails closed when planted content cannot be inspected",
		func(command, cause, operation string) {
			result := parse(command, fakeResolver{env: map[string]string{"HOME": "/home/u"}})

			Expect(result.Truncated).To(BeTrue(), command)
			Expect(result.Opacities).To(ContainElement(SatisfyAll(
				HaveField("Cause", parser.OpacityCause(cause)),
				HaveField("Operation", operation),
			)), command)
		},
		Entry("unseen redirect", `date > ~/.zshenv`, "startup-file", ".zshenv"),
		Entry(
			"unseen heredoc expansion",
			"cat > ~/.zshenv <<EOF\n$(cat /missing)\nEOF",
			"startup-file",
			".zshenv",
		),
		Entry(
			"copied unseen heredoc expansion",
			"cat > /source <<EOF\n$(cat /missing)\nEOF\ncp /source ~/.zshenv",
			"startup-file",
			".zshenv",
		),
		Entry(
			"overridden heredoc copier",
			"cat() { echo 'git push --force origin main'; }; "+
				"cat > ~/.zshenv <<'EOF'\ntrue\nEOF",
			"startup-file",
			".zshenv",
		),
		Entry(
			"overridden literal producer",
			"echo() { printf '%s' 'git push --force origin main'; }; "+
				"echo true > ~/.zshenv",
			"startup-file",
			".zshenv",
		),
		Entry(
			"overridden copy command",
			"cp() { printf '%s' 'different content' > \"$2\"; }; "+
				"cp /source ~/.zshenv",
			"startup-file",
			".zshenv",
		),
		Entry("unreadable copy", `cp /missing ~/.zshenv`, "startup-file", ".zshenv"),
		Entry(
			"unresolved directory-copy source",
			`SRC=$(printf /tmp/.zshenv); cp "$SRC" ~`,
			"startup-file",
			"HOME",
		),
		Entry(
			"unresolved dynamic redirect target",
			`echo 'git push --force origin main' > "$(target)"`,
			"startup-file",
			"dynamic redirect",
		),
		Entry(
			"partially substituted copy source",
			`cp "/tmp$(printf /unsafe)/.zshenv" ~`,
			"startup-file",
			"HOME",
		),
		Entry(
			"all-substitution copy source",
			`cp "$(printf /unsafe/.zshenv)" ~`,
			"startup-file",
			"HOME",
		),
		Entry("foreign shell syntax", `echo true > ~/.config/fish/config.fish`,
			"script-syntax", "config.fish"),
	)

	It("leaves a similarly named file outside startup directories alone", func() {
		result := parse(`echo 'git push --force origin main' > /tmp/.zshenv`,
			fakeResolver{env: map[string]string{"HOME": "/home/u"}})

		Expect(result.Truncated).To(BeFalse())
		Expect(result.GitOperations).To(BeEmpty())
	})
})
