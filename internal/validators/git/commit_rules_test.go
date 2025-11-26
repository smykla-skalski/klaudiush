package git_test

import (
	"strings"

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
			It("should not match numbers exceeding 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 20 digits exceeds the 10-digit limit
				errors := rule.Validate(commit, "test #12345678901234567890 test")
				Expect(errors).To(BeEmpty())
			})

			It("should handle extremely long digit sequences efficiently", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 1000 digits - would cause ReDoS without bounded quantifier
				longNumber := "#" + strings.Repeat("1", 1000)
				errors := rule.Validate(commit, "test "+longNumber+" test")

				// Should not match (exceeds 10 digits) and should complete quickly
				Expect(errors).To(BeEmpty())
			})

			It("should match numbers up to 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				errors := rule.Validate(commit, "issue #1234567890 fixed")
				Expect(errors).NotTo(BeEmpty())
			})

			It("should match exactly at the boundary (10 digits)", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 10 digits - exactly at the limit
				errors := rule.Validate(commit, "issue #1234567890 fixed")
				Expect(errors).NotTo(BeEmpty())

				// 11 digits - one over the limit
				errors = rule.Validate(commit, "issue #12345678901 fixed")
				Expect(errors).To(BeEmpty())
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

		Context("error message formatting", func() {
			It("should not produce malformed URLs in error messages", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				errors := rule.Validate(commit, "See https://github.com/owner/repo/pull/123")
				Expect(errors).NotTo(BeEmpty())

				// Check error messages don't contain malformed URLs
				for _, err := range errors {
					Expect(err).NotTo(ContainSubstring("https://://"))
					Expect(err).NotTo(ContainSubstring("https://https://"))
				}

				// Verify the correct URL format is shown
				foundURLError := false
				for _, err := range errors {
					match, _ := ContainSubstring("github.com/owner/repo/pull/123").Match(err)
					if match {
						foundURLError = true
						break
					}
				}
				Expect(foundURLError).To(BeTrue(), "Expected error to contain the GitHub URL")
			})

			It("should format error correctly for URL at start of body", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				errors := rule.Validate(commit, "fix:\n\ngithub.com/owner/repo/pull/456")
				Expect(errors).NotTo(BeEmpty())

				for _, err := range errors {
					Expect(err).NotTo(ContainSubstring("https://://"))
					Expect(err).NotTo(ContainSubstring("https:// "))
				}
			})
		})

		Context("bounded quantifier prevents ReDoS on URLs", func() {
			It("should not match PR numbers exceeding 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 20 digits exceeds the 10-digit limit
				errors := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/12345678901234567890",
				)
				Expect(errors).To(BeEmpty())
			})

			It("should handle extremely long PR numbers efficiently", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 1000 digits - would cause ReDoS without bounded quantifier
				longPRNum := strings.Repeat("1", 1000)
				errors := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/"+longPRNum,
				)

				// Should not match (exceeds 10 digits) and should complete quickly
				Expect(errors).To(BeEmpty())
			})

			It("should match PR numbers up to 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				errors := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/1234567890",
				)
				Expect(errors).NotTo(BeEmpty())
			})

			It("should match exactly at the boundary (10 digits)", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 10 digits - exactly at the limit
				errors := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/1234567890",
				)
				Expect(errors).NotTo(BeEmpty())

				// 11 digits - one over the limit
				errors = rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/12345678901",
				)
				Expect(errors).To(BeEmpty())
			})
		})
	})
})
