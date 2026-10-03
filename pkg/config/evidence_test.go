package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("EvidenceConfig", func() {
	It("is disabled by default", func() {
		var nilCfg *config.EvidenceConfig

		Expect(nilCfg.IsEnabled()).To(BeFalse())
		Expect((&config.EvidenceConfig{}).IsEnabled()).To(BeFalse())

		enabled := true
		Expect((&config.EvidenceConfig{Enabled: &enabled}).IsEnabled()).To(BeTrue())
	})

	It("uses check defaults", func() {
		for _, check := range []*config.EvidenceCheckConfig{nil, {}} {
			Expect(check.GetKind()).To(Equal(config.EvidenceKindTest))
			Expect(check.GetTimeout()).To(Equal(config.DefaultEvidenceTimeout))
		}
	})

	It("returns configured check values", func() {
		check := &config.EvidenceCheckConfig{
			Kind:    config.EvidenceKindReview,
			Base:    "origin/main",
			Timeout: config.Duration(time.Minute),
		}

		Expect(check.GetKind()).To(Equal("review"))
		Expect(check.GetTimeout()).To(Equal(time.Minute))
	})
})
