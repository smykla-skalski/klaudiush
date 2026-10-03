package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("ProtectionConfig", func() {
	It("is disabled with defaults when unset", func() {
		var cfg *config.ProtectionConfig

		Expect(cfg.IsEnabled()).To(BeFalse())
		Expect(cfg.GetPaths()).To(BeNil())
		Expect(cfg.GetAllow()).To(BeNil())
		Expect(cfg.GetConfigChangeSources()).To(Equal(config.DefaultConfigChangeSources()))
		Expect(cfg.BlocksConfigChange(config.ConfigSourceProjectSettings)).To(BeTrue())
		Expect(cfg.BlocksConfigChange(config.ConfigSourceSkills)).To(BeFalse())
	})

	It("returns configured values", func() {
		enabled := true
		cfg := &config.ProtectionConfig{
			Enabled:             &enabled,
			Paths:               []string{"a"},
			Allow:               []string{"b"},
			ConfigChangeSources: []string{},
		}

		Expect(cfg.IsEnabled()).To(BeTrue())
		Expect(cfg.GetPaths()).To(Equal([]string{"a"}))
		Expect(cfg.GetAllow()).To(Equal([]string{"b"}))
		Expect(cfg.BlocksConfigChange(config.ConfigSourceProjectSettings)).To(BeFalse())
	})

	It("is created on demand", func() {
		root := &config.Config{}
		Expect(root.GetProtection()).To(BeIdenticalTo(root.Protection))
		Expect(root.GetMCPTrust()).To(BeIdenticalTo(root.MCPTrust))
	})
})

var _ = Describe("MCPTrustConfig", func() {
	It("blocks by default", func() {
		var cfg *config.MCPTrustConfig

		Expect(cfg.IsEnabled()).To(BeFalse())
		Expect(cfg.GetUntrusted()).To(Equal(config.MCPTrustActionBlock))
		Expect(cfg.GetUnknownProvenance()).To(Equal(config.MCPTrustActionBlock))
	})

	It("returns configured actions", func() {
		enabled := true
		cfg := &config.MCPTrustConfig{
			Enabled:           &enabled,
			Untrusted:         config.MCPTrustActionWarn,
			UnknownProvenance: config.MCPTrustActionAllow,
		}

		Expect(cfg.IsEnabled()).To(BeTrue())
		Expect(cfg.GetUntrusted()).To(Equal(config.MCPTrustActionWarn))
		Expect(cfg.GetUnknownProvenance()).To(Equal(config.MCPTrustActionAllow))
	})

	It("requires provenance in server entries", func() {
		Expect((&config.MCPTrustedServer{Name: "x"}).HasProvenance()).To(BeFalse())
		Expect((*config.MCPTrustedServer)(nil).HasProvenance()).To(BeFalse())
		Expect((&config.MCPTrustedServer{Source: "user"}).HasProvenance()).To(BeTrue())
		Expect((&config.MCPTrustedServer{URL: "https://x"}).HasProvenance()).To(BeTrue())
		Expect((&config.MCPTrustedServer{Command: "node"}).HasProvenance()).To(BeTrue())
	})
})
