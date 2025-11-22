package git_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/claude-hooks/internal/validators/git"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

var _ = Describe("GitAddValidator", func() {
	var (
		val     *git.AddValidator
		log     logger.Logger
		mockGit *git.MockGitRunner
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		mockGit = git.NewMockGitRunner()
		val = git.NewAddValidator(log, mockGit)
	})

	Describe("Name", func() {
		It("should return the validator name", func() {
			Expect(val.Name()).To(Equal("validate-git-add"))
		})
	})

	Describe("Validate", func() {
		Context("when adding tmp/ files", func() {
			It("should block adding a single tmp/ file", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add tmp/test.txt",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(ContainSubstring("tmp/ directory"))
				Expect(result.Details).To(HaveKey("help"))
				Expect(result.Details["help"]).To(ContainSubstring("tmp/test.txt"))
				Expect(result.Details["help"]).To(ContainSubstring(".git/info/exclude"))
			})

			It("should block adding multiple tmp/ files", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add tmp/file1.txt tmp/file2.txt",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details).To(HaveKey("help"))
				Expect(result.Details["help"]).To(ContainSubstring("tmp/file1.txt"))
				Expect(result.Details["help"]).To(ContainSubstring("tmp/file2.txt"))
			})

			It("should block tmp/ files in chained commands", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add src/main.go && git add tmp/test.sh",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details).To(HaveKey("help"))
				Expect(result.Details["help"]).To(ContainSubstring("tmp/test.sh"))
			})

			It("should block tmp/ files with nested paths", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add tmp/nested/deep/file.txt",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details).To(HaveKey("help"))
				Expect(result.Details["help"]).To(ContainSubstring("tmp/nested/deep/file.txt"))
			})
		})

		Context("when adding non-tmp/ files", func() {
			It("should allow adding regular files", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add src/main.go",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
				Expect(result.ShouldBlock).To(BeFalse())
			})

			It("should allow adding multiple regular files", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add src/main.go pkg/parser/bash.go",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})

			It("should allow adding with flags", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add -A src/main.go",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})

			It("should allow adding files that contain 'tmp' but don't start with tmp/", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add src/temporary.go",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})

			It("should allow adding current directory", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add .",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})

			It("should allow adding all files with -A flag", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add -A",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when handling flags", func() {
			It("should ignore --chmod flag and its value", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add --chmod=+x src/script.sh",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})

			It("should not treat flags as files", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add -p -u src/main.go",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when command is not git add", func() {
			It("should pass for git commit commands", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m 'test'",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for non-git commands", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "echo test",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when command has complex syntax", func() {
			It("should handle quoted file paths", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: `git add "tmp/file with spaces.txt"`,
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("should handle subshells", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "(cd subdir && git add tmp/test.txt)",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})

			It("should handle command chains with ||", func() {
				ctx := &hook.Context{
					EventType: hook.PreToolUse,
					ToolName:  hook.Bash,
					ToolInput: hook.ToolInput{
						Command: "git add tmp/file.txt || echo failed",
					},
				}

				result := val.Validate(ctx)

				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
			})
		})
	})
})
