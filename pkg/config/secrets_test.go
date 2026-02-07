package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("SecretsValidatorConfig", func() {
	Describe("IsUseGitleaksEnabled", func() {
		It("returns false for nil config", func() {
			var cfg *config.SecretsValidatorConfig
			Expect(cfg.IsUseGitleaksEnabled()).To(BeFalse())
		})

		It("returns false when UseGitleaks is nil", func() {
			cfg := &config.SecretsValidatorConfig{}
			Expect(cfg.IsUseGitleaksEnabled()).To(BeFalse())
		})

		It("returns false when explicitly disabled", func() {
			useGitleaks := false
			cfg := &config.SecretsValidatorConfig{
				UseGitleaks: &useGitleaks,
			}
			Expect(cfg.IsUseGitleaksEnabled()).To(BeFalse())
		})

		It("returns true when explicitly enabled", func() {
			useGitleaks := true
			cfg := &config.SecretsValidatorConfig{
				UseGitleaks: &useGitleaks,
			}
			Expect(cfg.IsUseGitleaksEnabled()).To(BeTrue())
		})
	})

	Describe("IsBlockOnDetectionEnabled", func() {
		It("returns true for nil config (default to blocking)", func() {
			var cfg *config.SecretsValidatorConfig
			Expect(cfg.IsBlockOnDetectionEnabled()).To(BeTrue())
		})

		It("returns true when BlockOnDetection is nil (default)", func() {
			cfg := &config.SecretsValidatorConfig{}
			Expect(cfg.IsBlockOnDetectionEnabled()).To(BeTrue())
		})

		It("returns false when explicitly disabled", func() {
			blockOnDetection := false
			cfg := &config.SecretsValidatorConfig{
				BlockOnDetection: &blockOnDetection,
			}
			Expect(cfg.IsBlockOnDetectionEnabled()).To(BeFalse())
		})

		It("returns true when explicitly enabled", func() {
			blockOnDetection := true
			cfg := &config.SecretsValidatorConfig{
				BlockOnDetection: &blockOnDetection,
			}
			Expect(cfg.IsBlockOnDetectionEnabled()).To(BeTrue())
		})
	})

	Describe("GetMaxFileSize", func() {
		It("returns default for nil config", func() {
			var cfg *config.SecretsValidatorConfig
			Expect(cfg.GetMaxFileSize()).To(Equal(config.DefaultMaxFileSize))
		})

		It("returns default when MaxFileSize is 0", func() {
			cfg := &config.SecretsValidatorConfig{}
			Expect(cfg.GetMaxFileSize()).To(Equal(config.DefaultMaxFileSize))
		})

		It("returns configured value when set", func() {
			cfg := &config.SecretsValidatorConfig{
				MaxFileSize: 5 * config.MB,
			}
			Expect(cfg.GetMaxFileSize()).To(Equal(5 * config.MB))
		})
	})
})
