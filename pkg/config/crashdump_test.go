package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("CrashDumpConfig", func() {
	Describe("GetMaxAge", func() {
		It("returns the default when max age is unset", func() {
			cfg := &config.CrashDumpConfig{}

			Expect(cfg.GetMaxAge()).To(Equal(
				config.Duration(config.DefaultMaxAgeDays * 24 * time.Hour),
			))
		})

		It("returns the configured duration when max_age is set", func() {
			cfg := &config.CrashDumpConfig{
				MaxAge: config.Duration(48 * time.Hour),
			}

			Expect(cfg.GetMaxAge()).To(Equal(config.Duration(48 * time.Hour)))
		})

		It("uses the legacy max_age_days field when present", func() {
			days := 7
			cfg := &config.CrashDumpConfig{
				MaxAgeDays: &days,
			}

			Expect(cfg.GetMaxAge()).To(Equal(config.Duration(7 * 24 * time.Hour)))
		})

		It("allows legacy max_age_days zero to disable age pruning", func() {
			days := 0
			cfg := &config.CrashDumpConfig{
				MaxAgeDays: &days,
			}

			Expect(cfg.GetMaxAge()).To(BeZero())
		})
	})
})
