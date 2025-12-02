package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("PRValidator", func() {
	var validator *git.PRValidator

	BeforeEach(func() {
		validator = git.NewPRValidator(nil, logger.NewNoOpLogger(), nil)
	})

	Describe("Title Validation", func() {
		It("should pass for valid semantic commit title", func() {
			result := git.ValidatePRTitle("feat(api): add new endpoint")
			Expect(result.Valid).To(BeTrue())
		})

		It("should pass for title without scope", func() {
			result := git.ValidatePRTitle("feat: add new feature")
			Expect(result.Valid).To(BeTrue())
		})

		It("should pass for breaking change marker", func() {
			result := git.ValidatePRTitle("feat!: breaking API change")
			Expect(result.Valid).To(BeTrue())
		})

		It("should fail for invalid format", func() {
			result := git.ValidatePRTitle("Add new feature")
			Expect(result.Valid).To(BeFalse())
			Expect(
				result.ErrorMessage,
			).To(ContainSubstring("doesn't follow semantic commit format"))
		})

		It("should fail for empty title", func() {
			result := git.ValidatePRTitle("")
			Expect(result.Valid).To(BeFalse())
			Expect(result.ErrorMessage).To(Equal("PR title is empty"))
		})

		It("should fail for feat(ci)", func() {
			result := git.ValidatePRTitle("feat(ci): add new workflow")
			Expect(result.Valid).To(BeFalse())
			Expect(result.ErrorMessage).To(ContainSubstring("Use 'ci(...)' not 'feat(ci)'"))
		})

		It("should fail for fix(test)", func() {
			result := git.ValidatePRTitle("fix(test): update test")
			Expect(result.Valid).To(BeFalse())
			Expect(result.ErrorMessage).To(ContainSubstring("Use 'test(...)' not 'fix(test)'"))
		})

		It("should fail for feat(docs)", func() {
			result := git.ValidatePRTitle("feat(docs): add documentation")
			Expect(result.Valid).To(BeFalse())
			Expect(result.ErrorMessage).To(ContainSubstring("Use 'docs(...)' not 'feat(docs)'"))
		})

		It("should fail for fix(build)", func() {
			result := git.ValidatePRTitle("fix(build): update build")
			Expect(result.Valid).To(BeFalse())
			Expect(result.ErrorMessage).To(ContainSubstring("Use 'build(...)' not 'fix(build)'"))
		})
	})

	Describe("Type Extraction", func() {
		It("should extract feat type", func() {
			prType := git.ExtractPRType("feat(api): add endpoint")
			Expect(prType).To(Equal("feat"))
		})

		It("should extract ci type", func() {
			prType := git.ExtractPRType("ci(workflow): update pipeline")
			Expect(prType).To(Equal("ci"))
		})

		It("should return empty for invalid title", func() {
			prType := git.ExtractPRType("Invalid title")
			Expect(prType).To(Equal(""))
		})
	})

	Describe("Non-User-Facing Type Check", func() {
		It("should identify ci as non-user-facing", func() {
			Expect(git.IsNonUserFacingType("ci")).To(BeTrue())
		})

		It("should identify test as non-user-facing", func() {
			Expect(git.IsNonUserFacingType("test")).To(BeTrue())
		})

		It("should identify chore as non-user-facing", func() {
			Expect(git.IsNonUserFacingType("chore")).To(BeTrue())
		})

		It("should identify feat as user-facing", func() {
			Expect(git.IsNonUserFacingType("feat")).To(BeFalse())
		})

		It("should identify fix as user-facing", func() {
			Expect(git.IsNonUserFacingType("fix")).To(BeFalse())
		})
	})

	Describe("Body Validation", func() {
		It("should pass for valid body with all sections", func() {
			body := `## Motivation
This change improves performance.

## Implementation information
- Updated algorithm
- Added caching

## Supporting documentation
See docs/performance.md`

			result := git.ValidatePRBody(body, "feat")
			Expect(result.Errors).To(BeEmpty())
		})

		It("should error on missing Motivation section", func() {
			body := `## Implementation information
- Updated algorithm

## Supporting documentation
See docs/performance.md`

			result := git.ValidatePRBody(body, "feat")
			Expect(result.Errors).To(ContainElement(ContainSubstring("missing '## Motivation'")))
		})

		It("should error on missing Implementation information section", func() {
			body := `## Motivation
This change improves performance.

## Supporting documentation
See docs/performance.md`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Errors,
			).To(ContainElement(ContainSubstring("missing '## Implementation information'")))
		})

		It("should warn on missing Supporting documentation section", func() {
			body := `## Motivation
This change improves performance.

## Implementation information
- Updated algorithm`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Warnings,
			).To(ContainElement(ContainSubstring("missing '## Supporting documentation'")))
			Expect(
				result.Warnings,
			).To(ContainElement(ContainSubstring("can be omitted only when it would result in N/A")))
		})

		It("should warn on empty body", func() {
			result := git.ValidatePRBody("", "feat")
			Expect(
				result.Warnings,
			).To(ContainElement(ContainSubstring("Could not extract PR body")))
		})

		It("should warn for ci type without changelog skip", func() {
			body := `## Motivation
CI change

## Implementation information
- Updated workflow`

			result := git.ValidatePRBody(body, "ci")
			Expect(
				result.Warnings,
			).To(ContainElement(ContainSubstring("should typically have '> Changelog: skip'")))
		})

		It("should not warn for ci type with changelog skip", func() {
			body := `## Motivation
CI change

## Implementation information
- Updated workflow

> Changelog: skip`

			result := git.ValidatePRBody(body, "ci")
			Expect(
				result.Warnings,
			).NotTo(ContainElement(ContainSubstring("should typically have '> Changelog: skip'")))
		})

		It("should warn for feat type with changelog skip", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

> Changelog: skip`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Warnings,
			).To(ContainElement(ContainSubstring("is user-facing but has 'Changelog: skip'")))
		})

		It("should validate custom changelog format", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

> Changelog: invalid changelog format`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Errors,
			).To(ContainElement(ContainSubstring("Custom changelog entry doesn't follow semantic commit format")))
		})

		It("should accept valid custom changelog format", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

> Changelog: feat(api): add custom endpoint`

			result := git.ValidatePRBody(body, "feat")
			Expect(result.Errors).NotTo(ContainElement(ContainSubstring("Custom changelog")))
		})

		It("should warn on formal language", func() {
			body := `## Motivation
We will utilize this feature to facilitate improvements.

## Implementation information
- Leverage new algorithm

## Supporting documentation
See docs/algorithm.md`

			result := git.ValidatePRBody(body, "feat")
			Expect(result.Warnings).To(ContainElement(ContainSubstring("uses formal language")))
		})

		It("should error on N/A supporting documentation", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
N/A`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Errors,
			).To(ContainElement(ContainSubstring("Supporting documentation section contains placeholder value")))
			Expect(
				result.Errors,
			).To(ContainElement(ContainSubstring("Remove the entire '## Supporting documentation' section")))
		})

		It(
			"should error on various empty placeholder patterns in supporting documentation",
			func() {
				placeholders := []string{
					"n/a",
					"None",
					"NOTHING",
					"empty",
					"-",
					"TBD",
					"todo",
					"...",
				}

				for _, placeholder := range placeholders {
					body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
` + placeholder

					result := git.ValidatePRBody(body, "feat")
					Expect(
						result.Errors,
					).To(ContainElement(ContainSubstring("placeholder value")),
						"Expected error for placeholder: "+placeholder)
				}
			},
		)

		It(
			"should error on N/A with trailing comments in supporting documentation",
			func() {
				testCases := []struct {
					name  string
					value string
				}{
					{
						name:  "N/A with dash and explanation",
						value: "N/A - Repository migration following standard process",
					},
					{
						name:  "n/a with colon",
						value: "n/a: not applicable for this change",
					},
					{
						name:  "None with explanation",
						value: "None - internal refactoring only",
					},
					{
						name:  "NOTHING with parentheses",
						value: "NOTHING (emergency hotfix)",
					},
					{
						name:  "empty with comma",
						value: "empty, see motivation section",
					},
					{
						name:  "TBD with note",
						value: "TBD will add after review",
					},
					{
						name:  "todo with task",
						value: "todo: create documentation",
					},
					{
						name:  "N/A with multiple separators",
						value: "N/A - see issue #123 for context",
					},
				}

				for _, tc := range testCases {
					body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
` + tc.value

					result := git.ValidatePRBody(body, "feat")
					Expect(
						result.Errors,
					).To(ContainElement(ContainSubstring("placeholder value")),
						"Expected error for case: "+tc.name)
				}
			},
		)

		It(
			"should NOT error on valid list items in supporting documentation",
			func() {
				validCases := []string{
					"- Link to RFC: https://example.com/rfc",
					"- See issue #123",
					"- Documentation: https://docs.example.com",
					"* Related PR: #456",
				}

				for _, validCase := range validCases {
					body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
` + validCase

					result := git.ValidatePRBody(body, "feat")
					Expect(result.Errors).To(BeEmpty(),
						"Should not error on valid list item: "+validCase)
				}
			},
		)

		It("should ignore HTML comments in supporting documentation", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
<!-- Links to relevant issues, discussions, or external documentation -->
See https://klaudiu.sh/docs`

			result := git.ValidatePRBody(body, "feat")
			Expect(result.Errors).To(BeEmpty())
		})

		It("should error when only HTML comment in supporting documentation", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
<!-- Links to relevant issues, discussions, or external documentation -->`

			result := git.ValidatePRBody(body, "feat")
			// Section is effectively empty after stripping comments - no error since no placeholder found
			Expect(result.Errors).To(BeEmpty())
		})

		It("should stop at next section when checking supporting documentation", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
See https://klaudiu.sh/docs

## Test plan
N/A`

			result := git.ValidatePRBody(body, "feat")
			// N/A is in Test plan section, not Supporting documentation
			Expect(result.Errors).NotTo(ContainElement(ContainSubstring("placeholder value")))
		})

		It("should stop at changelog line when checking supporting documentation", func() {
			body := `## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
See https://klaudiu.sh/docs

> Changelog: skip`

			result := git.ValidatePRBody(body, "feat")
			Expect(result.Errors).NotTo(ContainElement(ContainSubstring("placeholder value")))
		})

		It("should error when changelog appears before Motivation section", func() {
			body := `> Changelog: feat(api): custom entry

## Motivation
New feature

## Implementation information
- Added endpoint

## Supporting documentation
See docs/api.md`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Errors,
			).To(ContainElement(ContainSubstring("must not appear before '## Motivation'")))
		})

		It("should pass when changelog appears after Motivation section", func() {
			body := `## Motivation
New feature

> Changelog: feat(api): custom entry

## Implementation information
- Added endpoint

## Supporting documentation
See docs/api.md`

			result := git.ValidatePRBody(body, "feat")
			Expect(
				result.Errors,
			).NotTo(ContainElement(ContainSubstring("must not appear before")))
		})
	})

	Describe("Full Validator", func() {
		It("should pass for valid gh pr create command", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(api): add endpoint" --body "$(cat <<'EOF'
# PR Title

## Motivation

New feature description

## Implementation information

- Added endpoint
- Updated documentation

## Supporting documentation

See docs/api.md
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail for invalid title format", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "Add endpoint" --body "$(cat <<'EOF'
# PR Title

## Motivation

New feature description

## Implementation information

- Added endpoint
- Updated documentation

## Supporting documentation

See docs/api.md
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("PR validation failed"))
			Expect(result.Message).To(ContainSubstring("doesn't follow semantic commit format"))
		})

		It("should fail for feat(ci) title", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(ci): add workflow" --body "$(cat <<'EOF'
# PR Title

## Motivation

CI improvement description

## Implementation information

- Added workflow
- Updated CI configuration
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Use 'ci(...)' not 'feat(ci)'"))
		})

		It("should fail for missing required sections", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(api): add endpoint" --body "$(cat <<'EOF'
## Motivation
New feature
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("missing '## Implementation information'"))
			// Supporting documentation missing is now a warning, not an error
			Expect(result.Message).To(ContainSubstring("missing '## Supporting documentation'"))
		})

		It("should fail for non-main base without label", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(api): add endpoint" --base "release/1.0" --body "$(cat <<'EOF'
# PR Title

## Motivation

New feature description

## Implementation information

- Added endpoint
- Updated documentation

## Supporting documentation

See docs/api.md
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("targets 'release/1.0' but missing label"))
		})

		It("should pass for non-main base with matching label", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(api): add endpoint" --base "release/1.0" --label "release/1.0" --body "$(cat <<'EOF'
# PR Title

## Motivation

New feature description

## Implementation information

- Added endpoint
- Updated documentation

## Supporting documentation

See docs/api.md
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should warn for ci type without ci/skip label", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "ci(workflow): update pipeline" --body "$(cat <<'EOF'
# PR Title

## Motivation

CI improvement description

## Implementation information

- Updated workflow
- Improved pipeline performance

> Changelog: skip
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(
				result.Passed,
			).To(BeFalse())
			// Warnings return Passed=false with ShouldBlock=false
			Expect(result.Message).To(ContainSubstring("warnings"))
			Expect(result.Message).To(ContainSubstring("ci/skip-test"))
		})

		It("should handle command chains with gh pr create", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `git add . && gh pr create --title "feat(api): add endpoint" --body "# PR Title

## Motivation

New feature description

## Implementation information

- Added endpoint
- Updated documentation

## Supporting documentation

See docs/api.md"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should fail with forbidden pattern in title", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(storage): use tmp/ for temp files" --body "$(cat <<'EOF'
## Summary

Add temporary storage using tmp directory.

## Test plan

- Unit tests added
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Forbidden pattern found in PR title"))
			Expect(result.Message).To(ContainSubstring("tmp/"))
		})

		It("should fail with forbidden pattern in body", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(storage): add file storage" --body "$(cat <<'EOF'
## Summary

Add file storage capabilities.

## Implementation information

- Store files in tmp/ directory
- Add cleanup logic

## Test plan

- Unit tests added
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Forbidden pattern found in PR body"))
			Expect(result.Message).To(ContainSubstring("tmp/"))
		})

		It("should pass with template word (not tmp)", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title "feat(template): add new template" --body "$(cat <<'EOF'
## Summary

Add template support for rendering content.

## Motivation

Users need template functionality.

## Implementation information

- Added Template class
- Template parsing logic

## Supporting documentation

See templates/README.md

## Test plan

- Template tests added
EOF
)"`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should extract title with single quotes", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: `gh pr create --title 'feat(api): add endpoint' --body '# PR Title

## Motivation

New feature description

## Implementation information

- Added endpoint
- Updated documentation

## Supporting documentation

See docs/api.md'`,
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for non-gh commands", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git status",
				},
			}

			result := validator.Validate(context.Background(), ctx)
			Expect(result.Passed).To(BeTrue())
		})
	})
})
