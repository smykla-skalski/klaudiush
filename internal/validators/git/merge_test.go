package git_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gitpkg "github.com/smykla-labs/klaudiush/internal/git"
	"github.com/smykla-labs/klaudiush/internal/validators/git"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

var _ = Describe("MergeValidator", func() {
	var (
		fakeGit   *gitpkg.FakeRunner
		validator *git.MergeValidator
	)

	BeforeEach(func() {
		fakeGit = gitpkg.NewFakeRunner()
	})

	Describe("Validate", func() {
		Context("with non-merge command", func() {
			BeforeEach(func() {
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
			})

			It("should pass for git commit command", func() {
				hookCtx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "git commit -m 'test'",
					},
				}

				result := validator.Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should pass for gh pr create command", func() {
				hookCtx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr create --title 'test' --body 'body'",
					},
				}

				result := validator.Validate(context.Background(), hookCtx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("all merge types require signoff", func() {
			BeforeEach(func() {
				// Disable signoff requirement for these tests to focus on merge type behavior
				required := false
				cfg := &config.MergeValidatorConfig{
					RequireSignoff: &required,
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)
			})

			It("should skip validation for rebase merge", func() {
				hookCtx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr merge --rebase",
					},
				}

				result := validator.Validate(context.Background(), hookCtx)
				// Rebase merges preserve individual commit messages, so validation is skipped
				Expect(result).NotTo(BeNil())
				Expect(result.Passed).To(BeTrue())
			})

			It("should skip validation for merge commit (--merge flag)", func() {
				hookCtx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: "gh pr merge --merge",
					},
				}

				result := validator.Validate(context.Background(), hookCtx)
				// Regular merge commits preserve individual commit messages, so validation is skipped
				Expect(result).NotTo(BeNil())
				Expect(result.Passed).To(BeTrue())
			})
		})
	})

	Describe("Title Validation", func() {
		BeforeEach(func() {
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		})

		Context("with default config", func() {
			It("should pass for valid conventional commit title", func() {
				errs := validator.ExportValidateTitle("feat(api): add new endpoint")
				Expect(errs).To(BeEmpty())
			})

			It("should pass for title with scope", func() {
				errs := validator.ExportValidateTitle("fix(auth): resolve login issue")
				Expect(errs).To(BeEmpty())
			})

			It("should fail for empty title", func() {
				errs := validator.ExportValidateTitle("")
				Expect(errs).To(ContainElement("❌ PR title is empty"))
			})

			It("should fail for title exceeding 50 characters", func() {
				longTitle := "feat(api): this is a very long title that exceeds the limit"
				errs := validator.ExportValidateTitle(longTitle)
				Expect(errs).To(ContainElement(ContainSubstring("exceeds 50 characters")))
			})

			It("should pass for revert title exceeding 50 characters", func() {
				revertTitle := `Revert "feat(api): this is a very long title"`
				errs := validator.ExportValidateTitle(revertTitle)
				Expect(errs).To(BeEmpty())
			})

			It("should fail for non-conventional format", func() {
				errs := validator.ExportValidateTitle("Add new feature")
				Expect(errs).To(ContainElement(
					ContainSubstring("doesn't follow conventional commits format"),
				))
			})

			It("should fail for feat(ci)", func() {
				errs := validator.ExportValidateTitle("feat(ci): add workflow")
				Expect(errs).To(ContainElement(ContainSubstring("Use 'ci(...)' not 'feat(ci)'")))
			})

			It("should fail for fix(test)", func() {
				errs := validator.ExportValidateTitle("fix(test): fix test")
				Expect(errs).To(ContainElement(ContainSubstring("Use 'test(...)' not 'fix(test)'")))
			})

			It("should fail for feat(docs)", func() {
				errs := validator.ExportValidateTitle("feat(docs): update docs")
				Expect(errs).To(ContainElement(
					ContainSubstring("Use 'docs(...)' not 'feat(docs)'"),
				))
			})

			It("should fail for fix(build)", func() {
				errs := validator.ExportValidateTitle("fix(build): fix build")
				Expect(errs).To(ContainElement(
					ContainSubstring("Use 'build(...)' not 'fix(build)'"),
				))
			})

			It("should pass for ci(...) type", func() {
				errs := validator.ExportValidateTitle("ci(workflow): update pipeline")
				Expect(errs).To(BeEmpty())
			})

			It("should pass for test(...) type", func() {
				errs := validator.ExportValidateTitle("test(unit): add unit tests")
				Expect(errs).To(BeEmpty())
			})
		})

		Context("with custom config", func() {
			It("should respect custom title max length", func() {
				maxLen := 100
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						TitleMaxLength: &maxLen,
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				longTitle := "feat(api): this is a moderately long title that is under 100 characters"
				errs := validator.ExportValidateTitle(longTitle)
				Expect(errs).To(BeEmpty())
			})

			It("should disable conventional commit check", func() {
				enabled := false
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						ConventionalCommits: &enabled,
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				errs := validator.ExportValidateTitle("Add new feature without type")
				Expect(errs).To(BeEmpty())
			})

			It("should disable scope requirement", func() {
				requireScope := false
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						RequireScope: &requireScope,
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				errs := validator.ExportValidateTitle("feat: no scope")
				Expect(errs).To(BeEmpty())
			})

			It("should disable infra scope misuse check", func() {
				blockMisuse := false
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						BlockInfraScopeMisuse: &blockMisuse,
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				errs := validator.ExportValidateTitle("feat(ci): allowed when disabled")
				// Only fails for scope requirement if enabled, not for misuse
				Expect(errs).NotTo(ContainElement(ContainSubstring("Use 'ci(...)'")))
			})
		})
	})

	Describe("Body Validation", func() {
		BeforeEach(func() {
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
		})

		Context("line length", func() {
			It("should pass for body with short lines", func() {
				body := "This is a short line.\nAnother short line."
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(BeEmpty())
			})

			It("should fail for body line exceeding 72+5 characters", func() {
				// Need a line that exceeds 77 chars (72 + 5 tolerance)
				// Note: Line 0 is skipped as the title, so we need a second line
				body := "Title line\n" +
					"This is an extremely long line of text that definitely exceeds " +
					"the maximum allowed line length of 72 characters plus tolerance of 5"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("exceeds 72 characters")))
			})

			It("should allow URLs to exceed line length", func() {
				body := "See: https://github.com/org/repo/very/long/path/to/something/important"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(BeEmpty())
			})
		})

		Context("PR references", func() {
			It("should fail for #123 PR reference", func() {
				body := "Fixes #123"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("PR references found")))
			})

			It("should fail for GitHub PR URL", func() {
				body := "Related to https://github.com/org/repo/pull/456"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("PR references found")))
			})
		})

		Context("AI attribution", func() {
			It("should fail for 'generated by Claude' attribution", func() {
				body := "This PR was generated by Claude."
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("AI attribution")))
			})

			It("should fail for 'Co-authored-by: Claude' attribution", func() {
				body := "Co-authored-by: Claude"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("AI attribution")))
			})

			It("should pass for legitimate Claude references", func() {
				body := "Updated CLAUDE.md with new instructions"
				errs := validator.ExportValidateBody(body)
				Expect(errs).NotTo(ContainElement(ContainSubstring("AI attribution")))
			})
		})

		Context("forbidden patterns", func() {
			It("should fail for tmp/ path reference", func() {
				body := "See the file at tmp/output.txt"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("Forbidden pattern")))
			})
		})

		Context("list formatting", func() {
			It("should fail for missing empty line before list", func() {
				body := "Title line\nSome text\n- list item 1\n- list item 2"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("Missing empty line")))
			})

			It("should pass for proper list formatting", func() {
				body := "Title line\nSome text\n\n- list item 1\n- list item 2"
				errs := validator.ExportValidateBody(body)
				Expect(errs).NotTo(ContainElement(ContainSubstring("Missing empty line")))
			})
		})

		Context("with custom config", func() {
			It("should disable PR reference check", func() {
				block := false
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						BlockPRReferences: &block,
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				body := "Fixes #123"
				errs := validator.ExportValidateBody(body)
				Expect(errs).NotTo(ContainElement(ContainSubstring("PR references")))
			})

			It("should disable AI attribution check", func() {
				block := false
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						BlockAIAttribution: &block,
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				body := "Generated by Claude"
				errs := validator.ExportValidateBody(body)
				Expect(errs).NotTo(ContainElement(ContainSubstring("AI attribution")))
			})

			It("should use custom forbidden patterns", func() {
				cfg := &config.MergeValidatorConfig{
					Message: &config.MergeMessageConfig{
						ForbiddenPatterns: []string{`\bsecret\b`},
					},
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				body := "Contains secret value"
				errs := validator.ExportValidateBody(body)
				Expect(errs).To(ContainElement(ContainSubstring("Forbidden pattern")))
			})
		})
	})

	Describe("Signoff Validation (in merge command body)", func() {
		Context("validateSignoffInText", func() {
			BeforeEach(func() {
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
			})

			It("should fail for missing signoff", func() {
				text := "Commit body without signoff"
				errs := validator.ExportValidateSignoffInText(text)
				Expect(errs).To(ContainElement(ContainSubstring("missing Signed-off-by")))
			})

			It("should pass for present signoff", func() {
				text := "Commit body\n\nSigned-off-by: Test User <test@klaudiu.sh>"
				errs := validator.ExportValidateSignoffInText(text)
				Expect(errs).To(BeEmpty())
			})
		})

		Context("validateMergeCommandSignoff", func() {
			BeforeEach(func() {
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
			})

			It("should fail when no --body flag provided", func() {
				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(ContainSubstring("missing Signed-off-by"))
			})

			It("should pass when --body contains signoff", func() {
				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
					Body:     "Commit body\n\nSigned-off-by: Test User <test@klaudiu.sh>",
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(BeEmpty())
			})

			It("should pass when --body-file is provided (assumes file has signoff)", func() {
				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
					BodyFile: "/path/to/body.md",
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(BeEmpty())
			})

			It("should fail when --body provided but no signoff", func() {
				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
					Body:     "Commit body without signoff",
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(ContainSubstring("missing Signed-off-by"))
			})
		})

		Context("with signoff disabled", func() {
			It("should pass without signoff when disabled", func() {
				required := false
				cfg := &config.MergeValidatorConfig{
					RequireSignoff: &required,
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(BeEmpty())
			})
		})

		Context("with expected signoff", func() {
			It("should pass for matching signoff in --body", func() {
				cfg := &config.MergeValidatorConfig{
					ExpectedSignoff: "Test User <test@klaudiu.sh>",
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
					Body:     "Commit body\n\nSigned-off-by: Test User <test@klaudiu.sh>",
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(BeEmpty())
			})

			It("should fail for mismatched signoff in --body", func() {
				cfg := &config.MergeValidatorConfig{
					ExpectedSignoff: "Test User <test@klaudiu.sh>",
				}
				validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

				mergeCmd := &parser.GHMergeCommand{
					PRNumber: 123,
					Squash:   true,
					Body:     "Commit body\n\nSigned-off-by: Other User <other@klaudiu.sh>",
				}
				errMsg := validator.ExportValidateMergeCommandSignoff(mergeCmd)
				Expect(errMsg).To(ContainSubstring("Wrong signoff identity"))
			})
		})
	})

	Describe("Message Validation Enabled", func() {
		It("should skip validation when disabled", func() {
			enabled := false
			cfg := &config.MergeValidatorConfig{
				Message: &config.MergeMessageConfig{
					Enabled: &enabled,
				},
			}
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

			Expect(validator.ExportIsMessageValidationEnabled()).To(BeFalse())
		})

		It("should enable validation by default", func() {
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
			Expect(validator.ExportIsMessageValidationEnabled()).To(BeTrue())
		})
	})

	Describe("Auto-merge Validation", func() {
		It("should validate auto-merge by default", func() {
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
			Expect(validator.ExportShouldValidateAutomerge()).To(BeTrue())
		})

		It("should skip auto-merge validation when disabled", func() {
			enabled := false
			cfg := &config.MergeValidatorConfig{
				ValidateAutomerge: &enabled,
			}
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, cfg, nil)

			Expect(validator.ExportShouldValidateAutomerge()).To(BeFalse())
		})
	})

	Describe("Category", func() {
		It("should return CategoryIO", func() {
			validator = git.NewMergeValidator(logger.NewNoOpLogger(), fakeGit, nil, nil)
			// MergeValidator uses external gh CLI, so it's IO category
			Expect(validator.Category().String()).To(Equal("IO"))
		})
	})
})
