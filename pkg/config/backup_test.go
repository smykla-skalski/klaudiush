package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

var _ = Describe("BackupConfig", func() {
	Describe("IsEnabled", func() {
		It("returns true by default", func() {
			cfg := &config.BackupConfig{}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.BackupConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			cfg := &config.BackupConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.BackupConfig
			Expect(cfg.IsEnabled()).To(BeTrue())
		})
	})

	Describe("IsAutoBackupEnabled", func() {
		It("returns true by default", func() {
			cfg := &config.BackupConfig{}
			Expect(cfg.IsAutoBackupEnabled()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.BackupConfig{AutoBackup: &enabled}
			Expect(cfg.IsAutoBackupEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			cfg := &config.BackupConfig{AutoBackup: &enabled}
			Expect(cfg.IsAutoBackupEnabled()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.BackupConfig
			Expect(cfg.IsAutoBackupEnabled()).To(BeTrue())
		})
	})

	Describe("IsAsyncBackupEnabled", func() {
		It("returns true by default", func() {
			cfg := &config.BackupConfig{}
			Expect(cfg.IsAsyncBackupEnabled()).To(BeTrue())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			cfg := &config.BackupConfig{AsyncBackup: &enabled}
			Expect(cfg.IsAsyncBackupEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			cfg := &config.BackupConfig{AsyncBackup: &enabled}
			Expect(cfg.IsAsyncBackupEnabled()).To(BeFalse())
		})

		It("returns true for nil config", func() {
			var cfg *config.BackupConfig
			Expect(cfg.IsAsyncBackupEnabled()).To(BeTrue())
		})
	})

	Describe("GetDelta", func() {
		It("creates delta config if nil", func() {
			cfg := &config.BackupConfig{}
			delta := cfg.GetDelta()

			Expect(delta).NotTo(BeNil())
			Expect(cfg.Delta).NotTo(BeNil())
		})

		It("returns existing delta config", func() {
			interval := 5
			delta := &config.DeltaConfig{FullSnapshotInterval: &interval}
			cfg := &config.BackupConfig{Delta: delta}

			result := cfg.GetDelta()

			Expect(result).To(Equal(delta))
			Expect(*result.FullSnapshotInterval).To(Equal(5))
		})
	})
})

var _ = Describe("Config.GetBackup", func() {
	It("creates backup config if nil", func() {
		cfg := &config.Config{}
		backup := cfg.GetBackup()

		Expect(backup).NotTo(BeNil())
		Expect(cfg.Backup).NotTo(BeNil())
	})

	It("returns existing backup config", func() {
		enabled := true
		backup := &config.BackupConfig{Enabled: &enabled}
		cfg := &config.Config{Backup: backup}

		result := cfg.GetBackup()

		Expect(result).To(Equal(backup))
		Expect(result.IsEnabled()).To(BeTrue())
	})
})
