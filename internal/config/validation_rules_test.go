package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// Note: Tests run as part of existing Koanf Rules Suite via TestKoanfRules

var _ = Describe("Validator", func() {
	var validator *Validator

	BeforeEach(func() {
		validator = NewValidator()
	})

	Describe("validateRulesConfig", func() {
		Context("when rules config is nil or empty", func() {
			It("should return nil for nil config", func() {
				err := validator.validateRulesConfig(nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return nil for empty rules slice", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{},
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when rules have valid configuration", func() {
			It("should pass for rule with validator_type", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "valid-rule",
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should pass for rule with tool_type", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "valid-tool-rule",
							Match: &config.RuleMatchConfig{
								ToolType: "Bash",
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should pass for rule with event_type", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "valid-event-rule",
							Match: &config.RuleMatchConfig{
								EventType: "PreToolUse",
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should pass for rule with file_pattern", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "file-pattern-rule",
							Match: &config.RuleMatchConfig{
								FilePattern: "*.go",
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should pass for rule with valid action type", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "action-rule",
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
							Action: &config.RuleActionConfig{
								Type: "allow",
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})

			It("should accept all valid action types", func() {
				for _, actionType := range []string{"allow", "block", "warn"} {
					err := validator.validateRulesConfig(&config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "action-rule",
								Match: &config.RuleMatchConfig{
									ValidatorType: "git.push",
								},
								Action: &config.RuleActionConfig{
									Type: actionType,
								},
							},
						},
					})
					Expect(err).NotTo(HaveOccurred(), "action type %s should be valid", actionType)
				}
			})

			It("should accept all valid tool types (case-insensitive)", func() {
				toolTypes := []string{
					"Bash", "bash", "BASH",
					"Write", "write",
					"Edit", "edit",
					"MultiEdit",
					"Grep", "grep",
					"Read", "read",
					"Glob", "glob",
				}

				for _, toolType := range toolTypes {
					err := validator.validateRulesConfig(&config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "tool-rule",
								Match: &config.RuleMatchConfig{
									ToolType: toolType,
								},
							},
						},
					})
					Expect(err).NotTo(HaveOccurred(), "tool type %s should be valid", toolType)
				}
			})

			It("should accept all valid event types (case-insensitive)", func() {
				eventTypes := []string{
					"PreToolUse", "pretooluse", "PRETOOLUSE",
					"PostToolUse", "posttooluse",
					"Notification", "notification",
				}

				for _, eventType := range eventTypes {
					err := validator.validateRulesConfig(&config.RulesConfig{
						Rules: []config.RuleConfig{
							{
								Name: "event-rule",
								Match: &config.RuleMatchConfig{
									EventType: eventType,
								},
							},
						},
					})
					Expect(err).NotTo(HaveOccurred(), "event type %s should be valid", eventType)
				}
			})
		})

		Context("when rules have invalid configuration", func() {
			It("should fail when rule has no match section", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name:  "no-match-rule",
							Match: nil,
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no match section"))
			})

			It("should fail when match section is empty", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name:  "empty-match-rule",
							Match: &config.RuleMatchConfig{},
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("empty match section"))
			})

			It("should fail when tool_type is invalid", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "invalid-tool-rule",
							Match: &config.RuleMatchConfig{
								ToolType: "InvalidTool",
							},
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid tool_type"))
				Expect(err.Error()).To(ContainSubstring("InvalidTool"))
			})

			It("should fail when event_type is invalid", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "invalid-event-rule",
							Match: &config.RuleMatchConfig{
								EventType: "InvalidEvent",
							},
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid event_type"))
				Expect(err.Error()).To(ContainSubstring("InvalidEvent"))
			})

			It("should fail when action type is invalid", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "invalid-action-rule",
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
							Action: &config.RuleActionConfig{
								Type: "invalid-action",
							},
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid action type"))
				Expect(err.Error()).To(ContainSubstring("invalid-action"))
			})

			It("should report multiple errors", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{Name: "rule1", Match: nil},
						{Name: "rule2", Match: nil},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("rule1"))
				Expect(err.Error()).To(ContainSubstring("rule2"))
			})

			It("should use rule name in error when available", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{Name: "my-named-rule", Match: nil},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(`"my-named-rule"`))
			})

			It("should use rule index in error when name is empty", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{Name: "", Match: nil},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("rule[0]"))
			})
		})

		Context("when rule has nil action", func() {
			It("should pass (defaults to block)", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "no-action-rule",
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
							Action: nil,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when rule has empty action type", func() {
			It("should pass (defaults to block)", func() {
				err := validator.validateRulesConfig(&config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name: "empty-action-type-rule",
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
							Action: &config.RuleActionConfig{
								Type: "",
							},
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
