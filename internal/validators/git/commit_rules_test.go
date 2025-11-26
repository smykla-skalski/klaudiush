package git_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators/git"
)

var _ = Describe("PRReferenceRule", func() {
	var rule *git.PRReferenceRule

	BeforeEach(func() {
		rule = git.NewPRReferenceRule()
	})

	Describe("Hash reference patterns", func() {
		Context("should match valid PR references", func() {
			DescribeTable(
				"detects hash references",
				func(message string) {
					commit := &git.ParsedCommit{Title: "test", Valid: true}
					errors := rule.Validate(commit, message)
					Expect(
						errors,
					).NotTo(BeEmpty(), "Expected PR reference to be detected in: %s", message)
					Expect(errors[0]).To(ContainSubstring("PR references found"))
				},
				Entry("simple hash reference", "fixes #123"),
				Entry("hash at start", "#123 is the issue"),
				Entry("hash after colon", "Related: #456"),
				Entry("hash in parentheses", "(see #789)"),
				Entry("hash after newline", "fix bug\n\nRelated to #123"),
				Entry("multiple hash refs", "closes #1 and #2"),
			)
		})

		Context("should NOT match embedded hash patterns", func() {
			DescribeTable(
				"ignores non-PR hash patterns",
				func(message string) {
					commit := &git.ParsedCommit{Title: "test", Valid: true}
					errors := rule.Validate(commit, message)
					Expect(errors).To(BeEmpty(), "Should not detect PR reference in: %s", message)
				},
				Entry("hash followed by letters", "version#123abc"),
				Entry("color hex code", "color: #ff0000"),
				Entry("anchor tag", "link to #section-name"),
				Entry("no hash at all", "plain text message"),
				Entry("plain numbers", "issue 123 is fixed"),
			)
		})

		Context("bounded quantifier prevents ReDoS", func() {
			It("should not match extremely long digit sequences", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				longNumber := "#" + string(make([]byte, 100)) // 100+ digits would be unrealistic
				for i := range longNumber[1:] {
					longNumber = longNumber[:i+1] + "1" + longNumber[i+2:]
				}
				// Pattern should still work efficiently with bounded quantifier
				errors := rule.Validate(commit, "test #12345678901234567890 test")
				// Should not match because it exceeds 10 digits
				Expect(errors).To(BeEmpty())
			})

			It("should match numbers up to 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				errors := rule.Validate(commit, "issue #1234567890 fixed")
				Expect(errors).NotTo(BeEmpty())
			})
		})
	})

	Describe("GitHub URL reference patterns", func() {
		Context("should match valid GitHub PR URLs", func() {
			DescribeTable(
				"detects GitHub PR URLs",
				func(message string) {
					commit := &git.ParsedCommit{Title: "test", Valid: true}
					errors := rule.Validate(commit, message)
					Expect(
						errors,
					).NotTo(BeEmpty(), "Expected PR URL to be detected in: %s", message)
					Expect(errors[0]).To(ContainSubstring("PR references found"))
				},
				Entry("full URL", "see https://github.com/owner/repo/pull/123"),
				Entry("URL without https", "see github.com/owner/repo/pull/456"),
				Entry("URL at line start", "github.com/foo/bar/pull/1"),
				Entry("URL in body", "fix:\n\nhttps://github.com/org/project/pull/99"),
			)
		})

		Context("should NOT match embedded GitHub URLs", func() {
			DescribeTable(
				"rejects embedded URLs (prevents URL injection attacks)",
				func(message string) {
					commit := &git.ParsedCommit{Title: "test", Valid: true}
					errors := rule.Validate(commit, message)
					Expect(errors).To(BeEmpty(), "Should not detect PR reference in: %s", message)
				},
				Entry(
					"URL in path",
					"evil.com/github.com/owner/repo/pull/123",
				),
				Entry(
					"URL after slash",
					"https://malicious.com/redirect/github.com/owner/repo/pull/456",
				),
			)
		})

		Context("should match URLs after valid prefixes", func() {
			DescribeTable(
				"detects URLs with valid prefixes",
				func(message string) {
					commit := &git.ParsedCommit{Title: "test", Valid: true}
					errors := rule.Validate(commit, message)
					Expect(
						errors,
					).NotTo(BeEmpty(), "Expected PR URL to be detected in: %s", message)
				},
				Entry("after space", "See github.com/owner/repo/pull/123"),
				Entry("after newline", "Related:\ngithub.com/owner/repo/pull/123"),
				Entry("after quote", `"github.com/owner/repo/pull/123"`),
				Entry("full https URL", "https://github.com/owner/repo/pull/123"),
			)
		})
	})
})
