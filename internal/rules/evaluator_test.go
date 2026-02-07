package rules_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/rules"
)

var _ = Describe("Evaluator", func() {
	var (
		registry  *rules.Registry
		evaluator *rules.Evaluator
	)

	BeforeEach(func() {
		registry = rules.NewRegistry()
	})

	Describe("Evaluate", func() {
		It("should return no match when registry is empty", func() {
			evaluator = rules.NewEvaluator(registry)

			result := evaluator.Evaluate(&rules.MatchContext{})
			Expect(result.Matched).To(BeFalse())
			Expect(result.Action).To(Equal(rules.ActionAllow))
		})

		It("should return no match when registry is nil", func() {
			evaluator = rules.NewEvaluator(nil)

			result := evaluator.Evaluate(&rules.MatchContext{})
			Expect(result.Matched).To(BeFalse())
		})

		It("should match first matching rule", func() {
			_ = registry.Add(&rules.Rule{
				Name:     "block-origin",
				Priority: 100,
				Enabled:  true,
				Match: &rules.RuleMatch{
					Remote: "origin",
				},
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "blocked origin",
				},
			})

			evaluator = rules.NewEvaluator(registry)

			result := evaluator.Evaluate(&rules.MatchContext{
				GitContext: &rules.GitContext{
					Remote: "origin",
				},
			})

			Expect(result.Matched).To(BeTrue())
			Expect(result.Action).To(Equal(rules.ActionBlock))
			Expect(result.Message).To(Equal("blocked origin"))
			Expect(result.Rule.Name).To(Equal("block-origin"))
		})

		It("should respect priority order", func() {
			_ = registry.Add(&rules.Rule{
				Name:     "low-priority",
				Priority: 10,
				Enabled:  true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{
					Type:    rules.ActionWarn,
					Message: "low priority",
				},
			})
			_ = registry.Add(&rules.Rule{
				Name:     "high-priority",
				Priority: 100,
				Enabled:  true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "high priority",
				},
			})

			evaluator = rules.NewEvaluator(registry)

			result := evaluator.Evaluate(&rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
			})

			Expect(result.Matched).To(BeTrue())
			Expect(result.Rule.Name).To(Equal("high-priority"))
		})

		It("should skip disabled rules", func() {
			_ = registry.Add(&rules.Rule{
				Name:     "disabled-rule",
				Priority: 100,
				Enabled:  false,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{
					Type: rules.ActionBlock,
				},
			})
			_ = registry.Add(&rules.Rule{
				Name:     "enabled-rule",
				Priority: 50,
				Enabled:  true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{
					Type:    rules.ActionWarn,
					Message: "enabled",
				},
			})

			evaluator = rules.NewEvaluator(registry)

			result := evaluator.Evaluate(&rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
			})

			Expect(result.Matched).To(BeTrue())
			Expect(result.Rule.Name).To(Equal("enabled-rule"))
		})

		It("should include reference in result", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "rule-with-ref",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:      rules.ActionBlock,
					Message:   "blocked",
					Reference: "GIT001",
				},
			})

			evaluator = rules.NewEvaluator(registry)

			result := evaluator.Evaluate(&rules.MatchContext{})
			Expect(result.Reference).To(Equal("GIT001"))
		})
	})

	Describe("EvaluateAll", func() {
		It("should return all matching rules", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "rule1",
				Enabled: true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "rule2",
				Enabled: true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{Type: rules.ActionWarn},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "rule3",
				Enabled: true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitCommit, // Different type.
				},
				Action: &rules.RuleAction{Type: rules.ActionAllow},
			})

			evaluator = rules.NewEvaluator(registry)

			results := evaluator.EvaluateAll(&rules.MatchContext{
				ValidatorType: rules.ValidatorGitPush,
			})

			Expect(len(results)).To(Equal(2))
		})

		It("should return nil when registry is nil", func() {
			evaluator = rules.NewEvaluator(nil)
			results := evaluator.EvaluateAll(&rules.MatchContext{})
			Expect(results).To(BeNil())
		})
	})

	Describe("FindMatchingRules", func() {
		It("should return matching rules", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "matching",
				Enabled: true,
				Match: &rules.RuleMatch{
					Remote: "origin",
				},
				Action: &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "non-matching",
				Enabled: true,
				Match: &rules.RuleMatch{
					Remote: "upstream",
				},
				Action: &rules.RuleAction{Type: rules.ActionWarn},
			})

			evaluator = rules.NewEvaluator(registry)

			matching := evaluator.FindMatchingRules(&rules.MatchContext{
				GitContext: &rules.GitContext{
					Remote: "origin",
				},
			})

			Expect(len(matching)).To(Equal(1))
			Expect(matching[0].Name).To(Equal("matching"))
		})
	})

	Describe("FilterByValidator", func() {
		It("should filter rules by validator type", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "git-push-rule",
				Enabled: true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitPush,
				},
				Action: &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "git-commit-rule",
				Enabled: true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitCommit,
				},
				Action: &rules.RuleAction{Type: rules.ActionWarn},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "universal-rule",
				Enabled: true,
				// No validator type filter.
				Action: &rules.RuleAction{Type: rules.ActionAllow},
			})

			evaluator = rules.NewEvaluator(registry)

			filtered := evaluator.FilterByValidator(rules.ValidatorGitPush)
			Expect(len(filtered)).To(Equal(2))

			names := make([]string, len(filtered))
			for i, r := range filtered {
				names[i] = r.Rule.Name
			}
			Expect(names).To(ContainElements("git-push-rule", "universal-rule"))
		})

		It("should match wildcard validator types", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "all-git-rule",
				Enabled: true,
				Match: &rules.RuleMatch{
					ValidatorType: rules.ValidatorGitAll,
				},
				Action: &rules.RuleAction{Type: rules.ActionBlock},
			})

			evaluator = rules.NewEvaluator(registry)

			filtered := evaluator.FilterByValidator(rules.ValidatorGitPush)
			Expect(len(filtered)).To(Equal(1))
			Expect(filtered[0].Rule.Name).To(Equal("all-git-rule"))
		})
	})

	Describe("Options", func() {
		It("should respect default action option", func() {
			evaluator = rules.NewEvaluator(
				registry,
				rules.WithDefaultAction(rules.ActionBlock),
			)

			result := evaluator.Evaluate(&rules.MatchContext{})
			Expect(result.Matched).To(BeFalse())
			Expect(result.Action).To(Equal(rules.ActionBlock))
		})
	})
})
