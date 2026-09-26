package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("OutputConfig", func() {
	trueValue := true
	falseValue := false

	It("enables everything for nil config", func() {
		var cfg *config.OutputConfig
		Expect(cfg.IsUserMessagesEnabled()).To(BeTrue())
		Expect(cfg.IsValidationMessagesEnabled()).To(BeTrue())
		Expect(cfg.IsAgentSummaryEnabled()).To(BeTrue())
	})

	It("enables everything when unset", func() {
		cfg := &config.OutputConfig{}
		Expect(cfg.IsUserMessagesEnabled()).To(BeTrue())
		Expect(cfg.IsValidationMessagesEnabled()).To(BeTrue())
		Expect(cfg.IsAgentSummaryEnabled()).To(BeTrue())
	})

	It("hides validation messages when disabled", func() {
		cfg := &config.OutputConfig{ValidationMessages: &falseValue}
		Expect(cfg.IsUserMessagesEnabled()).To(BeTrue())
		Expect(cfg.IsValidationMessagesEnabled()).To(BeFalse())
	})

	It("hides validation messages when the master switch is off", func() {
		cfg := &config.OutputConfig{
			UserMessages:       &falseValue,
			ValidationMessages: &trueValue,
		}
		Expect(cfg.IsUserMessagesEnabled()).To(BeFalse())
		Expect(cfg.IsValidationMessagesEnabled()).To(BeFalse())
	})

	It("keeps the agent summary independent of the master switch", func() {
		cfg := &config.OutputConfig{UserMessages: &falseValue}
		Expect(cfg.IsAgentSummaryEnabled()).To(BeTrue())
	})

	It("disables the agent summary when set to false", func() {
		cfg := &config.OutputConfig{AgentSummary: &falseValue}
		Expect(cfg.IsAgentSummaryEnabled()).To(BeFalse())
	})
})
