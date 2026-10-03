package factory_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/config/factory"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("PolicyValidatorFactory", func() {
	policyFactory := factory.NewPolicyValidatorFactory(logger.NewNoOpLogger())

	It("creates nothing by default", func() {
		Expect(policyFactory.CreateValidators(&config.Config{})).To(BeEmpty())
	})

	It("creates the enabled validators with their events", func() {
		cfg := &config.Config{
			Protection: &config.ProtectionConfig{Enabled: new(true)},
			MCPTrust:   &config.MCPTrustConfig{Enabled: new(true)},
		}

		validators := policyFactory.CreateValidators(cfg)
		Expect(validators).To(HaveLen(2))

		protection, trust := validators[0], validators[1]
		Expect(protection.Validator.Name()).To(Equal("protection"))
		Expect(trust.Validator.Name()).To(Equal("mcp-trust"))

		Expect(
			protection.Predicate(&hook.Context{Event: hook.CanonicalEventBeforeTool}),
		).To(BeTrue())
		Expect(
			protection.Predicate(&hook.Context{Event: hook.CanonicalEventConfigChange}),
		).To(BeTrue())
		Expect(
			protection.Predicate(&hook.Context{Event: hook.CanonicalEventAfterTool}),
		).To(BeFalse())
		Expect(protection.Predicate(&hook.Context{
			Event: hook.CanonicalEventAfterTool, ChangedFiles: []string{"a"},
		})).To(BeTrue())
		Expect(
			protection.Predicate(&hook.Context{Event: hook.CanonicalEventTurnStop}),
		).To(BeFalse())

		Expect(trust.Predicate(&hook.Context{
			Event: hook.CanonicalEventBeforeTool, RawToolName: "mcp__a__b",
		})).To(BeTrue())
		Expect(trust.Predicate(&hook.Context{
			Event: hook.CanonicalEventBeforeTool, RawToolName: "Bash",
		})).To(BeFalse())
	})

	It("honors overrides", func() {
		cfg := &config.Config{
			Protection: &config.ProtectionConfig{Enabled: new(true)},
			MCPTrust:   &config.MCPTrustConfig{Enabled: new(true)},
			Overrides: &config.OverridesConfig{Entries: map[string]*config.OverrideEntry{
				"policy.protection": {Disabled: new(true)},
				"policy.mcp_trust":  {Disabled: new(true)},
			}},
		}

		Expect(policyFactory.CreateValidators(cfg)).To(BeEmpty())
	})

	It("is part of CreateAll", func() {
		cfg := &config.Config{
			Validators: &config.ValidatorsConfig{
				Git:          &config.GitConfig{},
				GitHub:       &config.GitHubConfig{},
				File:         &config.FileConfig{},
				Notification: &config.NotificationConfig{},
				Secrets:      &config.SecretsConfig{},
				Shell:        &config.ShellConfig{},
			},
			Protection: &config.ProtectionConfig{Enabled: new(true)},
		}

		all := factory.NewValidatorFactory(logger.NewNoOpLogger()).CreateAll(cfg)
		names := make([]string, 0, len(all))

		for _, v := range all {
			names = append(names, v.Validator.Name())
		}

		Expect(names).To(ContainElement("protection"))
	})
})
