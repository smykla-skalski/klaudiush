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

	Describe("deep merge preserves defaults for unset fields", func() {
		Context("when project config sets only enabled on markdown", func() {
			BeforeEach(func() {
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				// Setting only enabled=true should NOT wipe use_markdownlint,
				// heading_spacing, etc.
				configContent := `[validators.file.markdown]
enabled = true
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should preserve default values for unset fields", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				Expect(md.IsEnabled()).To(BeTrue())
				Expect(md.UseMarkdownlint).NotTo(BeNil())
				Expect(*md.UseMarkdownlint).To(BeTrue())
				Expect(md.HeadingSpacing).NotTo(BeNil())
				Expect(*md.HeadingSpacing).To(BeTrue())
				Expect(md.CodeBlockFormatting).NotTo(BeNil())
				Expect(*md.CodeBlockFormatting).To(BeTrue())
				Expect(md.ListFormatting).NotTo(BeNil())
				Expect(*md.ListFormatting).To(BeTrue())
			})
		})

		Context("when project config sets only severity on commit", func() {
			BeforeEach(func() {
				configDir := filepath.Join(tempDir, ProjectConfigDir)
				err := os.MkdirAll(configDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				configContent := `[validators.git.commit]
severity = "warning"
`
				err = os.WriteFile(
					filepath.Join(configDir, ProjectConfigFile),
					[]byte(configContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should preserve default values for unset fields", func() {
				cfg, err := loader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				commit := cfg.Validators.Git.Commit
				Expect(commit.GetSeverity().String()).To(Equal("warning"))
				Expect(commit.IsEnabled()).To(BeTrue())
				Expect(commit.CheckStagingArea).NotTo(BeNil())
				Expect(*commit.CheckStagingArea).To(BeTrue())
				Expect(commit.RequiredFlags).To(ContainElements("-s", "-S"))
			})
		})

		Context("when global sets enabled=false and project sets severity=warning", func() {
			var separateLoader *KoanfLoader

			BeforeEach(func() {
				// Need separate homeDir and workDir so global and project
				// configs don't collide (both use .klaudiush/config.toml).
				homeDir, err := os.MkdirTemp("", "koanf-home-")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(homeDir) })

				workDir, err := os.MkdirTemp("", "koanf-work-")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(workDir) })

				separateLoader, err = NewKoanfLoaderWithDirs(homeDir, workDir)
				Expect(err).NotTo(HaveOccurred())

				globalDir := filepath.Join(homeDir, GlobalConfigDir)
				err = os.MkdirAll(globalDir, 0o755)
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

				projectDir := filepath.Join(workDir, ProjectConfigDir)
				err = os.MkdirAll(projectDir, 0o755)
				Expect(err).NotTo(HaveOccurred())

				projectContent := `[validators.file.markdown]
severity = "warning"
`
				err = os.WriteFile(
					filepath.Join(projectDir, ProjectConfigFile),
					[]byte(projectContent),
					0o644,
				)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should merge all three sources correctly", func() {
				cfg, err := separateLoader.Load(nil)
				Expect(err).NotTo(HaveOccurred())

				md := cfg.Validators.File.Markdown
				// enabled=false from global (project didn't override it)
				Expect(md.IsEnabled()).To(BeFalse())
				// severity=warning from project
				Expect(md.GetSeverity().String()).To(Equal("warning"))
				// defaults preserved
				Expect(md.UseMarkdownlint).NotTo(BeNil())
				Expect(*md.UseMarkdownlint).To(BeTrue())
			})
		})
	})
})
