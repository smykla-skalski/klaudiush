package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KoanfLoader embedded struct handling", func() {
	var (
		tempDir string
		loader  *KoanfLoader
	)

	BeforeEach(func() {
		var err error

		tempDir, err = os.MkdirTemp("", "koanf-embedded-test")
		Expect(err).NotTo(HaveOccurred())

		loader, err = NewKoanfLoaderWithDirs(tempDir, tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Describe("ValidatorConfig embedded field override", func() {
		Context("when project config disables a validator", func() {
			BeforeEach(func() {
				// Create project config directory and file
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				configContent := `[validators.file.markdown]
enabled = false
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should override the default enabled=true", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// Default is enabled=true, project config sets enabled=false
				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeFalse())
			})
		})

		Context("when project config enables a disabled validator", func() {
			BeforeEach(func() {
				// Create global config that disables markdown
				globalDir := filepath.Join(tempDir, GlobalConfigDir)
				err := os.MkdirAll(globalDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				globalContent := `[validators.file.markdown]
enabled = false
`
				err = os.WriteFile(
					filepath.Join(globalDir, GlobalConfigFile),
					[]byte(globalContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())

				// Create project config that enables markdown
				projectDir := filepath.Join(tempDir, ProjectConfigDir)
				err = os.MkdirAll(projectDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				projectContent := `[validators.file.markdown]
enabled = true
`
				err = os.WriteFile(
					filepath.Join(projectDir, ProjectConfigFile),
					[]byte(projectContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("project config should override global config", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// Global disables, project enables - project wins
				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeTrue())
			})
		})

		Context("when disabling multiple validators", func() {
			BeforeEach(func() {
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				configContent := `[validators.file.markdown]
enabled = false

[validators.file.shellscript]
enabled = false

[validators.git.commit]
enabled = false

[validators.git.push]
enabled = false
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should disable all specified validators", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeFalse())
				Expect(cfg.Validators.File.ShellScript.IsEnabled()).To(BeFalse())
				Expect(cfg.Validators.Git.Commit.IsEnabled()).To(BeFalse())
				Expect(cfg.Validators.Git.Push.IsEnabled()).To(BeFalse())
			})

			It("should not affect validators not mentioned in config", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// Validators not in config should use defaults (enabled=true)
				Expect(cfg.Validators.Git.Add.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Git.PR.IsEnabled()).To(BeTrue())
				Expect(cfg.Validators.Notification.Bell.IsEnabled()).To(BeTrue())
			})
		})

		Context("when setting severity on embedded ValidatorConfig", func() {
			BeforeEach(func() {
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				configContent := `[validators.file.markdown]
severity = "warning"

[validators.git.commit]
severity = "warning"
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should override the default severity", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// Default is error, config sets warning
				Expect(cfg.Validators.File.Markdown.GetSeverity().String()).To(Equal("warning"))
				Expect(cfg.Validators.Git.Commit.GetSeverity().String()).To(Equal("warning"))
			})
		})

		Context("when setting both enabled and severity", func() {
			BeforeEach(func() {
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				configContent := `[validators.file.markdown]
enabled = false
severity = "warning"
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should set both fields correctly", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(cfg.Validators.File.Markdown.IsEnabled()).To(BeFalse())
				Expect(cfg.Validators.File.Markdown.GetSeverity().String()).To(Equal("warning"))
			})
		})
	})

	Describe("session config override", func() {
		Context("when project config disables session tracking", func() {
			BeforeEach(func() {
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				configContent := `[session]
enabled = true
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should override the default enabled=false", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				// Default is enabled=false, project config sets enabled=true
				Expect(cfg.Session.IsEnabled()).To(BeTrue())
			})
		})
	})
})
