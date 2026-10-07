package git_test

import (
	"context"

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
	subjectLabel = "Squash commit subject"
	prTitleLabel = "PR title"
	signedBody   = `"Body. ` + signoff + `"`
	ghSquash     = `gh pr merge 42 --squash --body ` + signedBody + ` `
	ghAPISquash  = `gh api -X PUT repos/o/r/pulls/42/merge -f merge_method=squash ` +
		`-f commit_message=` + signedBody + ` `
	longSubject = "feat(api): add an endpoint whose subject is far too long"
)

// inputBody writes a JSON body with a heredoc and sends it with gh api --input.
func inputBody(json, fields string) string {
	return "cat > body.json <<'EOF'\n" + json + "\nEOF\n" +
		"gh api -X PUT repos/o/r/pulls/42/merge --input body.json " + fields
}

var _ = Describe("MergeValidator squash commit subject", func() {
	var (
		runner   *fakePRRunner
		validate func(command string) *validator.Result
	)

	newValidate := func(cfg *config.MergeValidatorConfig) func(string) *validator.Result {
		mergeValidator := git.NewMergeValidator(
			logger.NewNoOpLogger(), gitpkg.NewFakeRunner(), cfg, nil,
		)
		mergeValidator.ExportSetCommandRunner(runner)

		return func(command string) *validator.Result {
			return mergeValidator.Validate(context.Background(), &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: command},
			})
		}
	}

	BeforeEach(func() {
		runner = &fakePRRunner{stdout: validPRDetails}
		validate = newValidate(nil)
	})

	DescribeTable(
		"checks a subject that replaces the PR title with the PR title rules",
		func(command string) {
			result := validate(command)

			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
			Expect(result.Message).To(ContainSubstring(subjectLabel))
		},
		Entry("gh pr merge --subject", ghSquash+`--subject "Add endpoint"`),
		Entry("gh pr merge -t", ghSquash+`-t "Add endpoint"`),
		Entry("gh pr merge --subject=", ghSquash+`"--subject=Add endpoint"`),
		Entry("gh pr merge -t=", ghSquash+`"-t=Add endpoint"`),
		Entry("gh pr merge -tvalue", ghSquash+`"-tAdd endpoint"`),
		Entry("gh pr merge -st value", `gh pr merge 42 --body `+signedBody+` -st "Add endpoint"`),
		Entry("gh pr merge --auto", ghSquash+`--auto --subject "Add endpoint"`),
		Entry("gh pr merge with an overlong subject", ghSquash+`--subject "`+longSubject+`"`),
		Entry(
			"gh pr merge with a subject lacking a scope",
			ghSquash+`--subject "feat: add endpoint"`,
		),
		Entry("gh api -f commit_title", ghAPISquash+`-f commit_title="Add endpoint"`),
		Entry(
			"gh api with an overlong commit_title",
			ghAPISquash+`-f commit_title="`+longSubject+`"`,
		),
		Entry("gh api commit_title in the query string",
			`gh api -X PUT 'repos/o/r/pulls/42/merge?commit_title=Add+endpoint' `+
				`-f merge_method=squash -f commit_message=`+signedBody),
		Entry("curl JSON commit_title",
			`curl -X PUT `+restMergeURL+` -d '{"merge_method":"squash",`+
				`"commit_message":"Body. `+signoff+`","commit_title":"Add endpoint"}'`),
		Entry("httpie commit_title",
			`http PUT `+restMergeURL+` merge_method=squash `+
				`commit_message='Body. `+signoff+`' commit_title='Add endpoint'`),
	)

	DescribeTable(
		"passes a conventional subject",
		func(command string) {
			Expect(validate(command).Passed).To(BeTrue())
			Expect(runner.calls).To(HaveLen(1))
		},
		Entry("gh pr merge --subject", ghSquash+`--subject "feat(api): add endpoint"`),
		Entry(
			"gh pr merge with an empty --subject, which keeps the PR title",
			ghSquash+`--subject ""`,
		),
		Entry("gh api -f commit_title", ghAPISquash+`-f commit_title="feat(api): add endpoint"`),
		Entry("a revert subject over the limit",
			ghSquash+`--subject 'Revert "feat(api): add an endpoint that is far too long"'`),
	)

	DescribeTable(
		"leaves a subject it cannot read alone, like --body-file, and still checks the PR title",
		func(command string) {
			Expect(validate(command).Passed).To(BeTrue())

			runner.stdout = invalidPRDetails
			result := validate(command)

			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
			Expect(result.Message).To(ContainSubstring(prTitleLabel))
		},
		Entry("gh api commit_title read from a file", ghAPISquash+`-F commit_title=@title.txt`),
		Entry("a request body that cannot be read",
			`curl -X PUT `+restMergeURL+` -d @missing-body.json`),
	)

	DescribeTable("checks the PR title when no subject is given, in both forms",
		func(command string) {
			runner.stdout = invalidPRDetails
			result := validate(command)

			Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
			Expect(result.Message).To(ContainSubstring(prTitleLabel))
		},
		Entry("gh pr merge", ghSquash),
		Entry("gh api", ghAPISquash),
	)

	DescribeTable("checks the subject even when the pull request cannot be fetched",
		func(command string) {
			runner.stdout = "not json"
			result := validate(command)

			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
			Expect(result.Message).To(ContainSubstring(subjectLabel))
		},
		Entry("gh pr merge", ghSquash+`--subject "Add endpoint"`),
		Entry("gh api", ghAPISquash+`-f commit_title="Add endpoint"`),
	)

	It("checks a commit_title httpie sends in the query string", func() {
		result := validate(`http PUT ` + restMergeURL + ` merge_method=squash ` +
			`commit_message='Body. ` + signoff + `' commit_title=='Add endpoint'`)

		Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
		Expect(result.Message).To(ContainSubstring(subjectLabel))
	})

	DescribeTable("reads a body sent on stdin next to httpie query items or from a redirect",
		func(command string) {
			result := validate(command)

			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
			Expect(result.Message).To(ContainSubstring(subjectLabel))
		},
		Entry("httpie with a query item and a heredoc body",
			`http PUT `+restMergeURL+" page==1 <<'EOF'\n"+
				`{"merge_method":"squash","commit_message":"Body. `+signoff+`","commit_title":"Add endpoint"}`+
				"\nEOF"),
		Entry(
			"xh with a query item and a piped body",
			`echo '{"merge_method":"squash","commit_message":"Body. `+signoff+`","commit_title":"Add endpoint"}' | `+
				`xh PUT `+restMergeURL+` page==1`,
		),
		Entry("httpie with a query item and a multi-line raw body",
			`http PUT `+restMergeURL+` page==1 --raw '{"merge_method":`+"\n"+
				`"squash", "x==":`+"\n"+`"merge", "commit_message":"Body. `+signoff+
				`", "commit_title":"Add endpoint"}'`),
		Entry("httpie with a body redirected from a file written by a heredoc",
			"cat > body.json <<'EOF'\n"+
				`{"merge_method":"squash","commit_message":"Body. `+signoff+`","commit_title":"Add endpoint"}`+
				"\nEOF\nhttp PUT "+restMergeURL+" < body.json"),
	)

	DescribeTable("reads a subject from a variable or a heredoc, in both forms",
		func(command string, wantPass bool) {
			Expect(validate(command).Passed).To(Equal(wantPass))
		},
		Entry("gh pr merge with a valid subject variable",
			`T="feat(api): add endpoint"; `+ghSquash+`--subject "$T"`, true),
		Entry("gh pr merge with a bad subject variable",
			`T="Add endpoint"; `+ghSquash+`--subject "$T"`, false),
		Entry("gh pr merge with a bad subject variable reassigned after the merge",
			`T="Add endpoint"; `+ghSquash+`--subject "$T"; T="feat(api): add endpoint"`, false),
		Entry(
			"gh api with a bad commit_title variable reassigned after the merge",
			`T="Add endpoint"; `+ghAPISquash+`-f commit_title="$T"; T="feat(api): add endpoint"`,
			false,
		),
		Entry("gh pr merge with an unresolved subject variable, which is not inspected",
			ghSquash+`--subject "$UNSET"`, true),
		Entry("gh api with a bad commit_title variable",
			`T="Add endpoint"; `+ghAPISquash+`-f commit_title="$T"`, false),
		Entry("gh pr merge with a heredoc subject",
			ghSquash+"--subject \"$(cat <<'X'\nfeat(api): add endpoint\nX\n)\"", true),
		Entry("gh pr merge with a bad heredoc subject",
			ghSquash+"--subject \"$(cat <<'X'\nAdd endpoint\nX\n)\"", false),
	)

	It("leaves a gh pr merge body variable as written, as before", func() {
		result := validate(`B="Body. ` + signoff + `" gh pr merge 42 --squash --body "$B"`)

		Expect(result.Reference).To(Equal(validator.RefGitMergeSignoff))
	})

	DescribeTable("treats an httpie or xh item that embeds a file as unread",
		func(command string) {
			Expect(validate(command).Passed).To(BeTrue())

			runner.stdout = invalidPRDetails
			result := validate(command)

			Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
		},
		Entry("httpie commit_title=@file",
			`http PUT `+restMergeURL+` merge_method=squash commit_title=@title.txt `+
				`"commit_message=Body. `+signoff+`"`),
		Entry("xh commit_message=@file",
			`xh PUT `+restMergeURL+` merge_method=squash commit_message=@body.txt`),
	)

	It("reads httpie items whose value spans lines one by one", func() {
		result := validate(`http PUT ` + restMergeURL + ` merge_method=squash commit_title=bad ` +
			`commit_message="x` + "\n\n" + signoff + `" note="a` + "\n" + `b"`)

		Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
		Expect(result.Message).To(ContainSubstring(subjectLabel))
	})

	It("checks the PR title as well as a valid subject", func() {
		runner.stdout = invalidPRDetails
		result := validate(ghSquash + `--subject "feat(api): add endpoint"`)

		Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
		Expect(result.Message).To(ContainSubstring(prTitleLabel))
	})

	It("checks the subject of a merge whose pull request is not fetched", func() {
		result := validate(`curl -X PUT https://evil.example/api/v3/repos/o/r/pulls/42/merge ` +
			`-d '{"merge_method":"squash","commit_message":"` + signoff + `","commit_title":"Add endpoint"}'`)

		Expect(runner.calls).To(BeEmpty())
		Expect(result.Reference).To(Equal(validator.RefGitMergeMessage))
		Expect(result.Message).To(ContainSubstring(subjectLabel))
	})

	It("skips merge commits, whose subject is not a squash commit subject", func() {
		Expect(validate(`gh pr merge 42 --merge --subject "Add endpoint"`).Passed).To(BeTrue())
		Expect(
			validate(
				`gh api -X PUT repos/o/r/pulls/42/merge -f commit_title="Add endpoint"`,
			).Passed,
		).
			To(BeTrue())
	})

	It("follows the merge message switch", func() {
		disabled := false
		validate = newValidate(&config.MergeValidatorConfig{
			Message: &config.MergeMessageConfig{Enabled: &disabled},
		})

		Expect(validate(ghSquash + `--subject "Add endpoint"`).Passed).To(BeTrue())
		Expect(validate(ghAPISquash + `-f commit_title="Add endpoint"`).Passed).To(BeTrue())
	})

	It("follows the configured title length", func() {
		maxLength := 72
		validate = newValidate(&config.MergeValidatorConfig{
			Message: &config.MergeMessageConfig{TitleMaxLength: &maxLength},
		})

		Expect(validate(ghSquash + `--subject "` + longSubject + `"`).Passed).To(BeTrue())
		Expect(validate(ghAPISquash + `-f commit_title="` + longSubject + `"`).Passed).To(BeTrue())
	})
})

var _ = Describe("MergeValidator REST merge field sent twice", func() {
	var (
		runner   *fakePRRunner
		validate func(command string) *validator.Result
	)

	BeforeEach(func() {
		runner = &fakePRRunner{stdout: validPRDetails}
		mergeValidator := git.NewMergeValidator(
			logger.NewNoOpLogger(), gitpkg.NewFakeRunner(), nil, nil,
		)
		mergeValidator.ExportSetCommandRunner(runner)

		validate = func(command string) *validator.Result {
			return mergeValidator.Validate(context.Background(), &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: command},
			})
		}
	})

	DescribeTable("fails when any of the values fails",
		func(command string, reference validator.Reference) {
			result := validate(command)

			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(Equal(reference))
		},
		Entry("merge_method merge in the httpie body, squash as an httpie query item",
			`http PUT `+restMergeURL+` merge_method=merge merge_method==squash`,
			validator.RefGitMergeSignoff),
		Entry("merge_method merge in the body, squash as a gh field",
			inputBody(`{"merge_method":"merge"}`, `-f merge_method=squash`),
			validator.RefGitMergeSignoff),
		Entry("merge_method squash in the body, merge as a gh field",
			inputBody(`{"merge_method":"squash"}`, `-f merge_method=merge`),
			validator.RefGitMergeSignoff),
		Entry("merge_method merge in the curl body, squash in the URL query",
			`curl -X PUT '`+restMergeURL+`?merge_method=squash' -d '{"merge_method":"merge"}'`,
			validator.RefGitMergeSignoff),
		Entry("commit_message with the signoff in the body, without it as a gh field",
			inputBody(`{"merge_method":"squash","commit_message":"Body. `+signoff+`"}`,
				`-f commit_message=Body`),
			validator.RefGitMergeSignoff),
		Entry("commit_message without the signoff in the body, with it as a gh field",
			inputBody(`{"merge_method":"squash","commit_message":"Body"}`,
				`-f commit_message=`+signedBody),
			validator.RefGitMergeSignoff),
		Entry(
			"an unreadable --input body, a commit_message without the signoff as a gh field",
			`gh api -X PUT repos/o/r/pulls/42/merge --input missing-body.json -f commit_message=Body`,
			validator.RefGitMergeSignoff,
		),
		Entry("a valid commit_title in the body, a bad one as a gh field",
			inputBody(`{"merge_method":"squash","commit_message":"Body. `+signoff+`",`+
				`"commit_title":"feat(api): add endpoint"}`, `-f commit_title="Add endpoint"`),
			validator.RefGitMergeMessage),
		Entry("a bad commit_title in the body, a valid one as a gh field",
			inputBody(`{"merge_method":"squash","commit_message":"Body. `+signoff+`",`+
				`"commit_title":"Add endpoint"}`, `-f commit_title="feat(api): add endpoint"`),
			validator.RefGitMergeMessage),
		Entry(
			"commit_title repeated in the query string",
			`gh api -X PUT 'repos/o/r/pulls/42/merge?commit_title=feat(api):+ok&commit_title=Add+endpoint' `+
				`-f merge_method=squash -f commit_message=`+signedBody,
			validator.RefGitMergeMessage,
		),
	)

	DescribeTable("passes when every value passes",
		func(command string) {
			Expect(validate(command).Passed).To(BeTrue())
		},
		Entry("the same values in the body and as gh fields",
			inputBody(`{"merge_method":"squash","commit_message":"Body. `+signoff+`",`+
				`"commit_title":"feat(api): add endpoint"}`,
				`-f merge_method=squash -f commit_message=`+signedBody+
					` -f commit_title="feat(api): add endpoint"`)),
		Entry("two different valid subjects and signed messages",
			inputBody(`{"merge_method":"squash","commit_message":"One. `+signoff+`",`+
				`"commit_title":"feat(api): add endpoint"}`,
				`-f commit_message="Two. `+signoff+`" -f commit_title="fix(api): add endpoint"`)),
		Entry("merge and rebase, neither of which squashes",
			inputBody(`{"merge_method":"merge"}`, `-f merge_method=rebase`)),
	)
})
