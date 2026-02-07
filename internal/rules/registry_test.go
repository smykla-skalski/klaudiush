package rules_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/rules"
)

var _ = Describe("Registry", func() {
	var registry *rules.Registry

	BeforeEach(func() {
		registry = rules.NewRegistry()
	})

	Describe("Add", func() {
		It("should add a valid rule", func() {
			rule := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "blocked",
				},
			}

			err := registry.Add(rule)
			Expect(err).NotTo(HaveOccurred())
			Expect(registry.Size()).To(Equal(1))
		})

		It("should return error for nil rule", func() {
			err := registry.Add(nil)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for rule without name", func() {
			rule := &rules.Rule{
				Enabled: true,
				Action: &rules.RuleAction{
					Type: rules.ActionBlock,
				},
			}

			err := registry.Add(rule)
			Expect(err).To(HaveOccurred())
		})

		It("should return error for rule without action", func() {
			rule := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
			}

			err := registry.Add(rule)
			Expect(err).To(HaveOccurred())
		})

		It("should update existing rule with same name", func() {
			rule1 := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "first",
				},
			}

			rule2 := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:    rules.ActionWarn,
					Message: "second",
				},
			}

			_ = registry.Add(rule1)
			_ = registry.Add(rule2)

			Expect(registry.Size()).To(Equal(1))
			Expect(registry.Get("test-rule").Rule.Action.Type).To(Equal(rules.ActionWarn))
		})

		It("should return error for invalid match pattern", func() {
			rule := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Match: &rules.RuleMatch{
					RepoPattern: "[invalid",
				},
				Action: &rules.RuleAction{
					Type: rules.ActionBlock,
				},
			}

			err := registry.Add(rule)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("AddAll", func() {
		It("should add multiple rules", func() {
			rules := []*rules.Rule{
				{
					Name:    "rule1",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionBlock},
				},
				{
					Name:    "rule2",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionWarn},
				},
			}

			err := registry.AddAll(rules)
			Expect(err).NotTo(HaveOccurred())
			Expect(registry.Size()).To(Equal(2))
		})

		It("should stop on first error", func() {
			rules := []*rules.Rule{
				{
					Name:    "rule1",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionBlock},
				},
				{
					Name:    "", // Invalid
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionWarn},
				},
			}

			err := registry.AddAll(rules)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Remove", func() {
		It("should remove existing rule", func() {
			rule := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			}

			_ = registry.Add(rule)
			Expect(registry.Size()).To(Equal(1))

			removed := registry.Remove("test-rule")
			Expect(removed).To(BeTrue())
			Expect(registry.Size()).To(Equal(0))
		})

		It("should return false for non-existent rule", func() {
			removed := registry.Remove("non-existent")
			Expect(removed).To(BeFalse())
		})
	})

	Describe("Get", func() {
		It("should return existing rule", func() {
			rule := &rules.Rule{
				Name:    "test-rule",
				Enabled: true,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			}

			_ = registry.Add(rule)

			compiled := registry.Get("test-rule")
			Expect(compiled).NotTo(BeNil())
			Expect(compiled.Rule.Name).To(Equal("test-rule"))
		})

		It("should return nil for non-existent rule", func() {
			compiled := registry.Get("non-existent")
			Expect(compiled).To(BeNil())
		})
	})

	Describe("GetAll", func() {
		It("should return all rules sorted by priority", func() {
			_ = registry.Add(&rules.Rule{
				Name:     "low-priority",
				Priority: 10,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:     "high-priority",
				Priority: 100,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:     "medium-priority",
				Priority: 50,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionBlock},
			})

			all := registry.GetAll()
			Expect(len(all)).To(Equal(3))
			Expect(all[0].Rule.Name).To(Equal("high-priority"))
			Expect(all[1].Rule.Name).To(Equal("medium-priority"))
			Expect(all[2].Rule.Name).To(Equal("low-priority"))
		})

		It("should sort by name when priority is equal", func() {
			_ = registry.Add(&rules.Rule{
				Name:     "zebra",
				Priority: 50,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:     "alpha",
				Priority: 50,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionBlock},
			})

			all := registry.GetAll()
			Expect(len(all)).To(Equal(2))
			Expect(all[0].Rule.Name).To(Equal("alpha"))
			Expect(all[1].Rule.Name).To(Equal("zebra"))
		})
	})

	Describe("GetEnabled", func() {
		It("should return only enabled rules", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "enabled",
				Enabled: true,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "disabled",
				Enabled: false,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			})

			enabled := registry.GetEnabled()
			Expect(len(enabled)).To(Equal(1))
			Expect(enabled[0].Rule.Name).To(Equal("enabled"))
		})
	})

	Describe("Clear", func() {
		It("should remove all rules", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "rule1",
				Enabled: true,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			})
			_ = registry.Add(&rules.Rule{
				Name:    "rule2",
				Enabled: true,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			})

			Expect(registry.Size()).To(Equal(2))

			registry.Clear()
			Expect(registry.Size()).To(Equal(0))
		})
	})

	Describe("Merge", func() {
		It("should merge rules from source registry", func() {
			_ = registry.Add(&rules.Rule{
				Name:     "base-rule",
				Priority: 100,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionBlock},
			})

			source := rules.NewRegistry()
			_ = source.Add(&rules.Rule{
				Name:     "source-rule",
				Priority: 50,
				Enabled:  true,
				Action:   &rules.RuleAction{Type: rules.ActionWarn},
			})

			registry.Merge(source)

			Expect(registry.Size()).To(Equal(2))
			Expect(registry.Get("base-rule")).NotTo(BeNil())
			Expect(registry.Get("source-rule")).NotTo(BeNil())
		})

		It("should override existing rules with same name", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "shared-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:    rules.ActionBlock,
					Message: "base",
				},
			})

			source := rules.NewRegistry()
			_ = source.Add(&rules.Rule{
				Name:    "shared-rule",
				Enabled: true,
				Action: &rules.RuleAction{
					Type:    rules.ActionWarn,
					Message: "source",
				},
			})

			registry.Merge(source)

			Expect(registry.Size()).To(Equal(1))
			Expect(registry.Get("shared-rule").Rule.Action.Message).To(Equal("source"))
		})

		It("should handle nil source", func() {
			_ = registry.Add(&rules.Rule{
				Name:    "rule",
				Enabled: true,
				Action:  &rules.RuleAction{Type: rules.ActionBlock},
			})

			registry.Merge(nil)
			Expect(registry.Size()).To(Equal(1))
		})
	})
})

var _ = Describe("MergeRules", func() {
	It("should merge two rule slices", func() {
		base := []*rules.Rule{
			{
				Name:     "base1",
				Priority: 100,
				Enabled:  true,
			},
			{
				Name:     "shared",
				Priority: 50,
				Action:   &rules.RuleAction{Message: "base"},
			},
		}

		override := []*rules.Rule{
			{
				Name:     "override1",
				Priority: 75,
				Enabled:  true,
			},
			{
				Name:     "shared",
				Priority: 50,
				Action:   &rules.RuleAction{Message: "override"},
			},
		}

		merged := rules.MergeRules(base, override)

		Expect(len(merged)).To(Equal(3))

		// Should be sorted by priority.
		Expect(merged[0].Name).To(Equal("base1"))
		Expect(merged[1].Name).To(Equal("override1"))

		// Shared rule should be overridden.
		var sharedRule *rules.Rule
		for _, r := range merged {
			if r.Name == "shared" {
				sharedRule = r
				break
			}
		}

		Expect(sharedRule).NotTo(BeNil())
		Expect(sharedRule.Action.Message).To(Equal("override"))
	})
})
