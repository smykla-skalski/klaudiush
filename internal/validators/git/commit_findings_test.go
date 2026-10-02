package git_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-skalski/klaudiush/internal/git"
	"github.com/smykla-skalski/klaudiush/internal/validators/git"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("CommitValidator findings with AI attribution", func() {
	validate := func(command string) []string {
		fakeGit := gitpkg.NewFakeRunner()
		fakeGit.StagedFiles = []string{"file.txt"}

		v := git.NewCommitValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		result := v.Validate(context.Background(), &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: command},
		})
		Expect(result.Passed).To(BeFalse())

		codes := make([]string, 0, len(result.Findings))
		for _, f := range result.Findings {
			codes = append(codes, f.Code())
		}

		return codes
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
})
