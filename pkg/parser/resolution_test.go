package parser_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// fakeResolver answers from fixed tables instead of the running system.
type fakeResolver struct {
	env       map[string]string
	files     map[string]string
	opaque    map[string]bool
	programs  map[string]parser.Program
	aliases   map[string]string
	paths     map[string]string
	ghAliases map[string]string
}

func (f fakeResolver) LookupEnv(name string) (string, bool) {
	value, ok := f.env[name]

	return value, ok
}

func (f fakeResolver) ReadScript(path string) (string, parser.ScriptStatus) {
	if f.opaque[path] {
		return "", parser.ScriptOpaque
	}

	text, ok := f.files[path]
	if !ok {
		return "", parser.ScriptMissing
	}

	return text, parser.ScriptText
}

func (f fakeResolver) LookPath(name string) (string, bool) {
	path, ok := f.paths[name]

	return path, ok
}

func (f fakeResolver) GHAlias(name string) (string, bool) {
	value, ok := f.ghAliases[name]

	return value, ok
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

func (f fakeResolver) GitCommand(name string) bool {
	program, ok := f.programs["git-"+name]

	return ok && program != parser.ProgramMissing
}

var _ = Describe("Command resolution beyond the command text", func() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/home/user"
	}

	homeScript := filepath.Join(home, "bin", "release")

	resolver := fakeResolver{
		env: map[string]string{"GIT": "/usr/bin/git"},
		files: map[string]string{
			"deploy.sh":       "echo deploying\ngit commit -S -m x\n",
			"./deploy.sh":     "git commit -S -m x\n",
			"/repo/deploy.sh": "git commit -S -m x\n",
			"tool.py":         "import os\nos.system('git commit -S -m x')\n",
			"tool":            "#!/usr/bin/env python3\nimport os\nos.system('git commit -S -m x')\n",
			"tests/test_git.py": "import subprocess\n" +
				"def test_commit():\n    subprocess.run(['git', 'commit', '-m', 'x'])\n",
			homeScript:            "#!/bin/sh\ngit commit -S -m x\n",
			"/usr/local/bin/brew": "#!/bin/bash\ngit commit -S -m x\n",
		},
		opaque: map[string]bool{"huge.sh": true},
		paths: map[string]string{
			"release": homeScript,
			"brew":    "/usr/local/bin/brew",
		},
		ghAliases: map[string]string{
			"mk":     "pr create",
			"shipit": "!git commit -S -m x",
		},
		programs: map[string]parser.Program{
			"./g":     parser.ProgramGit,
			"mygit":   parser.ProgramMissing,
			"./h":     parser.ProgramGH,
			"docker":  parser.ProgramOther,
			"make":    parser.ProgramOther,
			"svn":     parser.ProgramOther,
			"unknown": parser.ProgramMissing,
			// No git-<name> command, so git's autocorrect would apply.
			"git-comit": parser.ProgramMissing,
			"git-pul":   parser.ProgramMissing,
			"git-zz":    parser.ProgramMissing,
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
		Entry(
			"a script whose same-name write went to another directory",
			"cd /elsewhere && printf 'echo hi\\n' > deploy.sh; cd /repo && bash deploy.sh",
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
		Entry("a variable that splits into words", `x="git commit -S -m y"; $x`),
		Entry("an array", `cmd=(git commit -S -m y); "${cmd[@]}"`),
		Entry("brace expansion", "{git,commit,-S,-m,y}"),
		Entry("escaped spaces in a shell script", `bash -c git\ commit\ -S\ -m\ y`),
		Entry("env -S with attached script", `env -S'git commit -S -m y'`),
		Entry(
			"python -c with attached code",
			`python3 -c'import os; os.system("git commit -S -m y")'`,
		),
		Entry("python flag cluster", `python3 -Sc 'import os; os.system("git commit -S -m y")'`),
		Entry("perl flag cluster", `perl -we 'system("git commit -S -m y")'`),
		Entry(
			"node flag cluster",
			`node -pe 'require("child_process").execSync("git commit -S -m y")'`,
		),
		Entry(
			"perl system with separate arguments",
			`perl -e 'system("git", "commit", "-S", "-m", "y")'`,
		),
		Entry(
			"python argv tuple",
			`python3 -c 'import subprocess; subprocess.run(("git", "commit", "-S"))'`,
		),
		Entry("a cat heredoc piped to a shell", "cat <<'EOF' | bash\ngit commit -S -m y\nEOF"),
		Entry("a cat file piped to a shell", "cat deploy.sh | bash"),
		Entry("a git alias set earlier on the line", "git config alias.cm commit; git cm -S -m y"),
		Entry(
			"a git alias in GIT_CONFIG_COUNT",
			"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=alias.cm GIT_CONFIG_VALUE_0=commit git cm -S -m y",
		),
		Entry("a git alias in GIT_CONFIG_PARAMETERS",
			`GIT_CONFIG_PARAMETERS="'alias.cm'='commit'" git cm -S -m y`),
		Entry("a git alias from --config-env", "CM=commit git --config-env=alias.cm=CM cm -S -m y"),
		Entry("a typo git autocorrects", "git comit -S -m y"),
		Entry(
			"an interpreter behind uv run",
			`uv run python3 -c 'import os; os.system("git commit -S -m y")'`,
		),
		Entry(
			"an interpreter behind bundle exec",
			`bundle exec ruby -e 'system("git commit -S -m y")'`,
		),
		Entry("trap", `trap 'git commit -S -m y' EXIT`),
		Entry("builtin eval", `builtin eval 'git commit -S -m y'`),
		Entry("a one-argument command line", `tmux new-session -d 'git commit -S -m y'`),
		Entry("fish", `fish -c 'git commit -S -m y'`),
		Entry("tcsh", `tcsh -c 'git commit -S -m y'`),
		Entry("nushell", `nu -c 'git commit -S -m y'`),
		Entry("awk system", `awk 'BEGIN { system("git commit -S -m y") }'`),
		Entry("vim running a shell command", `vim -es -c '!git commit -S -m y' -c q`),
		Entry("a makefile on stdin", "make -f - <<'EOF'\nall:\n\tgit commit -S -m y\nEOF"),
		Entry("git rebase --exec", `git rebase -x 'git commit -S -m y' HEAD~1`),
		Entry("git submodule foreach", `git submodule foreach 'git commit -S -m y'`),
		Entry("git bisect run", `git bisect run git commit -S -m y`),
		Entry("a shell-valued git setting", `git -c core.pager='git commit -S -m y' log`),
		Entry(
			"GIT_SEQUENCE_EDITOR",
			`GIT_SEQUENCE_EDITOR='git commit -S -m y' git rebase -i HEAD~2`,
		),
		Entry(
			"a script in a directory reached by relative cd",
			"cd / && cd repo && bash deploy.sh",
		),
		Entry("a script after pushd and popd", "cd /repo && pushd /tmp && popd && bash deploy.sh"),
		Entry(
			"a program run after PATH changes",
			"export PATH=/tmp/evil:$PATH; svn commit -S -m y",
		),
		Entry("a script under home found on PATH", "release"),
		Entry("a gh shell alias", "gh shipit"),
		Entry("a same-line gh shell alias", `gh alias set --shell go 'git commit -S -m y'; gh go`),
		Entry("a runner executing a script path", "pnpm exec ./deploy.sh"),
		Entry(
			"osascript after an option with a value",
			`osascript -l AppleScript -e 'do shell script "git commit -S -m y"'`,
		),
		Entry("php after an option with a value", `php -d x=1 -r 'system("git commit -S -m y");'`),
		Entry(
			"pwsh after word options",
			`pwsh -NoLogo -ExecutionPolicy Bypass -Command "git commit -S -m y"`,
		),
		Entry(
			"a script written earlier, run through eval",
			"cat > e.sh <<'EOF'\ngit commit -S -m y\nEOF\neval 'bash e.sh'",
		),
		Entry(
			"a git alias set earlier, used in a shell",
			`git config alias.cm commit; bash -c 'git cm -S -m y'`,
		),
		Entry(
			"a script written by a nested shell",
			`bash -c "printf 'git commit -S -m y\n' > n.sh" && bash n.sh`,
		),
	)

	DescribeTable(
		"marks what it cannot see truncated, so it fails closed",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), "not truncated: %q", command)
		},
		Entry("a script too large to read", "./huge.sh"),
		Entry("a script written with unknown content", `echo "$BODY" > s.sh && bash s.sh`),
		Entry(
			"a function using positional forms it cannot follow",
			`f() { git "${@:1}"; }; f commit`,
		),
		Entry("an unknown git command under a moved config", "HOME=/tmp/h git zz"),
		Entry(
			"a git alias from configuration it cannot read",
			`printf '[alias]\n\tcm = commit\n' >> .git/config && git cm -S -m y`,
		),
		Entry(
			"a git alias written to a named config file",
			"git config -f f alias.cm commit; git cm -S -m y",
		),
	)

	It("expands a gh alias to the command it stands for", func() {
		cmd := parse("gh mk --title x").Commands[0]
		Expect(cmd.Name).To(Equal("gh"))
		Expect(cmd.Args).To(Equal([]string{"pr", "create", "--title", "x"}))
	})

	It("expands a gh alias set earlier on the line", func() {
		cmds := parse("gh alias set pc 'pr create'; gh pc --title x").Commands
		Expect(cmds[len(cmds)-1].Args).To(Equal([]string{"pr", "create", "--title", "x"}))
	})

	It("puts gh options given before the command after it", func() {
		cmd := parse("gh -R o/r pr create --title x").Commands[0]
		Expect(cmd.Args).To(Equal([]string{"pr", "create", "-R", "o/r", "--title", "x"}))
	})

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
		Entry("a test runner given a file that mentions git", "pytest tests/test_git.py"),
		Entry("python running a module", "python3 -m pytest tests/test_git.py"),
		Entry("prose in interpreter code", `node -e 'console.log("run git commit -S to sign")'`),
		Entry("a linter given a script path", "shellcheck ./deploy.sh"),
		Entry("diff given script paths", "diff ./deploy.sh ./deploy.sh"),
		Entry("a system script on PATH", "brew update"),
	)

	It("ignores an alias named after a git builtin, as git does", func() {
		Expect(signedCommits(`git -c alias.status='!git commit -S -m y' status`)).To(BeZero())
	})

	It("never autocorrects a typo to a validated command that is not the closest", func() {
		// "pul" is closest to pull, so git would never run push for it.
		gitCmd, err := parser.ParseGitCommand(parse("git pul").GitOperations[0])
		Expect(err).NotTo(HaveOccurred())
		Expect(gitCmd.Subcommand).NotTo(Equal("push"))
	})

	It("resolves gh under another name", func() {
		Expect(parse("./h pr create --title x").Commands[0].Name).To(Equal("gh"))
	})

	It("keeps the word as written", func() {
		cmd := parse("/usr/bin/Git status").Commands[0]
		Expect(cmd.Name).To(Equal("git"))
		Expect(cmd.Invoked).To(Equal("/usr/bin/Git"))
	})
})
