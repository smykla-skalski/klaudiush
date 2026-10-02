package git_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	validatorpkg "github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("CommitValidator findings with AI attribution", func() {
	findings := func(command string) []validatorpkg.Finding {
		fakeGit := gitpkg.NewFakeRunner()
		fakeGit.StagedFiles = []string{"file.txt"}

		v := git.NewCommitValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		result := v.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		})
		Expect(result.Passed).To(BeFalse())

		return result.Findings
	}

	validate := func(command string) []string {
		found := findings(command)

		codes := make([]string, 0, len(found))
		for _, f := range found {
			codes = append(codes, f.Code())
		}

		return codes
	}

	attributionLocations := func(command string) []string {
		var locations []string

		for _, f := range findings(command) {
			if f.Reference == validatorpkg.RefGitClaudeAttr {
				locations = append(locations, f.Location)
			}
		}

		return locations
	}

	It("keeps every message finding next to the attribution", func() {
		codes := validate("git commit -sS -m 'feat(ci): " + strings.Repeat("x", 60) +
			"\n\nFixes #12\n\nGenerated with Claude Code'")

		Expect(codes).To(ContainElements("GIT006", "GIT004", "GIT011", "GIT012"))
	})

	It("adds attribution found outside the parsed message", func() {
		codes := validate("git commit -sS -m 'feat(api): " + strings.Repeat("x", 60) +
			"' --trailer 'Co-authored-by: Claude <noreply@anthropic.com>'")

		Expect(codes).To(ContainElements("GIT004", "GIT012"))
	})

	It("still reports attribution alone when nothing else is wrong", func() {
		codes := validate("git commit -sS -m 'feat(api): add x' " +
			"--trailer 'Co-authored-by: Claude <noreply@anthropic.com>'")

		Expect(codes).To(Equal([]string{"GIT012"}))
	})

	It("reports attribution in both the message and a trailer separately", func() {
		locations := attributionLocations("git commit -sS -m 'feat(api): " +
			strings.Repeat("x", 60) + "\n\nGenerated with Claude Code' " +
			"--trailer 'Co-authored-by: Claude <noreply@anthropic.com>'")

		Expect(locations).To(ConsistOf("message", "command arguments outside the message"))
	})

	It("reports attribution only in the message once", func() {
		locations := attributionLocations("git commit -sS -m 'feat(api): " +
			strings.Repeat("x", 60) + "\n\nGenerated with Claude Code'")

		Expect(locations).To(Equal([]string{"message"}))
	})

	It("reports attribution only in a heredoc message once", func() {
		locations := attributionLocations("git commit -sS -m \"$(cat <<'EOF'\nfeat(api): " +
			strings.Repeat("x", 60) +
			"\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)\nEOF\n)\"")

		Expect(locations).To(Equal([]string{"message"}))
	})
})

var _ = Describe("CommitValidator body markdown findings", func() {
	It("folds the context of one violation into its finding", func() {
		fakeGit := gitpkg.NewFakeRunner()
		fakeGit.StagedFiles = []string{"file.txt"}

		v := git.NewCommitValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		result := v.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{
				Command: "git commit -sS -m 'feat(api): add x\n\n## Summary\nText under it\n" +
					"Intro text\n```go\nx := 1\n```'",
			},
		})
		Expect(result.Passed).To(BeFalse())

		var messages []string

		for _, f := range result.Findings {
			if f.Reference == validatorpkg.RefGitBadBody && f.Location == "body" {
				messages = append(messages, f.Message)
			}
		}

		Expect(messages).To(HaveLen(2))
		Expect(messages[0]).To(HavePrefix("Line "))
		Expect(messages[0]).To(ContainSubstring("Header: '## Summary'"))
		Expect(messages[0]).To(ContainSubstring("Next line: 'Text under it'"))
		Expect(messages[1]).To(HavePrefix("Line "))
		Expect(messages[1]).To(ContainSubstring("Previous line: 'Intro text'"))
	})
})
