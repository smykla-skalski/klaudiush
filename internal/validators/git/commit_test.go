package git_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/claude-hooks/internal/validators/git"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

var _ = Describe("CommitValidator", func() {
	var (
		validator *git.CommitValidator
		log       logger.Logger
		mockGit   *git.MockGitRunner
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		mockGit = git.NewMockGitRunner()
		// By default, set up mock to have staged files so staging check passes
		mockGit.StagedFiles = []string{"file.txt"}
		validator = git.NewCommitValidator(log, mockGit)
	})

	Describe("Flag validation", func() {
		Context("when -sS flags are present", func() {
			It("should pass with -sS flags", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat: add new feature"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with -s and -S separately", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -s -S -m "fix: resolve bug"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with long flags", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit --signoff --gpg-sign -m "docs: update readme"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with --all flag instead of -a", func() {
				// Set no staged files to test that --all flag bypasses staging check
				mockGit.StagedFiles = []string{}
				mockGit.ModifiedFiles = []string{"file1.go", "file2.go"}

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS --all -m "feat: update files"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when -sS flags are missing", func() {
			It("should fail without -s flag", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -S -m "feat: test message"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git commit must use -sS flags"))
			})

			It("should fail without -S flag", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -s -m "feat: test message"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git commit must use -sS flags"))
			})

			It("should fail without any signing flags", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -m "feat: test message"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Git commit must use -sS flags"))
			})
		})
	})

	Describe("Commit message validation", func() {
		Context("when message is valid", func() {
			It("should pass with valid conventional commit", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add new endpoint"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with scope", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(auth): resolve login issue"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass without scope", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "chore: update dependencies"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with breaking change marker", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat!: remove deprecated API"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when title is invalid", func() {
			It("should fail with title over 50 characters", func() {
				longTitle := "feat(api): this is a very long commit message that exceeds the fifty character limit"
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + longTitle + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("Commit message validation failed"))
				Expect(result.Details["errors"]).To(ContainSubstring("Title exceeds 50 characters"))
			})

			It("should fail with non-conventional format", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "Add new feature"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("doesn't follow conventional commits format"))
			})

			It("should fail with invalid type", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "invalid(api): add endpoint"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("doesn't follow conventional commits format"))
			})
		})

		Context("when infrastructure scope is misused", func() {
			It("should fail with feat(ci)", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(ci): add new workflow"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'ci(...)' not 'feat(ci)'"))
			})

			It("should fail with fix(test)", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(test): update test helper"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'test(...)' not 'fix(test)'"))
			})

			It("should fail with feat(docs)", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(docs): add new section"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'docs(...)' not 'feat(docs)'"))
			})

			It("should fail with fix(build)", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(build): update makefile"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Use 'build(...)' not 'fix(build)'"))
			})

			It("should pass with ci(workflow)", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "ci(workflow): add new step"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when body has line length issues", func() {
			It("should pass with lines under 72 characters", func() {
				message := `feat(api): add endpoint

This is a normal commit body with lines that are well within the
seventy-two character limit for proper formatting.`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with URLs exceeding 72 characters", func() {
				message := `feat(api): add endpoint

Reference: https://github.com/smykla-labs/claude-hooks/pull/123/files#diff-abc123def456`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				// Should fail for PR reference, but pass for URL length
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).ToNot(ContainSubstring("exceeds 72 characters"))
			})

			It("should fail with lines over 77 characters", func() {
				message := `feat(api): add endpoint

This is a line that definitely exceeds the seventy-two character limit and even the tolerance of seventy-seven characters total`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
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
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail without empty line before first list item", func() {
				message := `feat(api): add endpoint
- Add new endpoint
- Update documentation`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
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
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(
					result.Details["errors"],
				).To(ContainSubstring("Missing empty line before first list item"))
			})
		})

		Context("when message contains PR references", func() {
			It("should fail with #123 reference", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(api): resolve issue #123"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("PR references found"))
				Expect(result.Details["errors"]).To(ContainSubstring("#123"))
			})

			It("should fail with GitHub URL reference", func() {
				message := `fix(api): resolve issue

See github.com/smykla-labs/claude-hooks/pull/123`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("PR references found"))
			})

			It("should pass with plain number", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "fix(api): resolve issue 123"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when message contains AI attribution", func() {
			It("should fail with AI attribution footer", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add endpoint\\n\\nGenerated by Claude"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should allow 'claude' in technical context", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add claude integration"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail with 'Claude AI' pattern", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "feat(api): add feature\\n\\nWith Claude AI assistance"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("AI attribution"))
			})

			It("should pass with CLAUDE.md file reference", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "docs(guide): add CLAUDE.md"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with CLAUDE.md in body", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "$(cat <<'EOF'
docs(guide): update project guide

Update CLAUDE.md with new architecture details.
EOF
)"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass with CLAUDE (uppercase) reference", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "docs(guide): update CLAUDE file"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when message has signoff", func() {
			BeforeEach(func() {
				// Set expected signoff for these tests
				git.ExpectedSignoff = "Test User <test@example.com>"
			})

			AfterEach(func() {
				// Reset to empty after tests
				git.ExpectedSignoff = ""
			})

			It("should pass with correct signoff", func() {
				message := `feat(api): add endpoint

Signed-off-by: Test User <test@example.com>`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail with wrong email", func() {
				message := `feat(api): add endpoint

Signed-off-by: Bart Smykla <wrong@example.com>`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("Wrong signoff identity"))
			})

			It("should fail with wrong name", func() {
				message := `feat(api): add endpoint

Signed-off-by: John Doe <bartek@smykla.com>`

				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git commit -sS -a -m "` + message + `"`,
					},
				}

				result := validator.Validate(ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("Wrong signoff identity"))
			})
		})
	})

	Describe("No message flag", func() {
		It("should pass when no -m flag (message from editor)", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git commit -sS`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Non-git commands", func() {
		It("should pass for non-git commands", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `echo hello`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for non-commit git commands", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git status`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Chained commands", func() {
		It("should validate git commit in chain", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git add file.txt && git commit -sS -a -m "feat: add file"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail with invalid message in chain", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git add file.txt && git commit -sS -a -m "Add file"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeFalse())
		})
	})

	Describe("Chained commands with git add", func() {
		It("should skip staging check when git add is in the chain", func() {
			// No staged files, but git add is in the command chain
			mockGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git add file.txt && git commit -sS -m "feat: add file"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip staging check with multiple files in git add", func() {
			mockGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git add mk/check.mk src/main.go && git commit -sS -m "build(makefile): add targets"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip staging check with git add -A in chain", func() {
			mockGit.StagedFiles = []string{}

			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git add -A && git commit -sS -m "chore: update all"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Amend and allow-empty flags", func() {
		It("should skip staging check with --amend", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git commit --amend -sS -m "feat: amend commit"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip staging check with --allow-empty", func() {
			ctx := &hook.Context{
				EventType: hook.PreToolUse,
				ToolName:  hook.Bash,
				ToolInput: hook.ToolInput{
					Command: `git commit --allow-empty -sS -m "chore: empty commit"`,
				},
			}

			result := validator.Validate(ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
