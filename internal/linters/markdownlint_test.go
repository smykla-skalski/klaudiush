package linters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/claude-hooks/internal/linters"
)

var _ = Describe("MarkdownLinter", func() {
	var (
		linter linters.MarkdownLinter
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		linter = linters.NewMarkdownLinter(nil) // runner not used for custom rules only
	})

	Describe("Lint", func() {
		Context("when content has custom rule violations", func() {
			It("should fail with custom rule output", func() {
				// Content with custom rule violation (no empty line before code block)
				content := `# Test
Some text
` + "```" + `
code
` + "```"

				result := linter.Lint(ctx, content, nil)

				// Should fail due to custom rules
				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeFalse())
				Expect(
					result.RawOut,
				).To(ContainSubstring("Code block should have empty line before it"))
			})
		})

		Context("when content has no custom rule violations", func() {
			It("should return success", func() {
				content := `# Test

Some text

` + "```" + `
code
` + "```"

				result := linter.Lint(ctx, content, nil)

				Expect(result).NotTo(BeNil())
				Expect(result.Success).To(BeTrue())
				Expect(result.Err).To(BeNil())
			})
		})
	})
})
