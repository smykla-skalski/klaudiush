package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("policy section validation", func() {
	It("accepts valid protection and MCP trust", func() {
		Expect(NewValidator().Validate(&config.Config{
			Protection: &config.ProtectionConfig{Paths: []string{"x/**"}},
			MCPTrust: &config.MCPTrustConfig{
				Untrusted:         config.MCPTrustActionWarn,
				UnknownProvenance: config.MCPTrustActionAllow,
				TrustedSources:    []string{"managed"},
				Servers: []*config.MCPTrustedServer{
					{Name: "a*", Source: "user", Tools: []string{"q"}},
				},
			},
		})).To(Succeed())
	})

	It("reports invalid protection", func() {
		cfg := &config.Config{
			Protection: &config.ProtectionConfig{ConfigChangeSources: []string{"nope"}},
		}
		Expect(NewValidator().Validate(cfg)).To(MatchError(ErrInvalidConfig))

		errs := validatePolicySections(cfg)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0]).To(MatchError(ContainSubstring("protection")))
		Expect(errs[0]).To(MatchError(ContainSubstring("nope")))
	})

	DescribeTable(
		"reports invalid MCP trust",
		func(cfg *config.MCPTrustConfig, contains string) {
			root := &config.Config{MCPTrust: cfg}
			Expect(NewValidator().Validate(root)).To(MatchError(ErrInvalidConfig))

			errs := validatePolicySections(root)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0]).To(MatchError(ContainSubstring(contains)))
		},
		Entry("untrusted", &config.MCPTrustConfig{Untrusted: "allow"}, "untrusted must be"),
		Entry(
			"unknown",
			&config.MCPTrustConfig{UnknownProvenance: "maybe"},
			"unknown_provenance must be",
		),
		Entry(
			"empty source",
			&config.MCPTrustConfig{TrustedSources: []string{""}},
			"trusted_sources",
		),
		Entry("name only", &config.MCPTrustConfig{
			Servers: []*config.MCPTrustedServer{{Name: "github"}},
		}, "must set source, command or url"),
		Entry("bad pattern", &config.MCPTrustConfig{
			Servers: []*config.MCPTrustedServer{{Source: "user", Tools: []string{"["}}},
		}, "invalid pattern"),
	)
})
