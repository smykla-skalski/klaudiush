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
		Expect(checkCommand(set, `git reset --soft changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git checkout -B main changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git switch -C main changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git branch -f main changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git update-ref refs/heads/main changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git read-tree -u --reset changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git update-ref refs/heads/main $(git commit-tree x -p HEAD)`)).
			NotTo(BeEmpty())
		Expect(checkCommand(set, `git branch -f other same`)).To(BeEmpty())
		Expect(checkCommand(set, `git branch -f other changed`)).To(BeEmpty())
		Expect(checkCommand(set, `git update-ref refs/heads/other changed`)).To(BeEmpty())
		Expect(checkCommand(set, `git update-ref HEAD changed`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git reset --soft same`)).To(BeEmpty())
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
		Expect(checkCommand(set, `git checkout -f`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git switch -f main`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git checkout-index -f -a`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git read-tree -u --reset HEAD`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `git clean -n`)).To(BeEmpty())
		Expect(checkCommand(set, `git clean -fd build`)).To(BeEmpty())
		Expect(
			checkCommand(set, `git stash push -- main.go`),
		).To(BeEmpty(), "%v", checkCommand(set, `git stash push -- main.go`))
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

var _ = Describe("CheckCommand for git pointed elsewhere", func() {
	It("blocks work tree writes from another repository or index", func() {
		set := newEnv(GinkgoT().TempDir(), "linux", nil).set()

		for _, command := range []string{
			`GIT_DIR=/tmp/evil/.git git checkout -f`,
			`env GIT_DIR=/tmp/evil/.git GIT_WORK_TREE=. git reset --hard`,
			`export GIT_DIR=/tmp/evil/.git; git checkout -f`,
			`GIT_INDEX_FILE=/tmp/idx git checkout-index -f -a`,
			`git -c core.worktree=. checkout -f`,
			`git --git-dir=/tmp/evil --work-tree=. checkout -f`,
		} {
			Expect(checkCommand(set, command)).NotTo(BeEmpty(), command)
		}

		Expect(checkCommand(set, `GIT_DIR=/tmp/evil/.git git log`)).To(BeEmpty())
		Expect(checkCommand(set, `source /tmp/gitenv.sh; git checkout -f`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `. /tmp/gitenv.sh && git reset --hard`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `eval "$(cat /tmp/gitenv.sh)"; git checkout -f`)).NotTo(BeEmpty())
		Expect(checkCommand(set, `source /tmp/env.sh; git status`)).To(BeEmpty())

		for _, command := range []string{
			`export $(cat ~/envonly); git checkout -f`,
			`export "$(cat ~/envonly)"; git checkout -f`,
			`env $(cat ~/envonly) git checkout -f`,
			`read -r GIT_DIR < /tmp/gd; export GIT_DIR; git checkout -f`,
			`printf -v GIT_DIR %s x; export GIT_DIR; git checkout -f`,
			`x=GIT_DIR; export $x=/tmp/e; git checkout -f`,
			`declare -x "GIT""_DIR=/tmp/e"; git checkout -f`,
			`BASH_ENV=~/env.sh bash -c "git checkout -f"`,
			`bash --rcfile ~/env.sh -ic "git checkout -f"`,
			`set -a; . ~/envonly; git checkout -f`,
		} {
			Expect(checkCommand(set, command)).NotTo(BeEmpty(), command)
		}
	})

	It("lets sourced scripts that leave git alone through", func() {
		e := newEnv(GinkgoT().TempDir(), "linux", nil)
		activate := e.write("project/.venv/bin/activate",
			"# This file must be used with \"source bin/activate\"\n"+
				"VIRTUAL_ENV=/x\nexport VIRTUAL_ENV\nPATH=\"$VIRTUAL_ENV/bin:$PATH\"\nexport PATH\n")
		gitenv := e.write("project/gitenv.sh", "export GIT_DIR=/tmp/evil/.git\n")
		set := e.set()

		Expect(checkCommand(set, "source "+activate+" && git checkout -b x")).To(BeEmpty())
		Expect(checkCommand(set, `export PATH="$PATH:/x"; git checkout -b y`)).To(BeEmpty())
		Expect(checkCommand(set, "source "+gitenv+" && git checkout -f")).NotTo(BeEmpty())
	})
})
