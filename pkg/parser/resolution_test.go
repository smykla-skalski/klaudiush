package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// fakeResolver answers from fixed tables instead of the running system.
type fakeResolver struct {
	env      map[string]string
	files    map[string]string
	programs map[string]parser.Program
	aliases  map[string]string
}

func (f fakeResolver) LookupEnv(name string) (string, bool) {
	value, ok := f.env[name]

	return value, ok
}

func (f fakeResolver) ReadScript(path string) (string, bool) {
	text, ok := f.files[path]

	return text, ok
}

func (f fakeResolver) Program(word, _ string) parser.Program {
	if program, ok := f.programs[word]; ok {
		return program
	}

	return parser.ProgramOther
}

func (f fakeResolver) GitAlias(_, name string) (string, bool) {
	value, ok := f.aliases[name]

	return value, ok
}

var _ = Describe("Command resolution beyond the command text", func() {
	resolver := fakeResolver{
		env: map[string]string{"GIT": "/usr/bin/git"},
		files: map[string]string{
			"deploy.sh":       "echo deploying\ngit commit -S -m x\n",
			"./deploy.sh":     "git commit -S -m x\n",
			"/repo/deploy.sh": "git commit -S -m x\n",
			"tool.py":         "import os\nos.system('git commit -S -m x')\n",
			"./tool":          "#!/usr/bin/env python3\nimport os\nos.system('git commit -S -m x')\n",
		},
		programs: map[string]parser.Program{
			"./g":     parser.ProgramGit,
			"mygit":   parser.ProgramMissing,
			"./h":     parser.ProgramGH,
			"docker":  parser.ProgramOther,
			"make":    parser.ProgramOther,
			"svn":     parser.ProgramOther,
			"unknown": parser.ProgramMissing,
		},
		aliases: map[string]string{
			"ci":   "commit",
			"c":    "ci",
			"ship": "!git commit -S",
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	signedCommits := func(command string) int {
		count := 0

		for _, cmd := range parse(command).GitOperations {
			gitCmd, err := parser.ParseGitCommand(cmd)
			if err == nil && gitCmd.Subcommand == "commit" && gitCmd.HasFlag("-S") {
				count++
			}
		}

		return count
	}

	DescribeTable(
		"finds a git commit hidden behind",
		func(command string) {
			Expect(signedCommits(command)).To(BeNumerically(">", 0), "no git commit in %q", command)
		},
		Entry("an environment variable", "$GIT commit -S -m x"),
		Entry("which in a substitution", "$(which git) commit -S -m x"),
		Entry("command -v in backticks", "`command -v git` commit -S -m x"),
		Entry("a quoted substitution", `"$(type -P git)" commit -S -m x`),
		Entry("the git exec path", "$(git --exec-path)/git-commit -S -m x"),
		Entry("a git-commit binary", "git-commit -S -m x"),
		Entry("a git-commit path", "/usr/libexec/git-core/git-commit -S -m x"),
		Entry("hub", "hub commit -S -m x"),
		Entry("uppercase", "GIT commit -S -m x"),
		Entry("mixed case path", "/usr/bin/Git commit -S -m x"),
		Entry("a shell script file", "bash deploy.sh"),
		Entry("a script run by path", "./deploy.sh"),
		Entry("a script in the cd directory", "cd /repo && bash deploy.sh"),
		Entry("source", "source deploy.sh"),
		Entry("dot", ". deploy.sh"),
		Entry("a script on stdin", "bash < deploy.sh"),
		Entry("a script written with printf", "printf 'git commit -S -m x\\n' > x.sh && bash x.sh"),
		Entry(
			"a script written with a heredoc",
			"cat > x.sh <<'EOF'\ngit commit -S -m x\nEOF\nbash x.sh",
		),
		Entry("a here-string", `bash <<< "git commit -S -m x"`),
		Entry("a process substitution", `bash <(echo "git commit -S -m x")`),
		Entry("a sourced process substitution", `source <(printf 'git commit -S -m x')`),
		Entry("python os.system", `python3 -c "import os; os.system('git commit -S -m x')"`),
		Entry(
			"python argv list",
			`python3 -c "import subprocess; subprocess.run(['git', 'commit', '-S', '-m', 'x'])"`,
		),
		Entry(
			"node execFileSync",
			`node -e "require('child_process').execFileSync('git', ['commit', '-S', '-m', 'x'])"`,
		),
		Entry("ruby backticks", "ruby -e '`git commit -S -m x`'"),
		Entry("perl system", `perl -e 'system("git commit -S -m x")'`),
		Entry("perl qx", `perl -e 'qx{git commit -S -m x}'`),
		Entry("osascript", `osascript -e 'do shell script "git commit -S -m x"'`),
		Entry("pwsh", `pwsh -c "git commit -S -m x"`),
		Entry("a python script file", "python3 tool.py"),
		Entry("an interpreter shebang", "./tool"),
		Entry("a same-line alias", "alias g=git; g commit -S -m x"),
		Entry("a same-line alias with arguments", "alias gc='git commit'; gc -S -m x"),
		Entry("a function passing $@", `f() { git "$@"; }; f commit -S -m x`),
		Entry("a function using $1", `f() { git commit "$1" -m x; }; f -S`),
		Entry("an inline git alias", "git -c alias.ci=commit ci -S -m x"),
		Entry("a configured git alias", "git ci -S -m x"),
		Entry("a chained git alias", "git c -S -m x"),
		Entry("a shell git alias", "git ship -m x"),
		Entry("xargs appending stdin", `echo "commit -S -m x" | xargs git`),
		Entry("xargs replacing stdin", "printf 'x\\n' | xargs -I{} git commit -S -m {}"),
		Entry("mise exec", "mise exec -- git commit -S -m x"),
		Entry("nix run", "nix run nixpkgs#git -- commit -S -m x"),
		Entry("docker run", "docker run --rm -v .:/w alpine/git commit -S -m x"),
		Entry("ssh", "ssh host git commit -S -m x"),
		Entry("a symlink to git", "./g commit -S -m x"),
		Entry("a name nothing on disk runs", "mygit commit -S -m x"),
		Entry("an unresolvable variable", "$NOPE commit -S -m x"),
		Entry("an unresolvable substitution", "$(pick-git) commit -S -m x"),
	)

	DescribeTable("leaves other programs alone",
		func(command string) {
			Expect(parse(command).GitOperations).To(BeEmpty(), "unexpected git in %q", command)
		},
		Entry("docker commit", "docker commit abc image"),
		Entry("make push", "make push"),
		Entry("svn commit", "svn commit -m x"),
		Entry("a shell builtin", "read commit"),
		Entry("echo", "echo add"),
		Entry("man", "man git commit"),
		Entry("python printing", `python3 -c "print('hello')"`),
		Entry("a binary run by path", "./bin/tool --flag"),
		Entry("an unknown program with other operands", "unknown build"),
	)

	It("resolves gh under another name", func() {
		Expect(parse("./h pr create --title x").Commands[0].Name).To(Equal("gh"))
	})

	It("keeps the word as written", func() {
		cmd := parse("/usr/bin/Git status").Commands[0]
		Expect(cmd.Name).To(Equal("git"))
		Expect(cmd.Invoked).To(Equal("/usr/bin/Git"))
	})
})
