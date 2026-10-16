package dispatcher_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/config/factory"
	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/parser"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("Git validators on real hook payloads", func() {
	var (
		disp *dispatcher.Dispatcher
		repo string
	)

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo

		cmd.Env = append(
			os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
		)

		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
	}

	BeforeEach(func() {
		repo = GinkgoT().TempDir()
		resolved, err := filepath.EvalSymlinks(repo)
		Expect(err).NotTo(HaveOccurred())

		repo = resolved

		runGit("init", "-q", "-b", "master")
		runGit("remote", "add", "origin", "https://example.com/team/project.git")
		runGit(
			"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false",
			"commit", "-q", "--allow-empty", "-m", "init",
		)

		Expect(os.WriteFile(filepath.Join(repo, "sanity.sh"), []byte(
			"#!/usr/bin/env bash\n"+
				"echo \"git commit -m data\"\n"+
				"python3 - <<PY\n"+
				"from pathlib import Path\n"+
				"Path(\"fixture.txt\").write_text(\"git -C repo commit -qm fresh\\n\")\n"+
				"PY\n"+
				"zakwas propose add-permission review git push\n"), 0o600)).To(Succeed())

		previous, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(repo)).To(Succeed())
		DeferCleanup(func() { Expect(os.Chdir(previous)).To(Succeed()) })

		log := logger.NewNoOpLogger()
		cfg := config.DefaultConfig()
		cfg.Validators.Git.Push.BlockedBranches = []string{"main", "master"}

		disp = dispatcher.NewDispatcher(factory.NewRegistryBuilder(log).Build(cfg), log)
	})

	codes := func(command string) []string {
		payload, err := json.Marshal(map[string]any{
			"hook_event_name": "PreToolUse",
			"session_id":      "s",
			"cwd":             repo,
			"tool_name":       "Bash",
			"tool_input":      map[string]any{"command": command},
		})
		Expect(err).NotTo(HaveOccurred())

		hookCtx, err := parser.NewJSONParser(strings.NewReader(string(payload))).
			Parse(hook.EventTypePreToolUse)
		Expect(err).NotTo(HaveOccurred())

		verrs := disp.Dispatch(context.Background(), hookCtx)
		found := make([]string, 0, len(verrs))

		for _, verr := range verrs {
			found = append(found, string(verr.Reference))
		}

		return found
	}

	DescribeTable("lets git that is only text through",
		func(command string) {
			Expect(codes(command)).NotTo(ContainElement(ContainSubstring("/GIT")), command)
		},
		Entry("zakwas with separate words",
			"zakwas propose add-permission review git push"),
		Entry("zakwas with a quoted command line",
			"zakwas propose add-permission deny 'git push --force'"),
		Entry("a python heredoc writing a fixture",
			"python3 - <<'PY'\nfrom pathlib import Path\n"+
				"Path('fixture.txt').write_text('git -C repo commit -qm fresh\\n')\nPY"),
		Entry("a script holding the words as data", "cd . && timeout 120 bash sanity.sh"),
		Entry("echo", "echo git commit -m x; echo git push origin main"),
		Entry("grep", "grep -rn 'git commit' . ; grep -rn 'git push' ."),
		Entry("a heredoc to a file", "cat <<EOF > notes.txt\ngit commit -m x\nEOF"),
		Entry("a comment", "echo hi # git commit -m x; git push origin main"),
	)

	DescribeTable("still blocks the git that really runs",
		func(command, code string) {
			Expect(codes(command)).To(ContainElement(ContainSubstring(code)), command)
		},
		Entry("an unsigned commit", "git commit -m x", "GIT010"),
		Entry("a commit in bash -c", "bash -c 'git commit -m x'", "GIT010"),
		Entry("a commit after cd", "cd . && git commit -m x", "GIT010"),
		Entry("a commit through env", "env GIT_X=1 git commit -m x", "GIT010"),
		Entry("a commit with -C", "git -C . commit -qm fresh", "GIT010"),
		Entry("a commit through xargs", "echo x | xargs git commit -m", "GIT010"),
		Entry("a commit in a subshell", "(git commit -m x)", "GIT010"),
		Entry("a commit in a substitution", "echo $(git commit -m x)", "GIT010"),
		Entry("a commit in a heredoc fed to bash", "bash <<EOF\ngit commit -m x\nEOF", "GIT010"),
		Entry("a push to master", "git push origin master", "GIT026"),
		Entry("a push to main", "git push origin main", "GIT026"),
		Entry("a push in bash -c", "sh -c 'git push origin main'", "GIT026"),
		Entry("a push after cd", "cd . && git push origin main", "GIT026"),
		Entry("a push through sudo", "sudo git push origin main", "GIT026"),
	)
})
