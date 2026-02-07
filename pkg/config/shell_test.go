package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("BacktickValidatorConfig", func() {
	Describe("CheckUnquotedOrDefault", func() {
		It("returns true for nil config (default)", func() {
			var cfg *config.BacktickValidatorConfig
			Expect(cfg.CheckUnquotedOrDefault()).To(BeTrue())
		})

		It("returns true when CheckUnquoted is nil (default)", func() {
			cfg := &config.BacktickValidatorConfig{}
			Expect(cfg.CheckUnquotedOrDefault()).To(BeTrue())
		})

		It("returns false when explicitly disabled", func() {
			checkUnquoted := false
			cfg := &config.BacktickValidatorConfig{
				CheckUnquoted: &checkUnquoted,
			}
			Expect(cfg.CheckUnquotedOrDefault()).To(BeFalse())
		})

		It("returns true when explicitly enabled", func() {
			checkUnquoted := true
			cfg := &config.BacktickValidatorConfig{
				CheckUnquoted: &checkUnquoted,
			}
			Expect(cfg.CheckUnquotedOrDefault()).To(BeTrue())
		})
	})

	Describe("SuggestSingleQuotesOrDefault", func() {
		It("returns true for nil config (default)", func() {
			var cfg *config.BacktickValidatorConfig
			Expect(cfg.SuggestSingleQuotesOrDefault()).To(BeTrue())
		})

		It("returns true when SuggestSingleQuotes is nil (default)", func() {
			cfg := &config.BacktickValidatorConfig{}
			Expect(cfg.SuggestSingleQuotesOrDefault()).To(BeTrue())
		})

		It("returns false when explicitly disabled", func() {
			suggestSingleQuotes := false
			cfg := &config.BacktickValidatorConfig{
				SuggestSingleQuotes: &suggestSingleQuotes,
			}
			Expect(cfg.SuggestSingleQuotesOrDefault()).To(BeFalse())
		})

		It("returns true when explicitly enabled", func() {
			suggestSingleQuotes := true
			cfg := &config.BacktickValidatorConfig{
				SuggestSingleQuotes: &suggestSingleQuotes,
			}
			Expect(cfg.SuggestSingleQuotesOrDefault()).To(BeTrue())
		})
	})
})
