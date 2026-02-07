package rules_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("RuleValidatorAdapter", func() {
	var (
		ctx     context.Context
		engine  *rules.RuleEngine
		adapter *rules.RuleValidatorAdapter
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("CheckRules", func() {
		Context("with blocking rule", func() {
			BeforeEach(func() {
				ruleList := []*rules.Rule{
					{
						Name:     "block-origin",
						Priority: 100,
						Enabled:  true,
						Match: &rules.RuleMatch{
							ValidatorType: rules.ValidatorGitPush,
							Remote:        "origin",
						},
						Action: &rules.RuleAction{
							Type:      rules.ActionBlock,
							Message:   "blocked origin push",
							Reference: "GIT019",
						},
					},
				}

				var err error
				engine, err = rules.NewRuleEngine(ruleList)
				Expect(err).NotTo(HaveOccurred())

				adapter = rules.NewRuleValidatorAdapter(
					engine,
					rules.ValidatorGitPush,
				)
			})

			It("should return fail result when rule blocks", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				adapter.GitContextProvider = func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "origin",
					}
				}

				result := adapter.CheckRules(ctx, hookCtx)
				Expect(result).NotTo(BeNil())
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeTrue())
				Expect(result.Message).To(Equal("blocked origin push"))
				Expect(string(result.Reference)).To(Equal("GIT019"))
			})

			It("should return nil when no rules match", func() {
				hookCtx := &hook.Context{
					EventType: hook.EventTypePreToolUse,
					ToolName:  hook.ToolTypeBash,
				}

				adapter.GitContextProvider = func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "upstream",
					}
				}

				result := adapter.CheckRules(ctx, hookCtx)
				Expect(result).To(BeNil())
			})
		})

		Context("with warning rule", func() {
			BeforeEach(func() {
				ruleList := []*rules.Rule{
					{
						Name:    "warn-upstream",
						Enabled: true,
						Match: &rules.RuleMatch{
							Remote: "upstream",
						},
						Action: &rules.RuleAction{
							Type:    rules.ActionWarn,
							Message: "warning: pushing to upstream",
						},
					},
				}

				var err error
				engine, err = rules.NewRuleEngine(ruleList)
				Expect(err).NotTo(HaveOccurred())

				adapter = rules.NewRuleValidatorAdapter(
					engine,
					rules.ValidatorGitPush,
				)
			})

			It("should return warn result when rule warns", func() {
				hookCtx := &hook.Context{}

				adapter.GitContextProvider = func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "upstream",
					}
				}

				result := adapter.CheckRules(ctx, hookCtx)
				Expect(result).NotTo(BeNil())
				Expect(result.Passed).To(BeFalse())
				Expect(result.ShouldBlock).To(BeFalse())
				Expect(result.Message).To(Equal("warning: pushing to upstream"))
			})
		})

		Context("with allow rule", func() {
			BeforeEach(func() {
				ruleList := []*rules.Rule{
					{
						Name:    "allow-all",
						Enabled: true,
						Action: &rules.RuleAction{
							Type: rules.ActionAllow,
						},
					},
				}

				var err error
				engine, err = rules.NewRuleEngine(ruleList)
				Expect(err).NotTo(HaveOccurred())

				adapter = rules.NewRuleValidatorAdapter(
					engine,
					rules.ValidatorGitPush,
				)
			})

			It("should return pass result when rule allows", func() {
				result := adapter.CheckRules(ctx, &hook.Context{})
				Expect(result).NotTo(BeNil())
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("with nil engine", func() {
			BeforeEach(func() {
				adapter = rules.NewRuleValidatorAdapter(
					nil,
					rules.ValidatorGitPush,
				)
			})

			It("should return nil", func() {
				result := adapter.CheckRules(ctx, &hook.Context{})
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("CheckRulesWithContext", func() {
		BeforeEach(func() {
			ruleList := []*rules.Rule{
				{
					Name:    "block-origin",
					Enabled: true,
					Match: &rules.RuleMatch{
						Remote: "origin",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked",
					},
				},
			}

			var err error
			engine, err = rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
			)
		})

		It("should use provided git context", func() {
			hookCtx := &hook.Context{}
			gitCtx := &rules.GitContext{
				Remote: "origin",
			}

			result := adapter.CheckRulesWithContext(ctx, hookCtx, gitCtx, nil)
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
		})

		It("should use provided file context", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "block-test-files",
					Enabled: true,
					Match: &rules.RuleMatch{
						FilePattern: "*_test.go",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked test file",
					},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(engine, rules.ValidatorFileAll)

			hookCtx := &hook.Context{}
			fileCtx := &rules.FileContext{
				Path: "main_test.go",
			}

			result := adapter.CheckRulesWithContext(ctx, hookCtx, nil, fileCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
		})
	})

	Describe("HasRulesForValidator", func() {
		It("should return true when rules exist for validator", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "git-push-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{Type: rules.ActionBlock},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)

			Expect(adapter.HasRulesForValidator()).To(BeTrue())
		})

		It("should return false when no rules exist for validator", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "git-commit-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitCommit,
					},
					Action: &rules.RuleAction{Type: rules.ActionBlock},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)

			Expect(adapter.HasRulesForValidator()).To(BeFalse())
		})

		It("should return false when engine is nil", func() {
			adapter = rules.NewRuleValidatorAdapter(nil, rules.ValidatorGitPush)
			Expect(adapter.HasRulesForValidator()).To(BeFalse())
		})
	})

	Describe("GetApplicableRules", func() {
		It("should return rules for this validator", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "git-push-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{Type: rules.ActionBlock},
				},
				{
					Name:    "git-commit-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitCommit,
					},
					Action: &rules.RuleAction{Type: rules.ActionWarn},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)

			applicable := adapter.GetApplicableRules()
			Expect(len(applicable)).To(Equal(1))
			Expect(applicable[0].Name).To(Equal("git-push-rule"))
		})

		It("should return nil when engine is nil", func() {
			adapter = rules.NewRuleValidatorAdapter(nil, rules.ValidatorGitPush)
			Expect(adapter.GetApplicableRules()).To(BeNil())
		})
	})

	Describe("Context Providers", func() {
		BeforeEach(func() {
			ruleList := []*rules.Rule{
				{
					Name:    "repo-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						RepoPattern: "**/myorg/**",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked org repo",
					},
				},
			}

			var err error
			engine, err = rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use GitContextProvider", func() {
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						RepoRoot: "/home/user/myorg/project",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
		})

		It("should use FileContextProvider", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "content-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						ContentPattern: "password",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked password",
					},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorSecrets,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Content: "const password = 'secret'",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
		})
	})

	Describe("Warn with reference", func() {
		It("should return warn result with reference", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "warn-with-ref",
					Enabled: true,
					Match: &rules.RuleMatch{
						Remote: "upstream",
					},
					Action: &rules.RuleAction{
						Type:      rules.ActionWarn,
						Message:   "warning message",
						Reference: "GIT020",
					},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "upstream",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(Equal("warning message"))
			Expect(string(result.Reference)).To(Equal("GIT020"))
		})
	})

	Describe("Block without reference", func() {
		It("should return fail result without reference", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "block-no-ref",
					Enabled: true,
					Match: &rules.RuleMatch{
						Remote: "blocked",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked message",
						// No reference
					},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "blocked",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("blocked message"))
			Expect(result.Reference).To(BeEmpty())
		})
	})

	Describe("Unknown action type", func() {
		It("should return nil for unknown action type", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "unknown-action",
					Enabled: true,
					Action: &rules.RuleAction{
						Type:    rules.ActionType("unknown"),
						Message: "test",
					},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})
	})

	Describe("CheckRulesWithContext with nil engine", func() {
		It("should return nil", func() {
			adapter = rules.NewRuleValidatorAdapter(nil, rules.ValidatorGitPush)

			result := adapter.CheckRulesWithContext(ctx, &hook.Context{}, nil, nil)
			Expect(result).To(BeNil())
		})
	})

	Describe("Warn without reference", func() {
		It("should return warn result without reference", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "warn-no-ref",
					Enabled: true,
					Match: &rules.RuleMatch{
						Remote: "warning",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionWarn,
						Message: "just a warning",
						// No reference
					},
				},
			}

			engine, _ = rules.NewRuleEngine(ruleList)
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "warning",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Reference).To(BeEmpty())
		})
	})

	Describe("AdapterOption functions", func() {
		It("should apply WithAdapterLogger option", func() {
			ruleList := []*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionAllow}},
			}
			engine, _ = rules.NewRuleEngine(ruleList)

			// Should not panic.
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithAdapterLogger(nil), // nil logger should be handled
			)

			Expect(adapter).NotTo(BeNil())
		})
	})
})
