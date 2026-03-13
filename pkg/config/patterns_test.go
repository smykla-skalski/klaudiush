package config_test

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("PatternsConfig", func() {
	Describe("IsEnabled", func() {
		It("returns true by default", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.PatternsConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			cfg := &config.PatternsConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.IsEnabled()).To(BeTrue())
		})
	})

	Describe("GetMinCount", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetMinCount()).To(Equal(config.DefaultPatternsMinCount))
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{MinCount: 10}
			Expect(cfg.GetMinCount()).To(Equal(10))
		})

		It("returns default for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.GetMinCount()).To(Equal(config.DefaultPatternsMinCount))
		})
	})

	Describe("GetMaxAge", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetMaxAge()).To(Equal(config.DefaultPatternsMaxAge))
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{
				MaxAge: config.Duration(48 * time.Hour),
			}
			Expect(cfg.GetMaxAge()).To(Equal(48 * time.Hour))
		})

		It("returns default for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.GetMaxAge()).To(Equal(config.DefaultPatternsMaxAge))
		})
	})

	Describe("GetMaxWarningsPerError", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetMaxWarningsPerError()).To(
				Equal(config.DefaultPatternsMaxWarningsPerError),
			)
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{MaxWarningsPerError: 5}
			Expect(cfg.GetMaxWarningsPerError()).To(Equal(5))
		})

		It("returns default for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.GetMaxWarningsPerError()).To(
				Equal(config.DefaultPatternsMaxWarningsPerError),
			)
		})
	})

	Describe("GetMaxWarningsTotal", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetMaxWarningsTotal()).To(
				Equal(config.DefaultPatternsMaxWarningsTotal),
			)
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{MaxWarningsTotal: 10}
			Expect(cfg.GetMaxWarningsTotal()).To(Equal(10))
		})

		It("returns default for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.GetMaxWarningsTotal()).To(
				Equal(config.DefaultPatternsMaxWarningsTotal),
			)
		})
	})

	Describe("GetProjectDataFile", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetProjectDataFile()).To(
				Equal(config.DefaultPatternsProjectDataFile),
			)
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{
				ProjectDataFile: "custom/patterns.json",
			}
			Expect(cfg.GetProjectDataFile()).To(
				Equal("custom/patterns.json"),
			)
		})

		It("returns default for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.GetProjectDataFile()).To(
				Equal(config.DefaultPatternsProjectDataFile),
			)
		})
	})

	Describe("GetGlobalDataDir", func() {
		It("returns default for empty config", func() {
			GinkgoT().Setenv("XDG_DATA_HOME", filepath.Join(GinkgoT().TempDir(), "xdg-data"))

			cfg := &config.PatternsConfig{}
			Expect(cfg.GetGlobalDataDir()).To(Equal(
				filepath.Join(os.Getenv("XDG_DATA_HOME"), "klaudiush", "patterns"),
			))
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{
				GlobalDataDir: "/custom/patterns",
			}
			Expect(cfg.GetGlobalDataDir()).To(Equal("/custom/patterns"))
		})

		It("returns default for nil config", func() {
			GinkgoT().Setenv("XDG_DATA_HOME", filepath.Join(GinkgoT().TempDir(), "xdg-data"))

			var cfg *config.PatternsConfig
			Expect(cfg.GetGlobalDataDir()).To(Equal(
				filepath.Join(os.Getenv("XDG_DATA_HOME"), "klaudiush", "patterns"),
			))
		})
	})

	Describe("IsUseSeedData", func() {
		It("returns true by default", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.IsUseSeedData()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.PatternsConfig{UseSeedData: &enabled}
			Expect(cfg.IsUseSeedData()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			disabled := false
			cfg := &config.PatternsConfig{UseSeedData: &disabled}
			Expect(cfg.IsUseSeedData()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.PatternsConfig
			Expect(cfg.IsUseSeedData()).To(BeTrue())
		})
	})

	Describe("GetMaxPatterns", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetMaxPatterns()).To(Equal(config.DefaultPatternsMaxPatterns))
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{MaxPatterns: 42}
			Expect(cfg.GetMaxPatterns()).To(Equal(42))
		})
	})

	Describe("GetMaxSessions", func() {
		It("returns default for empty config", func() {
			cfg := &config.PatternsConfig{}
			Expect(cfg.GetMaxSessions()).To(Equal(config.DefaultPatternsMaxSessions))
		})

		It("returns custom value when set", func() {
			cfg := &config.PatternsConfig{MaxSessions: 84}
			Expect(cfg.GetMaxSessions()).To(Equal(84))
		})
	})
})

var _ = Describe("Config.GetPatterns", func() {
	It("creates patterns config if nil", func() {
		cfg := &config.Config{}
		patterns := cfg.GetPatterns()

		Expect(patterns).NotTo(BeNil())
		Expect(cfg.Patterns).NotTo(BeNil())
	})

	It("returns existing patterns config", func() {
		enabled := true
		patterns := &config.PatternsConfig{Enabled: &enabled}
		cfg := &config.Config{Patterns: patterns}

		result := cfg.GetPatterns()

		Expect(result).To(Equal(patterns))
		Expect(result.IsEnabled()).To(BeTrue())
	})
})
