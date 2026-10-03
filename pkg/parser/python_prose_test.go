package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("Git words in interpreter prose", func() {
	resolver := fakeResolver{
		files: map[string]string{
			"./tool.py": "#!/usr/bin/env python3\nfail(\"git zz is not set up\")\n",
			"helper.py": "print(\"git zz is not set up\")\n",
			"hotspots.py": `#!/usr/bin/env python3
"""Rank hotspots from git history.

Exit codes:
    2  usage error, git unavailable, or not inside a git work tree.
"""
import shutil
import subprocess

GIT_EXECUTABLE = shutil.which("git") or "git"


def run_git(args):
    """Run a git subcommand with a fixed argument list."""
    try:
        result = subprocess.run([GIT_EXECUTABLE, *args], capture_output=True)
    except FileNotFoundError:
        fail("git executable not found on PATH")
    if result.returncode != 0:
        fail("git is required and must run inside a git work tree")
    return subprocess.run(["git", "log", "--name-only"]).stdout
`,
		},
		programs: map[string]parser.Program{
			"git-executable": parser.ProgramMissing,
			"git-is":         parser.ProgramMissing,
			"git-zz":         parser.ProgramMissing,
			"git-pf":         parser.ProgramMissing,
		},
	}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	hasGit := func(result *parser.ParseResult, subcommand, flag string) bool {
		for _, cmd := range result.GitOperations {
			gitCmd, err := parser.ParseGitCommand(cmd)
			if err == nil && gitCmd.Subcommand == subcommand &&
				(flag == "" || gitCmd.HasFlag(flag)) {
				return true
			}
		}

		return false
	}

	DescribeTable("reads messages and docstrings as prose, not commands",
		func(command string) {
			result := parse(command)
			Expect(result.Truncated).To(BeFalse(), "truncated %q: %v", command, result.Opacities)

			for _, cmd := range result.GitOperations {
				Expect(cmd.Args).NotTo(ContainElement(BeElementOf("executable", "is")))
			}
		},
		Entry("a script calling git through argv lists", "python3 hotspots.py --since 1.month"),
		Entry("an error message", `python3 -c 'print("git executable not found on PATH")'`),
		Entry("a docstring", `python3 -c '"""git is required here."""'`),
		Entry("a message in a node program", `node -e 'console.error("git is missing")'`),
	)

	It("still records the real git calls of a script with prose", func() {
		Expect(hasGit(parse("python3 hotspots.py"), "log", "")).To(BeTrue())
	})

	DescribeTable(
		"still inspects git run from a string",
		func(command, subcommand, flag string) {
			Expect(
				hasGit(parse(command), subcommand, flag),
			).To(BeTrue(), "no git %s in %q", subcommand, command)
		},
		Entry(
			"a shell=True string",
			`python3 -c 'import subprocess; subprocess.run("git push --force", shell=True)'`,
			"push",
			"--force",
		),
		Entry("os.system", `python3 -c 'import os; os.system("git push --force origin main")'`,
			"push", "--force"),
		Entry(
			"a string after a prose message",
			`python3 -c 'print("git is slow"); import os; os.system("git commit -m x")'`,
			"commit",
			"",
		),
	)

	DescribeTable(
		"fails closed on a git word it cannot resolve",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), "not truncated: %q", command)
		},
		Entry("an argv list", `python3 -c 'import subprocess; subprocess.run(["git", "zz"])'`),
		Entry(
			"a shell started from an argv list",
			`python3 -c 'import subprocess; subprocess.run(["sh", "-c", "git zz"])'`,
		),
		Entry("a string with git options", `python3 -c 'import os; os.system("git -C /repo zz")'`),
		Entry(
			"a string under a moved HOME",
			`python3 -c 'import os; os.system("HOME=/tmp/h git zz")'`,
		),
		Entry(
			"code that sets HOME",
			`python3 -c 'import os; os.environ["HOME"] = "/tmp/h"; os.system("git zz")'`,
		),
		Entry(
			"code that sets config parameters",
			`python3 -c 'import os; os.environ["GIT_CONFIG_PARAMETERS"] = "x"; os.system("git pf")'`,
		),
		Entry(
			"code that sets an alias",
			`python3 -c 'import os, subprocess; subprocess.run(["git", "config", "alias.pf", "push --force"]); os.system("git pf")'`,
		),
		Entry("code that writes a config file",
			`python3 -c 'open(".git/config", "a").write("x"); import os; os.system("git pf")'`),
		Entry("code that changes directory",
			`python3 -c 'import os; os.chdir("/repo"); os.system("git pf")'`),
		Entry("a call run in another directory",
			`python3 -c 'import subprocess; subprocess.run("git pf", shell=True, cwd="/repo")'`),
		Entry(
			"a string that changes directory",
			`python3 -c 'import os; os.system("cd \"$D\" && git zz")'`,
		),
		Entry(
			"code that changes PATH",
			`python3 -c 'import os; os.environ["PATH"] = "./bin:" + os.environ["PATH"]; os.system("git zz")'`,
		),
		Entry(
			"code that passes PATH to a call",
			`python3 -c 'import os, subprocess; subprocess.run("git zz", shell=True, env=dict(os.environ, PATH="./bin"))'`,
		),
		Entry("a node program that changes PATH",
			`node -e 'process.env.PATH = "./bin"; require("child_process").execSync("git zz")'`),
		Entry("code that sets the exec path",
			`python3 -c 'import os; os.environ["GIT_EXEC_PATH"] = "./bin"; os.system("git zz")'`),
		Entry(
			"a brace the shell expands",
			`python3 -c 'import os; os.system("git {push,} --force")'`,
		),
		Entry(
			"a glob the shell expands",
			`python3 -c 'import os; os.system("git pu[s]h --force")'`,
		),
		Entry("os.system", `python3 -c 'import os; os.system("git zz")'`),
		Entry(
			"a shell=True call",
			`python3 -c 'import subprocess; subprocess.run("git zz", shell=True)'`,
		),
		Entry(
			"a call split across lines",
			"python3 -c 'import subprocess\nsubprocess.run(\n    \"git zz\",\n    shell=True)'",
		),
		Entry("a wrapper that runs commands", `python3 -c 'run_cmd("git zz")'`),
		Entry("an assigned command", `python3 -c 'CMD = "git zz"; import os; os.system(CMD)'`),
		Entry("a returned command", "python3 -c 'def c():\n    return \"git zz\"'"),
		Entry("a triple-quoted command", `python3 -c 'import os; os.system("""git zz""")'`),
		Entry("a second argument", `python3 -c 'print("x", "git zz")'`),
		Entry("node execSync", `node -e 'require("child_process").execSync("git zz")'`),
		Entry("a perl string", `perl -e 'print "git zz"'`),
		Entry(
			"a script that installs the command it runs",
			`python3 -c 'import os; p = os.path.expanduser("~/.local/bin/git-" + "zz"); os.system("git zz")'`,
		),
		Entry(
			"a config variable the guard does not list",
			`python3 -c 'import os; os.environ["GIT_COMMON_DIR"] = "/tmp/e"; os.system("git zz")'`,
		),
	)

	DescribeTable(
		"reads text in these places as prose",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse(), "truncated: %q", command)
		},
		Entry("a message call", `python3 -c 'fail("git zz is not set up")'`),
		Entry("an f-string message", `python3 -c 'fail(f"git zz failed: {err}")'`),
		Entry("a raised error", `python3 -c 'raise RuntimeError("git zz is not set up")'`),
		Entry("a function docstring", "python3 -c 'def f():\n    \"\"\"git zz helper.\"\"\"\n'"),
		Entry(
			"a module docstring after a comment",
			"python3 -c '# tool\n\"\"\"git zz helper.\"\"\"\n'",
		),
		Entry("a console message", `node -e 'console.log("git zz is not set up")'`),
		Entry("a script run by its shebang", "./tool.py"),
		Entry(
			"a message beside output reads",
			`python3 -c 'import sys; out = r.stdout; sys.stdout.write(out); fail("git zz failed")'`,
		),
	)

	DescribeTable(
		"fails closed on a git word in a string some call may run",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), "not truncated: %q", command)
		},
		Entry("ruby backticks in a call", "ruby -e 'x = JSON.parse(`git zz`)'"),
		Entry("perl backticks printed", "perl -e 'print(`git zz`)'"),
		Entry("perl readpipe", `perl -e 'my $o = readpipe("git zz")'`),
		Entry("ruby Open3", `ruby -e 'Open3.capture2("git zz")'`),
		Entry("a fabric runner", `python3 -c 'local("git zz")'`),
		Entry("sudo", `python3 -c 'sudo("git zz")'`),
		Entry("a dict value after a colon", "python3 -c 'C = {\"a\":\n    \"\"\"git zz\"\"\"}'"),
		Entry("a docstring-like string in a call", "python3 -c 'run(\n\"\"\"git zz\"\"\")'"),
		Entry("an unusual subcommand word", `python3 -c 'print("git z+z")'`),
		Entry("awk print piped to a shell", `awk 'BEGIN { print("git zz") | "sh" }'`),
		Entry("perl print to a shell", `perl -e 'open(SH, "|sh"); select SH; print("git zz\n")'`),
		Entry(
			"print to a process stdin",
			`python3 -c 'import subprocess as s; p = s.Popen("sh", stdin=s.PIPE); print("git zz", file=p.stdin)'`,
		),
		Entry("print to a replaced stdout",
			`python3 -c 'import os, sys; sys.stdout = os.popen("sh", "w"); print("git zz")'`),
		Entry("ruby puts to a shell", `ruby -e 'IO.popen("sh", "w") { |io| io.puts("git zz") }'`),
		Entry("a message piped to a shell", `python3 -c 'print("git zz")' | sh`),
		Entry("a message piped through another command", `python3 -c 'print("git zz")' | cat | sh`),
		Entry("a message in a command substitution", `sh -c "$(python3 -c 'print("git zz")')"`),
		Entry("a message in a process substitution", `sh <(python3 -c 'print("git zz")')`),
		Entry("a nested message piped to a shell", `bash -c "python3 -c 'print(\"git zz\")'" | sh`),
		Entry(
			"cd printed into a shell",
			`awk 'BEGIN { print("cd /tmp/e") | "sh"; print("git zz") | "sh" }'`,
		),
		Entry("ruby interpolating backticks", "ruby -e 'puts(\"x#{`git zz`}\")'"),
		Entry("perl interpolating backticks", "perl -e 'print(\"@{[`git zz`]}\")'"),
		Entry("a ruby docstring-like interpolation", "ruby -e '\"\"\"\nx#{`git zz`}\n\"\"\"'"),
		Entry("output sent to a process substitution", `python3 -c 'print("git zz")' > >(sh)`),
		Entry("output after exec to a process substitution",
			`exec > >(sh); python3 -c 'print("git zz")'`),
		Entry("output after exec of another descriptor",
			`exec 3> >(sh); python3 -c 'print("git zz")' >&3`),
		Entry(
			"output after a coprocess",
			`coproc sh; python3 -c 'print("git zz")' >&"${COPROC[1]}"`,
		),
		Entry(
			"a message in a language without prose rules",
			`perl -e 'die("git zz is not set up")'`,
		),
		Entry("gawk printing to a coprocess", `gawk 'BEGIN { print("git zz") |& "sh" }'`),
		Entry("awk piping to a variable", `awk 'BEGIN { c = "sh"; print("git zz") | c }'`),
		Entry("php buffering into system", `php -r 'ob_start("system"); echo("git zz");'`),
		Entry("perl reopening stdout", `perl -e 'open(STDOUT, q{|sh}); print("git zz")'`),
		Entry("python duplicating a pipe onto stdout",
			`python3 -c 'import os; r, w = os.pipe(); os.dup2(w, 1); print("git zz")'`),
		Entry(
			"a wrapped interpreter piped to a shell",
			`timeout 5 python3 -c 'print("git zz")' | sh`,
		),
		Entry(
			"env before an interpreter piped to a shell",
			`env python3 -c 'print("git zz")' | bash`,
		),
		Entry("xargs running an interpreter piped to a shell",
			`echo x | xargs python3 -c 'print("git zz")' | sh`),
		Entry("output after exec inside a function",
			`f() { exec > >(sh); }; f; python3 -c 'print("git zz")'`),
		Entry(
			"output after a sourced exec",
			`source <(echo 'exec > >(sh)'); python3 -c 'print("git zz")'`,
		),
	)

	DescribeTable("still reads prose when the output is only displayed",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse(), "truncated: %q", command)
		},
		Entry("no pipe", `python3 -c 'print("git zz is not set up")'; echo done`),
		Entry("a pipe into head", `python3 -c 'print("git zz is not set up")' | head -5`),
		Entry("a pipe into jq and grep", `python3 hotspots.py | jq -c . | grep x`),
	)

	DescribeTable(
		"treats a filter chain ending in a program as captured",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), "not truncated: %q", command)
		},
		Entry("head then a shell", `python3 -c 'print("git zz")' | head | sh`),
		Entry(
			"sort, which can run a program",
			`python3 -c 'print("git zz")' | sort --compress-program=sh`,
		),
		Entry("a pipe in a command string the interpreter runs",
			`python3 -c 'import subprocess; subprocess.run("python3 helper.py | sh", shell=True)'`),
		Entry("a filter name redefined as a function",
			`grep() { sh; }; python3 -c 'print("git zz")' | grep`),
		Entry("gettext handed to an exec call", `python3 -c 'import os; os.system(_("git zz"))'`),
		Entry("an exception turned back into a string",
			`python3 -c 'import os; os.system(str(ValueError("git zz")))'`),
		Entry("a call named in a trailing comment",
			"python3 -c 'import os\nos.system(  # print(\n    \"git zz\")'"),
		Entry("a module docstring read back",
			"python3 -c '\"\"\"git zz\"\"\"\nimport os; os.system(__doc__)'"),
		Entry("a function docstring read back",
			"python3 -c 'def f():\n    \"\"\"git zz\"\"\"\nimport os; os.system(f.__doc__)'"),
		Entry(
			"a raised message caught and run",
			"python3 -c 'import subprocess\ntry:\n    raise RuntimeError(\"git zz\")\nexcept Exception as e:\n    subprocess.run(str(e), shell=True)'",
		),
		Entry("a default argument split after a colon",
			"python3 -c 'def run(cmd={\"k\":\n\"\"\"git zz\"\"\"}):\n    pass'"),
		Entry("output written into a named pipe",
			`mkfifo p; sh < p & python3 -c 'print("git zz")' > p`),
	)

	DescribeTable(
		"reads raised and thrown errors as prose",
		func(command string) {
			Expect(parse(command).Truncated).To(BeFalse(), "truncated: %q", command)
		},
		Entry(
			"a qualified python error",
			`python3 -c 'raise errors.SetupError("git zz is not set up")'`,
		),
		Entry("a thrown js error", `node -e 'throw new Error("git zz is not set up")'`),
	)
})
