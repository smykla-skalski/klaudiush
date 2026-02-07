package git_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-labs/klaudiush/internal/git"
	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("CommitValidator", func() {
	var (
		validator *git.CommitValidator
		log       logger.Logger
		fakeGit   *gitpkg.FakeRunner
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		fakeGit = gitpkg.NewFakeRunner()
		// By default, set up fake to have staged files so staging check passes
		fakeGit.StagedFiles = []string{"file.txt"}
		validator = git.NewCommitValidator(log, fakeGit, nil, nil)
	})

	Describe("Flag validation", func() {
		Context("when -sS flags are present", func() {
			It("should pass with -sS flags", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add new feature"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with -s and -S separately", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -s -S -m "fix(auth): resolve bug"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with long flags", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit --signoff --gpg-sign -m "docs(readme): update readme"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with --all flag instead of -a", func() {
				// Set no staged files to test that --all flag bypasses staging check
				fakeGit.StagedFiles = []string{}
				fakeGit.ModifiedFiles = []string{"file1.go", "file2.go"}

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS --all -m "feat(files): update files"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when -sS flags are missing", func() {
			It("should fail without -s flag", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -S -m "feat(test): test message"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git commit missing required flags:"))
			})

			It("should fail without -S flag", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -s -m "feat(test): test message"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git commit missing required flags:"))
			})

			It("should fail without any signing flags", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -m "feat(test): test message"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git commit missing required flags:"))
			})
		})
	})

	Describe("Commit message validation", func() {
		Context("when message is valid", func() {
			It("should pass with valid conventional commit", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add new endpoint"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with scope", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(auth): resolve login issue"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail without scope", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "chore: update dependencies"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("Scope is mandatory"))
			})

			It("should pass with breaking change marker", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api)!: remove deprecated API"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when title is invalid", func() {
			It("should fail with title over 50 characters", func() {
				longTitle := "feat(api): this is a very long commit message that exceeds the fifty character limit"
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + longTitle + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Commit message validation failed"))
				Expect(result.Details["errors"]).To(ContainSubstring("Title exceeds 50 characters"))
			})

			It("should pass with Unicode ellipsis at exactly 50 characters", func() {
				// Unicode ellipsis (…) is 3 bytes but counts as 1 character
				// This message is 50 characters (runes) but 52 bytes
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "chore(makefile): correct quote escaping in gke.mk…"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail with Unicode ellipsis over 50 characters", func() {
				// Unicode ellipsis (…) is 3 bytes but counts as 1 character
				// This message is 51 characters (runes) but 53 bytes
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "chore(makefile): correct quote escaping in gke.mk…x"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Commit message validation failed"))
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Title exceeds 50 characters (51 chars)"))
			})

			It("should fail with non-conventional format", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "Add new feature"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("doesn't follow conventional commits format"))
			})

			It("should fail with invalid type", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "invalid(api): add endpoint"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("doesn't follow conventional commits format"))
			})
		})

		Context("when infrastructure scope is misused", func() {
			It("should fail with feat(ci)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(ci): add new workflow"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'ci(...)' not 'feat(ci)'"))
			})

			It("should fail with fix(test)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(test): update test helper"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'test(...)' not 'fix(test)'"))
			})

			It("should fail with feat(docs)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(docs): add new section"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'docs(...)' not 'feat(docs)'"))
			})

			It("should fail with fix(build)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(build): update makefile"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'build(...)' not 'fix(build)'"))
			})

			It("should pass with ci(workflow)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "ci(workflow): add new step"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when commit is a revert", func() {
			It("should pass with git revert format using double quotes", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m 'Revert "feat(api): add new endpoint"'`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with git revert format using single quotes", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "Revert 'fix(auth): resolve login issue'"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with revert of non-conventional commit", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m 'Revert "Add new feature"'`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with revert title over 50 characters (unlimited by default)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m 'Revert "feat(api): this is a very long commit message title"'`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It(
				"should fail with revert title over 50 characters when unlimited is disabled",
				func() {
					allowUnlimited := false
					cfg := &config.CommitValidatorConfig{
						Message: &config.CommitMessageConfig{
							AllowUnlimitedRevertTitle: &allowUnlimited,
						},
					}

					strictValidator := git.NewCommitValidator(log, fakeGit, cfg, nil)

					ctx := &hook.Context{
						EventType: hook.EventTypePreToolUse,
						ToolName:  hook.ToolTypeBash,
						ToolInput: hook.ToolInput{
							Command: `git commit -sS -a -m 'Revert "feat(api): this is a very long commit message title"'`,
						},
					}

					result := strictValidator.Validate(context.Background(), ctx)
					Expect(result.Passed).To(BeFalse())
					Expect(
						result.Details["errors"],
					).To(ContainSubstring("Title exceeds 50 characters"))
				},
			)

			It("should fail without quotes around original message", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "Revert feat(api): add endpoint"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("doesn't follow conventional commits format"))
			})
		})

		Context("when body has line length issues", func() {
			It("should pass with lines under 72 characters", func() {
				message := `feat(api): add endpoint

This is a normal commit body with lines that are well within the
seventy-two character limit for proper formatting.`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with URLs exceeding 72 characters", func() {
				message := `feat(api): add endpoint

Reference: https://github.com/smykla-labs/klaudiush/pull/123/files#diff-abc123def456`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				// Should fail for PR reference, but pass for URL length
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).ToNot(ContainSubstring("exceeds 72 characters"))
			})

			It("should fail with lines over 77 characters", func() {
				message := `feat(api): add endpoint

This is a line that definitely exceeds the seventy-two character limit and even the tolerance of seventy-seven characters total`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("exceeds 72 characters"))
			})
		})

		Context("when body has list formatting issues", func() {
			It("should pass with empty line before first list item", func() {
				message := `feat(api): add endpoint

Changes:

- Add new endpoint
- Update documentation`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail without empty line before first list item", func() {
				message := `feat(api): add endpoint
- Add new endpoint
- Update documentation`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Missing empty line before first list item"))
			})

			It("should handle numbered lists", func() {
				message := `feat(api): add endpoint
1. First item
2. Second item`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Missing empty line before first list item"))
			})
		})

		Context("when message has trailer-like patterns with commas", func() {
			It("should pass with 'Solution:' in body containing commas", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
build(makefile): fix version script output parsing

Fix "too many arguments" error in make test by properly
parsing multi-line output from script.

Solution: Use foreach to evaluate each line separately,
ensuring each variable assignment is processed independently.
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with 'Changes:' list containing commas", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
fix(parser): fix strict trailer validation

Fix issue with go-conventionalcommits library.

Changes:

- Parse full message first, fall back on errors
- Manually extract body, footers in fallback mode
- Preserve functionality (breaking changes, footers)
- Add extractBodyAndFooters helper
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should still detect BREAKING CHANGE in fallback mode", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
feat(api): remove deprecated endpoint

Remove old API endpoint.

Solution: Use new endpoint, migrate existing code.

BREAKING CHANGE: old endpoint no longer available
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should validate title format even in fallback mode", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
invalid commit title

This has a Solution: Use X, Y, and Z pattern that
would trigger trailer validation.
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("conventional commits format"))
			})
		})

		Context("when message contains PR references", func() {
			It("should fail with #123 reference", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(api): resolve issue #123"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("PR references found"))
				Expect(result.Details["errors"]).To(ContainSubstring("#123"))
			})

			It("should fail with GitHub URL reference", func() {
				message := `fix(api): resolve issue

See github.com/smykla-labs/klaudiush/pull/123`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("PR references found"))
			})

			It("should pass with plain number", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(api): resolve issue 123"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when message contains AI attribution", func() {
			It("should fail with AI attribution footer", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add endpoint\\n\\nGenerated by Claude"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should allow 'claude' in technical context", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(integration): add claude integration"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail with 'Claude AI' pattern", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add feature\\n\\nWith Claude AI assistance"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should pass with CLAUDE.md file reference", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "docs(guide): add CLAUDE.md"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with CLAUDE.md in body", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
docs(guide): update project guide

Update CLAUDE.md with new architecture details.
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with CLAUDE (uppercase) reference", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "docs(guide): update CLAUDE file"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail with 'Generated with Claude' markdown link", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
chore(styles): add duplicate btn-radius definition

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should fail with Co-Authored-By: Claude", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
chore(styles): add duplicate btn-radius definition

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should fail with 'generated with claude' pattern", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add endpoint\\n\\nGenerated with Claude assistance"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should fail with full Claude Code attribution footer", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
chore(styles): add duplicate btn-radius definition

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})
		})

		Context("when message contains forbidden patterns", func() {
			It("should fail with tmp/ directory reference", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add temp storage in tmp/"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("Forbidden pattern found"))
				Expect(result.Details["errors"]).To(ContainSubstring("tmp/"))
			})

			It("should fail with standalone tmp word", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(storage): store files in tmp directory"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("Forbidden pattern found"))
				Expect(result.Details["errors"]).To(ContainSubstring("tmp"))
			})

			It("should pass when tmp is part of a longer word", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(template): add new template file"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail with tmp in commit body", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
feat(storage): add file storage

Store temporary files in tmp/ directory for processing.
EOF
)"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("Forbidden pattern found"))
				Expect(result.Details["errors"]).To(ContainSubstring("tmp/"))
			})
		})

		Context("when message has signoff", func() {
			It("should pass with signoff when no expected signoff configured", func() {
				message := `feat(api): add endpoint

Signed-off-by: Test User <test@klaudiu.sh>`

				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("File-based commit messages", func() {
		var tmpFile string

		AfterEach(func() {
			if tmpFile != "" {
				_ = os.Remove(tmpFile)
				tmpFile = ""
			}
		})

		It("should pass with valid message from file using -F", func() {
			file, err := os.CreateTemp("", "commit-msg-*.txt")
			Expect(err).ToNot(HaveOccurred())
			tmpFile = file.Name()

			_, err = file.WriteString(
				"feat(api): add new endpoint\n\nThis adds a new API endpoint.",
			)
			Expect(err).ToNot(HaveOccurred())
			file.Close()

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS -a -F ` + tmpFile,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass with valid message from file using --file", func() {
			file, err := os.CreateTemp("", "commit-msg-*.txt")
			Expect(err).ToNot(HaveOccurred())
			tmpFile = file.Name()

			_, err = file.WriteString("fix(auth): resolve login issue")
			Expect(err).ToNot(HaveOccurred())
			file.Close()

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS -a --file ` + tmpFile,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail with invalid message from file", func() {
			file, err := os.CreateTemp("", "commit-msg-*.txt")
			Expect(err).ToNot(HaveOccurred())
			tmpFile = file.Name()

			_, err = file.WriteString("Add new feature")
			Expect(err).ToNot(HaveOccurred())
			file.Close()

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS -a -F ` + tmpFile,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(
				result.Details["errors"],
			).To(ContainSubstring("doesn't follow conventional commits format"))
		})

		It("should fail when file does not exist", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS -a -F /nonexistent/file.txt`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Failed to read commit message"))
		})

		It("should pass with empty file (message from editor)", func() {
			file, err := os.CreateTemp("", "commit-msg-*.txt")
			Expect(err).ToNot(HaveOccurred())
			tmpFile = file.Name()
			file.Close()

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS -a -F ` + tmpFile,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should handle combined flags with file", func() {
			file, err := os.CreateTemp("", "commit-msg-*.txt")
			Expect(err).ToNot(HaveOccurred())
			tmpFile = file.Name()

			_, err = file.WriteString("feat(api): add endpoint")
			Expect(err).ToNot(HaveOccurred())
			file.Close()

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sSF ` + tmpFile,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("No message flag", func() {
		It("should pass when no -m flag (message from editor)", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Non-git commands", func() {
		It("should pass for non-git commands", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `echo hello`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for non-commit git commands", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git status`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Chained commands", func() {
		It("should validate git commit in chain", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git add file.txt && git commit -sS -a -m "feat(file): add file"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail with invalid message in chain", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git add file.txt && git commit -sS -a -m "Add file"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
		})
	})

	Describe("Chained commands with git add", func() {
		It("should skip staging check when git add is in the chain", func() {
			// No staged files, but git add is in the command chain
			fakeGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git add file.txt && git commit -sS -m "feat(file): add file"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip staging check with multiple files in git add", func() {
			fakeGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git add mk/check.mk src/main.go && git commit -sS -m "build(makefile): add targets"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip staging check with git add -A in chain", func() {
			fakeGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git add -A && git commit -sS -m "chore(deps): update all"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Amend and allow-empty flags", func() {
		It("should skip staging check with --amend", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit --amend -sS -m "feat(api): amend commit"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip staging check with --allow-empty", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git commit --allow-empty -sS -m "chore(deps): empty commit"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Global options (-C flag)", func() {
		It("should validate commit with -C directory option", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /path/to/repo commit -sS -a -m "feat(api): add endpoint"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail invalid commit message with -C directory option", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /path/to/repo commit -sS -a -m "add new feature"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(
				result.Details["errors"],
			).To(ContainSubstring("doesn't follow conventional commits format"))
		})

		It("should fail commit with long title and -C option", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /path/to/repo commit -sS -a -m "fix(mdtable): prevent false positives in spacing detection"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Details["errors"]).To(ContainSubstring("Title exceeds 50 characters"))
		})

		It("should validate commit in chained command with -C option", func() {
			fakeGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /path/to/repo add file.txt && git -C /path/to/repo commit -sS -m "feat(file): add new file"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should detect git add with -C option in chain", func() {
			fakeGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /some/path add . && git -C /some/path commit -sS -m "chore(all): stage all"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail missing flags with -C option", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /path/to/repo commit -m "feat(api): add endpoint"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Git commit missing required flags"))
		})

		It("should handle multiple global options", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git -C /path/to/repo --no-pager commit -sS -a -m "feat(api): add endpoint"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("HEREDOC message format", func() {
		It("should pass valid commit with trailing newline from HEREDOC", func() {
			// HEREDOC syntax adds trailing newline: -m "$(cat <<'EOF'\n...\nEOF\n)"
			// The parser extracts: "chore(deploy): migrate\n"
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git commit -sS -m \"$(cat <<'EOF'\nchore(deploy): migrate to Frankfurt region\nEOF\n)\"",
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass valid commit with multiple trailing newlines", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git commit -sS -m \"$(cat <<'EOF'\nfeat(api): add endpoint\n\n\nEOF\n)\"",
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass commit with body and trailing newlines from HEREDOC", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git commit -sS -m \"$(cat <<'EOF'\nfeat(api): add endpoint\n\nThis adds a new API endpoint.\nEOF\n)\"",
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
