package git_test

import (
	"context"
	"io"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/exec"
	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

type fakePRRunner struct {
	stdout string
	calls  [][]string
}

func (r *fakePRRunner) Run(_ context.Context, name string, args ...string) exec.CommandResult {
	r.calls = append(r.calls, append([]string{name}, args...))

	return exec.CommandResult{Stdout: r.stdout}
}

func (r *fakePRRunner) RunWithStdin(
	ctx context.Context,
	_ io.Reader,
	name string,
	args ...string,
) exec.CommandResult {
	return r.Run(ctx, name, args...)
}

func (r *fakePRRunner) RunWithTimeout(
	_ time.Duration,
	name string,
	args ...string,
) exec.CommandResult {
	return r.Run(context.Background(), name, args...)
}

const (
	validPRDetails   = `{"number":42,"title":"feat(api): add endpoint","body":"","state":"open"}`
	invalidPRDetails = `{"number":42,"title":"Add endpoint","body":"","state":"open"}`
	restMergeURL     = "https://api.github.com/repos/o/r/pulls/42/merge"
	signoff          = "Signed-off-by: Test User <test@klaudiu.sh>"
)

var _ = Describe("MergeValidator REST pull request merge", func() {
	var (
		runner        *fakePRRunner
		mergeValidate func(command string) *validator.Result
	)

	BeforeEach(func() {
		runner = &fakePRRunner{stdout: validPRDetails}
		mergeValidator := git.NewMergeValidator(
			logger.NewNoOpLogger(), gitpkg.NewFakeRunner(), nil, nil,
		)
		mergeValidator.ExportSetCommandRunner(runner)

		mergeValidate = func(command string) *validator.Result {
			return mergeValidator.Validate(context.Background(), &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: command},
			})
		}
	})

	DescribeTable("requires a signoff in the squash commit message, like gh pr merge --body",
		func(command string) {
			result := mergeValidate(command)

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
			Expect(result.Details["errors"]).To(ContainSubstring("commit_message"))
			Expect(runner.calls).To(HaveLen(1))
			Expect(runner.calls[0]).To(Or(
				Equal([]string{"gh", "api", "repos/o/r/pulls/42", "--jq", "."}),
				Equal([]string{"gh", "api", "repositories/123/pulls/42", "--jq", "."}),
			))
		},
		Entry("gh api -X PUT",
			`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash`),
		Entry("gh api --method PUT with a leading slash",
			`gh api --method PUT /repos/o/r/pulls/42/merge -F merge_method=squash`),
		Entry("gh api with a commit message lacking the signoff",
			`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash -f commit_message=body`),
		Entry("gh api with the number in a variable",
			`N=42; gh api -X PUT "repos/o/r/pulls/$N/merge" -f merge_method=squash`),
		Entry("curl with a JSON body",
			`curl -X PUT `+restMergeURL+` -d '{"merge_method":"squash"}'`),
		Entry(
			"curl without a scheme",
			`curl -X PUT api.github.com/repos/o/r/pulls/42/merge --data '{"merge_method":"squash"}'`,
		),
		Entry("gh api run through a launcher",
			`env GH_TOKEN=x gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash`),
		Entry("gh api inside bash -c",
			`bash -c 'gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash'`),
		Entry("the numeric repository ID alias",
			`curl -X PUT -d '{"merge_method":"squash"}' `+
				`https://api.github.com/repositories/123/pulls/42/merge`),
		Entry("fields gh sends in the query string next to an --input body",
			"gh api -X PUT repos/o/r/pulls/42/merge --input - -f merge_method=squash "+
				"-f commit_message=bad <<< '{}'"),
		Entry("merge_method in the query string",
			`gh api -X PUT 'repos/o/r/pulls/42/merge?merge_method=squash'`),
		Entry(
			"a quoted path segment",
			`curl -X PUT https://api.github.com/repos/o/r/pulls/42/'merge' -d '{"merge_method":"squash"}'`,
		),
		Entry(
			"a URL built from a variable",
			`u=https://api.github.com/repos/o/r/pulls/42/; curl -X PUT "${u}merge" -d '{"merge_method":"squash"}'`,
		),
		Entry("httpie request items",
			`http PUT `+restMergeURL+` merge_method=squash`),
		Entry("gh api with a heredoc body file",
			"cat > body.json <<'EOF'\n{\"merge_method\":\"squash\"}\nEOF\n"+
				"gh api -X PUT repos/o/r/pulls/42/merge --input body.json"),
	)

	DescribeTable("passes a squash merge whose commit message carries the signoff",
		func(command string) {
			Expect(mergeValidate(command).Passed).To(BeTrue())
			Expect(runner.calls).To(HaveLen(1))
		},
		Entry("gh api field",
			`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash `+
				`-f commit_message="Body text. `+signoff+`"`),
		Entry("curl JSON body",
			`curl -X PUT `+restMergeURL+
				` -d '{"merge_method":"squash","commit_message":"Body. `+signoff+`"}'`),
		Entry(
			"commit message read from a file, as with --body-file",
			`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash -F commit_message=@body.txt`,
		),
	)

	DescribeTable("leaves merges that do not squash alone, like gh pr merge --merge and --rebase",
		func(command string) {
			Expect(mergeValidate(command).Passed).To(BeTrue())
			Expect(runner.calls).To(BeEmpty())
		},
		Entry("no merge_method, which REST treats as a merge commit",
			`gh api -X PUT repos/o/r/pulls/42/merge`),
		Entry("merge_method=merge",
			`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=merge`),
		Entry("merge_method=rebase",
			`curl -X PUT `+restMergeURL+` -d '{"merge_method":"rebase"}'`),
	)

	DescribeTable("ignores requests that do not merge a pull request",
		func(command string) {
			Expect(mergeValidate(command).Passed).To(BeTrue())
			Expect(runner.calls).To(BeEmpty())
		},
		Entry("checking whether a pull request is merged",
			`gh api repos/o/r/pulls/42/merge`),
		Entry(
			"a host that is not the GitHub API",
			`curl -X PUT https://example.com/repos/o/r/pulls/42/merge -d '{"merge_method":"squash"}'`,
		),
		Entry("a pull request number that is not a number",
			`gh api -X PUT repos/o/r/pulls/abc/merge -f merge_method=squash`),
	)

	It("checks the pull request title like gh pr merge", func() {
		runner.stdout = invalidPRDetails

		result := mergeValidate(
			`gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash ` +
				`-f commit_message="` + signoff + `"`,
		)

		Expect(result.Passed).To(BeFalse())
		Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
	})

	It("validates a body it cannot read as a squash, like gh pr merge without a method", func() {
		result := mergeValidate(`curl -X PUT ` + restMergeURL + ` -d @missing-body.json`)

		Expect(result.Passed).To(BeTrue())
		Expect(runner.calls).To(HaveLen(1))
	})

	DescribeTable("fetches the pull request from the host gh itself talks to",
		func(command string) {
			mergeValidate(command)

			Expect(runner.calls).To(Equal([][]string{
				{
					"gh", "api", "--hostname=ghe.example.com",
					"repos/o/r/pulls/42", "--jq", ".",
				},
			}))
		},
		Entry(
			"gh api --hostname",
			`gh api --hostname ghe.example.com -X PUT repos/o/r/pulls/42/merge -f merge_method=squash`,
		),
		Entry(
			"gh api --hostname in another case",
			`gh api --hostname GHE.example.com -X PUT repos/o/r/pulls/42/merge -f merge_method=squash`,
		),
		Entry(
			"gh api with a full GitHub Enterprise URL",
			`gh api -X PUT https://ghe.example.com/api/v3/repos/o/r/pulls/42/merge -f merge_method=squash`,
		),
	)

	It("fetches from github.com for a full api.github.com URL", func() {
		mergeValidate(`gh api -X PUT ` + restMergeURL + ` -f merge_method=squash`)

		Expect(runner.calls).To(Equal([][]string{
			{"gh", "api", "repos/o/r/pulls/42", "--jq", "."},
		}))
	})

	Describe("a curl host that is not a configured GitHub API host", func() {
		const unlisted = `curl -X PUT https://evil.example/api/v3/repos/o/r/pulls/42/merge `

		It("never hands the host to gh", func() {
			result := mergeValidate(
				unlisted + `-d '{"merge_method":"squash","commit_message":"` + signoff + `"}'`,
			)

			Expect(runner.calls).To(BeEmpty())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("evil.example"))
		})

		It("still requires the signoff", func() {
			result := mergeValidate(unlisted + `-d '{"merge_method":"squash"}'`)

			Expect(runner.calls).To(BeEmpty())
			Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
		})
	})

	It("does not pass a host that reads as a flag to gh", func() {
		result := mergeValidate(
			`gh api --hostname=--evil -X PUT repos/o/r/pulls/42/merge -f merge_method=squash`,
		)

		Expect(runner.calls).To(BeEmpty())
		Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
	})

	DescribeTable("checks every merge on the line, not only the first",
		func(command string) {
			result := mergeValidate(command)

			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
		},
		Entry("gh pr merge followed by a REST merge",
			`gh pr merge 1 --rebase; gh api -X PUT repos/o/r/pulls/2/merge -f merge_method=squash`),
		Entry("two REST merges",
			`gh api -X PUT repos/o/r/pulls/1/merge; `+
				`gh api -X PUT repos/o/r/pulls/2/merge -f merge_method=squash`),
		Entry("two URLs in one curl",
			`curl -X PUT -d '{"merge_method":"squash"}' `+restMergeURL+` `+
				`https://api.github.com/repos/o/r/pulls/43/merge`),
	)

	It("resolves an endpoint assembled from variables", func() {
		result := mergeValidate(
			`P=pulls; gh api -X PUT "repos/o/r/$P/42/merge" -f merge_method=squash`,
		)

		Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
	})

	It("recognises a configured GitHub API host", func() {
		mergeValidator := git.NewMergeValidator(
			logger.NewNoOpLogger(), gitpkg.NewFakeRunner(), nil, nil,
		).WithAPIHosts([]string{"github.proxy.internal"})
		mergeValidator.ExportSetCommandRunner(runner)

		result := mergeValidator.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{
				Command: `curl -X PUT https://github.proxy.internal/repos/o/r/pulls/42/merge ` +
					`-d '{"merge_method":"squash"}'`,
			},
		})

		Expect(result.Passed).To(BeFalse())
		Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
		Expect(runner.calls).To(Equal([][]string{
			{"gh", "api", "--hostname=github.proxy.internal", "repos/o/r/pulls/42", "--jq", "."},
		}))
	})
})
