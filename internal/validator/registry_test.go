package validator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("Git Predicates", func() {
	Describe("GitSubcommandIs", func() {
		It("matches git checkout command", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git checkout with -C global option", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git -C /path/to/repo checkout main"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git checkout with long path in -C option", func() {
			ctx := &hook.Context{
				ToolName: hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git -C /Users/test/Projects/github.com/org/repo checkout -b feature",
				},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match different subcommand", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git commit -m 'test'"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match non-git command", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "ls -la"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match non-Bash tool", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeWrite,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitSubcommandIs("checkout")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandIn", func() {
		It("matches any of the specified subcommands", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git -C /repo checkout -b feature"},
			}

			predicate := validator.GitSubcommandIn("checkout", "switch", "branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match unlisted subcommand", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git commit -m 'test'"},
			}

			predicate := validator.GitSubcommandIn("checkout", "switch", "branch")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitHasFlag", func() {
		It("matches flag in command", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git checkout -b feature"},
			}

			predicate := validator.GitHasFlag("-b")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches flag with -C global option", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git -C /path/to/repo checkout -b feature"},
			}

			predicate := validator.GitHasFlag("-b")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match missing flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitHasFlag("-b")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitHasAnyFlag", func() {
		It("matches any of the specified flags", func() {
			ctx := &hook.Context{
				ToolName: hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git -C /repo checkout -b feature upstream/main",
				},
			}

			predicate := validator.GitHasAnyFlag("-b", "--branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches long flag variant", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git checkout --branch feature"},
			}

			predicate := validator.GitHasAnyFlag("-b", "--branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match when none of the flags are present", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitHasAnyFlag("-b", "--branch")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandWithFlag", func() {
		It("matches subcommand with specific flag", func() {
			ctx := &hook.Context{
				ToolName: hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git -C /repo checkout -b feature upstream/main",
				},
			}

			predicate := validator.GitSubcommandWithFlag("checkout", "-b")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match subcommand without flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git checkout main"},
			}

			predicate := validator.GitSubcommandWithFlag("checkout", "-b")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match different subcommand with flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git commit -m 'test'"},
			}

			predicate := validator.GitSubcommandWithFlag("checkout", "-b")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandWithAnyFlag", func() {
		It("matches checkout with -b flag", func() {
			ctx := &hook.Context{
				ToolName: hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git -C /path/to/repo checkout -b feature upstream/main",
				},
			}

			predicate := validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches switch with -c flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git switch -c feature"},
			}

			predicate := validator.GitSubcommandWithAnyFlag(
				"switch",
				"-c",
				"--create",
				"-C",
				"--force-create",
			)
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match checkout without branch creation flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git -C /repo checkout main"},
			}

			predicate := validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("GitSubcommandWithoutAnyFlag", func() {
		It("matches branch without delete flags", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git -C /repo branch new-feature"},
			}

			predicate := validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete")
			Expect(predicate(ctx)).To(BeTrue())
		})

		It("does not match branch with delete flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git branch -d old-feature"},
			}

			predicate := validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete")
			Expect(predicate(ctx)).To(BeFalse())
		})

		It("does not match branch with -D flag", func() {
			ctx := &hook.Context{
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{Command: "git -C /repo branch -D old-feature"},
			}

			predicate := validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete")
			Expect(predicate(ctx)).To(BeFalse())
		})
	})

	Describe("Command chains", func() {
		Describe("GitSubcommandIs with command chains", func() {
			It("matches git commit in 'git add && git commit' chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -sS -m "feat: add feature"`,
					},
				}

				predicate := validator.GitSubcommandIs("commit")
				Expect(predicate(ctx)).To(BeTrue())
			})

			It("matches git add in 'git add && git commit' chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -sS -m "feat: add feature"`,
					},
				}

				predicate := validator.GitSubcommandIs("add")
				Expect(predicate(ctx)).To(BeTrue())
			})

			It("matches git push in 'git add && git commit && git push' chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -m "fix: bug" && git push origin main`,
					},
				}

				predicate := validator.GitSubcommandIs("push")
				Expect(predicate(ctx)).To(BeTrue())
			})

			It("matches all subcommands in a triple chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -m "test" && git push`,
					},
				}

				Expect(validator.GitSubcommandIs("add")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("commit")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("push")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("checkout")(ctx)).To(BeFalse())
			})

			It("does not match subcommand not in chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -m "test"`,
					},
				}

				predicate := validator.GitSubcommandIs("push")
				Expect(predicate(ctx)).To(BeFalse())
			})

			It("handles semicolon-separated commands", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add .; git commit -m "test"`,
					},
				}

				Expect(validator.GitSubcommandIs("add")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("commit")(ctx)).To(BeTrue())
			})

			It("handles pipe chains (only first command is git)", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git log --oneline | head -5`,
					},
				}

				predicate := validator.GitSubcommandIs("log")
				Expect(predicate(ctx)).To(BeTrue())
			})
		})

		Describe("GitSubcommandIn with command chains", func() {
			It("matches any subcommand from chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -m "test"`,
					},
				}

				predicate := validator.GitSubcommandIn("commit", "push")
				Expect(predicate(ctx)).To(BeTrue())
			})

			It("matches when chain contains one of many options", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git stash && git checkout main && git stash pop`,
					},
				}

				predicate := validator.GitSubcommandIn("checkout", "switch")
				Expect(predicate(ctx)).To(BeTrue())
			})
		})

		Describe("GitHasFlag with command chains", func() {
			It("matches flag from any command in chain", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -sS -m "test"`,
					},
				}

				Expect(validator.GitHasFlag("-A")(ctx)).To(BeTrue())
				Expect(validator.GitHasFlag("-s")(ctx)).To(BeTrue())
				Expect(validator.GitHasFlag("-S")(ctx)).To(BeTrue())
				Expect(validator.GitHasFlag("-m")(ctx)).To(BeTrue())
			})

			It("does not match flag not present in any command", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -m "test"`,
					},
				}

				predicate := validator.GitHasFlag("--force")
				Expect(predicate(ctx)).To(BeFalse())
			})
		})

		Describe("GitSubcommandWithFlag with command chains", func() {
			It("matches specific subcommand with its flag", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -sS -m "test"`,
					},
				}

				// commit has -s flag
				Expect(validator.GitSubcommandWithFlag("commit", "-s")(ctx)).To(BeTrue())
				// add has -A flag
				Expect(validator.GitSubcommandWithFlag("add", "-A")(ctx)).To(BeTrue())
			})

			It("does not match when subcommand doesn't have the flag", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -m "test"`,
					},
				}

				// add has -A but commit doesn't have -A
				Expect(validator.GitSubcommandWithFlag("commit", "-A")(ctx)).To(BeFalse())
				// commit has -m but add doesn't have -m
				Expect(validator.GitSubcommandWithFlag("add", "-m")(ctx)).To(BeFalse())
			})

			It("does not match subcommand not in chain even if flag exists elsewhere", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -m "test"`,
					},
				}

				// push is not in chain
				Expect(validator.GitSubcommandWithFlag("push", "-A")(ctx)).To(BeFalse())
			})
		})

		Describe("GitSubcommandWithoutFlag with command chains", func() {
			It("matches subcommand without specific flag", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -m "test"`,
					},
				}

				// commit doesn't have --no-verify flag
				Expect(
					validator.GitSubcommandWithoutFlag("commit", "--no-verify")(ctx),
				).To(BeTrue())
			})

			It("does not match when subcommand has the flag", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit --no-verify -m "test"`,
					},
				}

				Expect(
					validator.GitSubcommandWithoutFlag("commit", "--no-verify")(ctx),
				).To(BeFalse())
			})
		})

		Describe("GitSubcommandWithAnyFlag with command chains", func() {
			It("matches subcommand with any of specified flags", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -sS -m "test"`,
					},
				}

				predicate := validator.GitSubcommandWithAnyFlag("commit", "-s", "--signoff")
				Expect(predicate(ctx)).To(BeTrue())
			})
		})

		Describe("GitSubcommandWithoutAnyFlag with command chains", func() {
			It("matches subcommand without any of specified flags", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git push origin main`,
					},
				}

				predicate := validator.GitSubcommandWithoutAnyFlag("push", "--force", "-f")
				Expect(predicate(ctx)).To(BeTrue())
			})

			It("does not match when subcommand has one of the flags", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git push --force origin main`,
					},
				}

				predicate := validator.GitSubcommandWithoutAnyFlag("push", "--force", "-f")
				Expect(predicate(ctx)).To(BeFalse())
			})
		})

		Describe("Complex real-world command chains", func() {
			It("validates the exact failing command from bug report", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add -A && git commit -sS -m "feat(enums): add enumer code generation for type-safe enums

Replace string-based enums with int-based enums using enumer code
generation for ` + "`EventType`" + `, ` + "`ToolType`" + `, and ` + "`Severity`" + ` types."`,
					},
				}

				// The commit validator predicate
				predicate := validator.And(
					validator.EventTypeIs(hook.EventTypePreToolUse),
					validator.GitSubcommandIs("commit"),
				)

				Expect(predicate(ctx)).To(BeTrue())
			})

			It("handles git status && git add && git commit pattern", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git status && git add -A && git commit -sS -m "fix: resolve issue"`,
					},
				}

				Expect(validator.GitSubcommandIs("status")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("add")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("commit")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandWithFlag("commit", "-s")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandWithFlag("commit", "-S")(ctx)).To(BeTrue())
			})

			It("handles mixed git and non-git commands", func() {
				ctx := &hook.Context{
					ToolName: hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `echo "Committing..." && git add . && git commit -m "test" && echo "Done"`,
					},
				}

				Expect(validator.GitSubcommandIs("add")(ctx)).To(BeTrue())
				Expect(validator.GitSubcommandIs("commit")(ctx)).To(BeTrue())
			})

			It("handles git command with HEREDOC body", func() {
				ctx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
					ToolInput: hook.ToolInput{
						Command: `git add . && git commit -sS -m "$(cat <<'EOF'
feat(api): add new endpoint

This is a multi-line commit message body.
EOF
)"`,
					},
				}

				predicate := validator.GitSubcommandIs("commit")
				Expect(predicate(ctx)).To(BeTrue())
			})
		})
	})

	Describe("Real-world scenarios", func() {
		It("matches the original failing case: git -C with long path checkout -b", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git -C /Users/bart.smykla@konghq.com/Projects/github.com/kong/kong-mesh checkout -b revert-mink-workflow-dispatch upstream/master",
				},
			}

			// This is the predicate used by the branch validator
			predicate := validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.Or(
					validator.GitSubcommandWithAnyFlag("checkout", "-b", "--branch"),
					validator.GitSubcommandWithAnyFlag(
						"switch",
						"-c",
						"--create",
						"-C",
						"--force-create",
					),
					validator.GitSubcommandWithoutAnyFlag("branch", "-d", "-D", "--delete"),
				),
			)

			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git commit with -C path", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git -C /path/to/repo commit -sS -m 'feat: add feature'",
				},
			}

			predicate := validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.GitSubcommandIs("commit"),
			)

			Expect(predicate(ctx)).To(BeTrue())
		})

		It("matches git push with --git-dir option", func() {
			ctx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git --git-dir=/custom/.git push origin main",
				},
			}

			predicate := validator.And(
				validator.EventTypeIs(hook.EventTypePreToolUse),
				validator.GitSubcommandIs("push"),
			)

			Expect(predicate(ctx)).To(BeTrue())
		})
	})
})
