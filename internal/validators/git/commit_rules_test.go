package git_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validators/git"
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
					result := rule.Validate(commit, message)
					Expect(
						result,
					).NotTo(BeNil(), "Expected PR reference to be detected in: %s", message)
					Expect(result.Errors).NotTo(BeEmpty())
					Expect(result.Errors[0]).To(ContainSubstring("PR references found"))
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
					result := rule.Validate(commit, message)
					Expect(result).To(BeNil(), "Should not detect PR reference in: %s", message)
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
				result := rule.Validate(commit, "test #12345678901234567890 test")
				Expect(result).To(BeNil())
			})

			It("should handle extremely long digit sequences efficiently", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 1000 digits - would cause ReDoS without bounded quantifier
				longNumber := "#" + strings.Repeat("1", 1000)
				result := rule.Validate(commit, "test "+longNumber+" test")

				// Should not match (exceeds 10 digits) and should complete quickly
				Expect(result).To(BeNil())
			})

			It("should match numbers up to 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				result := rule.Validate(commit, "issue #1234567890 fixed")
				Expect(result).NotTo(BeNil())
			})

			It("should match exactly at the boundary (10 digits)", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 10 digits - exactly at the limit
				result := rule.Validate(commit, "issue #1234567890 fixed")
				Expect(result).NotTo(BeNil())

				// 11 digits - one over the limit
				result = rule.Validate(commit, "issue #12345678901 fixed")
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("GitHub URL reference patterns", func() {
		Context("should match valid GitHub PR URLs", func() {
			DescribeTable(
				"detects GitHub PR URLs",
				func(message string) {
					commit := &git.ParsedCommit{Title: "test", Valid: true}
					result := rule.Validate(commit, message)
					Expect(
						result,
					).NotTo(BeNil(), "Expected PR URL to be detected in: %s", message)
					Expect(result.Errors).NotTo(BeEmpty())
					Expect(result.Errors[0]).To(ContainSubstring("PR references found"))
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
					result := rule.Validate(commit, message)
					Expect(result).To(BeNil(), "Should not detect PR reference in: %s", message)
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
					result := rule.Validate(commit, message)
					Expect(
						result,
					).NotTo(BeNil(), "Expected PR URL to be detected in: %s", message)
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
				result := rule.Validate(commit, "See https://github.com/owner/repo/pull/123")
				Expect(result).NotTo(BeNil())
				Expect(result.Errors).NotTo(BeEmpty())

				// Check error messages don't contain malformed URLs
				for _, err := range result.Errors {
					Expect(err).NotTo(ContainSubstring("https://://"))
					Expect(err).NotTo(ContainSubstring("https://https://"))
				}

				// Verify the correct URL format is shown
				foundURLError := false

				for _, err := range result.Errors {
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
				result := rule.Validate(commit, "fix:\n\ngithub.com/owner/repo/pull/456")
				Expect(result).NotTo(BeNil())
				Expect(result.Errors).NotTo(BeEmpty())

				for _, err := range result.Errors {
					Expect(err).NotTo(ContainSubstring("https://://"))
					Expect(err).NotTo(ContainSubstring("https:// "))
				}
			})
		})

		Context("bounded quantifier prevents ReDoS on URLs", func() {
			It("should not match PR numbers exceeding 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 20 digits exceeds the 10-digit limit
				result := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/12345678901234567890",
				)
				Expect(result).To(BeNil())
			})

			It("should handle extremely long PR numbers efficiently", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 1000 digits - would cause ReDoS without bounded quantifier
				longPRNum := strings.Repeat("1", 1000)
				result := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/"+longPRNum,
				)

				// Should not match (exceeds 10 digits) and should complete quickly
				Expect(result).To(BeNil())
			})

			It("should match PR numbers up to 10 digits", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				result := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/1234567890",
				)
				Expect(result).NotTo(BeNil())
			})

			It("should match exactly at the boundary (10 digits)", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}

				// 10 digits - exactly at the limit
				result := rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/1234567890",
				)
				Expect(result).NotTo(BeNil())

				// 11 digits - one over the limit
				result = rule.Validate(
					commit,
					"see https://github.com/owner/repo/pull/12345678901",
				)
				Expect(result).To(BeNil())
			})
		})
	})
})

var _ = Describe("ScopeOnlyFormatRule", func() {
	var rule *git.ScopeOnlyFormatRule

	BeforeEach(func() {
		rule = &git.ScopeOnlyFormatRule{}
	})

	DescribeTable("valid scope-only titles",
		func(title string) {
			commit := &git.ParsedCommit{Title: title, Valid: false}
			Expect(rule.Validate(commit, title)).To(BeNil())
		},
		Entry("simple scope", "home-environment: use nix profile add instead of install"),
		Entry("path scope", "modules/systemd: add new unit"),
		Entry("dotted scope", "modules.home: configure shell"),
		Entry("short scope", "cli: add flag"),
		Entry("numeric in scope", "go1.21: update minimum version"),
		Entry("underscore in scope", "my_module: fix typo"),
	)

	DescribeTable("invalid scope-only titles",
		func(title string) {
			commit := &git.ParsedCommit{Title: title, Valid: false}
			result := rule.Validate(commit, title)
			Expect(result).NotTo(BeNil())
			Expect(result.Errors).NotTo(BeEmpty())
		},
		Entry("no colon", "just a plain message"),
		Entry("uppercase start", "Home-environment: something"),
		Entry("no space after colon", "home:description"),
		Entry("conventional type prefix", "feat(auth): add login"),
		Entry("empty description after colon", "scope: "),
	)

	It("should exempt revert commits", func() {
		commit := &git.ParsedCommit{Title: `Revert "home-environment: remove package"`, Valid: true}
		Expect(rule.Validate(commit, commit.Title)).To(BeNil())
	})
})

var _ = Describe("CustomPatternRule", func() {
	It("should pass when title matches pattern", func() {
		rule := git.NewCustomPatternRule(`^[A-Z]+-\d+: .+`)
		commit := &git.ParsedCommit{Title: "PROJ-123: implement feature", Valid: true}
		Expect(rule.Validate(commit, commit.Title)).To(BeNil())
	})

	It("should fail when title does not match pattern", func() {
		rule := git.NewCustomPatternRule(`^[A-Z]+-\d+: .+`)
		commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
		result := rule.Validate(commit, commit.Title)
		Expect(result).NotTo(BeNil())
		Expect(result.Errors[0]).To(ContainSubstring("doesn't match the required pattern"))
	})

	It("should exempt revert commits", func() {
		rule := git.NewCustomPatternRule(`^[A-Z]+-\d+: .+`)
		commit := &git.ParsedCommit{Title: `Revert "something"`, Valid: true}
		Expect(rule.Validate(commit, commit.Title)).To(BeNil())
	})
})

var _ = Describe("ListFormattingRule", func() {
	var rule *git.ListFormattingRule

	BeforeEach(func() {
		rule = git.NewListFormattingRule()
	})

	Context("should detect actual list items without preceding blank line", func() {
		It("detects unordered list directly after title", func() {
			msg := "feat(api): add endpoint\n- first item\n- second item"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).NotTo(BeNil())
			Expect(result.Errors).NotTo(BeEmpty())
		})

		It("detects ordered list directly after title", func() {
			msg := "feat(api): add endpoint\n1. first item\n2. second item"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).NotTo(BeNil())
			Expect(result.Errors).NotTo(BeEmpty())
		})

		It("detects list after prose without blank line", func() {
			msg := "feat(api): add endpoint\n\nSome description here.\n- first item"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).NotTo(BeNil())
		})
	})

	Context("should pass for properly formatted lists", func() {
		It("passes with blank line before list", func() {
			msg := "feat(api): add endpoint\n\nChanges:\n\n- first item\n- second item"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})
	})

	Context("should NOT false-positive on git trailer lines", func() {
		It("passes with Signed-off-by directly after body text", func() {
			msg := "feat(api): add endpoint\n\nSome body text.\nSigned-off-by: Test User <test@example.com>"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})

		It("passes with Co-authored-by trailer", func() {
			msg := "feat(api): add endpoint\n\nSome body text.\n\nCo-authored-by: Other User <other@example.com>"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})

		It("passes with multiple trailers after body", func() {
			msg := "feat(api): add endpoint\n\nSome body text.\n\nSigned-off-by: Test User <test@example.com>\nCo-authored-by: Other <o@e.com>"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})

		It("passes with BREAKING CHANGE trailer", func() {
			msg := "feat(api)!: remove endpoint\n\nRemoved the old endpoint.\n\nBREAKING CHANGE: API v1 removed"
			commit := &git.ParsedCommit{Title: "feat(api)!: remove endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})
	})

	Context("should NOT false-positive on prose containing number-dot patterns", func() {
		It("passes when prose has mid-line number-dot like 0. Only", func() {
			msg := "feat(output): use JSON stdout\n\n" +
				"Always exits 0. Only exit 3 remains non-zero."
			commit := &git.ParsedCommit{Title: "feat(output): use JSON stdout", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})

		It("passes when prose has version numbers like 1.2.3", func() {
			msg := "fix(deps): update dependency\n\n" +
				"Updates from version 1.2.3 to version 2.0.0 which\n" +
				"fixes the compatibility with Go 1.25.4 runtime."
			commit := &git.ParsedCommit{Title: "fix(deps): update dependency", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})

		It("passes for the actual commit message that triggered GIT016", func() {
			msg := "feat(output): use JSON stdout instead of exit 2\n\n" +
				"Claude Code conflates exit-code-2 hook blocks with user\n" +
				"permission denials, causing stop-and-wait behavior instead\n" +
				"of self-correction. Using systemMessage alone means Claude\n" +
				"only sees a generic \"Hook denied this tool\" and never gets\n" +
				"the actual error or fix hint.\n\n" +
				"Switches to structured JSON on stdout with permissionDecision,\n" +
				"permissionDecisionReason, additionalContext, and systemMessage\n" +
				"fields. Always exits 0. Only exit 3 (crash) remains non-zero.\n\n" +
				"Adds new hookresponse package that builds the JSON response.\n" +
				"Bypassed exceptions now use permissionDecision \"allow\" with\n" +
				"additionalContext instead of the old block then convert flow.\n" +
				"Removes FormatErrors and related formatting functions from\n" +
				"the dispatcher package, replaced by hookresponse formatters."
			commit := &git.ParsedCommit{
				Title: "feat(output): use JSON stdout instead of exit 2",
				Valid: true,
			}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})

		It("passes with Signed-off-by trailer", func() {
			msg := "feat(output): use JSON stdout\n\n" +
				"Some description.\n\n" +
				"Signed-off-by: Test User <test@example.com>"
			commit := &git.ParsedCommit{Title: "feat(output): use JSON stdout", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).To(BeNil())
		})
	})
})
