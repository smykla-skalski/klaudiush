package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("FailurePolicyConfig", func() {
	It("uses defaults for nil and empty config", func() {
		for _, cfg := range []*config.FailurePolicyConfig{nil, {}} {
			Expect(cfg.GetMode()).To(BeEmpty())
			Expect(cfg.GetMissingTools()).To(Equal(config.FailureModeIgnore))
			Expect(cfg.GetCritical()).To(BeEmpty())
			Expect(cfg.GetDeadline()).To(Equal(config.DefaultFailureDeadline))
		}
	})

	It("returns configured values", func() {
		cfg := &config.FailurePolicyConfig{
			Mode:         config.FailureModeBlock,
			MissingTools: config.FailureModeWarn,
			Critical:     []string{"git.commit"},
			Deadline:     config.Duration(7 * time.Second),
		}

		Expect(cfg.GetMode()).To(Equal("block"))
		Expect(cfg.GetMissingTools()).To(Equal("warn"))
		Expect(cfg.GetCritical()).To(ConsistOf("git.commit"))
		Expect(cfg.GetDeadline()).To(Equal(7 * time.Second))
	})

	It("creates the section on demand", func() {
		cfg := &config.Config{}
		Expect(cfg.GetFailurePolicy()).NotTo(BeNil())
		Expect(cfg.FailurePolicy).NotTo(BeNil())
	})
})
