package rules_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("Matcher", func() {
	Describe("RepoPatternMatcher", func() {
		It("should match repo root with glob pattern", func() {
			matcher, err := rules.NewRepoPatternMatcher("**/myorg/**")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/myorg/project",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
			Expect(matcher.Name()).To(ContainSubstring("repo_pattern"))
		})

		It("should not match when GitContext is nil", func() {
			matcher, err := rules.NewRepoPatternMatcher("**/myorg/**")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should not match when RepoRoot is empty", func() {
			matcher, err := rules.NewRepoPatternMatcher("**/myorg/**")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					RepoRoot: "",
				},
			}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should match with regex pattern", func() {
			matcher, err := rules.NewRepoPatternMatcher("(?i).*/myorg/.*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/MyOrg/project",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})

		Describe("NewRepoPatternMatcherWithOpts", func() {
			It("should create matcher with case-insensitive option", func() {
				opts := rules.PatternOptions{CaseInsensitive: true}
				matcher, err := rules.NewRepoPatternMatcherWithOpts("**/MyOrg/**", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						RepoRoot: "/home/user/myorg/project",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should create matcher with negate option", func() {
				opts := rules.PatternOptions{Negate: true}
				matcher, err := rules.NewRepoPatternMatcherWithOpts("**/vendor/**", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						RepoRoot: "/home/user/myproject/src",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.RepoRoot = "/home/user/vendor/lib"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return error for invalid pattern", func() {
				opts := rules.PatternOptions{}
				_, err := rules.NewRepoPatternMatcherWithOpts("[invalid", opts)
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("NewRepoMultiPatternMatcher", func() {
			It("should match any of multiple patterns", func() {
				patterns := []string{"**/myorg/**", "**/theirorg/**"}
				matcher, err := rules.NewRepoMultiPatternMatcher(
					patterns,
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						RepoRoot: "/home/user/myorg/project",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.RepoRoot = "/home/user/theirorg/project"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.RepoRoot = "/home/user/other/project"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should require all patterns to match", func() {
				// Test with regex patterns that work reliably for path matching.
				patterns := []string{"^.*myorg.*$", "^.*project.*$"}
				matcher, err := rules.NewRepoMultiPatternMatcher(
					patterns,
					rules.MultiPatternAll,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				// Should match when both "myorg" and "project" are in the path.
				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						RepoRoot: "/home/user/myorg/project",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				// Should not match when only one pattern matches.
				ctx.GitContext.RepoRoot = "/home/user/other/project"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return nil for empty patterns", func() {
				matcher, err := rules.NewRepoMultiPatternMatcher(
					[]string{},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(matcher).To(BeNil())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewRepoMultiPatternMatcher(
					[]string{"[invalid"},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("RemoteMatcher", func() {
		It("should match exact remote name", func() {
			matcher := rules.NewRemoteMatcher("origin")

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					Remote: "origin",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
			Expect(matcher.Name()).To(Equal("remote:origin"))
		})

		It("should not match different remote", func() {
			matcher := rules.NewRemoteMatcher("origin")

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					Remote: "upstream",
				},
			}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should not match when GitContext is nil", func() {
			matcher := rules.NewRemoteMatcher("origin")

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})
	})

	Describe("BranchPatternMatcher", func() {
		It("should match branch with glob pattern", func() {
			matcher, err := rules.NewBranchPatternMatcher("feature/*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					Branch: "feature/new-feature",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
			Expect(matcher.Name()).To(ContainSubstring("branch_pattern"))
		})

		It("should match branch with regex pattern", func() {
			matcher, err := rules.NewBranchPatternMatcher("^release-\\d+\\.\\d+$")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					Branch: "release-1.2",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())

			ctx.GitContext.Branch = "release-test"
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should not match when GitContext is nil", func() {
			matcher, err := rules.NewBranchPatternMatcher("feature/*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should not match when Branch is empty", func() {
			matcher, err := rules.NewBranchPatternMatcher("feature/*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				GitContext: &rules.GitContext{
					Branch: "",
				},
			}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		Describe("NewBranchPatternMatcherWithOpts", func() {
			It("should create matcher with case-insensitive option", func() {
				opts := rules.PatternOptions{CaseInsensitive: true}
				matcher, err := rules.NewBranchPatternMatcherWithOpts("Feature/*", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						Branch: "feature/new-feature",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewBranchPatternMatcherWithOpts("[invalid", rules.PatternOptions{})
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("NewBranchMultiPatternMatcher", func() {
			It("should match any of multiple patterns", func() {
				patterns := []string{"main", "master", "develop"}
				matcher, err := rules.NewBranchMultiPatternMatcher(
					patterns,
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{Branch: "main"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.Branch = "feature/test"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return nil for empty patterns", func() {
				matcher, err := rules.NewBranchMultiPatternMatcher(
					[]string{},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(matcher).To(BeNil())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewBranchMultiPatternMatcher(
					[]string{"[invalid"},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("FilePatternMatcher", func() {
		It("should match file path from FileContext", func() {
			matcher, err := rules.NewFilePatternMatcher("**/test/**")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				FileContext: &rules.FileContext{
					Path: "src/test/file.go",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
			Expect(matcher.Name()).To(ContainSubstring("file_pattern"))
		})

		It("should fall back to HookContext file path", func() {
			matcher, err := rules.NewFilePatternMatcher("*.go")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				HookContext: &hook.Context{
					ToolInput: hook.ToolInput{
						FilePath: "main.go",
					},
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})

		It("should return false when no path available", func() {
			matcher, err := rules.NewFilePatternMatcher("*.go")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		Describe("NewFilePatternMatcherWithOpts", func() {
			It("should create matcher with case-insensitive option", func() {
				opts := rules.PatternOptions{CaseInsensitive: true}
				matcher, err := rules.NewFilePatternMatcherWithOpts("*.Go", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "main.go"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "main.GO"
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewFilePatternMatcherWithOpts("[invalid", rules.PatternOptions{})
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("NewFileMultiPatternMatcher", func() {
			It("should match any of multiple patterns", func() {
				patterns := []string{"*.go", "*.ts", "*.js"}
				matcher, err := rules.NewFileMultiPatternMatcher(
					patterns,
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "main.go"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "style.css"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return nil for empty patterns", func() {
				matcher, err := rules.NewFileMultiPatternMatcher(
					[]string{},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(matcher).To(BeNil())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewFileMultiPatternMatcher(
					[]string{"[invalid"},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("ContentPatternMatcher", func() {
		It("should match content with regex", func() {
			matcher, err := rules.NewContentPatternMatcher("(?i)password")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				FileContext: &rules.FileContext{
					Content: "const PASSWORD = 'secret'",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
			Expect(matcher.Name()).To(ContainSubstring("content_pattern"))
		})

		It("should fall back to HookContext content", func() {
			matcher, err := rules.NewContentPatternMatcher("func main")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				HookContext: &hook.Context{
					ToolInput: hook.ToolInput{
						Content: "package main\n\nfunc main() {}",
					},
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})

		It("should return false when no content available", func() {
			matcher, err := rules.NewContentPatternMatcher("pattern")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should return error for invalid regex", func() {
			_, err := rules.NewContentPatternMatcher("[invalid")
			Expect(err).To(HaveOccurred())
		})

		Describe("NewContentPatternMatcherWithOpts", func() {
			It("should create matcher with case-insensitive option", func() {
				opts := rules.PatternOptions{CaseInsensitive: true}
				matcher, err := rules.NewContentPatternMatcherWithOpts("TODO", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "// todo: fix this"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should create matcher with negated option", func() {
				opts := rules.PatternOptions{Negate: true}
				matcher, err := rules.NewContentPatternMatcherWithOpts("password", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "safe content"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "password = secret"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should handle negated pattern with ! prefix", func() {
				opts := rules.PatternOptions{}
				matcher, err := rules.NewContentPatternMatcherWithOpts("!secret", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "normal text"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should not duplicate (?i) flag", func() {
				opts := rules.PatternOptions{CaseInsensitive: true}
				matcher, err := rules.NewContentPatternMatcherWithOpts("(?i)test", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "TEST"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should return error for invalid regex", func() {
				_, err := rules.NewContentPatternMatcherWithOpts("[invalid", rules.PatternOptions{})
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("NewContentMultiPatternMatcher", func() {
			It("should match any of multiple content patterns", func() {
				patterns := []string{"TODO", "FIXME", "HACK"}
				matcher, err := rules.NewContentMultiPatternMatcher(
					patterns,
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "// TODO: fix this"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "// FIXME: broken"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "// normal comment"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should match all content patterns", func() {
				patterns := []string{"func", "main"}
				matcher, err := rules.NewContentMultiPatternMatcher(
					patterns,
					rules.MultiPatternAll,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "func main() {}"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "func helper() {}"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return nil for empty patterns", func() {
				matcher, err := rules.NewContentMultiPatternMatcher(
					[]string{},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(matcher).To(BeNil())
			})

			It("should use single pattern path for one pattern", func() {
				matcher, err := rules.NewContentMultiPatternMatcher(
					[]string{"TODO"},
					rules.MultiPatternAny,
					rules.PatternOptions{CaseInsensitive: true},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "// todo: fix"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should support negated patterns in multi-pattern", func() {
				patterns := []string{"secret", "!test_"}
				matcher, err := rules.NewContentMultiPatternMatcher(
					patterns,
					rules.MultiPatternAll,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				// Must contain "secret" AND NOT contain "test_"
				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "secret value"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "test_secret"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewContentMultiPatternMatcher(
					[]string{"valid", "[invalid"},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("CommandPatternMatcher", func() {
		It("should match command with glob pattern", func() {
			matcher, err := rules.NewCommandPatternMatcher("git push*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				Command: "git push origin main",
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
			Expect(matcher.Name()).To(ContainSubstring("command_pattern"))
		})

		It("should fall back to HookContext command", func() {
			matcher, err := rules.NewCommandPatternMatcher("git*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{
				HookContext: &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git status",
					},
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})

		It("should return false when no command available", func() {
			matcher, err := rules.NewCommandPatternMatcher("git*")
			Expect(err).NotTo(HaveOccurred())

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		Describe("NewCommandPatternMatcherWithOpts", func() {
			It("should create matcher with case-insensitive option", func() {
				opts := rules.PatternOptions{CaseInsensitive: true}
				matcher, err := rules.NewCommandPatternMatcherWithOpts("*GIT*", opts)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					Command: "git push origin",
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewCommandPatternMatcherWithOpts("[invalid", rules.PatternOptions{})
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("NewCommandMultiPatternMatcher", func() {
			It("should match any of multiple command patterns", func() {
				patterns := []string{"git push*", "git commit*", "git merge*"}
				matcher, err := rules.NewCommandMultiPatternMatcher(
					patterns,
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					Command: "git push origin main",
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.Command = "git status"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should return nil for empty patterns", func() {
				matcher, err := rules.NewCommandMultiPatternMatcher(
					[]string{},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(matcher).To(BeNil())
			})

			It("should return error for invalid pattern", func() {
				_, err := rules.NewCommandMultiPatternMatcher(
					[]string{"[invalid"},
					rules.MultiPatternAny,
					rules.PatternOptions{},
				)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("ValidatorTypeMatcher", func() {
		It("should match exact validator type", func() {
			matcher := rules.NewValidatorTypeMatcher(rules.ValidatorGitPush)

			ctx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})

		It("should match wildcard all", func() {
			matcher := rules.NewValidatorTypeMatcher(rules.ValidatorAll)

			ctx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})

		It("should match category wildcard", func() {
			matcher := rules.NewValidatorTypeMatcher(rules.ValidatorGitAll)

			ctx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
			}
			Expect(matcher.Match(ctx)).To(BeTrue())

			ctx.ValidatorType = rules.ValidatorGitCommit
			Expect(matcher.Match(ctx)).To(BeTrue())

			ctx.ValidatorType = rules.ValidatorFileMarkdown
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should not match when ValidatorType is empty", func() {
			matcher := rules.NewValidatorTypeMatcher(rules.ValidatorGitPush)

			ctx := &rules.MatchContext{}
			Expect(matcher.Match(ctx)).To(BeFalse())
		})
	})

	Describe("ToolTypeMatcher", func() {
		It("should match tool type case-insensitively", func() {
			matcher := rules.NewToolTypeMatcher("Bash")

			ctx := &rules.MatchContext{
				HookContext: &hook.Context{
					ToolName: hook.ToolTypeBash,
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})
	})

	Describe("EventTypeMatcher", func() {
		It("should match event type case-insensitively", func() {
			matcher := rules.NewEventTypeMatcher("PreToolUse")

			ctx := &rules.MatchContext{
				HookContext: &hook.Context{
					EventType: hook.EventTypePreToolUse,
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())
		})
	})

	Describe("CompositeMatcher", func() {
		Describe("AND", func() {
			It("should match when all conditions match", func() {
				matcher := rules.NewAndMatcher(
					rules.NewRemoteMatcher("origin"),
					rules.NewValidatorTypeMatcher(rules.ValidatorGitPush),
				)

				ctx := &rules.MatchContext{
					ValidatorType: rules.ValidatorGitPush,
					GitContext: &rules.GitContext{
						Remote: "origin",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
				Expect(matcher.Name()).To(Equal("AND"))
			})

			It("should not match when any condition fails", func() {
				matcher := rules.NewAndMatcher(
					rules.NewRemoteMatcher("origin"),
					rules.NewValidatorTypeMatcher(rules.ValidatorGitPush),
				)

				ctx := &rules.MatchContext{
					ValidatorType: rules.ValidatorGitPush,
					GitContext: &rules.GitContext{
						Remote: "upstream",
					},
				}
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should match with empty matchers", func() {
				matcher := rules.NewAndMatcher()
				ctx := &rules.MatchContext{}
				Expect(matcher.Match(ctx)).To(BeTrue())
			})
		})

		Describe("OR", func() {
			It("should match when any condition matches", func() {
				matcher := rules.NewOrMatcher(
					rules.NewRemoteMatcher("origin"),
					rules.NewRemoteMatcher("upstream"),
				)

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						Remote: "upstream",
					},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())
				Expect(matcher.Name()).To(Equal("OR"))
			})

			It("should not match when no conditions match", func() {
				matcher := rules.NewOrMatcher(
					rules.NewRemoteMatcher("origin"),
					rules.NewRemoteMatcher("upstream"),
				)

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						Remote: "other",
					},
				}
				Expect(matcher.Match(ctx)).To(BeFalse())
			})
		})

		Describe("NOT", func() {
			It("should invert the result", func() {
				matcher := rules.NewNotMatcher(
					rules.NewRemoteMatcher("origin"),
				)

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{
						Remote: "origin",
					},
				}
				Expect(matcher.Match(ctx)).To(BeFalse())

				ctx.GitContext.Remote = "upstream"
				Expect(matcher.Match(ctx)).To(BeTrue())
				Expect(matcher.Name()).To(Equal("NOT"))
			})
		})
	})

	Describe("BuildMatcher", func() {
		It("should build composite matcher from RuleMatch", func() {
			match := &rules.RuleMatch{
				ValidatorType: rules.ValidatorGitPush,
				RepoPattern:   "**/myorg/**",
				Remote:        "origin",
			}

			matcher, err := rules.BuildMatcher(match)
			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())

			ctx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/myorg/project",
					Remote:   "origin",
				},
			}
			Expect(matcher.Match(ctx)).To(BeTrue())

			// Should not match with different remote.
			ctx.GitContext.Remote = "upstream"
			Expect(matcher.Match(ctx)).To(BeFalse())
		})

		It("should return nil for nil RuleMatch", func() {
			matcher, err := rules.BuildMatcher(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).To(BeNil())
		})

		It("should return nil for empty RuleMatch", func() {
			matcher, err := rules.BuildMatcher(&rules.RuleMatch{})
			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).To(BeNil())
		})

		It("should return single matcher for single condition", func() {
			match := &rules.RuleMatch{
				Remote: "origin",
			}

			matcher, err := rules.BuildMatcher(match)
			Expect(err).NotTo(HaveOccurred())
			Expect(matcher).NotTo(BeNil())

			// Should be a RemoteMatcher, not a CompositeMatcher.
			Expect(matcher.Name()).To(Equal("remote:origin"))
		})

		It("should return error for invalid pattern", func() {
			match := &rules.RuleMatch{
				RepoPattern: "[invalid",
			}

			_, err := rules.BuildMatcher(match)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AlwaysMatcher", func() {
		It("should always match", func() {
			matcher := &rules.AlwaysMatcher{}
			Expect(matcher.Match(&rules.MatchContext{})).To(BeTrue())
			Expect(matcher.Match(nil)).To(BeTrue())
			Expect(matcher.Name()).To(Equal("always"))
		})
	})

	Describe("NeverMatcher", func() {
		It("should never match", func() {
			matcher := &rules.NeverMatcher{}
			Expect(matcher.Match(&rules.MatchContext{})).To(BeFalse())
			Expect(matcher.Match(nil)).To(BeFalse())
			Expect(matcher.Name()).To(Equal("never"))
		})
	})

	Describe("Advanced Pattern Matchers", func() {
		Describe("BuildMatcher with CaseInsensitive", func() {
			It("should match file patterns case-insensitively", func() {
				match := &rules.RuleMatch{
					FilePattern:     "*.Md",
					CaseInsensitive: true,
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "README.md"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "README.MD"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "file.txt"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should match branch patterns case-insensitively", func() {
				match := &rules.RuleMatch{
					BranchPattern:   "Feature/*",
					CaseInsensitive: true,
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{Branch: "feature/new-feature"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.Branch = "FEATURE/new-feature"
				Expect(matcher.Match(ctx)).To(BeTrue())
			})
		})

		Describe("BuildMatcher with Multi-Patterns", func() {
			It("should match any of multiple file patterns", func() {
				match := &rules.RuleMatch{
					FilePatterns: []string{"*.go", "*.ts", "*.js"},
					PatternMode:  "any",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "main.go"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "index.ts"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "app.js"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "style.css"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should match all of multiple branch patterns", func() {
				match := &rules.RuleMatch{
					BranchPatterns: []string{"feat*", "*-wip"},
					PatternMode:    "all",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{Branch: "feature-wip"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				// Doesn't match all patterns.
				ctx.GitContext.Branch = "feature-done"
				Expect(matcher.Match(ctx)).To(BeFalse())

				ctx.GitContext.Branch = "bugfix-wip"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should match any of multiple repo patterns", func() {
				match := &rules.RuleMatch{
					RepoPatterns: []string{"**/myorg/**", "**/theirorg/**"},
					PatternMode:  "any",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					GitContext: &rules.GitContext{RepoRoot: "/home/user/myorg/project"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.RepoRoot = "/home/user/theirorg/project"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.GitContext.RepoRoot = "/home/user/other/project"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})
		})

		Describe("BuildMatcher with Negated Patterns", func() {
			It("should match negated file patterns", func() {
				match := &rules.RuleMatch{
					FilePattern: "!*.tmp",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "src/main.go"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "cache.tmp"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})

			It("should combine negated patterns with multi-patterns", func() {
				match := &rules.RuleMatch{
					FilePatterns: []string{"*.go", "!*_test.go"},
					PatternMode:  "all",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "main.go"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				// Test files should not match.
				ctx.FileContext.Path = "main_test.go"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})
		})

		Describe("BuildMatcher with Combined Options", func() {
			It("should combine case-insensitive and multi-patterns", func() {
				match := &rules.RuleMatch{
					FilePatterns:    []string{"*.Go", "*.TS"},
					CaseInsensitive: true,
					PatternMode:     "any",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "main.GO"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "index.ts"
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should use legacy builder when no advanced features", func() {
				match := &rules.RuleMatch{
					FilePattern: "*.go",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Path: "main.go"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Path = "main.GO"
				Expect(matcher.Match(ctx)).To(BeFalse()) // Case-sensitive by default.
			})
		})

		Describe("Content Pattern Matchers", func() {
			It("should match content with case-insensitive patterns", func() {
				match := &rules.RuleMatch{
					ContentPattern:  "TODO",
					CaseInsensitive: true,
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "// todo: fix this"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "// TODO: fix this"
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should match multiple content patterns", func() {
				match := &rules.RuleMatch{
					ContentPatterns: []string{"TODO", "FIXME"},
					PatternMode:     "any",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					FileContext: &rules.FileContext{Content: "// TODO: fix this"},
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "// FIXME: broken"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.FileContext.Content = "// Normal comment"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})
		})

		Describe("Command Pattern Matchers", func() {
			It("should match commands with case-insensitive patterns", func() {
				match := &rules.RuleMatch{
					CommandPattern:  "*GIT*",
					CaseInsensitive: true,
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					Command: "git push origin main",
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.Command = "GIT push origin main"
				Expect(matcher.Match(ctx)).To(BeTrue())
			})

			It("should match multiple command patterns", func() {
				match := &rules.RuleMatch{
					CommandPatterns: []string{"git push*", "git commit*"},
					PatternMode:     "any",
				}

				matcher, err := rules.BuildMatcher(match)
				Expect(err).NotTo(HaveOccurred())

				ctx := &rules.MatchContext{
					Command: "git push origin main",
				}
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.Command = "git commit -m 'test'"
				Expect(matcher.Match(ctx)).To(BeTrue())

				ctx.Command = "git status"
				Expect(matcher.Match(ctx)).To(BeFalse())
			})
		})

		Describe("parsePatternMode", func() {
			// This is tested implicitly through BuildMatcher tests above.
			// The function correctly handles "any" (default) and "all".
		})
	})
})
