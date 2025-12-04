package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("SessionConfig", func() {
	Describe("IsEnabled", func() {
		It("returns true by default", func() {
			cfg := &config.SessionConfig{}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.SessionConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			cfg := &config.SessionConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.SessionConfig
			Expect(cfg.IsEnabled()).To(BeTrue())
		})
	})

	Describe("GetStateFile", func() {
		It("returns default for empty config", func() {
			cfg := &config.SessionConfig{}
			Expect(cfg.GetStateFile()).To(Equal(config.DefaultSessionStateFile))
		})

		It("returns custom state file when set", func() {
			cfg := &config.SessionConfig{StateFile: "/custom/state.json"}
			Expect(cfg.GetStateFile()).To(Equal("/custom/state.json"))
		})

		It("returns default for nil config", func() {
			var cfg *config.SessionConfig
			Expect(cfg.GetStateFile()).To(Equal(config.DefaultSessionStateFile))
		})
	})

	Describe("GetMaxSessionAge", func() {
		It("returns default for empty config", func() {
			cfg := &config.SessionConfig{}
			Expect(cfg.GetMaxSessionAge()).To(Equal(config.DefaultMaxSessionAge))
		})

		It("returns custom max session age when set", func() {
			cfg := &config.SessionConfig{
				MaxSessionAge: config.Duration(12 * time.Hour),
			}
			Expect(cfg.GetMaxSessionAge()).To(Equal(12 * time.Hour))
		})

		It("returns default for nil config", func() {
			var cfg *config.SessionConfig
			Expect(cfg.GetMaxSessionAge()).To(Equal(config.DefaultMaxSessionAge))
		})
	})

	Describe("GetAudit", func() {
		It("creates audit config if nil", func() {
			cfg := &config.SessionConfig{}
			audit := cfg.GetAudit()

			Expect(audit).NotTo(BeNil())
		})

		It("returns existing audit config", func() {
			enabled := true
			audit := &config.SessionAuditConfig{Enabled: &enabled}
			cfg := &config.SessionConfig{Audit: audit}

			result := cfg.GetAudit()

			Expect(result).To(Equal(audit))
			Expect(result.IsAuditEnabled()).To(BeTrue())
		})

		It("returns default audit config for nil session config", func() {
			var cfg *config.SessionConfig
			audit := cfg.GetAudit()

			Expect(audit).NotTo(BeNil())
		})
	})
})

var _ = Describe("SessionAuditConfig", func() {
	Describe("IsAuditEnabled", func() {
		It("returns true by default", func() {
			cfg := &config.SessionAuditConfig{}
			Expect(cfg.IsAuditEnabled()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.SessionAuditConfig{Enabled: &enabled}
			Expect(cfg.IsAuditEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			cfg := &config.SessionAuditConfig{Enabled: &enabled}
			Expect(cfg.IsAuditEnabled()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.SessionAuditConfig
			Expect(cfg.IsAuditEnabled()).To(BeTrue())
		})
	})

	Describe("GetLogFile", func() {
		It("returns default for empty config", func() {
			cfg := &config.SessionAuditConfig{}
			Expect(cfg.GetLogFile()).To(Equal(config.DefaultSessionAuditLogFile))
		})

		It("returns custom log file when set", func() {
			cfg := &config.SessionAuditConfig{LogFile: "/custom/audit.jsonl"}
			Expect(cfg.GetLogFile()).To(Equal("/custom/audit.jsonl"))
		})

		It("returns default for nil config", func() {
			var cfg *config.SessionAuditConfig
			Expect(cfg.GetLogFile()).To(Equal(config.DefaultSessionAuditLogFile))
		})
	})

	Describe("GetMaxSizeMB", func() {
		It("returns default for empty config", func() {
			cfg := &config.SessionAuditConfig{}
			Expect(cfg.GetMaxSizeMB()).To(Equal(config.DefaultSessionAuditMaxSizeMB))
		})

		It("returns custom max size when set", func() {
			cfg := &config.SessionAuditConfig{MaxSizeMB: 50}
			Expect(cfg.GetMaxSizeMB()).To(Equal(50))
		})

		It("returns default for nil config", func() {
			var cfg *config.SessionAuditConfig
			Expect(cfg.GetMaxSizeMB()).To(Equal(config.DefaultSessionAuditMaxSizeMB))
		})
	})

	Describe("GetMaxAgeDays", func() {
		It("returns default for empty config", func() {
			cfg := &config.SessionAuditConfig{}
			Expect(cfg.GetMaxAgeDays()).To(Equal(config.DefaultSessionAuditMaxAgeDays))
		})

		It("returns custom max age when set", func() {
			cfg := &config.SessionAuditConfig{MaxAgeDays: 60}
			Expect(cfg.GetMaxAgeDays()).To(Equal(60))
		})

		It("returns default for nil config", func() {
			var cfg *config.SessionAuditConfig
			Expect(cfg.GetMaxAgeDays()).To(Equal(config.DefaultSessionAuditMaxAgeDays))
		})
	})

	Describe("GetMaxBackups", func() {
		It("returns default for empty config", func() {
			cfg := &config.SessionAuditConfig{}
			Expect(cfg.GetMaxBackups()).To(Equal(config.DefaultSessionAuditMaxBackups))
		})

		It("returns custom max backups when set", func() {
			cfg := &config.SessionAuditConfig{MaxBackups: 10}
			Expect(cfg.GetMaxBackups()).To(Equal(10))
		})

		It("returns default for nil config", func() {
			var cfg *config.SessionAuditConfig
			Expect(cfg.GetMaxBackups()).To(Equal(config.DefaultSessionAuditMaxBackups))
		})
	})
})

var _ = Describe("Config.GetSession", func() {
	It("creates session config if nil", func() {
		cfg := &config.Config{}
		session := cfg.GetSession()

		Expect(session).NotTo(BeNil())
		Expect(cfg.Session).NotTo(BeNil())
	})

	It("returns existing session config", func() {
		enabled := true
		session := &config.SessionConfig{Enabled: &enabled}
		cfg := &config.Config{Session: session}

		result := cfg.GetSession()

		Expect(result).To(Equal(session))
		Expect(result.IsEnabled()).To(BeTrue())
	})
})
