package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const dataOnlyScript = `#!/usr/bin/env bash
set -euo pipefail
echo "git commit -m data"
cat > fixture.txt <<EOF
git commit -m heredoc-to-a-file
EOF
python3 - <<PY
from pathlib import Path
Path("fixture2.txt").write_text("git -C repo commit -qm fresh\n")
PY
zakwas propose add-permission review git push
grep -q "git push" log.txt
`

var _ = Describe("Git validators and command position", func() {
	var (
		commitValidator *git.CommitValidator
		pushValidator   *git.PushValidator
		scriptDir       string
	)

	BeforeEach(func() {
		log := logger.NewNoOpLogger()

		fakeGit := gitpkg.NewFakeRunner()
		fakeGit.StagedFiles = []string{"file.txt"}
		fakeGit.CurrentBranch = "master"

		pushCfg := &config.PushValidatorConfig{}
		pushCfg.BlockedBranches = []string{"main", "master"}

		commitValidator = git.NewCommitValidator(log, fakeGit, nil, nil)
		pushValidator = git.NewPushValidator(log, fakeGit, pushCfg, nil)

		scriptDir = GinkgoT().TempDir()
		Expect(os.WriteFile(
			filepath.Join(scriptDir, "sanity.sh"), []byte(dataOnlyScript), 0o600,
		)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(scriptDir, "real.sh"),
			[]byte("#!/usr/bin/env bash\ngit commit -m real\ngit push origin main\n"),
			0o600,
		)).To(Succeed())
	})

	run := func(v validator.Validator, command string) *validator.Result {
		return v.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		})
	}

	DescribeTable("GIT010 and GIT026 ignore git that is only text",
		func(command string) {
			command = strings.ReplaceAll(command, "@DIR@", scriptDir)

			Expect(run(commitValidator, command).Passed).To(BeTrue(), "GIT010 on %q", command)
			Expect(run(pushValidator, command).Passed).To(BeTrue(), "GIT026 on %q", command)
		},
		Entry("zakwas with separate words",
			"zakwas propose add-permission review git push"),
		Entry("zakwas with a quoted command line",
			"zakwas propose add-permission deny 'git push --force'"),
		Entry("zakwas with a commit rule",
			"zakwas propose add-permission allow 'Bash(git commit:*)'"),
		Entry("a python heredoc writing a fixture",
			"python3 - <<'PY'\nfrom pathlib import Path\n"+
				"Path('fixture.txt').write_text('git -C repo commit -qm fresh\\n')\nPY"),
		Entry("a script holding the words as data",
			"cd @DIR@ && timeout 120 bash sanity.sh"),
		Entry("a script by path", "@DIR@/sanity.sh"),
		Entry("echo", "echo git commit -m x && echo git push origin main"),
		Entry("printf", `printf '%s\n' 'git push origin main' 'git commit -m x'`),
		Entry("grep", "grep -rn 'git commit' docs/ && grep -rn 'git push' docs/"),
		Entry("python -c", `python3 -c "print('git commit -m x; git push origin main')"`),
		Entry("tee", "echo 'git commit -m x' | tee out.txt"),
		Entry("cat heredoc to a file",
			"cat <<EOF > notes.txt\ngit commit -m x\ngit push origin main\nEOF"),
		Entry("a comment", "echo hi # git commit -m x; git push origin main"),
		Entry("a gh body", `gh pr comment 1 --body "run git push --force; git commit -m x"`),
		Entry("an unknown program with a quoted message",
			`mytool --message "git commit -m x" --other "git push origin main"`),
	)

	DescribeTable("GIT010 still blocks an unsigned commit",
		func(command string) {
			command = strings.ReplaceAll(command, "@DIR@", scriptDir)

			result := run(commitValidator, command)

			Expect(result.Reference).To(Equal(validator.RefGitMissingFlags), "in %q", command)
		},
		Entry("a plain commit", "git commit -m x"),
		Entry("bash -c", "bash -c 'git commit -m x'"),
		Entry("sh -c", `sh -c "git commit -m x"`),
		Entry("after cd", "cd d && git commit -m x"),
		Entry("env", "env GIT_X=1 git commit -m x"),
		Entry("a directory option", "git -C repo commit -qm fresh"),
		Entry("xargs", "echo x | xargs git commit -m"),
		Entry("a subshell", "(git commit -m x)"),
		Entry("a command substitution", "echo $(git commit -m x)"),
		Entry("a heredoc fed to bash", "bash <<EOF\ngit commit -m x\nEOF"),
		Entry("a here-string fed to bash", "bash <<< 'git commit -m x'"),
		Entry("sudo", "sudo git commit -m x"),
		Entry("timeout", "timeout 10 git commit -m x"),
		Entry("a script that commits", "cd @DIR@ && timeout 120 bash real.sh"),
		Entry("a python heredoc that runs git",
			"python3 - <<'PY'\nimport subprocess\nsubprocess.run(['git','commit','-m','x'])\nPY"),
	)

	DescribeTable("GIT026 still blocks a push to a blocked branch",
		func(command string) {
			command = strings.ReplaceAll(command, "@DIR@", scriptDir)

			result := run(pushValidator, command)

			Expect(result.Reference).To(Equal(validator.RefGitBlockedBranch), "in %q", command)
		},
		Entry("a push to main", "git push origin main"),
		Entry("a push to master", "git push origin master"),
		Entry("a bare push on master", "git push"),
		Entry("bash -c", "bash -c 'git push origin main'"),
		Entry("sh -c", `sh -c "git push origin main"`),
		Entry("after cd", "cd d && git push origin main"),
		Entry("env", "env GIT_X=1 git push origin main"),
		Entry("a directory option", "git -C repo push origin main"),
		Entry("xargs", "echo origin | xargs git push"),
		Entry("a subshell", "(git push origin main)"),
		Entry("a command substitution", "echo $(git push origin main)"),
		Entry("a heredoc fed to bash", "bash <<EOF\ngit push origin main\nEOF"),
		Entry("a here-string fed to sh", "sh <<< 'git push origin main'"),
		Entry("sudo", "sudo git push origin main"),
		Entry("timeout", "timeout 10 git push origin main"),
		Entry("mise exec", "mise exec -- git push origin main"),
		Entry("ssh", "ssh host 'git push origin main'"),
		Entry("a script that pushes", "timeout 120 bash @DIR@/real.sh"),
	)

	It("allows a push to another branch", func() {
		Expect(run(pushValidator, "git push origin feat/x").Passed).To(BeTrue())
	})
})
