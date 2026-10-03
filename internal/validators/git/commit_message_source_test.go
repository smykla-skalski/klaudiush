package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	goodMessage = "fix(git): block opaque message files"
	badMessage  = "not a conventional message"
)

// expectOpaqueMessage asserts a SHELL002 block whose finding names the reason
// and a repair.
func expectOpaqueMessage(result *validator.Result, reason string) {
	GinkgoHelper()

	Expect(result.Passed).To(BeFalse())
	Expect(result.ShouldBlock).To(BeTrue())
	Expect(result.Reference).To(Equal(validator.RefShellNesting))
	Expect(result.Message).To(HavePrefix("Commit message cannot be inspected: "))
	Expect(result.Message).To(ContainSubstring(reason))
	Expect(result.Findings).To(HaveLen(1))
	Expect(result.Findings[0].Location).To(Equal("commit message"))
	Expect(result.Findings[0].Repair).ToNot(BeEmpty())
	Expect(result.Findings[0].Required).ToNot(BeEmpty())
}

var _ = Describe("CommitValidator message sources", func() {
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

	writeFile := func(name, content string) string {
		path := filepath.Join(dir, name)
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

		return path
	}

	expand := func(command string) string {
		return strings.NewReplacer(
			"{dir}", dir,
			"{good}", filepath.Join(dir, "good.txt"),
			"{bad}", filepath.Join(dir, "bad.txt"),
		).Replace(command)
	}

	BeforeEach(func() {
		fakeGit := gitpkg.NewFakeRunner()
		fakeGit.StagedFiles = []string{"file.txt"}
		v = git.NewCommitValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		dir = GinkgoT().TempDir()

		writeFile("good.txt", goodMessage+"\n")
		writeFile("bad.txt", badMessage+"\n")
		writeFile("empty.txt", "")
	})

	DescribeTable("blocks a message klaudiush cannot see",
		func(command, reason string) {
			expectOpaqueMessage(validate(expand(command)), reason)
		},
		Entry("literal process substitution",
			"git commit -sS -F <(echo msg)", "process substitution"),
		Entry("process substitution of command output",
			"git commit -sS -F <(git log -1 --format=%B)", "command output"),
		Entry("process substitution in --file=",
			"git commit -sS --file=<(echo msg)", "command output"),
		Entry("process substitution glued to -F",
			"git commit -sS -F<(echo msg)", "command output"),
		Entry("process substitution glued to combined flags",
			"git commit -sSF<(git log -1)", "command output"),
		Entry("process substitution before a readable path",
			"git commit -sS -F <(git log -1) {good}", "command output"),
		Entry("process substitution before --",
			"git commit -sS -F <(git log -1) -- file.txt", "command output"),
		Entry("command substitution path",
			`git commit -sS -F "$(mktemp)"`, "command output"),
		Entry("partly substituted path",
			`git commit -sS -F "$(dirname {good})/good.txt"`, "command output"),
		Entry("file descriptor path",
			"git commit -sS -F /dev/fd/3 3<{good}", "file descriptor"),
		Entry("proc file descriptor path",
			"git commit -sS -F /proc/self/fd/5", "file descriptor"),
		Entry("stdin with nothing visible",
			"git commit -sS -F -", "reads stdin"),
		Entry("/dev/stdin with nothing visible",
			"git commit -sS -F /dev/stdin", "reads stdin"),
		Entry("stdin piped from a command",
			"git log -1 --format=%B | git commit -sS -F -", "reads stdin"),
		Entry("unresolved variable path",
			`git commit -sS -F "$MSG_FILE"`, "variable"),
		Entry("variable assigned from command output",
			`f=$(mktemp); git commit -sS -F "$f"`, "variable"),
		Entry("HOME set on the line",
			`export HOME=/elsewhere; git commit -sS -F "$HOME/good.txt"`, "cannot be read"),
		Entry("HOME the line may change",
			`read -r HOME; git commit -sS -F "$HOME/good.txt"`, "variable"),
		Entry("relative path after an unresolved cd",
			`cd "$WORK" && git commit -sS -F good.txt`, "directory"),
		Entry("relative path under an unresolved git -C",
			`git -C "$WORK" commit -sS -F good.txt`, "directory"),
		Entry("relative path under a substituted git -C",
			`git -C "$(pwd)" commit -sS -F good.txt`, "directory"),
		Entry("file written earlier with unknown content",
			`echo "$X" > {good} && git commit -sS -F {good}`, "written earlier"),
		Entry("redirect to a substituted name before the commit",
			`echo x > "$(mktemp)"; git commit -sS -F {good}`, "whose name"),
		Entry("missing file",
			"git commit -sS -F {dir}/missing.txt", "cannot be read"),
		Entry("directory",
			"git commit -sS -F {dir}", "cannot be read"),
		Entry("missing stdin redirect file",
			"git commit -sS -F - < {dir}/missing.txt", "cannot be read"),
	)

	It("blocks a FIFO without hanging", func() {
		fifo := filepath.Join(dir, "fifo")
		Expect(syscall.Mkfifo(fifo, 0o600)).To(Succeed())

		expectOpaqueMessage(validate("git commit -sS -F "+fifo), "cannot be read")
	})

	It("blocks a message file over 1 MiB", func() {
		big := writeFile("big.txt", goodMessage+"\n\n"+strings.Repeat("a\n", 1<<20))

		expectOpaqueMessage(validate("git commit -sS -F "+big), "cannot be read")
	})

	It("keeps attribution findings next to the opaque source", func() {
		result := validate("git commit -sS -F <(echo msg) --trailer 'Co-Authored-By: Claude'")

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefShellNesting))
		Expect(len(result.Findings)).To(BeNumerically(">", 1))
	})

	DescribeTable(
		"validates a message it can read",
		func(command string, passes bool) {
			result := validate(expand(command))

			if passes {
				Expect(result.Passed).To(BeTrue(), result.Message)

				return
			}

			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefGitConventionalCommit))
		},
		Entry("-F readable file", "git commit -sS -F {good}", true),
		Entry("-F readable bad file", "git commit -sS -F {bad}", false),
		Entry("--file readable file", "git commit -sS --file {good}", true),
		Entry("--file= readable bad file", "git commit -sS --file={bad}", false),
		Entry("combined -sSF", "git commit -sSF {bad}", false),
		Entry("relative path after cd", "cd {dir} && git commit -sS -F bad.txt", false),
		Entry("relative path under git -C", "git -C {dir} commit -sS -F good.txt", true),
		Entry("variable assigned on the line", `D={dir}; git commit -sS -F "$D/bad.txt"`, false),
		Entry("heredoc on -F -", "git commit -sS -F - <<'EOF'\n"+badMessage+"\nEOF", false),
		Entry(
			"heredoc on /dev/stdin",
			"git commit -sS -F /dev/stdin <<'EOF'\n"+goodMessage+"\nEOF",
			true,
		),
		Entry(
			"heredoc on /dev/fd/0",
			"git commit -sS -F /dev/fd/0 <<'EOF'\n"+badMessage+"\nEOF",
			false,
		),
		Entry("here-string on -F -", "git commit -sS -F - <<< '"+badMessage+"'", false),
		Entry("literal echo piped to -F -", "echo '"+goodMessage+"' | git commit -sS -F -", true),
		Entry("file redirected to -F -", "git commit -sS -F - < {bad}", false),
		Entry("file catted to -F -", "cat {good} | git commit -sS -F -", true),
		Entry(
			"heredoc written file",
			"cat > {dir}/m.txt <<'EOF'\n"+badMessage+"\nEOF\ngit commit -sS -F {dir}/m.txt",
			false,
		),
		Entry("substitution elsewhere on the line",
			`git commit -sS -F {bad} --date="$(date)"`, false),
		Entry("variable path with substitution elsewhere",
			`D={dir}; git commit -sS -F "$D/good.txt" --date "$(date)"`, true),
		Entry("-m message", "git commit -sS -m '"+badMessage+"'", false),
		Entry("-m heredoc", "git commit -sS -m \"$(cat <<'EOF'\n"+goodMessage+"\nEOF\n)\"", true),
		Entry("empty file", "git commit -sS -F {dir}/empty.txt", true),
	)

	It("expands HOME from the environment", func() {
		GinkgoT().Setenv("HOME", dir)

		result := validate(`git commit -sS -F "$HOME/bad.txt"`)

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefGitConventionalCommit))
	})

	It("passes an opaque source while message validation is disabled", func() {
		disabled := false
		cfg := &config.CommitValidatorConfig{
			Message: &config.CommitMessageConfig{Enabled: &disabled},
		}
		v = git.NewCommitValidator(logger.NewNoOpLogger(), gitpkg.NewFakeRunner(), cfg, nil)

		Expect(validate("git commit -sS -a -F <(echo msg)").Passed).To(BeTrue())
	})
})
