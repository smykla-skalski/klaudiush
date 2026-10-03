package parser_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const (
	chainLength     = 10
	manyWrites      = 1500
	manyCalls       = 20
	manyAssignments = 160
	largeValue      = 60 << 10
)

var _ = Describe("Scripts that run themselves", func() {
	files := map[string]string{
		"/s/file_issues.py": `#!/usr/bin/env python3
"""File issues with gh.

Usage:
    python3 file_issues.py --dry-run
"""
import subprocess
HELP = "rerun: python3 file_issues.py --apply, then check gh"

def main():
    print("python3 file_issues.py --apply  # files via gh")
    subprocess.run(["gh", "issue", "create", "--title", "x", "--body", "y"])
`,
		"/s/push.py": "import subprocess\n" +
			"print('python3 /s/push.py; git log')\n" +
			"subprocess.run(['git', 'push', '--force'])\n",
		"/s/again.sh": "git status\nbash /s/again.sh\n",
		"/s/a.py":     "import os\nos.system('python3 /s/b.py  # then gh')\n",
		"/s/b.py": "import os\nos.system('python3 /s/a.py  # then gh')\n" +
			"os.system('git push --force')\n",
		"/s/move.sh": "if [ \"$1\" = inner ]; then git push --force; " +
			"else cd /other && bash /s/move.sh inner; fi\n",
		"/s/vars.sh":  "git status\nX=1 bash /s/vars.sh\n",
		"/s/hash.sh":  "ls push --force\nhash -p /usr/bin/git ls\n. /s/hash.sh\n",
		"/s/alias.sh": "git config alias.ls push\ngit ls --force\n. /s/alias.sh\n",
		"/s/staged.sh": "[ \"$SHLVL\" -gt 4 ] || bash /s/staged.sh\nbash /t/y.sh\nbash /t/x.sh\n" +
			"bash /t/w.sh\ncat > /t/w.sh <<'EOF'\ncat > /t/x.sh <<'EOT'\ncat > /t/y.sh <<'EOU'\n" +
			"git push --force\nEOU\nEOT\nEOF\n",
		"/s/q1.py": "#!/usr/bin/env python3\n\"\"\"Usage:\n    python3 q1.py\n" +
			"    sh -c 'python3 q1.py'\n\"\"\"\nimport subprocess\n" +
			"subprocess.run([\"gh\", \"issue\", \"list\"])\n",
		"/s/sh2.sh": "gh issue list\nbash /s/sh2.sh\nsh -c 'bash /s/sh2.sh'\n",
		"/s/cond.sh": "[ \"$SHLVL\" -gt 4 ] || bash /s/cond.sh\nbash /t/c\n" +
			"cat > /t/c <<'X'\ngit push --force\nX\n[ -e /x ] && cat > /t/c <<'Z'\n\nZ\n",
		"/s/true.sh": "true\n",
		"/s/unknowndir.sh": "bash /t/run.sh\ncd \"$(printf /t)\"\n" +
			"printf 'git push --force\\n' > run.sh\nbash /s/unknowndir.sh\n",
		"/s/ghflag.sh": "gh pr list\ngh x\ngh alias set x 'repo delete foo --yes' --clobber\n" +
			"bash /s/ghflag.sh\ngh alias set --clobber x 'pr list'\n",
		"/s/gitflag.sh": "git status\ngit x\ngit config alias.x 'push --force'\n" +
			"bash /s/gitflag.sh\ngit -C alias.q config alias.x status\n",
		"/s/g.sh": "git x\ngit config alias.x 'push --force'\n" +
			"[ \"$SHLVL\" -gt 3 ] || bash /s/g.sh\ngit config alias.x status\n",
		"/s/top.sh": "bash /t/f.sh\ncat > /t/f.sh <<'EOF'\n# nothing\nEOF\nbash /s/rewrite.sh\n",
		"/s/rewrite.sh": "bash /t/f.sh\ncat > /t/f.sh <<'EOF'\ngit push --force\nEOF\n" +
			"bash /s/rewrite.sh\n",
	}

	for i := range chainLength {
		files[fmt.Sprintf("/s/chain%d.sh", i)] = fmt.Sprintf("bash /s/chain%d.sh\n", i+1)
	}

	files[fmt.Sprintf("/s/chain%d.sh", chainLength)] = "git push --force\n"

	resolver := fakeResolver{files: files}

	parse := func(command string) *parser.ParseResult {
		result, err := parser.NewBashParserWithResolver(resolver).Parse(command)
		Expect(err).NotTo(HaveOccurred())

		return result
	}

	gitRuns := func(result *parser.ParseResult, sub string) []parser.Command {
		var found []parser.Command

		for _, op := range result.GitOperations {
			if len(op.Args) > 0 && op.Args[0] == sub {
				found = append(found, op)
			}
		}

		return found
	}

	DescribeTable("inspects a script that names itself without failing closed",
		func(command string) {
			result := parse(command)

			Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
			Expect(result.HasCommand("gh")).To(BeTrue())
		},
		Entry("python script by absolute path", "python3 /s/file_issues.py --dry-run"),
		Entry("python script after cd", "cd /s && python3 file_issues.py --dry-run"),
		Entry("python script by path from its directory", "cd /s && python3 /s/file_issues.py"),
		Entry("python script naming itself directly and through sh -c", "cd /s && python3 q1.py"),
		Entry("shell script rerunning itself directly and through sh -c", "bash /s/sh2.sh"),
	)

	It("still sees the git command a self-naming python script runs", func() {
		result := parse("python3 /s/push.py")

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		Expect(gitRuns(result, "push")).NotTo(BeEmpty())
	})

	It("follows a shell script that reruns itself once", func() {
		result := parse("bash /s/again.sh")

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		Expect(gitRuns(result, "status")).NotTo(BeEmpty())
	})

	It("follows scripts that run each other", func() {
		result := parse("python3 /s/a.py")

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		Expect(gitRuns(result, "push")).NotTo(BeEmpty())
	})

	It("follows a script into itself again after it changes directory", func() {
		result := parse("cd /repo && bash /s/move.sh")

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)

		pushes := gitRuns(result, "push")
		dirs := make([]string, 0, len(pushes))

		for _, op := range pushes {
			dirs = append(dirs, op.WorkingDirectory)
		}

		Expect(dirs).To(ContainElements("/repo", "/other"))
	})

	It("follows a script into itself again after an assignment", func() {
		result := parse("bash /s/vars.sh")

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
		Expect(len(gitRuns(result, "status"))).To(BeNumerically(">=", 2))
	})

	It("follows a sourced script into itself again after it changes the command table", func() {
		result := parse(". /s/hash.sh")

		Expect(gitRuns(result, "push")).NotTo(BeEmpty(), "truncated=%v", result.Truncated)
	})

	It("follows a sourced script into itself again after it records new commands", func() {
		result := parse(". /s/alias.sh")

		Expect(result.HasGitCommand()).To(BeTrue())
		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})

	It("follows a script into itself again after it rewrites a file it runs", func() {
		result := parse("bash /s/top.sh")

		Expect(gitRuns(result, "push")).NotTo(BeEmpty(), "truncated=%v", result.Truncated)
	})

	It("fails closed on a script whose later passes stage a hidden command", func() {
		result := parse("bash /s/staged.sh")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities[0].Cause).To(Equal(parser.OpacityDepthLimit))
	})

	It("fails closed on a self-running script whose file writes are conditional", func() {
		result := parse("bash /s/cond.sh")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities[0].Cause).To(Equal(parser.OpacityDepthLimit))
	})

	It("fails closed on a self-running script that redefines a git alias", func() {
		result := parse("sh -c 'git config alias.x status; bash /s/g.sh'")

		Expect(result.Truncated).To(BeTrue())
	})

	DescribeTable("fails closed on a self-running script that redefines an alias with moved flags",
		func(command string) {
			Expect(parse(command).Truncated).To(BeTrue())
		},
		Entry("gh", `bash -c 'gh alias set --clobber x "pr list"; bash /s/ghflag.sh'`),
		Entry("git", `bash -c 'git -C alias.q config alias.x status; bash /s/gitflag.sh'`),
	)

	It("does not cut the repeat of a script that moved to an unknown directory", func() {
		result := parse("bash -c 'bash /s/unknowndir.sh'")

		Expect(result.Truncated).To(BeTrue())
	})

	It("still allows a script run after a cd to a computed directory", func() {
		result := parse(`cd "$(git rev-parse --show-toplevel)" && python3 tools/x.py`)

		Expect(result.Truncated).To(BeFalse(), "opacities: %v", result.Opacities)
	})

	It("keeps following scripts cheap with large definitions in scope", func() {
		var line strings.Builder

		value := strings.Repeat("v", largeValue)
		for i := range manyAssignments {
			fmt.Fprintf(&line, "V%d='%s'; ", i, value)
		}

		for range manyCalls {
			line.WriteString("bash /s/true.sh; ")
		}

		start := time.Now()
		result := parse(line.String())

		Expect(result.Truncated).To(BeFalse())
		Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
	})

	It("keeps following scripts cheap after many file writes", func() {
		var line strings.Builder

		for i := range manyWrites {
			fmt.Fprintf(&line, "echo x > /t/a%d; ", i)
		}

		for range manyCalls {
			line.WriteString("bash /s/true.sh; ")
		}

		start := time.Now()
		result := parse(line.String())

		Expect(result.Truncated).To(BeFalse())
		Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second))
	})

	It("still fails closed on a chain of distinct scripts past the limit", func() {
		result := parse("bash /s/chain0.sh")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).NotTo(BeEmpty())
		Expect(result.Opacities[0].Cause).To(Equal(parser.OpacityDepthLimit))
		Expect(strings.Join(result.Opacities[0].Origin, " ")).To(HavePrefix("bash"))
	})
})
