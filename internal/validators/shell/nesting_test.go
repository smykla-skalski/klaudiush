package shell_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/shell"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("NestingValidator", func() {
	var v *shell.NestingValidator

	BeforeEach(func() {
		v = shell.NewNestingValidator(logger.NewNoOpLogger())
	})

	bash := func(command string) *hook.Context {
		return &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		}
	}

	blocked := func(command string) *validator.Result {
		result := v.Validate(context.Background(), bash(command))

		Expect(result.Passed).To(BeFalse(), "passed: %q", command)
		Expect(result.ShouldBlock).To(BeTrue(), "not blocked: %q", command)
		Expect(result.Reference).To(Equal(validator.RefShellNesting))

		return result
	}

	It("blocks a command nested past the limit", func() {
		result := blocked(strings.Repeat("env ", 12) + "git commit -m x")

		Expect(
			result.Message,
		).To(ContainSubstring("nests launchers, scripts or aliases too deeply"))
		Expect(result.Findings).To(ConsistOf(SatisfyAll(
			HaveField("Location", "via "+strings.Repeat("env > ", 7)+"env"),
			HaveField("Message", ContainSubstring("env launches commands nested more than 8")),
			HaveField("Required", ContainSubstring("at most 8")),
			HaveField("Repair", ContainSubstring("Run the inner command directly")),
		)))
	})

	It("blocks a command that does not parse and says where", func() {
		result := blocked(`git commit -m "x" && (`)

		Expect(result.Message).To(ContainSubstring("does not parse as bash"))
		Expect(result.Message).To(ContainSubstring("parses commands as bash, not zsh"))
		Expect(result.Findings).To(ConsistOf(SatisfyAll(
			HaveField("Location", MatchRegexp(`^line 1, column \d+$`)),
			HaveField("Repair", ContainSubstring("fix it")),
			HaveField("Repair", ContainSubstring("rewrite zsh syntax in bash")),
		)))
	})

	It("names zsh syntax instead of calling it broken shell", func() {
		result := blocked(
			`typeset -A NUM; NUM[a]=1; list=a,b; for x in ${(s:,:)list}; do echo $x; done`,
		)

		Expect(result.Message).To(ContainSubstring("zsh syntax (parameter expansion flags)"))
		Expect(result.Message).NotTo(ContainSubstring("does not parse as shell"))
		Expect(result.Findings).To(ConsistOf(SatisfyAll(
			HaveField("Location", "line 1, column 46"),
			HaveField(
				"Message",
				ContainSubstring("zsh syntax bash does not parse (parameter expansion flags)"),
			),
			HaveField("Required", "valid bash syntax"),
			HaveField("Repair", ContainSubstring("Rewrite the command in bash syntax")),
		)))
	})

	It("reports zsh syntax bash cannot name without a construct", func() {
		result := blocked(`{ git push }`)

		Expect(result.Message).To(
			Equal(
				"Command does not parse as bash and may use zsh syntax, so klaudiush cannot inspect it",
			),
		)
	})

	It("does not call a zsh short form it cannot recognize broken shell", func() {
		result := blocked(`if [[ -n x ]] { git push }`)

		Expect(result.Message).To(ContainSubstring("parses commands as bash, not zsh"))
		Expect(result.Message).NotTo(ContainSubstring("does not parse as shell"))
	})

	It("points a broken command with zsh syntax at the real break", func() {
		result := blocked(`echo ${(s:,:)list} && (`)

		Expect(result.Message).To(ContainSubstring("does not parse as bash"))
		Expect(result.Findings).To(ConsistOf(
			HaveField("Location", "line 1, column 23"),
		))
	})

	It("points a broken command with a glob qualifier at the real break", func() {
		result := blocked(`ls *.go(N) && (`)

		Expect(result.Findings).To(ConsistOf(
			HaveField("Location", "line 1, column 15"),
		))
	})

	It("hedges a zsh loop form the zsh grammar does not know", func() {
		result := blocked(`for x (a b) git push`)

		Expect(result.Message).To(Equal(
			"Command does not parse as bash and uses zsh syntax (short for loops) " +
				"klaudiush cannot inspect",
		))
		Expect(result.Findings).To(ConsistOf(SatisfyAll(
			HaveField("Location", MatchRegexp(`^line 1, column \d+$`)),
			HaveField("Message", ContainSubstring("short for loops are zsh syntax")),
			HaveField("Repair", ContainSubstring(
				"Rewrite short for loops in bash, and fix the syntax at that position if it is broken",
			)),
		)))
	})

	DescribeTable("explains each kind of opaque operation distinctly",
		func(command, summary, location, message, repair string) {
			result := blocked(command)

			Expect(result.Message).To(ContainSubstring(summary))
			Expect(result.Findings).To(ConsistOf(SatisfyAll(
				HaveField("Reference", validator.RefShellNesting),
				HaveField("Location", location),
				HaveField("Message", ContainSubstring(message)),
				HaveField("Repair", ContainSubstring(repair)),
			)))
		},
		Entry("a script path from a variable",
			`sudo bash "$KLAUDIUSH_TEST_UNSET_DIR/run.sh"`,
			"runs a script klaudiush cannot read",
			"via sudo > bash",
			"script run.sh cannot be inspected: its path depends on a variable",
			"Use a literal script path",
		),
		Entry("a relative script after an unknown cd",
			`cd "$KLAUDIUSH_TEST_UNSET_DIR" && bash run.sh`,
			"runs a script klaudiush cannot read",
			"via bash",
			"relative path after a cd",
			"Use an absolute script path",
		),
		Entry("a script written with unknown content",
			`echo "$KLAUDIUSH_TEST_UNSET_BODY" > s.sh && bash s.sh`,
			"runs a script klaudiush cannot read",
			"via bash",
			"written earlier on the line",
			"Write the script with literal content",
		),
		Entry("a nested script that does not parse",
			`env bash -c 'git status && ('`,
			"runs a script that does not parse as bash",
			"via env > bash",
			"inline script does not parse as bash",
			"Fix the syntax of the nested script",
		),
		Entry("an unknown git subcommand",
			"HOME=/nonexistent-klaudiush git zz",
			"runs a git subcommand klaudiush cannot resolve",
			"command",
			"git zz is not a git builtin",
			"Use the builtin subcommand",
		),
		Entry("a function forwarding arguments it cannot follow",
			`f() { git "${@:1}"; }; f commit`,
			"calls a function whose arguments klaudiush cannot follow",
			"command",
			"function f forwards arguments",
			`forward arguments with plain "$@"`,
		),
		Entry("a program from an unknown variable",
			`sudo "$KLAUDIUSH_TEST_UNSET_PROGRAM" origin main`,
			"runs a program whose name klaudiush cannot resolve",
			"via sudo",
			"the program name comes from a variable klaudiush cannot resolve",
			"assign the variable a literal value earlier on the same line",
		),
		Entry("a program from a variable in a loop",
			`for f in a; do $KLAUDIUSH_TEST_UNSET_PROGRAM "$f"; done`,
			"runs a program whose name klaudiush cannot resolve",
			"command",
			"klaudiush resolves no variable inside a loop",
			"run the command outside the loop",
		),
		Entry("a program from a variable in a new shell",
			`bash -c '$KLAUDIUSH_TEST_UNSET_PROGRAM x'`,
			"runs a program whose name klaudiush cannot resolve",
			"via bash",
			"inside a new shell or script",
			"Write the program name literally inside the nested shell",
		),
		Entry("a program from a variable after IFS changes",
			`IFS=,; $KLAUDIUSH_TEST_UNSET_PROGRAM x`,
			"runs a program whose name klaudiush cannot resolve",
			"command",
			"after an earlier command changed how variables expand",
			"separate command from the one that changed IFS",
		),
		Entry("a program from command output",
			`$(echo git) push`,
			"runs a program whose name klaudiush cannot resolve",
			"command",
			"the program name comes from command output",
			"Write the program name or path literally instead of computing it",
		),
		Entry("a git subcommand from an unknown variable",
			`sudo git "$KLAUDIUSH_TEST_UNSET_SUB"`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"via sudo",
			"the git command word comes from a variable klaudiush cannot resolve",
			"assign the variable a literal value earlier on the same line",
		),
		Entry("a gh action from command output",
			`gh pr $(echo create)`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"command",
			"the gh command word comes from command output",
			"Write the subcommand literally instead of computing it",
		),
		Entry(
			"a container entrypoint from an unknown variable",
			`docker run --entrypoint "$KLAUDIUSH_TEST_UNSET_EP" alpine push`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"via docker",
			"the container --entrypoint or a word before it comes from a variable klaudiush cannot resolve",
			"Write the entrypoint, options and image literally",
		),
		Entry("a container entrypoint from command output",
			`podman run --entrypoint=$(which git) alpine push`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"via podman",
			"the container --entrypoint or a word before it comes from command output",
			"Write the entrypoint, options and image literally",
		),
		Entry(
			"a container exec container from an unknown variable",
			`docker exec "$KLAUDIUSH_TEST_UNSET_CTR" echo git push`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"via docker",
			"the exec container or an option before it comes from a variable klaudiush cannot resolve",
			"Write the container name and exec options literally",
		),
		Entry("a container exec container from command output",
			`podman exec $(podman ps -q) ls`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"via podman",
			"the exec container or an option before it comes from command output",
			"Write the container name and exec options literally",
		),
		Entry("container exec options of unknown arity",
			"docker exec"+strings.Repeat(" --x -v", 40)+" c ls",
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"via docker",
			"the exec container or an option before it follows more options",
			"Attach option values with = (--opt=value), or drop options before the container",
		),
		Entry("a zsh glob qualifier that runs code",
			`ls *(e:'git push --no-verify':)`,
			"it uses a glob that runs code klaudiush cannot inspect",
			"command",
			"glob qualifier (e) runs shell code for every file",
			"Select the files another way",
		),
		Entry("a zsh glob qualifier that calls a function",
			`ls *(+fn)`,
			"it uses a glob that runs code klaudiush cannot inspect",
			"command",
			"glob qualifier (+func) runs shell code",
			"quote the word if it is meant literally",
		),
		Entry("a command substitution inside an extended glob",
			"ls *(a$(git push))",
			"it uses a glob that runs code klaudiush cannot inspect",
			"command",
			"an extended glob holds a command substitution",
			"Run the command separately",
		),
		Entry("eval of an unknown variable",
			`eval "$KLAUDIUSH_TEST_UNSET_LINE"`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"command",
			"eval runs a command line that comes from a variable",
			"Run the commands directly instead of through eval",
		),
		Entry("eval of ssh-agent's setup",
			`eval "$(ssh-agent -s)"`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"command",
			"eval runs the shell setup ssh-agent prints",
			"ssh-agent <command>",
		),
		Entry("eval of mise's setup",
			`eval "$(mise activate bash)"`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"command",
			"eval runs the shell setup mise prints",
			"mise exec -- <command>",
		),
		Entry("eval of direnv's setup",
			`eval "$(direnv export bash)"`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"command",
			"eval runs the shell setup direnv prints",
			"direnv exec . <command>",
		),
		Entry("eval of an unknown tool's output",
			`eval "$(evil-mise activate bash)"`,
			"runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve",
			"command",
			"eval runs a command line that comes from command output",
			"Run the commands directly instead of through eval",
		),
		Entry(
			"a BASH_ENV from command output",
			`BASH_ENV=$(mktemp) bash -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"the startup file BASH_ENV names cannot be inspected: its value comes from command output",
			"Assign BASH_ENV a literal file path",
		),
		Entry("an ENV from an unknown variable",
			`env ENV="$KLAUDIUSH_TEST_UNSET_DIR/rc" sh -i -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"via env",
			"the startup file ENV names cannot be inspected: its path depends on a variable",
			"Assign ENV a literal file path",
		),
		Entry("a relative BASH_ENV after an unknown cd",
			`cd "$KLAUDIUSH_TEST_UNSET_DIR" && BASH_ENV=rc.sh bash -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"relative path after a cd",
			"Use an absolute path for BASH_ENV",
		),
		Entry("a BASH_ENV written with unknown content",
			`echo "$KLAUDIUSH_TEST_UNSET_BODY" > /tmp/rc.sh && BASH_ENV=/tmp/rc.sh bash -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"written earlier on the line",
			"Write the startup file in a separate command",
		),
		Entry("a BASH_ENV that is not a regular file",
			`BASH_ENV=/dev/fd/0 bash -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"cannot be read in full",
			"Keep the startup file a readable regular file",
		),
		Entry("an --rcfile from command output",
			`bash --rcfile "$(mktemp)" -i -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"the startup file --rcfile names cannot be inspected",
			"Pass --rcfile a literal path of a readable file, or drop the option",
		),
		Entry("a HOME from command output",
			`HOME=$(mktemp -d) zsh -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"the startup files under HOME cannot be inspected: its value comes from command output",
			"Assign HOME a literal directory",
		),
		Entry("a zshenv written with unknown content",
			`echo "$KLAUDIUSH_TEST_UNSET_BODY" > /nonexistent-k/.zshenv && `+
				`ZDOTDIR=/nonexistent-k zsh -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"the startup file .zshenv cannot be inspected: it is written earlier on the line",
			"Write the startup file in a separate command",
		),
		Entry("a relative ZDOTDIR after an unknown cd",
			`cd "$KLAUDIUSH_TEST_UNSET_DIR" && ZDOTDIR=. zsh -c true`,
			"starts a shell whose startup file klaudiush cannot read",
			"command",
			"the startup file .zshenv cannot be inspected",
			"Set HOME or ZDOTDIR to an absolute directory",
		),
	)

	It("offers an exception token for eval of a tool's setup", func() {
		result := blocked(`eval "$(direnv export bash)"`)

		Expect(result.Findings).To(ConsistOf(
			HaveField("Repair", HaveSuffix("add # EXC:SHELL002:<reason> to the command")),
		))
	})

	It("does not show the arguments of a tool whose setup eval runs", func() {
		result := blocked(`eval "$(mise activate SECRET-VALUE)"`)

		for _, f := range result.Findings {
			Expect(f.Message + f.Location + f.Actual + f.Repair).
				NotTo(ContainSubstring("SECRET-VALUE"))
		}

		Expect(result.Message).NotTo(ContainSubstring("SECRET-VALUE"))
	})

	It("passes plain extended globs", func() {
		for _, command := range []string{`ls @(a|b).go`, `ls *(foo)`, `ls !(x).go`} {
			Expect(v.Validate(context.Background(), bash(command)).Passed).
				To(BeTrue(), "blocked: %q", command)
		}
	})

	It("passes eval and git words it can resolve", func() {
		for _, command := range []string{
			`X=status; git $X`, `eval "echo hi"`, `G=git; $G status`,
		} {
			Expect(v.Validate(context.Background(), bash(command)).Passed).To(BeTrue(), command)
		}
	})

	It("explains a script that cannot be read in full", func() {
		script := filepath.Join(GinkgoT().TempDir(), "huge.sh")
		Expect(os.WriteFile(script, []byte(strings.Repeat("#\n", 200_000)), 0o600)).To(Succeed())

		result := blocked("bash " + script)

		Expect(result.Findings).To(ConsistOf(SatisfyAll(
			HaveField("Location", "via bash"),
			HaveField("Message", ContainSubstring("script huge.sh cannot be inspected: the file")),
			HaveField("Required", ContainSubstring("256 KiB")),
			HaveField("Repair", ContainSubstring("within the size limit")),
		)))
	})

	It("explains an exhausted inspection budget", func() {
		calls := func(name string) string {
			return strings.Repeat(name+"; ", 60)
		}

		result := blocked("f() { " + calls("g") + "}; g() { " + calls("git status") + "}; f")

		Expect(result.Message).To(ContainSubstring("runs more commands and scripts"))
		Expect(result.Findings).To(ContainElement(SatisfyAll(
			HaveField("Message", ContainSubstring("inspection stopped at")),
			HaveField("Repair", ContainSubstring("Split the work")),
		)))
	})

	It("lists every opaque operation", func() {
		result := blocked(
			`bash "$KLAUDIUSH_TEST_UNSET_DIR/a.sh"; HOME=/nonexistent-klaudiush git zz`,
		)

		Expect(result.Message).To(ContainSubstring("2 parts are opaque"))
		Expect(result.Findings).To(HaveLen(2))
	})

	It("says when it lists only the first opaque operations", func() {
		parts := make([]string, 0, 12)
		for i := range 12 {
			parts = append(parts, "HOME=/nonexistent-klaudiush git zz"+strings.Repeat("z", i))
		}

		result := blocked(strings.Join(parts, "; "))

		Expect(result.Message).To(ContainSubstring("more than 7 parts are opaque"))
		Expect(result.Findings).To(HaveLen(7))
	})

	It("does not show command arguments", func() {
		result := blocked(`bash -c 'git commit -m "SECRET-VALUE" && ('`)

		for _, f := range result.Findings {
			Expect(f.Message + f.Location + f.Actual).NotTo(ContainSubstring("SECRET-VALUE"))
		}
	})

	It("passes ordinary nesting", func() {
		result := v.Validate(context.Background(), bash(`sudo env FOO=1 bash -c "git status"`))

		Expect(result.Passed).To(BeTrue())
	})

	It("passes an empty command", func() {
		Expect(v.Validate(context.Background(), bash("")).Passed).To(BeTrue())
	})
})
