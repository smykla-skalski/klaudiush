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
			"{base}", filepath.Base(dir),
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

	DescribeTable(
		"blocks a message klaudiush cannot see",
		func(command, reason string) {
			expectOpaqueMessage(validate(expand(command)), reason)
		},
		Entry("literal process substitution",
			"git commit -sS -F <(echo msg)", "process substitution"),
		Entry("process substitution of command output",
			"git commit -sS -F <(git log -1 --format=%B)", "substitution"),
		Entry("process substitution in --file=",
			"git commit -sS --file=<(echo msg)", "substitution"),
		Entry("process substitution glued to -F",
			"git commit -sS -F<(echo msg)", "substitution"),
		Entry("process substitution glued to combined flags",
			"git commit -sSF<(git log -1)", "substitution"),
		Entry("process substitution before a readable path",
			"git commit -sS -F <(git log -1) {good}", "substitution"),
		Entry("substitution on combined flags before a readable path",
			"git commit -sSF $(mktemp) {good}", "substitution"),
		Entry("process substitution before --",
			"git commit -sS -F <(git log -1) -- file.txt", "substitution"),
		Entry("command substitution path",
			`git commit -sS -F "$(mktemp)"`, "substitution"),
		Entry("partly substituted path",
			`git commit -sS -F "$(dirname {good})/good.txt"`, "substitution"),
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
			`export HOME=/elsewhere; git commit -sS -F "$HOME/good.txt"`, "does not exist"),
		Entry("HOME the line may change",
			`read -r HOME; git commit -sS -F "$HOME/good.txt"`, "variable"),
		Entry("relative path after an unresolved cd",
			`cd "$WORK" && git commit -sS -F good.txt`, "directory"),
		Entry("relative path under an unresolved git -C",
			`git -C "$WORK" commit -sS -a -F good.txt`, "directory"),
		Entry("relative path under a substituted git -C",
			`git -C "$(pwd)" commit -sS -F good.txt`, "directory"),
		Entry("file written earlier with unknown content",
			`echo "$X" > {good} && git commit -sS -F {good}`, "written earlier"),
		Entry("redirect to a substituted name before the commit",
			`echo x > "$(mktemp)"; git commit -sS -F {good}`, "whose name"),
		Entry("unzip before the commit",
			`unzip -o a.zip -d {dir} && git commit -sS -F {good}`, "whose name"),
		Entry("stash pop outside a known work tree before the commit",
			`git -C {dir} stash pop && git commit -sS -F {good}`, "whose name"),
		Entry("relative path after a computed cd",
			`cd "$(mktemp -d)" && git commit -sS -F good.txt`, "directory"),
		Entry("$PWD after a computed cd",
			`cd "$(mktemp -d)" && git commit -sS -F "$PWD/good.txt"`, "variable"),
		Entry("missing file",
			"git commit -sS -F {dir}/missing.txt", "does not exist"),
		Entry("directory",
			"git commit -sS -F {dir}", "not a regular file"),
		Entry("missing stdin redirect file",
			"git commit -sS -F - < {dir}/missing.txt", "does not exist"),
		Entry("repeated -F", "git commit -sS -F {good} -F {bad}", "more than one"),
		Entry("-F then --file", "git commit -sS -F {good} --file={bad}", "more than one"),
		Entry("-F then --no-file", "git commit -sS -F {good} --no-file", "more than one"),
		Entry("-F then a substituted -F",
			"git commit -sS -F {good} -F <(echo bad)", "more than one"),
		Entry("abbreviated --file", "git commit -sS --fil {bad}", "abbreviates"),
		Entry("abbreviated --file=", "git commit -sS --fil={bad}", "abbreviates"),
		Entry("abbreviated --message", "git commit -sS --mes='bad'", "abbreviates"),
		Entry("pipe and redirect on stdin",
			"echo '"+goodMessage+"' | git commit -sS -F - < {bad}", "redirect and a pipe"),
		Entry("here-string and redirect on stdin",
			"git commit -sS -F - <<< '"+goodMessage+"' < {bad}", "redirect and a pipe"),
		Entry("partial substitution with a decoy -F in a comment",
			`cd {dir} && git commit -sS -F $(echo sub/)good.txt # -F good.txt`, "substitution"),
		Entry("partial substitution with a decoy -F after ;",
			`cd {dir} && git commit -sS -F "$(echo sub/)"good.txt; : -F good.txt`, "substitution"),
		Entry("uncaptured write under another spelling",
			`cd {dir} && echo "$X" > ../`+"{base}"+`/good.txt && git commit -sS -F {good}`,
			"written earlier"),
		Entry("write to an unresolved variable target",
			`echo bad > "$F"; git commit -sS -F {good}`, "whose name"),
		Entry(
			"sed -i on the file",
			`sed -i '' 's/fix/bad/' {good} && git commit -sS -F {good}`,
			"written earlier",
		),
		Entry(
			"ln over the file",
			`ln -sf {bad} {good} && git commit -sS -F {good}`,
			"written earlier",
		),
		Entry(
			"dd onto the file",
			`dd if={bad} of={good} && git commit -sS -F {good}`,
			"written earlier",
		),
		Entry("copy into the file's directory",
			`cp /elsewhere/good.txt {dir} && git commit -sS -F {good}`, "may change"),
		Entry("copy into the directory as .",
			`cd {dir} && cp sub/good.txt . && git commit -sS -F good.txt`, "may change"),
		Entry("copy to a substituted destination",
			`cd {dir} && cp sub/good.txt "$(pwd)" && git commit -sS -F good.txt`, "whose name"),
		Entry(
			"dd to ~+ after of=",
			`cd {dir} && dd if=sub/good.txt of=~+/good.txt && git commit -sS -F good.txt`,
			"may change",
		),
		Entry(
			"copy to ~+",
			`cd {dir} && cp sub/good.txt ~+ && git commit -sS -F good.txt`,
			"may change",
		),
		Entry(
			"copy to a glob",
			`cd {dir} && cp sub/good.txt * && git commit -sS -F good.txt`,
			"may change",
		),
		Entry("move a parent directory",
			`mv {dir} /elsewhere/old && git commit -sS -F {good}`, "may change"),
		Entry(
			"remove a parent directory",
			`rm -rf {dir} && git commit -sS -F {good}`,
			"may change",
		),
		Entry("abbreviated --m", "git commit -sS --m 'bad'", "abbreviates"),
		Entry("substituted stdin redirect",
			`cd {dir} && git commit -sS -F - < "$(printf sub/)good.txt"`, "reads stdin"),
		Entry(
			"captured write then sed -i",
			"echo '"+goodMessage+"' > {dir}/m.txt; sed -i 's/.*/bad/' {dir}/m.txt; git commit -sS -F {dir}/m.txt",
			"written earlier",
		),
		Entry(
			"captured write then uncaptured write through a variable",
			"echo '"+goodMessage+"' > {dir}/m.txt; echo bad > \"$F\"; git commit -sS -F {dir}/m.txt",
			"whose name",
		),
		Entry("partly substituted git -C",
			`git -C "$(printf sub/)repo" commit -sS -a -F msg.txt`, "directory"),
		Entry("partly substituted cd",
			`cd "$(printf sub/)repo" && git commit -sS -a -F msg.txt`, "directory"),
		Entry(
			"arithmetic in the -F path",
			"git commit -sS -F {dir}/good$((1)).txt",
			"substitution",
		),
		Entry("perl writing a named file",
			`perl -e 'open my $f, ">", $ARGV[0]' {good}; git commit -sS -F {good}`, "may change"),
		Entry("sed naming the file", "sed -n 1p {good} && git commit -sS -F {good}", "may change"),
		Entry(
			"repeated git -C",
			"git -C {dir} -C sub commit -sS -a -F good.txt",
			"repeated git -C",
		),
		Entry("captured write skipped by &&",
			"false && echo '"+goodMessage+"' > {bad}; git commit -sS -F {bad}", "may not run"),
		Entry("captured write skipped by ||",
			"true || echo '"+goodMessage+"' > {bad}; git commit -sS -F {bad}", "may not run"),
		Entry(
			"captured write inside if",
			"if false; then echo '"+goodMessage+"' > {bad}; fi; git commit -sS -F {bad}",
			"may not run",
		),
		Entry("captured write in the background",
			"echo '"+goodMessage+"' > {bad} & git commit -sS -F {bad}", "may not run"),
		Entry(
			"captured write in a loop",
			"for i in; do echo '"+goodMessage+"' > {bad}; done; git commit -sS -F {bad}",
			"may not run",
		),
		Entry("captured write in a nested script",
			"bash -c \"echo '"+goodMessage+"' > {bad}\"; git commit -sS -F {bad}", "may not run"),
		Entry(
			"copy onto a file whose name has =",
			"touch {dir}/msg=x && chmod 600 {dir}/msg=x && git commit -sS -F {dir}/msg=x",
			"may change",
		),
	)

	It("validates the last write through a dangling symlink", func() {
		link := filepath.Join(dir, "link.txt")
		target := filepath.Join(dir, "new.txt")
		Expect(os.Symlink(target, link)).To(Succeed())

		result := validate("echo '" + goodMessage + "' > " + target + "; echo '" + badMessage +
			"' > " + link + "; git commit -sS -F " + target)

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefGitConventionalCommit))
	})

	It("names a permission error instead of the file type", func() {
		if os.Geteuid() == 0 {
			Skip("root reads files regardless of mode")
		}

		locked := writeFile("locked.txt", goodMessage)
		Expect(os.Chmod(locked, 0)).To(Succeed())

		expectOpaqueMessage(validate("git commit -sS -F "+locked), "permission or I/O error")
	})

	It("blocks a write through a symlink to the message file", func() {
		link := filepath.Join(dir, "link.txt")
		Expect(os.Symlink(filepath.Join(dir, "good.txt"), link)).To(Succeed())

		expectOpaqueMessage(
			validate("echo \"$X\" > "+link+" && git commit -sS -F "+filepath.Join(dir, "good.txt")),
			"written earlier",
		)
	})

	It("blocks a relative write the provider's directory places on the file", func() {
		result := v.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{
				Command: `echo "$X" > good.txt && git commit -sS -F ` + filepath.Join(
					dir,
					"good.txt",
				),
			},
			WorkingDir: dir,
		})

		expectOpaqueMessage(result, "written earlier")
	})

	It("resolves a relative path against the provider's working directory", func() {
		result := v.Validate(context.Background(), &hook.Context{
			EventType:  hook.EventTypePreToolUse,
			ToolName:   hook.ToolTypeBash,
			ToolInput:  hook.ToolInput{Command: "git commit -sS -F bad.txt"},
			WorkingDir: dir,
		})

		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Reference).To(Equal(validator.RefGitConventionalCommit))
	})

	It("blocks a FIFO without hanging", func() {
		fifo := filepath.Join(dir, "fifo")
		Expect(syscall.Mkfifo(fifo, 0o600)).To(Succeed())

		expectOpaqueMessage(validate("git commit -sS -F "+fifo), "not a regular file")
	})

	It("blocks a message file over 1 MiB", func() {
		big := writeFile("big.txt", goodMessage+"\n\n"+strings.Repeat("a\n", 1<<20))

		expectOpaqueMessage(validate("git commit -sS -F "+big), "larger than 1 MiB")
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
		Entry("$PWD after cd", `cd {dir} && git commit -sS -F "$PWD/bad.txt"`, false),
		Entry("unzip elsewhere before the commit",
			"unzip -o a.zip -d /elsewhere && git commit -sS -F {good}", true),
		Entry("reset --soft before the commit",
			"git reset --soft HEAD~1 && git commit -sS -F {good}", true),
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
		Entry("captured write earlier in the commit's && chain",
			"cd {dir} && echo '"+goodMessage+"' > m.txt && git commit -sS -F m.txt", true),
		Entry("captured write in the if condition branch the commit is in",
			"if true; then echo '"+badMessage+"' > {dir}/m.txt; git commit -sS -F {dir}/m.txt; fi",
			false),
		Entry("git -C with a variable set on the line",
			`D={dir}; git -C "$D" commit -sS -a -F bad.txt`, false),
		Entry("stdin redirect read from the shell's directory, not -C",
			"cd {dir} && git -C sub commit -sS -a -F - < bad.txt", false),
		Entry("substituted write after the commit",
			`git commit -sS -F {good}; echo x > "$(mktemp)"`, true),
		Entry("copy to a known variable destination first",
			`D={dir}/other; cp {bad} "$D/out.txt"; git commit -sS -F {good}`, true),
		Entry("read-only commands naming the file first",
			"cat {good} && grep fix {good} && git add {good} && git commit -sS -F {good}", true),
		Entry("cd and mkdir in the file's directory first",
			"cd {dir} && mkdir -p {dir}/sub && git commit -sS -F good.txt", true),
		Entry("copy into another directory first",
			"cp {good} {dir}/other/ && git commit -sS -F {bad}", false),
		Entry(
			"unrelated write first",
			"echo x > {dir}/other.txt && git commit -sS -F {bad}",
			false,
		),
		Entry("quoted path with substitution elsewhere",
			`git commit -sS -F "{good}" --date "$(date)"`, true),
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
