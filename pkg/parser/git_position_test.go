package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// sanityScript is a script a command runs: git words appear only as data
// in it (comments, strings, a heredoc written to a file, arguments of other
// programs, a fixture written by Python), never in command position.
const sanityScript = `#!/usr/bin/env bash
set -euo pipefail
# git commit -m "only a comment"
echo "git commit -m data"
printf '%s\n' "git push origin main"
cat > fixture.txt <<EOF
git commit -m heredoc-to-a-file
git push origin main
EOF
python3 - <<PY
from pathlib import Path
Path("fixture2.txt").write_text("git -C repo commit -qm fresh\n")
PY
zakwas propose add-permission review git push
zakwas propose add-permission deny 'git push --force'
grep -q "git push" log.txt
mytool --message "git commit -m x"
`

// realScript runs git for real, in command position.
const realScript = `#!/usr/bin/env bash
set -e
echo "start"
cd repo
git commit -m real
`

var _ = Describe("Git in command position", func() {
	resolver := fakeResolver{
		files: map[string]string{
			"/w/sanity.sh": sanityScript,
			"/w/real.sh":   realScript,
			"/w/mixed.sh":  sanityScript + "git commit -m real\n",
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	subcommands := func(result *parser.ParseResult) []string {
		found := make([]string, 0, len(result.GitOperations))

		for _, cmd := range result.GitOperations {
			gitCmd, err := parser.ParseGitCommand(cmd)
			Expect(err).NotTo(HaveOccurred())

			found = append(found, gitCmd.Subcommand)
		}

		return found
	}

	DescribeTable("records no git command when git is only text",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "truncated %q: %v", command, result.Opacities)
			Expect(result.GitOperations).To(BeEmpty(), "git found in %q", command)
		},

		Entry("zakwas with the words as separate arguments",
			"zakwas propose add-permission review git push"),
		Entry("zakwas with a quoted command line",
			"zakwas propose add-permission deny 'git push --force'"),
		Entry("zakwas with a double-quoted command line",
			`zakwas propose add-permission allow "git commit -m x"`),
		Entry("zakwas with a permission rule",
			"zakwas propose add-permission allow 'Bash(git commit:*)'"),
		Entry("zakwas after another zakwas command",
			"zakwas apply && zakwas propose add-permission review git push"),
		Entry("an unknown program with a git command line", "mytool run git push origin main"),
		Entry("an unknown program with a git option value",
			`mytool --message "git push origin main"`),
		Entry("an unknown program with a git word and a flag", "mytool --flag git push"),
		Entry("an unknown program by path", "./mytool git push"),
		Entry("an unknown program by absolute path", "/opt/tools/mytool git commit -m x"),
		Entry("curl data", "curl -d 'git push origin main' https://example.com"),
		Entry("curl with a header", `curl -H "X-Cmd: git commit -m x" https://example.com`),
		Entry("a task runner hands the words to the task", "mise run test -- git push"),
		Entry("a task runner shorthand", "mise r lint -- git commit -m x"),
		Entry("an npm script", "npm run build -- git push"),
		Entry("a package runner executable", "npx cowsay git push"),
		Entry("make targets", "make git push"),
		Entry("a variable of words given to an unknown program", "FOO='git push'; zakwas x $FOO"),
		Entry("gh with a body", `gh pr comment 1 --body "run git push --force"`),
		Entry("gh issue with a title", `gh issue create --title "git commit fails" --body "x"`),
		Entry("man page lookups", "man git commit"),
		Entry("less search", "less +/'git commit' README"),
		Entry("which", "which git push"),

		Entry("echo", "echo git commit -m x"),
		Entry("echo quoted", `echo "git push origin main"`),
		Entry("echo single quoted", `echo 'git push --force origin main'`),
		Entry("echo -n", "echo -n git push"),
		Entry("printf", `printf '%s\n' 'git push origin main'`),
		Entry("printf to a file", `printf 'git commit -m x\n' > notes.txt`),
		Entry("grep", "grep -rn 'git commit' docs/"),
		Entry("grep without quotes", "grep git push log.txt"),
		Entry("grep in a pipe", "echo foo | grep 'git commit'"),
		Entry("grep then head", `grep -r "git commit" . | head`),
		Entry("rg", `rg "git commit" .`),
		Entry("sed script", "sed -i 's/git commit/git push/' f.txt"),
		Entry("sed print", "sed -n '/git commit/p' README.md"),
		Entry("awk script", "awk '/git commit/ {print}' f"),
		Entry("jq filter", `jq -r '.[] | select(.cmd == "git push")' f.json`),
		Entry("head on a here-string", "head -1 <<< 'git commit -m x'"),
		Entry("read from a here-string", "read -r line <<< 'git commit -m x'"),
		Entry("append to a file", "echo 'git commit' >> notes.md"),
		Entry("a variable holding the words", "VAR='git push origin main'; echo $VAR"),
		Entry("a variable assignment alone", "CMD='git commit -m x'"),
		Entry("printf into a variable", `printf -v cmd '%s' 'git push'`),
		Entry("test with the words", `[ "$1" = "git push" ]`),
		Entry("a case pattern", `case "$1" in "git push") echo no;; esac`),

		Entry("cat heredoc to a file",
			"cat <<EOF > notes.txt\ngit commit -m x\ngit push origin main\nEOF"),
		Entry("cat redirect first", "cat > notes.txt <<'EOF'\ngit commit -m x\nEOF"),
		Entry("cat append", "cat >> notes.md <<'EOF'\n$ git push origin main\nEOF"),
		Entry("tee heredoc", "tee out.txt <<'EOF'\ngit commit -m x\nEOF"),
		Entry("tee of a pipe", "echo 'git commit -m x' | tee out.txt"),
		Entry("cat heredoc piped to tee", "cat <<EOF | tee out.txt\ngit push origin main\nEOF"),
		Entry("cat heredoc alone", "cat <<'EOF'\ngit push origin main\nEOF"),
		Entry("an unknown program fed a heredoc", "mytool <<EOF\ngit commit -m x\nEOF"),
		Entry("an unknown program fed a here-string", "mytool <<< 'git commit -m x'"),
		Entry("a heredoc piped to an unknown program",
			"cat <<EOF | mytool\ngit commit -m x\nEOF"),
		Entry("a script written to a file but not run",
			"cat > hook.sh <<'EOF'\n#!/bin/sh\ngit commit -m x\nEOF\nchmod +x hook.sh"),
		Entry("a markdown file written with a heredoc",
			"tee notes.md <<'EOF'\nUse `git commit -sS`.\nEOF"),
		Entry("a heredoc inside bash -c that is written to a file",
			"bash -c 'cat <<EOF > f.txt\ngit commit -m x\nEOF'"),

		Entry("python print", `python3 -c "print('git commit -m x')"`),
		Entry("python heredoc writing a fixture",
			"python3 - <<'PY'\nfrom pathlib import Path\n"+
				"Path('fixture.txt').write_text('git -C repo commit -qm fresh\\n')\nPY"),
		Entry("python heredoc printing", "python3 - <<'PY'\nprint(\"git commit -m x\")\nPY"),
		Entry("python heredoc dumping json",
			"python3 - <<'PY'\nimport json\n"+
				"open('f.json','w').write(json.dumps({'cmd': 'git commit -m x'}))\nPY"),
		Entry("python open and write",
			`python3 -c 'open("notes.txt", "w").write("git commit -m x")'`),
		Entry("node console.log", `node -e "console.log('git push')"`),
		Entry("node writing a file",
			"node - <<'JS'\nrequire('fs').writeFileSync('f.txt', 'git commit -m x')\nJS"),
		Entry("python writing then listing the directory",
			`python3 -c 'open("n.txt", "w").write("git push")' && ls -l`),
		Entry("python behind sudo", `sudo python3 -c 'open("n.txt", "w").write("git push")'`),
		Entry("python inside bash -c",
			`bash -c 'python3 -c "open(\"n.txt\", \"w\").write(\"git push\")"'`),

		Entry("a trailing comment", "echo hi # git commit -m x"),
		Entry("a comment line", "# git push origin main\necho hi"),
		Entry("a comment after a command", "ls # git push origin main"),
		Entry("a comment block", "# git commit -m x\n# git push\ntrue"),

		Entry("a script with git words in data only", "cd /w && timeout 120 bash sanity.sh"),
		Entry("the same script by path", "/w/sanity.sh"),
		Entry("the same script with sh", "cd /w && sh sanity.sh"),
		Entry("the same script sourced", "cd /w && source sanity.sh"),
	)

	DescribeTable(
		"records the git command that really runs",
		func(command string, want ...string) {
			result := parse(command)

			Expect(subcommands(result)).To(ContainElements(want), "in %q", command)
		},
		Entry("an unsigned commit", "git commit -m x", "commit"),
		Entry("a push to a branch", "git push origin main", "push"),
		Entry("a bare push", "git push", "push"),
		Entry("a force push", "git push --force origin main", "push"),
		Entry("a global option before the subcommand", "git -c user.name=x commit -m x", "commit"),
		Entry("a directory option", "git -C repo commit -m x", "commit"),
		Entry("a directory option and a combined flag", "git -C repo commit -qm fresh", "commit"),
		Entry("a path to git", "/usr/bin/git commit -m x", "commit"),
		Entry("an escaped command name", `\git commit -m x`, "commit"),
		Entry("an upper-case command name", "GIT commit -m x", "commit"),
		Entry("a git-prefixed program", "git-commit -m x", "commit"),
		Entry("hub", "hub commit -m x", "commit"),
		Entry("a git command that mentions the words", "git log --grep 'git push'", "log"),
		Entry("a message that mentions push", "git commit -m 'run git push later'", "commit"),
		Entry("after cd", "cd d && git commit -m x", "commit"),
		Entry("after &&", "true && git push origin main", "push"),
		Entry("after ||", "false || git push origin main", "push"),
		Entry("after ;", "true; git push origin main", "push"),
		Entry("after a newline", "true\ngit push origin main", "push"),
		Entry("in the background", "git push origin main &", "push"),
		Entry("the end of a pipe", "echo x | git push origin main", "push"),
		Entry("the start of a pipe", "git commit -m x | cat", "commit"),
		Entry("a command group", "{ git push origin main; }", "push"),
		Entry("a subshell", "(git commit -m x)", "commit"),
		Entry("a command substitution", "echo $(git commit -m x)", "commit"),
		Entry("a quoted command substitution", `echo "$(git push origin main)"`, "push"),
		Entry("backticks", "echo `git commit -m x`", "commit"),
		Entry("an assignment from a substitution", "VAR=$(git push origin main)", "push"),
		Entry("an if body", "if true; then git push origin main; fi", "push"),
		Entry("a for body", "for i in 1; do git push origin main; done", "push"),
		Entry("a while body", "while true; do git commit -m x; done", "commit"),
		Entry("a case body", "case $x in a) git push origin main;; esac", "push"),
		Entry("a negation", "! git push origin main", "push"),
		Entry("a function body", "f() { git commit -m x; }; f", "commit"),
		Entry("an alias", "alias gc='git commit -m x'; gc", "commit"),
		Entry(
			"a status check before a push",
			"git status && git push origin main",
			"status",
			"push",
		),
		Entry("an assignment prefix", "FOO=1 git commit -m x", "commit"),
		Entry("env with an assignment", "env GIT_X=1 git commit -m x", "commit"),
		Entry("env with options", "env -u HOME git push origin main", "push"),
		Entry("env -S", "env -S 'git push origin main'", "push"),
		Entry("command", "command git push origin main", "push"),
		Entry("builtin command", "builtin command git push origin main", "push"),
		Entry("exec", "exec git push origin main", "push"),
		Entry("sudo", "sudo git push origin main", "push"),
		Entry("sudo with a user", "sudo -u deploy git push origin main", "push"),
		Entry("doas", "doas git push origin main", "push"),
		Entry("time", "time git push origin main", "push"),
		Entry("nohup", "nohup git push origin main", "push"),
		Entry("timeout", "timeout 10 git push origin main", "push"),
		Entry("timeout with options", "timeout -s KILL 10 git push origin main", "push"),
		Entry("gtimeout", "gtimeout 10 git push origin main", "push"),
		Entry("nice", "nice -n 5 git push origin main", "push"),
		Entry("stacked wrappers", "sudo -E env A=1 timeout 5 nohup git push origin main", "push"),
		Entry("xargs", "echo x | xargs git commit -m", "commit"),
		Entry("xargs with options", "echo origin | xargs -n1 git push", "push"),
		Entry("find -exec", `find . -exec git push origin main \;`, "push"),
		Entry("watch", "watch git push origin main", "push"),
		Entry("parallel", "parallel git push ::: origin", "push"),
		Entry("bash -c", "bash -c 'git commit -m x'", "commit"),
		Entry("sh -c", `sh -c "git commit -m x"`, "commit"),
		Entry("zsh -c", "zsh -c 'git push origin main'", "push"),
		Entry("bash -lc", "bash -lc 'git push origin main'", "push"),
		Entry("nested shells", `bash -c "sh -c 'git push origin main'"`, "push"),
		Entry("a shell in a wrapper", "sudo bash -c 'git push origin main'", "push"),
		Entry("a heredoc fed to bash", "bash <<EOF\ngit commit -m x\nEOF", "commit"),
		Entry("a quoted heredoc fed to sh", "sh <<'EOF'\ngit push origin main\nEOF", "push"),
		Entry("a heredoc fed to bash -s", "bash -s <<'EOF'\ngit push origin main\nEOF", "push"),
		Entry("a here-string fed to bash", "bash <<< 'git commit -m x'", "commit"),
		Entry("a here-string fed to zsh", "zsh <<< 'git push origin main'", "push"),
		Entry("a heredoc piped to bash", "cat <<EOF | bash\ngit commit -m x\nEOF", "commit"),
		Entry("a heredoc piped to sh -s", "cat <<EOF | sh -s\ngit commit -m x\nEOF", "commit"),
		Entry("echo piped to bash", "echo 'git commit -m x' | bash", "commit"),
		Entry("printf piped to sh", "printf 'git push origin main\\n' | sh", "push"),
		Entry("eval", "eval 'git push origin main'", "push"),
		Entry("eval with double quotes", `eval "git commit -m x"`, "commit"),
		Entry("a script written then run",
			"cat > f.sh <<'EOF'\ngit commit -m x\nEOF\nbash f.sh", "commit"),
		Entry("a process substitution sourced", "source <(echo 'git push origin main')", "push"),
		Entry("mise exec", "mise exec -- git push origin main", "push"),
		Entry("mise x", "mise x -- git commit -m x", "commit"),
		Entry("npm exec", "npm exec -- git push origin main", "push"),
		Entry("pnpm exec", "pnpm exec git push origin main", "push"),
		Entry("ssh", "ssh host git push origin main", "push"),
		Entry("ssh with a quoted command", "ssh host 'git push origin main'", "push"),
		Entry("docker run", "docker run --rm img git push origin main", "push"),
		Entry("docker exec", "docker exec c git push origin main", "push"),
		Entry("docker entrypoint", "docker run --entrypoint git img push origin main", "push"),
		Entry("nix run", "nix run nixpkgs#git -- push origin main", "push"),
		Entry("tmux", "tmux new-session 'git push origin main'", "push"),
		Entry("kubectl exec", "kubectl exec pod -- git push origin main", "push"),
		Entry("bundle exec", "bundle exec git push origin main", "push"),
		Entry("direnv exec", "direnv exec . git push origin main", "push"),
		Entry("python subprocess list",
			`python3 -c "import subprocess; subprocess.run(['git','push','origin','main'])"`,
			"push"),
		Entry("python os.system",
			`python3 -c "import os; os.system('git push origin main')"`, "push"),
		Entry("python heredoc with os.system",
			"python3 - <<'PY'\nimport os\nos.system('git commit -m x')\nPY", "commit"),
		Entry("python heredoc with subprocess",
			"python3 - <<'PY'\nimport subprocess\nsubprocess.run(['git','commit','-m','x'])\nPY",
			"commit"),
		Entry("python writing a script then running it",
			`python3 -c 'open("/tmp/h.sh", "w").write("git push origin main")'; bash /tmp/h.sh`,
			"push"),
		Entry("node child_process",
			`node -e "require('child_process').execSync('git push origin main')"`, "push"),
		Entry("a script with git in command position", "cd /w && timeout 120 bash real.sh",
			"commit"),
		Entry("the same script by path", "/w/real.sh", "commit"),
	)

	It("keeps the git command of a script that also holds git words as data", func() {
		result := parse("cd /w && bash mixed.sh")

		Expect(subcommands(result)).To(Equal([]string{"commit"}))
	})
})
