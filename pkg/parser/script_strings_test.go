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

var _ = Describe("Plain strings in interpreter code", func() {
	resolver := fakeResolver{
		files: map[string]string{
			"/s/feeder.py": hookFeeder,
			"/s/commit.py": hookFeeder + "subprocess.run([\"git\", \"commit\", \"-m\", \"x\"])\n",
			"/s/system.py": hookFeeder + "import os\nos.system(\"git push --force origin main\")\n",
			"/s/helper.py": "from release_lib import sh\nsh(\"git push --force origin main\")\n",
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
	)

	DescribeTable("still reads them when the code can run a string",
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
