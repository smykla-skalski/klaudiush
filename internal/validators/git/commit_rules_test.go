package git_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
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
					Expect(result.Message).NotTo(BeEmpty())
					Expect(result.Message).To(ContainSubstring("PR references found"))
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
					Expect(result.Message).NotTo(BeEmpty())
					Expect(result.Message).To(ContainSubstring("PR references found"))
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
				Expect(result.Message).NotTo(BeEmpty())

				// Check Message and Context don't contain malformed URLs
				allMessages := append([]string{result.Message}, result.Context...)
				for _, msg := range allMessages {
					Expect(msg).NotTo(ContainSubstring("https://://"))
					Expect(msg).NotTo(ContainSubstring("https://https://"))
				}

				// Verify the correct URL format is shown
				allText := result.Message + "\n" + strings.Join(result.Context, "\n")
				Expect(allText).To(ContainSubstring("github.com/owner/repo/pull/123"))
			})

			It("should format error correctly for URL at start of body", func() {
				commit := &git.ParsedCommit{Title: "test", Valid: true}
				result := rule.Validate(commit, "fix:\n\ngithub.com/owner/repo/pull/456")
				Expect(result).NotTo(BeNil())
				Expect(result.Message).NotTo(BeEmpty())

				allMessages := append([]string{result.Message}, result.Context...)
				for _, msg := range allMessages {
					Expect(msg).NotTo(ContainSubstring("https://://"))
					Expect(msg).NotTo(ContainSubstring("https:// "))
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
			Expect(result.Message).NotTo(BeEmpty())
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
		Expect(result.Message).To(ContainSubstring("doesn't match the required pattern"))
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
			Expect(result.Message).NotTo(BeEmpty())
		})

		It("detects ordered list directly after title", func() {
			msg := "feat(api): add endpoint\n1. first item\n2. second item"
			commit := &git.ParsedCommit{Title: "feat(api): add endpoint", Valid: true}
			result := rule.Validate(commit, msg)
			Expect(result).NotTo(BeNil())
			Expect(result.Message).NotTo(BeEmpty())
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

var _ = Describe("TitleLengthRule cascading warning", func() {
	It("includes 50-char limit cause hint when title is too long", func() {
		rule := &git.TitleLengthRule{MaxLength: 50, AllowUnlimitedRevertTitle: true}
		commit := &git.ParsedCommit{
			Title: "feat(api): this is a very long commit title that exceeds fifty chars",
			Valid: true,
		}

		result := rule.Validate(commit, commit.Title)
		Expect(result).NotTo(BeNil())
		Expect(result.Reference).To(Equal(validator.RefGitBadTitle))

		allText := result.Message + "\n" + strings.Join(result.Context, "\n")
		Expect(
			allText,
		).To(ContainSubstring("type(scope): prefix counts toward the limit"))
	})
})

var _ = Describe("ConventionalFormatRule cascading warning", func() {
	var rule *git.ConventionalFormatRule

	BeforeEach(func() {
		rule = &git.ConventionalFormatRule{
			ValidTypes:   []string{"feat", "fix", "chore"},
			RequireScope: true,
		}
	})

	It("includes 50-char limit warning for invalid format", func() {
		commit := &git.ParsedCommit{
			Title:      "Add new feature",
			Valid:      false,
			ParseError: "no type prefix found",
		}

		result := rule.Validate(commit, commit.Title)
		Expect(result).NotTo(BeNil())
		Expect(result.Reference).To(Equal(validator.RefGitConventionalCommit))

		allText := result.Message + "\n" + strings.Join(result.Context, "\n")
		Expect(allText).To(ContainSubstring("type(scope): prefix counts toward 50-char limit"))
	})

	It("includes 50-char limit warning for missing scope", func() {
		commit := &git.ParsedCommit{
			Title: "feat: add endpoint",
			Valid: true,
			Type:  "feat",
			Scope: "",
		}

		result := rule.Validate(commit, commit.Title)
		Expect(result).NotTo(BeNil())

		allText := result.Message + "\n" + strings.Join(result.Context, "\n")
		Expect(allText).To(ContainSubstring("type(scope): prefix counts toward 50-char limit"))
	})
})

var _ = Describe("rule findings", func() {
	It("reports every PR URL as written in the message", func() {
		result := git.NewPRReferenceRule().Validate(
			&git.ParsedCommit{Title: "test", Valid: true},
			"see github.com/o/r/pull/12 and http://github.com/o/r/pull/34 and #5",
		)
		Expect(result).NotTo(BeNil())

		repairs := make([]string, 0, len(result.Findings))
		for _, f := range result.Findings {
			repairs = append(repairs, f.Repair)
		}

		Expect(repairs).To(ConsistOf(
			"Replace '#5' with '5'",
			"Replace 'github.com/o/r/pull/12' with '12'",
			"Replace 'http://github.com/o/r/pull/34' with '34'",
		))
	})

	It("counts body line length in characters", func() {
		rule := git.NewBodyLineLengthRule(72, 0)
		line := strings.Repeat("ż", 70)

		Expect(rule.Validate(nil, "title\n\n"+line)).To(BeNil())

		result := rule.Validate(nil, "title\n\n"+line+"ąąą")
		Expect(result).NotTo(BeNil())
		Expect(result.Findings).To(HaveLen(1))
		Expect(result.Findings[0].Message).To(Equal("Body line is 73 characters long"))
		Expect(result.Findings[0].Actual).To(Equal(line + "ąąą"))
		Expect(result.Findings[0].Required).To(Equal("at most 72 characters per body line"))
	})

	It("mentions the tolerance in the requirement", func() {
		result := git.NewBodyLineLengthRule(72, 5).Validate(nil, "t\n\n"+strings.Repeat("x", 80))
		Expect(result).NotTo(BeNil())
		Expect(result.Findings[0].Required).To(ContainSubstring("up to 77 tolerated"))
	})

	It("gives scope-only, custom pattern, infra scope and signoff findings", func() {
		commit := &git.ParsedCommit{Title: "Bad Title", Valid: false}

		scope := (&git.ScopeOnlyFormatRule{}).Validate(commit, "Bad Title")
		Expect(scope.Findings).To(HaveLen(1))
		Expect(scope.Findings[0].Repair).To(ContainSubstring("scope: description"))

		custom := git.NewCustomPatternRule(`^JIRA-\d+`).Validate(commit, "Bad Title")
		Expect(custom.Findings[0].Required).To(Equal(`matches ^JIRA-\d+`))

		infra := git.NewInfraScopeMisuseRule().Validate(
			&git.ParsedCommit{Title: "feat(ci): x", Valid: true}, "feat(ci): x",
		)
		Expect(infra.Findings[0].Repair).To(Equal("Replace 'feat(ci):' with 'ci(<scope>):'"))

		signoff := (&git.SignoffRule{ExpectedSignoff: "A <a@b.c>"}).Validate(
			nil, "t\n\nSigned-off-by: B <b@b.c>",
		)
		Expect(signoff.Findings[0].Required).To(Equal("Signed-off-by: A <a@b.c>"))

		ai := git.NewAIAttributionRule().Validate(nil, "t\n\nGenerated with Claude Code")
		Expect(ai.Findings).To(HaveLen(1))

		forbidden := (&git.ForbiddenPatternRule{Patterns: []string{`tmp/`, `TODO`}}).Validate(
			nil, "t\n\nsee tmp/x TODO",
		)
		Expect(forbidden.Findings).To(HaveLen(2))
	})

	It("names the missing scope", func() {
		rule := &git.ConventionalFormatRule{ValidTypes: []string{"feat"}, RequireScope: true}
		result := rule.Validate(&git.ParsedCommit{Title: "feat: x", Type: "feat", Valid: true}, "")
		Expect(result.Findings[0].Message).To(Equal("Title has no scope"))
		Expect(result.Findings[0].Repair).To(ContainSubstring("within 50 characters"))
	})
})

var _ = Describe("AIAttributionRule path words", func() {
	rule := git.NewAIAttributionRule()

	DescribeTable("an assistant name inside a path is not a mention",
		func(msg string, blocked bool) {
			Expect(rule.Validate(nil, msg) != nil).To(Equal(blocked))
		},
		Entry("an agent worktree dir beside a marker word",
			"fix(git): drop paths\n\nA name under .claude/, paired with \"written\".", false),
		Entry("a temp dir beside a marker word",
			"fix(git): drop paths\n\nCreated /private/tmp/claude-502/x for the run.", false),
		Entry("a home-relative path",
			"fix(git): drop paths\n\nGenerated ~/.codex/config.toml from defaults.", false),
		Entry("a path in a code span",
			"fix(git): drop paths\n\nOutput written to `/private/tmp/claude-502/x`.", false),
		Entry("a home-relative path in a code span",
			"fix(git): drop paths\n\nTests read `~/.codex/config.toml` created by setup.", false),
		Entry("a variable-rooted path",
			"cd $HOME/.claude/worktrees/w && git commit -m \"fix(a): written\"", false),
		Entry("a redirect target",
			"echo x >/tmp/claude-502/out && git commit -m \"fix(a): written\"", false),
		Entry("a session link behind a double slash still blocks",
			"t\n\nSession: //claude.ai/code/session_01ABC", true),
		Entry("a footer behind a non-breaking space still blocks",
			"t\n\n/x\u00a0Generated with Claude Code", true),
		Entry("a relative directory",
			"cd src/claude && git commit -m \"fix(a): written\"", false),
		Entry("a relative file",
			"git add docs/claude-notes.md && git commit -m \"fix(a): written\"", false),
		Entry("a file URL",
			"see file:///private/tmp/claude-502/x, written by hand", false),
		Entry("a path glued to a short flag",
			"git -C/private/tmp/claude-502/w commit -m \"fix(a): written\"", false),
		Entry("a path under a defaulted variable",
			"cd ${tmpdir:-/tmp}/claude-502/w && git commit -m \"fix(a): written\"", false),
		Entry("an @-prefixed path value",
			"gh pr create --body-file @/tmp/claude-502/b -t \"fix(a): written\"", false),
		Entry("a path with an escaped space",
			"cd /users/a/my\\ dir/claude-x && git commit -m \"fix(a): written\"", false),
		Entry("assistant names joined by a slash still block",
			"t\n\nGenerated with Claude/Codex", true),
		Entry("an assistant and model joined by a slash still block",
			"t\n\nCo-authored-by: Claude/Opus-4.1", true),
		Entry("a vendor and assistant joined by a slash still block",
			"t\n\nWritten by OpenAI/Codex", true),
		Entry("a scheme-less product link still blocks",
			"t\n\nGenerated with claude.com/claude-code", true),
		Entry("a footer behind an escaped space still blocks",
			"t\n\nsee /tmp/x\\ Generated with Claude Code", true),
		Entry("a footer behind a zero-width space still blocks",
			"t\n\n/x\u200bGenerated with Claude Code", true),
		Entry("a footer behind a word joiner still blocks",
			"t\n\nsee /x\u2060Generated\u2060with\u2060Claude", true),
		Entry("a footer glued with a comma still blocks",
			"t\n\nlog /tmp/x,Generated with Claude Code", true),
		Entry("a robot emoji glued to a path still blocks",
			"t\n\nout /tmp/x\U0001F916 Claude", true),
		Entry("a single-slash session link still blocks",
			"t\n\nSession: /claude.ai/code/session_01ABC", true),
		Entry("credit beside a path still blocks",
			"fix(git): drop paths\n\nWritten by Claude in /tmp/x.", true),
		Entry("a footer link still blocks",
			"t\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)", true),
		Entry("a scheme-less session link still blocks",
			"t\n\nclaude.ai/code/session_01ABC", true),
		Entry("a co-author trailer still blocks",
			"t\n\nCo-authored-by: Claude <noreply@anthropic.com>", true),
	)
})
