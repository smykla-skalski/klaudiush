package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("MetricsConfig", func() {
	It("records by default", func() {
		var nilCfg *config.MetricsConfig
		Expect(nilCfg.IsEnabled()).To(BeTrue())
		Expect((&config.MetricsConfig{}).IsEnabled()).To(BeTrue())

		disabled := false
		Expect((&config.MetricsConfig{Enabled: &disabled}).IsEnabled()).To(BeFalse())
	})

	It("defaults and bounds retention and file size", func() {
		var nilCfg *config.MetricsConfig
		Expect(nilCfg.GetRetention()).To(Equal(config.DefaultMetricsRetention))
		Expect(nilCfg.GetMaxFileSize()).To(Equal(int64(8 << 20)))

		cfg := &config.MetricsConfig{
			Retention:     config.Duration(time.Hour),
			MaxFileSizeMB: 2,
		}
		Expect(cfg.GetRetention()).To(Equal(time.Hour))
		Expect(cfg.GetMaxFileSize()).To(Equal(int64(2 << 20)))

		Expect(
			(&config.MetricsConfig{MaxFileSizeMB: -1}).GetMaxFileSize(),
		).To(Equal(int64(8 << 20)))
		Expect((&config.MetricsConfig{MaxFileSizeMB: 100000}).GetMaxFileSize()).
			To(Equal(int64(256 << 20)))
	})

	It("is created on demand", func() {
		cfg := &config.Config{}
		Expect(cfg.GetMetrics()).NotTo(BeNil())
		Expect(cfg.Metrics).NotTo(BeNil())
	})
})
