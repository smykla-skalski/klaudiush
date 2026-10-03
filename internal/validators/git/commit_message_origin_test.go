package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// setEditorEnv sets the editor variables for one spec, unsetting those
// given as "".
func setEditorEnv(gitEditor string) {
	GinkgoHelper()

	unsetEnv("GIT_EDITOR", "EDITOR", "VISUAL")

	if gitEditor != "" {
		GinkgoT().Setenv("GIT_EDITOR", gitEditor)
	}
}

func unsetEnv(names ...string) {
	GinkgoHelper()

	for _, name := range names {
		GinkgoT().Setenv(name, "")
		Expect(os.Unsetenv(name)).To(Succeed())
	}
}

// commitRepo creates a repository whose HEAD has message, and returns its
// directory and HEAD's object name.
func commitRepo(message string) (string, string) {
	GinkgoHelper()

	dir := GinkgoT().TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{
			"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null",
		}, args...)...)
		cmd.Dir = dir
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"), "HOME=" + dir,
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		}

		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))

		return strings.TrimSpace(string(out))
	}

	run("init", "-q", "-b", "main", ".")
	Expect(os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o600)).To(Succeed())
	run("add", "f.txt")
	run("commit", "-q", "-m", message)
	Expect(os.WriteFile(filepath.Join(dir, "g.txt"), []byte("y"), 0o600)).To(Succeed())
	run("add", "g.txt")

	return dir, run("rev-parse", "HEAD")
}

var _ = Describe("CommitValidator message origins", func() {
	var (
		v   *git.CommitValidator
		dir string
	)

	validate := func(command string) *validator.Result {
		return v.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		})
	}

	expectRuleBlock := func(result *validator.Result) {
		GinkgoHelper()

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).NotTo(Equal(validator.RefShellNesting))
	}

	BeforeEach(func() {
		fakeGit := gitpkg.NewFakeRunner()
		fakeGit.StagedFiles = []string{"file.txt"}
		v = git.NewCommitValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		dir = GinkgoT().TempDir()

		setEditorEnv("true")
		unsetEnv("GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE")
		GinkgoT().Setenv("HOME", dir)
	})

	Describe("variables in -m and heredocs", func() {
		DescribeTable("validates the expanded message",
			func(command string, pass bool) {
				result := validate(command)
				if pass {
					Expect(result.Passed).To(BeTrue(), result.Message)

					return
				}

				expectRuleBlock(result)
			},
			Entry("variable set on the line (bad)",
				`TITLE='bad title'; git commit -sS -m "$TITLE"`, false),
			Entry("variable set on the line (good)",
				`TITLE='`+goodMessage+`'; git commit -sS -m "$TITLE"`, true),
			Entry("exported variable",
				`export TITLE='`+badMessage+`'; git commit -sS -m "$TITLE"`, false),
			Entry("glued long flag",
				`TITLE='`+badMessage+`'; git commit -sS --message="$TITLE"`, false),
			Entry("variable inside the title",
				`S=git; git commit -sS -m "fix($S): block opaque message files"`, true),
			Entry("single-quoted reference stays literal",
				`git commit -sS -m 'fix(git): expand ${HOME} in paths'`, true),
			Entry(
				"quoted heredoc stays literal",
				"git commit -sS -m \"$(cat <<'EOF'\nfix(git): expand ${NOPE} in paths\nEOF\n)\"",
				true,
			),
			Entry(
				"unquoted heredoc expands",
				"S=git; git commit -sS -m \"$(cat <<EOF\nfix($S): block opaque message files\nEOF\n)\"",
				true,
			),
			Entry("unquoted heredoc expands to a bad title",
				"S='bad title'; git commit -sS -m \"$(cat <<EOF\n$S\nEOF\n)\"", false),
			Entry("second -m is the body",
				`git commit -sS -m '`+goodMessage+`' -m '`+strings.Repeat("word ", 30)+`'`, false),
			Entry("heredoc on -F - expands",
				"T='bad title'; git commit -sS -F - <<EOF\n$T\nEOF", false),
			Entry("quoted heredoc on -F - stays literal",
				"git commit -sS -F - <<'EOF'\nfix(git): expand ${NOPE} in paths\nEOF", true),
			Entry("here-string on -F - expands",
				`T='`+goodMessage+`'; git commit -sS -F - <<< "$T"`, true),
		)

		DescribeTable(
			"blocks a message it cannot build",
			func(command, reason string) {
				expectOpaqueMessage(validate(command), reason)
			},
			Entry("unset variable", `git commit -sS -m "$TITLE"`, "$TITLE, which is not set"),
			Entry("prefix assignment the shell does not expand",
				`TITLE='`+goodMessage+`' git commit -sS -m "$TITLE"`, "not set"),
			Entry("variable from command output",
				`TITLE=$(git log -1 --format=%s); git commit -sS -m "$TITLE"`, "cannot know"),
			Entry(
				"expansion with an operator",
				`git commit -sS -m "${MSG:-default}"`,
				"${MSG:-default}",
			),
			Entry("command output in the title",
				`git commit -sS -m "fix(git): bump to $(cat VERSION)"`, "command output"),
			Entry("transforming heredoc",
				"git commit -sS -m \"$(sed 's/x/y/' <<'EOF'\n"+goodMessage+"\nEOF\n)\"",
				"command output"),
			Entry("unset variable in a later -m",
				`git commit -sS -m '`+goodMessage+`' -m "$BODY"`, "$BODY"),
			Entry("unset variable in a -F - heredoc",
				"git commit -sS -F - <<EOF\n$TITLE\nEOF", "$TITLE"),
			Entry("cat defined on the line",
				"cat() { echo x; }; git commit -sS -m \"$(cat <<'EOF'\n"+goodMessage+"\nEOF\n)\"",
				"alias or function"),
		)
	})

	Describe("editor", func() {
		DescribeTable(
			"lets an editor that changes nothing through",
			func(env, command string) {
				setEditorEnv(env)
				Expect(validate(command).Passed).To(BeTrue())
			},
			Entry("no message with GIT_EDITOR=true", "true", "git commit -sS"),
			Entry("amend with GIT_EDITOR=:", ":", "git commit -sS --amend"),
			Entry("GIT_EDITOR=true on the line", "", "GIT_EDITOR=true git commit -sS --amend"),
			Entry("exported GIT_EDITOR", "", "export GIT_EDITOR=true; git commit -sS --amend"),
			Entry("core.editor from git -c", "", "git -c core.editor=true commit -sS --amend"),
			Entry("--no-edit", "", "git commit -sS --amend --no-edit"),
			Entry("plain --fixup", "", "git commit -sS --fixup HEAD"),
			Entry("--dry-run", "", "git commit -sS --dry-run"),
			Entry("-m with no -e", "vim", "git commit -sS -m '"+goodMessage+"'"),
			Entry("-e then --no-edit", "vim", "git commit -sS -m '"+goodMessage+"' -e --no-edit"),
			Entry(
				"--fixup=reword with GIT_EDITOR=true",
				"true",
				"git commit -sS --fixup=reword:HEAD",
			),
		)

		DescribeTable("blocks an editor that may write the message",
			func(env, command, reason string) {
				setEditorEnv(env)
				expectOpaqueMessage(validate(command), reason)
			},
			Entry("no editor set", "", "git commit -sS", "VISUAL, EDITOR"),
			Entry("vim from the environment", "vim", "git commit -sS", `"vim"`),
			Entry("custom GIT_EDITOR on the line", "true",
				"GIT_EDITOR='cp /tmp/msg' git commit -sS --amend", `"cp /tmp/msg"`),
			Entry("GIT_EDITOR from command output", "true",
				"GIT_EDITOR=$(which vim) git commit -sS --amend", "VISUAL, EDITOR"),
			Entry("core.editor from git -c", "", "git -c core.editor=vim commit -sS", `"vim"`),
			Entry("core.editor from --config-env", "",
				"git --config-env=core.editor=ED commit -sS", "VISUAL, EDITOR"),
			Entry("GIT_CONFIG_PARAMETERS on the line", "",
				"GIT_CONFIG_PARAMETERS=x git -c core.editor=true commit -sS", "VISUAL, EDITOR"),
			Entry("-e with -m", "vim", "git commit -sS -e -m '"+goodMessage+"'", `"vim"`),
			Entry("-c reuse opens the editor", "vim", "git commit -sS -c HEAD", `"vim"`),
			Entry("--fixup=reword", "vim", "git commit -sS --fixup=reword:HEAD", `"vim"`),
			Entry("--fixup=amend", "vim", "git commit -sS --fixup amend:HEAD", `"vim"`),
			Entry("--squash", "vim", "git commit -sS --squash HEAD", `"vim"`),
			Entry("amend", "vim", "git commit -sS --amend", `"vim"`),
			Entry("through a launcher", "true", "env git commit -sS --amend", "VISUAL, EDITOR"),
			Entry("abbreviated --no-edit", "true", "git commit -sS --amend --no-ed", "abbreviates"),
		)
	})

	Describe("-C and -c", func() {
		It("validates the reused message", func() {
			good, _ := commitRepo(goodMessage)
			bad, _ := commitRepo(badMessage)

			Expect(validate("cd " + good + " && git commit -sS -C HEAD").Passed).To(BeTrue())
			expectRuleBlock(validate("cd " + bad + " && git commit -sS -C HEAD"))
			expectRuleBlock(validate("git -C " + bad + " commit -sS --reuse-message=HEAD"))
			expectRuleBlock(validate("cd " + bad + " && git commit -sS -c HEAD"))
		})

		It("reads a full object name after a commit earlier on the line", func() {
			bad, oid := commitRepo(badMessage)

			expectRuleBlock(validate("cd " + bad + " && git commit -sS -m '" + goodMessage +
				"' && git commit -sS --amend -C " + oid))
		})

		DescribeTable(
			"blocks a reused message it cannot read",
			func(command, reason string) {
				good, _ := commitRepo(goodMessage)
				expectOpaqueMessage(validate(strings.ReplaceAll(command, "{repo}", good)), reason)
			},
			Entry(
				"ref moved earlier on the line",
				"cd {repo} && git commit -sS -m '"+goodMessage+"' && git commit -sS --amend -C HEAD",
				"may move",
			),
			Entry("missing commit", "cd {repo} && git commit -sS -C nope", "cannot be read"),
			Entry("rev from command output",
				"cd {repo} && git commit -sS -C $(git rev-parse HEAD)", "command output"),
			Entry("rev from an unset variable", `cd {repo} && git commit -sS -C "$REV"`, "$REV"),
			Entry(
				"rev that looks like an option",
				"cd {repo} && git commit -sS -C --x",
				"cannot be read",
			),
			Entry("unknown directory", `cd "$WORK" && git commit -sS -C HEAD`, "directory"),
			Entry("abbreviated --reuse-message", "cd {repo} && git commit -sS --reuse HEAD",
				"abbreviates"),
		)
	})

	Describe("-t", func() {
		write := func(name, content string) string {
			path := filepath.Join(dir, name)
			Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

			return path
		}

		It("validates the template an unchanged editor commits", func() {
			good := write("good.txt", "# comment\n"+goodMessage+"\n")
			bad := write("bad.txt", badMessage+"\n")

			Expect(validate("git commit -sS -t " + good).Passed).To(BeTrue())
			expectRuleBlock(validate("git commit -sS --template=" + bad))
			Expect(
				validate("git commit -sS -m '" + goodMessage + "' -t " + bad).Passed,
			).To(BeTrue())
			Expect(validate("git commit -sS --amend -t " + bad).Passed).To(BeTrue())
		})

		DescribeTable("blocks a template it cannot read",
			func(command, reason string) {
				expectOpaqueMessage(validate(command), reason)
			},
			Entry("missing file", "git commit -sS -t /nonexistent/tpl.txt", "does not exist"),
			Entry("command output", `git commit -sS -t "$(mktemp)"`, "command output"),
			Entry("stdin", "git commit -sS -t -", "process substitution"),
			Entry("commit.template with --allow-empty-message",
				"git commit -sS --allow-empty-message", "commit.template"),
		)

		It("blocks a template while the editor may change it", func() {
			setEditorEnv("vim")
			expectOpaqueMessage(validate("git commit -sS -t "+write("t.txt", goodMessage)), `"vim"`)
		})
	})
})
