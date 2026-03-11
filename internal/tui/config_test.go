package tui

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("buildConfigFromResult", func() {
	It("does not enable Codex provider config by default", func() {
		cfg := buildConfigFromResult(&InitFormResult{
			BellEnabled: true,
		})

		Expect(cfg.Providers).To(BeNil())
	})

	It("enables experimental Codex hooks when hooks path is provided", func() {
		cfg := buildConfigFromResult(&InitFormResult{
			BellEnabled:    true,
			CodexHooksPath: "/tmp/hooks.json",
		})

		Expect(cfg.Providers).NotTo(BeNil())
		Expect(cfg.GetProviders().GetCodex().IsEnabled()).To(BeTrue())
		Expect(cfg.GetProviders().GetCodex().IsExperimentalEnabled()).To(BeTrue())
		Expect(cfg.GetProviders().GetCodex().HooksConfigPath).To(Equal("/tmp/hooks.json"))
	})
})
