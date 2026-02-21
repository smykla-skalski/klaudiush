package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("OverrideEntry", func() {
	Describe("IsExpired", func() {
		It("returns false when ExpiresAt is empty", func() {
			entry := &config.OverrideEntry{ExpiresAt: ""}
			Expect(entry.IsExpired()).To(BeFalse())
		})

		It("returns false when ExpiresAt is in the future", func() {
			future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
			entry := &config.OverrideEntry{ExpiresAt: future}
			Expect(entry.IsExpired()).To(BeFalse())
		})

		It("returns true when ExpiresAt is in the past", func() {
			past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
			entry := &config.OverrideEntry{ExpiresAt: past}
			Expect(entry.IsExpired()).To(BeTrue())
		})

		It("returns false when ExpiresAt is invalid format", func() {
			entry := &config.OverrideEntry{ExpiresAt: "not-a-timestamp"}
			Expect(entry.IsExpired()).To(BeFalse())
		})

		It("returns false for nil entry", func() {
			var entry *config.OverrideEntry
			Expect(entry.IsExpired()).To(BeFalse())
		})
	})

	Describe("IsActive", func() {
		It("returns true when not expired", func() {
			future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
			entry := &config.OverrideEntry{ExpiresAt: future}
			Expect(entry.IsActive()).To(BeTrue())
		})

		It("returns false when expired", func() {
			past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
			entry := &config.OverrideEntry{ExpiresAt: past}
			Expect(entry.IsActive()).To(BeFalse())
		})

		It("returns false for nil entry", func() {
			var entry *config.OverrideEntry
			Expect(entry.IsActive()).To(BeFalse())
		})
	})
})

var _ = Describe("OverridesConfig", func() {
	Describe("IsDisabled", func() {
		It("returns false on nil config", func() {
			var cfg *config.OverridesConfig
			Expect(cfg.IsDisabled("GIT001")).To(BeFalse())
		})

		It("returns false on empty entries", func() {
			cfg := &config.OverridesConfig{}
			Expect(cfg.IsDisabled("GIT001")).To(BeFalse())
		})

		It("returns true for disabled entry", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {Disabled: new(true)},
				},
			}
			Expect(cfg.IsDisabled("GIT001")).To(BeTrue())
		})

		It("returns false for expired entry", func() {
			past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {
						Disabled:  new(true),
						ExpiresAt: past,
					},
				},
			}
			Expect(cfg.IsDisabled("GIT001")).To(BeFalse())
		})

		It("returns false for non-existent key", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {Disabled: new(true)},
				},
			}
			Expect(cfg.IsDisabled("GIT999")).To(BeFalse())
		})

		It("returns false for enable override", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {Disabled: new(false)},
				},
			}
			Expect(cfg.IsDisabled("GIT001")).To(BeFalse())
		})
	})

	Describe("IsExplicitlyEnabled", func() {
		It("returns true for enable override", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {Disabled: new(false)},
				},
			}
			Expect(cfg.IsExplicitlyEnabled("GIT001")).To(BeTrue())
		})

		It("returns false for disable override", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {Disabled: new(true)},
				},
			}
			Expect(cfg.IsExplicitlyEnabled("GIT001")).To(BeFalse())
		})

		It("returns false on nil config", func() {
			var cfg *config.OverridesConfig
			Expect(cfg.IsExplicitlyEnabled("GIT001")).To(BeFalse())
		})
	})

	Describe("IsCodeDisabled", func() {
		It("returns true when disabled by exact code match", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {Disabled: new(true)},
				},
			}
			Expect(cfg.IsCodeDisabled("GIT001")).To(BeTrue())
		})

		It("returns true when disabled by parent validator match", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"git.commit": {Disabled: new(true)},
				},
			}
			// GIT001 maps to git.commit in CodeToValidator
			Expect(cfg.IsCodeDisabled("GIT001")).To(BeTrue())
		})

		It("returns false when no match", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"git.push": {Disabled: new(true)},
				},
			}
			// GIT001 maps to git.commit, not git.push
			Expect(cfg.IsCodeDisabled("GIT001")).To(BeFalse())
		})

		It("returns false when code is explicitly enabled overriding parent disable", func() {
			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"git.commit": {Disabled: new(true)},
					"GIT001":     {Disabled: new(false)},
				},
			}
			Expect(cfg.IsCodeDisabled("GIT001")).To(BeFalse())
		})

		It("returns false on nil config", func() {
			var cfg *config.OverridesConfig
			Expect(cfg.IsCodeDisabled("GIT001")).To(BeFalse())
		})
	})

	Describe("ActiveEntries", func() {
		It("returns only non-expired entries", func() {
			future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
			past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {
						Disabled:  new(true),
						ExpiresAt: future,
					},
					"GIT002": {
						Disabled:  new(true),
						ExpiresAt: past,
					},
					"GIT003": {
						Disabled: new(true),
					},
				},
			}

			active := cfg.ActiveEntries()
			Expect(active).To(HaveLen(2))
			Expect(active).To(HaveKey("GIT001"))
			Expect(active).To(HaveKey("GIT003"))
			Expect(active).NotTo(HaveKey("GIT002"))
		})

		It("returns nil for empty config", func() {
			cfg := &config.OverridesConfig{}
			Expect(cfg.ActiveEntries()).To(BeNil())
		})
	})

	Describe("ExpiredEntries", func() {
		It("returns only expired entries", func() {
			future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
			past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

			cfg := &config.OverridesConfig{
				Entries: map[string]*config.OverrideEntry{
					"GIT001": {
						Disabled:  new(true),
						ExpiresAt: future,
					},
					"GIT002": {
						Disabled:  new(true),
						ExpiresAt: past,
					},
					"GIT003": {
						Disabled: new(true),
					},
				},
			}

			expired := cfg.ExpiredEntries()
			Expect(expired).To(HaveLen(1))
			Expect(expired).To(HaveKey("GIT002"))
			Expect(expired).NotTo(HaveKey("GIT001"))
			Expect(expired).NotTo(HaveKey("GIT003"))
		})

		It("returns nil for empty config", func() {
			cfg := &config.OverridesConfig{}
			Expect(cfg.ExpiredEntries()).To(BeNil())
		})
	})
})

var _ = Describe("IsKnownTarget", func() {
	It("returns true for known error code", func() {
		Expect(config.IsKnownTarget("GIT001")).To(BeTrue())
	})

	It("returns true for known validator name", func() {
		Expect(config.IsKnownTarget("git.commit")).To(BeTrue())
	})

	It("returns false for unknown target", func() {
		Expect(config.IsKnownTarget("UNKNOWN999")).To(BeFalse())
	})
})
