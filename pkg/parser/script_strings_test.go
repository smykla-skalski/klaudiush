package parser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// hookFeeder builds hook payloads from git-looking strings and pipes them
// into klaudiush; it never runs git.
const hookFeeder = `import json, subprocess, sys, re

repo = sys.argv[1]
full = open(sys.argv[2]).read().rstrip("\n")
subj = 'git commit -s -S -q -m "fix(git): keep merge body as written"'
cases = [
    ("baseline full", full),
    ("cd path + full", "cd /Users/x/repo/.claude/worktrees/w && " + full),
    ("git -C path + subject", subj.replace("git commit", "git -C /Users/x/w commit")),
    ("git -C path, no 'written'", 'git -C /Users/x/w commit -s -S -q -m "fix(git): keep"'),
    ("git -C path on line before", "cd /Users/x/w\n" + subj),
    ("staged path", "git add internal/x_test.go && " + subj),
]
for name, cmd in cases:
    payload = json.dumps({"session_id": "t", "hook_event_name": "PreToolUse", "tool_name": "Bash",
                          "cwd": repo, "tool_input": {"command": cmd}})
    out = subprocess.run(["/opt/homebrew/bin/klaudiush", "--hook-type", "PreToolUse"], input=payload,
                         capture_output=True, text=True, cwd=repo).stdout
    codes = sorted(set(re.findall(r"GIT0\d\d", out)))
    print(f"{codes} <= {name}")
`

// heredocGen writes gen.py, a script holding f-string git commands, with a
// heredoc whose loop body is body.
func heredocGen(body string) string {
	return "cat > gen.py <<'EOF'\n" +
		"import subprocess\n" +
		"M = 'x'\n" +
		"C = 'y'\n" +
		"cases = [\n" +
		"    (1, True, f'git commit -sS -m \"{M}\"'),\n" +
		"    (2, False, f'git checkout -b feat/{C}-x'),\n" +
		"]\n" +
		"for ac, block, cmd in cases:\n" +
		"    " + body + "\n" +
		"EOF"
}

var _ = Describe("Plain strings in interpreter code", func() {
	resolver := fakeResolver{
		files: map[string]string{
			"/s/feeder.py":  hookFeeder,
			"/s/commit.py":  hookFeeder + "subprocess.run([\"git\", \"commit\", \"-m\", \"x\"])\n",
			"/s/system.py":  hookFeeder + "import os\nos.system(\"git push --force origin main\")\n",
			"/s/helper.py":  "from release_lib import sh\nsh(\"git push --force origin main\")\n",
			"/s/shebang.py": "#!/usr/bin/env python3\nx=\"git zz\"\n",
			"/s/tool.js": "#!/usr/bin/env node\nconst label = \"git zz\";\n" +
				"console.log(`${label}`.length);\n",
		},
		programs: map[string]parser.Program{"git-zz": parser.ProgramMissing},
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

	DescribeTable("reads them as data when nothing in the code can run a string",
		func(command string) {
			result := parse(command)
			Expect(result.Truncated).To(BeFalse(), "truncated %q: %v", command, result.Opacities)
			Expect(result.GitOperations).To(BeEmpty(), "git found in %q", command)
		},
		Entry("a script that feeds hook payloads to klaudiush",
			"python3 -I /s/feeder.py /s/repo /s/cmd1.txt"),
		Entry("a label tuple", `python3 -c 'cases = [("git -C x path + y", 1)]; print(cases)'`),
		Entry("a str.replace fragment",
			`python3 -c 's = "x"; print(s.replace("git commit", "git -C /w commit"))'`),
		Entry("a returned string", "python3 -c 'def c():\n    return \"git zz\"\nprint(c())'"),
		Entry("an unknown call", `python3 -c 'local("git zz")'`),
		Entry("an odd subcommand word in a message", `python3 -c 'print("git z+z")'`),
		Entry(
			"a hook tool name beside stdin input",
			`python3 -c 'import subprocess; subprocess.run(["/bin/true"], input="Bash"); x = "git zz"'`,
		),
		Entry("a node string", "node /s/tool.js"),
		Entry("an asyncio entry point",
			"python3 -c 'import asyncio\nasync def m():\n    print(\"git zz\")\nasyncio.run(m())'"),
		Entry("a compiled regexp", `python3 -c 'import re; p = re.compile("x"); y = "git zz"'`),
		Entry("output written to stdout",
			`python3 -c 'import sys; sys.stdout.write("x"); y = "git zz"'`),
		Entry("an environment variable read",
			`python3 -c 'import os; h = os.environ.get("HOME"); y = "git zz"'`),
		Entry("the interpreter path", `python3 -c 'import sys; print(sys.executable, "git zz")'`),
		Entry("a shellcheck label", `python3 -c 'tools = ["shellcheck", "git zz"]; print(tools)'`),
		Entry("an HTTP client", `python3 -c 'import requests; y = "git zz"'`),
	)

	DescribeTable(
		"still reads them when the code can run a string",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue(), "not truncated: %q", command)
		},
		Entry("a helper module", `python3 -c 'from release_lib import sh; sh("git zz")'`),
		Entry("a relative import", `python3 -c 'from . import sh; sh("git zz")'`),
		Entry("a plain import", `python3 -c 'import invoke; invoke.run("git zz")'`),
		Entry("a split argv", `python3 -c 'import subprocess; subprocess.run("git zz".split())'`),
		Entry("a shell fed on stdin",
			`python3 -c 'import subprocess; subprocess.run(["sh"], input="git zz")'`),
		Entry("a shell path fed on stdin",
			`python3 -c 'import subprocess; subprocess.run(["/bin/bash"], input="git zz")'`),
		Entry(
			"a -c flag beside a computed shell",
			`python3 -c 'import subprocess, os; subprocess.run([os.environ["X"], "-c", "git zz"])'`,
		),
		Entry("a name looked up at run time",
			`python3 -c 'import os; getattr(os, "sys" + "tem")("git zz")'`),
		Entry("os.system imported under another name",
			`python3 -c 'from os import system as go; go("git zz")'`),
		Entry("a node module", `node -e 'require("./lib").run("git zz")'`),
		Entry("a computed node module", `node -e 'require(m).run("git zz")'`),
		Entry("an ES module", `node -e 'import { $ } from "zx"; const c = "git zz"'`),
		Entry("a node template literal", "node -e 'const c = `git zz`'"),
		Entry("a ruby string", `ruby -e 'x = "git zz"'`),
		Entry("output piped to a shell", `python3 -c 'x = "git zz"; print(x)' | sh`),
		Entry("getstatusoutput",
			`python3 -c 'import subprocess; subprocess.getstatusoutput("git zz")'`),
		Entry("getstatusoutput imported under another name",
			`python3 -c 'from subprocess import getstatusoutput as g; g("git zz")'`),
		Entry("subprocess under another name",
			`python3 -c 'import subprocess as sp; sp.getstatusoutput("git zz")'`),
		Entry("an unknown subprocess attribute",
			`python3 -c 'import subprocess; f = subprocess.getoutpu; x = "git zz"'`),
		Entry("the subprocess module passed on",
			`python3 -c 'import subprocess; m = subprocess; x = "git zz"'`),
		Entry("a script written and run",
			`python3 -c 'import pathlib, subprocess; pathlib.Path("/tmp/h").`+
				`write_text("git zz"); subprocess.run(["/tmp/h"])'`),
		Entry("a JSON dump into a file",
			`python3 -c 'import json; json.dump("git zz", open("/tmp/h", "a"))'`),
		Entry("an editor variable git runs",
			`python3 -c 'import os, subprocess; os.environ["GIT_EDITOR"] = "git zz"; `+
				`subprocess.run(["git", "commit"])'`),
		Entry("a scheduler fed on stdin",
			`python3 -c 'import subprocess; subprocess.run(["at", "now"], input="git zz")'`),
		Entry("a capitalized shell fed on stdin",
			`python3 -c 'import subprocess; subprocess.run(["Bash"], input="git zz")'`),
		Entry("a launcher before a shell",
			`python3 -c 'import subprocess; subprocess.run(["nohup", "zsh"], input="git zz")'`),
		Entry("an argv held in a variable",
			`python3 -c 'import subprocess; a = ["sh"]; subprocess.run(a, input="git zz")'`),
		Entry("a computed program",
			`python3 -c 'import subprocess, sys; subprocess.run([sys.argv[1]], input="git zz")'`),
		Entry("a bare imported call",
			`python3 -c 'from subprocess import run; run(cmd, input="git zz")'`),
		Entry("a continued import line",
			"python3 -c 'import json, \\\n    helper\nhelper.go(\"git zz\")'"),
		Entry("a computed child_process member",
			`node -e 'const cp = require("child_process"); cp["ex" + "ecSync"]("git zz")'`),
		Entry(
			"a deno shell argv",
			`deno eval 'const c = "git zz"; Deno.run({cmd: ["sh", "-c", c]})'`,
		),
		Entry(
			"a wrapper with a command flag",
			`python3 -c 'import subprocess; subprocess.run(["runuser", "-l", "bob", "-c", "git zz"])'`,
		),
		Entry("an unlisted wrapper with a command flag",
			`python3 -c 'import subprocess; c = "git zz"; subprocess.run(["newtool", "-c", c])'`),
		Entry("an attribute fetched by attrgetter",
			`python3 -c 'import operator, os; operator.attrgetter("sys" + "tem")(os)("git zz")'`),
		Entry("a computed node global",
			`node -e 'globalThis["req" + "uire"]("./h").run("git zz")'`),
		Entry("subprocess.run handed to partial",
			`python3 -c 'import functools, subprocess; `+
				`functools.partial(subprocess.run, ["bash"], input="git zz")()'`),
		Entry("subprocess.run handed to a thread",
			`python3 -c 'import asyncio, subprocess; `+
				`asyncio.run(asyncio.to_thread(subprocess.run, ["bash"], input="git zz"))'`),
		Entry("subprocess.run under an assigned name",
			`python3 -c 'import subprocess; r = subprocess.run; r(["bash"], input="git zz")'`),
		Entry("a spawn function imported by name",
			`python3 -c 'from subprocess import run; f = run; f(["sh"], input="git zz")'`),
		Entry("tee writing stdin to a file",
			`python3 -c 'import subprocess; subprocess.run(["tee", "/tmp/h"], input="git zz")'`),
		Entry("stdout redirected into a file",
			`python3 -c 'import subprocess; `+
				`subprocess.run(["/bin/echo"], input="git zz", stdout=open("/tmp/h", "w"))'`),
		Entry("a command line handed to an unknown wrapper",
			`python3 -c 'import subprocess; subprocess.run(["hyperfine", "git zz"])'`),
		Entry("a python-shebang file run by bash", "bash /s/shebang.py"),
		Entry("a python-shebang file sourced", "source /s/shebang.py"),
		Entry("a log file handler",
			`python3 -c 'import logging; logging.FileHandler("/tmp/h"); logging.info("git zz")'`),
		Entry("a make clone fed on stdin",
			`python3 -c 'import subprocess; subprocess.run(["gmake", "-f", "-"], input="git zz")'`),
		Entry("a comment inside the argv",
			"python3 -c 'import subprocess\nsubprocess.run([\"/bin/echo\",  # x\n"+
				"    \"bash\"], input=\"git zz\")'"),
		Entry(
			"an escaped quote inside the argv",
			`python3 -c 'import subprocess; subprocess.run(["/opt/w", "a\"b", "bash"], input="git zz")'`,
		),
		Entry(
			"a yaml dump into a path opened for writing",
			`python3 -c 'import yaml, pathlib; yaml.safe_dump("git zz", pathlib.Path("/s/x").open("w"))'`,
		),
		Entry(
			"a command string a git subcommand runs",
			`python3 -c 'import subprocess; c = "git zz"; subprocess.run(["git", "submodule", "foreach", c])'`,
		),
		Entry("a command string an unknown wrapper runs",
			`python3 -c 'import subprocess; c = "git zz"; subprocess.run(["devbox", "run", c])'`),
		Entry(
			"an argv picked by an expression",
			`python3 -c 'import subprocess; subprocess.run(["true"] if u else ["sh"], input="git zz")'`,
		),
		Entry("a remote shell fed on stdin",
			`python3 -c 'import subprocess; subprocess.run(["oc", "rsh", "pod"], input="git zz")'`),
	)

	DescribeTable(
		"reads a heredoc-written script like one on disk",
		func(command string) {
			result := parse(command)
			Expect(result.Truncated).To(BeFalse(), "truncated %q: %v", command, result.Opacities)
			Expect(result.GitOperations).To(BeEmpty(), "git found in %q", command)
		},
		Entry(
			"a script that prints f-string commands",
			heredocGen("print(cmd)")+"\npython3 gen.py",
		),
		Entry("the heredoc write alone", heredocGen("print(cmd)")),
	)

	DescribeTable(
		"still records git a heredoc really runs",
		func(command, subcommand string) {
			Expect(hasGit(parse(command), subcommand, "")).
				To(BeTrue(), "no git %s in %q", subcommand, command)
		},
		Entry("a written script that runs its strings",
			heredocGen("subprocess.run(cmd, shell=True)")+"\npython3 gen.py", "checkout"),
		Entry("bash reading a heredoc", "bash <<'EOF'\ngit push --force origin main\nEOF", "push"),
		Entry(
			"sh -s reading a heredoc",
			"sh -s <<'EOF'\ngit push --force origin main\nEOF",
			"push",
		),
		Entry("python reading a heredoc that calls os.system",
			"python3 - <<'EOF'\nimport os\nos.system('git push --force origin main')\nEOF", "push"),
	)

	DescribeTable("still records git the code really runs",
		func(command, subcommand, flag string) {
			Expect(hasGit(parse(command), subcommand, flag)).
				To(BeTrue(), "no git %s in %q", subcommand, command)
		},
		Entry("an argv list in a script file", "python3 -I /s/commit.py r c", "commit", ""),
		Entry("os.system in a script file", "python3 /s/system.py r c", "push", "--force"),
		Entry("a helper module call in a script file", "python3 /s/helper.py", "push", "--force"),
		Entry("an argv list in -c code",
			`python3 -c "import subprocess; subprocess.run(['git', 'commit', '-m', 'x'])"`,
			"commit", ""),
		Entry("bash -c", `bash -c 'git push --force origin main'`, "push", "--force"),
		Entry("sh -c", `sh -c 'git push --force origin main'`, "push", "--force"),
		Entry("eval", `eval 'git push --force origin main'`, "push", "--force"),
		Entry("a python print piped to sh",
			`python3 -c 'print("git push --force origin main")' | sh`, "push", "--force"),
	)
})
