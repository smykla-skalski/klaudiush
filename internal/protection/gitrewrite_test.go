package protection_test

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/protection"
)

var _ = Describe("CheckCommand for commands that rewrite the work tree", func() {
	var (
		e   *env
		set *protection.Set
	)

	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", e.project}, args...)...)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))

		return string(out)
	}

	BeforeEach(func() {
		if _, err := exec.LookPath("git"); err != nil {
			Skip("git is not installed")
		}

		for name, value := range map[string]string{
			"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
			"GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
			"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
		} {
			GinkgoT().Setenv(name, value)
		}

		e = newEnv(GinkgoT().TempDir(), "linux", nil)
		Expect(os.RemoveAll(filepath.Join(e.project, ".git"))).To(Succeed())
		git("init", "-q", "-b", "main")
		e.write("project/.gitignore", "ignored/\n")
		git("add", "-A")
		git("commit", "-qm", "init")
		git("branch", "same")
		git("checkout", "-q", "-b", "changed")
		e.write("project/.klaudiush/config.toml", "[protection]\nenabled = false\n")
		git("commit", "-qam", "weaken")
		git("checkout", "-q", "main")

		set = e.set()
	})

	It("blocks moving to a revision with other policy files", func() {
		Expect(checkCommand(set, `git checkout changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git switch changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git reset --hard changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git merge changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git cherry-pick changed`)).NotTo(BeEmpty())

		Expect(checkCommand(set, `git checkout same`)).To(BeEmpty())
		Expect(checkCommand(set, `git reset --soft changed`)).To(BeEmpty())
		Expect(checkCommand(set, `git checkout -b new`)).To(BeEmpty())
	})

	It("blocks discarding changed or untracked policy files", func() {
		Expect(checkCommand(set, `git stash`)).To(BeEmpty())
		Expect(checkCommand(set, `git reset --hard`)).To(BeEmpty())

		e.write("project/.claude/settings.json", `{"hooks":{}}`)
		e.write("project/.mcp.json", "{}")

		Expect(checkCommand(set, `git stash`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git reset --hard`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git clean -fd`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git stash list`)).To(BeEmpty())
	})

	It("blocks a stash that brings policy files back", func() {
		e.write("project/.claude/settings.json", `{"hooks":{}}`)
		git("stash")

		Expect(checkCommand(set, `git stash pop`)).NotTo(BeEmpty())
	})

	It("blocks repository-root pathspecs", func() {
		Expect(checkCommand(set, `git restore --source=changed :/`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git checkout -- :/`)).NotTo(BeEmpty())
	})

	It("reads patches for the files they change", func() {
		e.write(
			"project/weaken.patch",
			"diff --git a/.klaudiush/config.toml b/.klaudiush/config.toml\n"+
				"--- a/.klaudiush/config.toml\n+++ b/.klaudiush/config.toml\n@@ -1 +1 @@\n-x\n+y\n",
		)
		e.write("project/fine.patch", "--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-x\n+y\n")

		Expect(checkCommand(set, `git apply weaken.patch`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git am weaken.patch`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `patch -p1 < weaken.patch`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `patch -p1 -i weaken.patch`)).NotTo(BeEmpty())
		Expect(
			checkCommand(set, "patch -p1 <<'EOF'\n+++ b/.claude/settings.json\nEOF"),
		).NotTo(BeEmpty())

		Expect(checkCommand(set, `git apply fine.patch`)).To(BeEmpty())
		Expect(checkCommand(set, `patch -p1 < fine.patch`)).To(BeEmpty())
	})
})
