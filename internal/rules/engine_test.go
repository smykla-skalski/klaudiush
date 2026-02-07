package rules_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("RuleEngine", func() {
	var (
		ctx    context.Context
		engine *rules.RuleEngine
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("NewRuleEngine", func() {
		It("should create engine with rules", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "test-rule",
					Enabled: true,
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked",
					},
				},
			}

			var err error
			engine, err = rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine.Size()).To(Equal(1))
		})

		It("should return error for invalid rule", func() {
			ruleList := []*rules.Rule{
				{
					Name:    "test-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						RepoPattern: "[invalid",
					},
					Action: &rules.RuleAction{Type: rules.ActionBlock},
				},
			}

			_, err := rules.NewRuleEngine(ruleList)
			Expect(err).To(HaveOccurred())
		})

		It("should create engine with empty rules", func() {
			var err error
			engine, err = rules.NewRuleEngine(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine.Size()).To(Equal(0))
		})
	})

	Describe("Evaluate", func() {
		BeforeEach(func() {
			ruleList := []*rules.Rule{
				{
					Name:     "block-org-origin",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
						RepoPattern:   "**/myorg/**",
						Remote:        "origin",
					},
					Action: &rules.RuleAction{
						Type:      rules.ActionBlock,
						Message:   "Organization repos should push to upstream",
						Reference: "GIT019",
					},
				},
				{
					Name:     "warn-team-upstream",
					Priority: 90,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
						RepoPattern:   "**/team-project/**",
						Remote:        "upstream",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionWarn,
						Message: "Warning: pushing to team upstream",
					},
				},
			}

			var err error
			engine, err = rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should block org origin push", func() {
			matchCtx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/myorg/project",
					Remote:   "origin",
				},
			}

			result := engine.Evaluate(ctx, matchCtx)
			Expect(result.Matched).To(BeTrue())
			Expect(result.Action).To(Equal(rules.ActionBlock))
			Expect(result.Reference).To(Equal("GIT019"))
		})

		It("should warn on team upstream push", func() {
			matchCtx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/team-project/repo",
					Remote:   "upstream",
				},
			}

			result := engine.Evaluate(ctx, matchCtx)
			Expect(result.Matched).To(BeTrue())
			Expect(result.Action).To(Equal(rules.ActionWarn))
		})

		It("should allow org upstream push", func() {
			matchCtx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/myorg/project",
					Remote:   "upstream",
				},
			}

			result := engine.Evaluate(ctx, matchCtx)
			Expect(result.Matched).To(BeFalse())
			Expect(result.Action).To(Equal(rules.ActionAllow))
		})

		It("should allow team origin push", func() {
			matchCtx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/team-project/repo",
					Remote:   "origin",
				},
			}

			result := engine.Evaluate(ctx, matchCtx)
			Expect(result.Matched).To(BeFalse())
		})

		It("should not match for unrelated repos", func() {
			matchCtx := &rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
				GitContext: &rules.GitContext{
					RepoRoot: "/home/user/personal/project",
					Remote:   "origin",
				},
			}

			result := engine.Evaluate(ctx, matchCtx)
			Expect(result.Matched).To(BeFalse())
		})
	})

	Describe("EvaluateHook", func() {
		BeforeEach(func() {
			ruleList := []*rules.Rule{
				{
					Name:    "test-rule",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
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
		})

		It("should evaluate hook context", func() {
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main",
				},
			}

			gitCtx := &rules.GitContext{
				RepoRoot: "/home/user/project",
				Remote:   "origin",
			}

			result := engine.EvaluateHook(ctx, hookCtx, rules.ValidatorGitPush, gitCtx, nil)
			Expect(result.Matched).To(BeTrue())
			Expect(result.Action).To(Equal(rules.ActionBlock))
		})
	})

	Describe("Rule Management", func() {
		BeforeEach(func() {
			var err error
			engine, err = rules.NewRuleEngine(nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add and remove rules", func() {
			rule := &rules.Rule{
				Name:    "dynamic-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type: rules.ActionBlock,
				},
			}

			err := engine.AddRule(rule)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine.Size()).To(Equal(1))

			removed := engine.RemoveRule("dynamic-rule")
			Expect(removed).To(BeTrue())
			Expect(engine.Size()).To(Equal(0))
		})

		It("should get rule by name", func() {
			rule := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "test message",
				},
			}

			_ = engine.AddRule(rule)

			retrieved := engine.GetRule("test-rule")
			Expect(retrieved).NotTo(BeNil())
			Expect(retrieved.Action.Message).To(Equal("test message"))
		})

		It("should return nil for non-existent rule", func() {
			retrieved := engine.GetRule("non-existent")
			Expect(retrieved).To(BeNil())
		})
	})

	Describe("GetAllRules and GetEnabledRules", func() {
		BeforeEach(func() {
			ruleList := []*rules.Rule{
				{
					Name:    "enabled",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionBlock},
				},
				{
					Name:    "disabled",
					Enabled: false,
					Action:  &rules.RuleAction{Type: rules.ActionWarn},
				},
			}

			var err error
			engine, err = rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return all rules", func() {
			all := engine.GetAllRules()
			Expect(len(all)).To(Equal(2))
		})

		It("should return only enabled rules", func() {
			enabled := engine.GetEnabledRules()
			Expect(len(enabled)).To(Equal(1))
			Expect(enabled[0].Name).To(Equal("enabled"))
		})
	})

	Describe("FilterByValidator", func() {
		BeforeEach(func() {
			ruleList := []*rules.Rule{
				{
					Name:    "git-push",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{Type: rules.ActionBlock},
				},
				{
					Name:    "git-commit",
					Enabled: true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitCommit,
					},
					Action: &rules.RuleAction{Type: rules.ActionWarn},
				},
			}

			var err error
			engine, err = rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should filter rules by validator type", func() {
			filtered := engine.FilterByValidator(rules.ValidatorGitPush)
			Expect(len(filtered)).To(Equal(1))
			Expect(filtered[0].Name).To(Equal("git-push"))
		})
	})

	Describe("Merge", func() {
		It("should merge rules from another engine", func() {
			engine1, _ := rules.NewRuleEngine([]*rules.Rule{
				{
					Name:    "rule1",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionBlock},
				},
			})

			engine2, _ := rules.NewRuleEngine([]*rules.Rule{
				{
					Name:    "rule2",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionWarn},
				},
			})

			engine1.Merge(engine2)
			Expect(engine1.Size()).To(Equal(2))
		})

		It("should handle nil engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{
					Name:    "rule1",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionBlock},
				},
			})

			engine.Merge(nil)
			Expect(engine.Size()).To(Equal(1))
		})
	})

	Describe("Options", func() {
		It("should respect default action option", func() {
			engine, _ := rules.NewRuleEngine(
				nil,
				rules.WithEngineDefaultAction(rules.ActionBlock),
			)

			result := engine.Evaluate(ctx, &rules.MatchContext{})
			Expect(result.Matched).To(BeFalse())
			Expect(result.Action).To(Equal(rules.ActionBlock))
		})
	})
})
