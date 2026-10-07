package dispatcher_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators/file"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("Dispatcher Bash heredoc appending to a Rust file", func() {
	var repo string

	BeforeEach(func() {
		dir, err := filepath.EvalSymlinks(GinkgoT().TempDir())
		Expect(err).NotTo(HaveOccurred())

		repo = dir
		Expect(os.MkdirAll(filepath.Join(repo, "src"), 0o700)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(repo, "src", "lib.rs"),
			[]byte("pub fn one() -> u8 {\n    1\n}\n"),
			0o600,
		)).To(Succeed())
	})

	dispatch := func(appended string) []*dispatcher.ValidationError {
		reg := validator.NewRegistry()
		reg.Register(
			file.NewAICommentValidator(
				logger.NewNoOpLogger(),
				&config.AICommentValidatorConfig{Mode: config.AICommentModeStrict},
				nil,
			),
			validator.ToolTypeIs(hook.ToolTypeWrite),
		)

		return dispatcher.NewDispatcher(reg, logger.NewNoOpLogger()).Dispatch(
			context.Background(),
			&hook.Context{
				Provider:   hook.ProviderClaude,
				Event:      hook.CanonicalEventBeforeTool,
				EventType:  hook.EventTypePreToolUse,
				ToolName:   hook.ToolTypeBash,
				ToolFamily: hook.ToolFamilyShell,
				WorkingDir: repo,
				ToolInput: hook.ToolInput{
					Command: "cat >> src/lib.rs <<'EOF'\n" + appended + "EOF",
				},
			},
		)
	}

	It("allows a test module with attributes", func() {
		Expect(dispatch(
			"#[cfg(test)]\nmod tests {\n    #[test]\n    fn one_is_one() {\n        assert_eq!(super::one(), 1);\n    }\n}\n",
		)).To(BeEmpty())
	})

	It("blocks a comment in the appended text", func() {
		errs := dispatch("#[must_use]\npub fn two() -> u8 {\n    2 // holds the second value\n}\n")
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Validator).To(Equal("validate-ai-comments"))
		Expect(errs[0].Message).To(ContainSubstring("Line 3: // holds the second value"))
	})
})
