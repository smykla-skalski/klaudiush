package parser_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const chainLength = 10

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
		"/s/a.py":     "import os\nos.system('sh -c \"python3 /s/b.py\"')\n",
		"/s/b.py": "import os\nos.system('sh -c \"python3 /s/a.py\"')\n" +
			"os.system('git push --force')\n",
		"/s/move.sh": "if [ \"$1\" = inner ]; then git push --force; " +
			"else cd /other && bash /s/move.sh inner; fi\n",
		"/s/vars.sh": "git status\nX=1 bash /s/vars.sh\n",
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

	It("still fails closed on a chain of distinct scripts past the limit", func() {
		result := parse("bash /s/chain0.sh")

		Expect(result.Truncated).To(BeTrue())
		Expect(result.Opacities).NotTo(BeEmpty())
		Expect(result.Opacities[0].Cause).To(Equal(parser.OpacityDepthLimit))
		Expect(strings.Join(result.Opacities[0].Origin, " ")).To(HavePrefix("bash"))
	})
})
