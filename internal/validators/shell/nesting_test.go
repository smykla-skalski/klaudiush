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
			HaveField("Message", ContainSubstring("is zsh syntax bash does not parse")),
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

	It("hedges a zsh loop form the zsh grammar does not know", func() {
		result := blocked(`for x (a b) git push`)

		Expect(result.Message).To(Equal(
			"Command does not parse as bash and uses zsh syntax (short for loops) " +
				"klaudiush cannot inspect",
		))
		Expect(result.Findings).To(ConsistOf(SatisfyAll(
			HaveField("Location", MatchRegexp(`^line 1, column \d+$`)),
			HaveField("Message", ContainSubstring("short for loops are zsh syntax")),
			HaveField("Repair", ContainSubstring("Fix the shell syntax at that position")),
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
		Entry("a git subcommand from an unknown variable",
			`sudo git "$KLAUDIUSH_TEST_UNSET_SUB"`,
			"runs eval, git or gh with a word klaudiush cannot resolve",
			"via sudo",
			"the git command word comes from a variable klaudiush cannot resolve",
			"assign the variable a literal value earlier on the same line",
		),
		Entry("a gh action from command output",
			`gh pr $(echo create)`,
			"runs eval, git or gh with a word klaudiush cannot resolve",
			"command",
			"the gh command word comes from command output",
			"Write the subcommand literally instead of computing it",
		),
		Entry("eval of an unknown variable",
			`eval "$KLAUDIUSH_TEST_UNSET_LINE"`,
			"runs eval, git or gh with a word klaudiush cannot resolve",
			"command",
			"eval runs a command line that comes from a variable",
			"Run the commands directly instead of through eval",
		),
	)

	It("passes eval and git words it can resolve", func() {
		for _, command := range []string{`X=status; git $X`, `eval "echo hi"`} {
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
