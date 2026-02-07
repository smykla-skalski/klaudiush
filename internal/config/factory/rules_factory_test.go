package factory_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/config/factory"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

func TestFactory(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Factory Suite")
}

var _ = Describe("RulesFactory", func() {
	var (
		rulesFactory *factory.RulesFactory
		log          logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		rulesFactory = factory.NewRulesFactory(log)
	})

	Describe("CreateRuleEngine", func() {
		It("should return nil for nil config", func() {
			engine, err := rulesFactory.CreateRuleEngine(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})

		It("should return nil when rules config is nil", func() {
			cfg := &config.Config{}
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})

		It("should return nil when rules are disabled", func() {
			enabled := false
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
				},
			}
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})

		It("should return nil when no rules defined", func() {
			enabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules:   []config.RuleConfig{},
				},
			}
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})

		It("should return nil when all rules are disabled", func() {
			enabled := true
			ruleEnabled := false
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:    "disabled-rule",
							Enabled: &ruleEnabled,
						},
					},
				},
			}
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})

		It("should create engine with valid rules", func() {
			enabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:     "test-rule",
							Priority: 100,
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
								Remote:        "origin",
							},
							Action: &config.RuleActionConfig{
								Type:    "block",
								Message: "blocked",
							},
						},
					},
				},
			}

			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).NotTo(BeNil())
			Expect(engine.Size()).To(Equal(1))
		})

		It("should create engine with multiple rules", func() {
			enabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:   "rule1",
							Action: &config.RuleActionConfig{Type: "block"},
						},
						{
							Name:   "rule2",
							Action: &config.RuleActionConfig{Type: "warn"},
						},
						{
							Name:   "rule3",
							Action: &config.RuleActionConfig{Type: "allow"},
						},
					},
				},
			}

			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).NotTo(BeNil())
			Expect(engine.Size()).To(Equal(3))
		})

		It("should handle stop_on_first_match option", func() {
			enabled := true
			stop := false
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled:          &enabled,
					StopOnFirstMatch: &stop,
					Rules: []config.RuleConfig{
						{
							Name:   "test-rule",
							Action: &config.RuleActionConfig{Type: "block"},
						},
					},
				},
			}

			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).NotTo(BeNil())
		})

		It("should convert action types correctly", func() {
			enabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:   "block-rule",
							Action: &config.RuleActionConfig{Type: "block"},
						},
						{
							Name:   "warn-rule",
							Action: &config.RuleActionConfig{Type: "warn"},
						},
						{
							Name:   "allow-rule",
							Action: &config.RuleActionConfig{Type: "allow"},
						},
					},
				},
			}

			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			blockRule := engine.GetRule("block-rule")
			Expect(blockRule.Action.Type).To(Equal(rules.ActionBlock))

			warnRule := engine.GetRule("warn-rule")
			Expect(warnRule.Action.Type).To(Equal(rules.ActionWarn))

			allowRule := engine.GetRule("allow-rule")
			Expect(allowRule.Action.Type).To(Equal(rules.ActionAllow))
		})

		It("should default unknown action types to block", func() {
			enabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:   "unknown-action-rule",
							Action: &config.RuleActionConfig{Type: "unknown"},
						},
					},
				},
			}

			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			rule := engine.GetRule("unknown-action-rule")
			Expect(rule.Action.Type).To(Equal(rules.ActionBlock))
		})
	})
})
