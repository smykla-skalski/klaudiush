package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Provider validation", func() {
	var validator *Validator

	BeforeEach(func() {
		validator = NewValidator()
	})

	It("accepts Codex hooks config when experimental mode is enabled", func() {
		enabled := true
		experimental := true
		err := validator.validateProvidersConfig(&config.ProvidersConfig{
			Codex: &config.CodexProviderConfig{
				Enabled:         &enabled,
				Experimental:    &experimental,
				HooksConfigPath: "/tmp/hooks.json",
			},
		})

		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects Codex hooks config without experimental mode", func() {
		enabled := true
		err := validator.validateProvidersConfig(&config.ProvidersConfig{
			Codex: &config.CodexProviderConfig{
				Enabled:         &enabled,
				HooksConfigPath: "/tmp/hooks.json",
			},
		})

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("providers.codex.experimental"))
	})
})
