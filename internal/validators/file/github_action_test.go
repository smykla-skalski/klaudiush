package file_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	execpkg "github.com/smykla-labs/klaudiush/internal/exec"
	"github.com/smykla-labs/klaudiush/internal/github"
	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validators/file"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// mockGitHubClient is a mock implementation of github.Client for testing
type mockGitHubClient struct {
	authenticated bool
}

func (*mockGitHubClient) GetLatestRelease(
	_ context.Context,
	_, _ string,
) (*github.Release, error) {
	return nil, github.ErrNoReleases
}

func (*mockGitHubClient) GetTags(
	_ context.Context,
	_, _ string,
) ([]*github.Tag, error) {
	return nil, github.ErrNoTags
}

func (m *mockGitHubClient) IsAuthenticated() bool {
	return m.authenticated
}

var _ = Describe("WorkflowValidator", func() {
	var (
		validator *file.WorkflowValidator
		log       logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		runner := execpkg.NewCommandRunner(10 * time.Second)
		linter := linters.NewActionLinter(runner)
		githubClient := &mockGitHubClient{authenticated: false}
		validator = file.NewWorkflowValidator(linter, githubClient, log, nil, nil)
	})

	Describe("Validate", func() {
		Context("when file is not a workflow file", func() {
			It("should pass for non-workflow paths", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/path/to/regular/file.yml",
						Content:  "some: yaml",
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for workflows in wrong directory", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/workflows/test.yml",
						Content:  "some: yaml",
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when file is a workflow file", func() {
			It("should pass for digest-pinned action with version comment (inline)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for digest-pinned action with version comment (previous line)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # v4.1.1
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail for digest-pinned action without version comment", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(
					result.Message,
				).To(ContainSubstring("GitHub Actions workflow/action validation failed"))
				Expect(result.Details["errors"]).To(ContainSubstring("missing version comment"))
			})

			It(
				"should pass for tag-pinned action with explanation comment (previous line)",
				func() {
					ctx := &hook.Context{
						EventType: hook.EventTypePreToolUse,
						ToolName:  hook.ToolTypeWrite,
						ToolInput: hook.ToolInput{
							FilePath: "/project/.github/workflows/test.yml",
							Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # Cannot pin by digest: marketplace action with frequent updates
      - uses: vendor/custom-action@v1
`,
						},
					}

					result := validator.Validate(context.Background(), ctx)
					Expect(result.Passed).To(BeTrue())
				},
			)

			It("should pass for tag-pinned action with explanation comment (inline)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: vendor/custom-action@v1 # Third-party marketplace action
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail for tag-pinned action without explanation", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["errors"]).To(ContainSubstring("uses tag without digest"))
			})

			It("should skip local actions (./...)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: ./local-action
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip Docker actions (docker://...)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: docker://alpine:3.8
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should handle multiple actions with mixed violations", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
      - uses: actions/setup-go@v4
      - uses: vendor/action@abc123def456abc123def456abc123def456abc1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["errors"]).To(ContainSubstring("uses tag without digest"))
				Expect(result.Details["errors"]).To(ContainSubstring("missing version comment"))
			})

			It("should handle YAML list item syntax (- uses:)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should handle .yaml extension", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yaml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should handle empty content", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content:  "",
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should handle SHA-256 digests (64 chars)", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@e2f20e631ae6d7dd3b768f56a5d2af784dd54791f319c921c5fd53fe67f0f5b8 # v4.1.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should handle version with patch and prerelease", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1-beta.2
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should reject 'version' prefix without 'v'", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # version 4.1.1
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("missing version comment"))
			})
		})

		Context("when validating Edit operations", func() {
			It("should skip Edit operations in PreToolUse", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeEdit,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when file is a composable action", func() {
			It("should validate action.yml with digest-pinned actions", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/actions/my-action/action.yml",
						Content: `name: My Action
description: A custom action
runs:
  using: composite
  steps:
    - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should validate action.yaml with digest-pinned actions", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/actions/setup/action.yaml",
						Content: `name: Setup Action
description: Setup environment
runs:
  using: composite
  steps:
    - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # v5.0.2
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should fail for action.yml with missing version comment", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/actions/my-action/action.yml",
						Content: `name: My Action
description: A custom action
runs:
  using: composite
  steps:
    - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["errors"]).To(ContainSubstring("missing version comment"))
			})

			It("should fail for action with tag-pinned action without explanation", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/actions/deploy/action.yml",
						Content: `name: Deploy Action
description: Deploy application
runs:
  using: composite
  steps:
    - uses: vendor/deploy-action@v2
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Details["errors"]).To(ContainSubstring("uses tag without digest"))
			})

			It("should pass for action with tag-pinned action with explanation", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/actions/deploy/action.yml",
						Content: `name: Deploy Action
description: Deploy application
runs:
  using: composite
  steps:
    # Cannot pin by digest: third-party marketplace action
    - uses: vendor/deploy-action@v2
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for non-action files in .github/actions/", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/actions/my-action/README.md",
						Content:  "# My Action Documentation",
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("version comment extraction", func() {
			It("should extract version without 'v' prefix", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # 4.1.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should extract major.minor version", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("explanation comment detection", func() {
			It("should accept explanation with version-like numbers", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # Marketplace action updated every 2 days
      - uses: vendor/action@v1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should reject version-only comment as explanation", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeWrite,
					ToolInput: hook.ToolInput{
						FilePath: "/project/.github/workflows/test.yml",
						Content: `name: Test
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      # v1.2.3
      - uses: vendor/action@v1
`,
					},
				}

				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeFalse())
				Expect(result.Details["errors"]).To(ContainSubstring("uses tag without digest"))
			})
		})
	})
})
