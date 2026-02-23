package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newWalkupLoader creates a temp directory tree under a fake $HOME and returns
// a loader whose workDir is a deep subdirectory.
func newWalkupLoader(depth int) (loader *KoanfLoader, homeDir, projectRoot, workDir string) {
	homeDir, err := os.MkdirTemp("", "walkup-home-")
	Expect(err).NotTo(HaveOccurred())

	projectRoot = filepath.Join(homeDir, "projects", "myrepo")
	workDir = projectRoot

	for i := range depth {
		_ = i
		workDir = filepath.Join(workDir, "sub")
	}

	err = os.MkdirAll(workDir, 0o755)
	Expect(err).NotTo(HaveOccurred())

	loader, err = NewKoanfLoaderWithDirs(homeDir, workDir)
	Expect(err).NotTo(HaveOccurred())

	return loader, homeDir, projectRoot, workDir
}

func writeConfigAt(dir, content string) {
	cfgDir := filepath.Join(dir, ProjectConfigDir)
	Expect(os.MkdirAll(cfgDir, 0o755)).To(Succeed())
	Expect(os.WriteFile(
		filepath.Join(cfgDir, ProjectConfigFile),
		[]byte(content), 0o644,
	)).To(Succeed())
}

func writeAltConfigAt(dir, content string) {
	Expect(os.WriteFile(
		filepath.Join(dir, ProjectConfigFileAlt),
		[]byte(content), 0o644,
	)).To(Succeed())
}

var _ = Describe("Walk-up project config discovery", func() {
	Describe("findProjectConfig", func() {
		It("finds config in cwd (no walk-up needed)", func() {
			loader, homeDir, _, workDir := newWalkupLoader(0)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(workDir, "version = 1\n")
			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(workDir, ProjectConfigDir, ProjectConfigFile),
			))
		})

		It("finds config one level up", func() {
			loader, homeDir, projectRoot, _ := newWalkupLoader(1)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(projectRoot, "version = 1\n")
			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(projectRoot, ProjectConfigDir, ProjectConfigFile),
			))
		})

		It("finds config two levels up", func() {
			loader, homeDir, projectRoot, _ := newWalkupLoader(2)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(projectRoot, "version = 1\n")
			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(projectRoot, ProjectConfigDir, ProjectConfigFile),
			))
		})

		It("finds alt format (klaudiush.toml) via walk-up", func() {
			loader, homeDir, projectRoot, _ := newWalkupLoader(1)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeAltConfigAt(projectRoot, "version = 1\n")
			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(projectRoot, ProjectConfigFileAlt),
			))
		})

		It("prefers cwd config over parent config", func() {
			loader, homeDir, projectRoot, workDir := newWalkupLoader(1)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(projectRoot, "version = 1\n")
			writeConfigAt(workDir, "version = 1\n")
			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(workDir, ProjectConfigDir, ProjectConfigFile),
			))
		})

		It("prefers .klaudiush/config.toml over klaudiush.toml at same level", func() {
			loader, homeDir, projectRoot, _ := newWalkupLoader(1)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(projectRoot, "version = 1\n")
			writeAltConfigAt(projectRoot, "version = 1\n")
			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(projectRoot, ProjectConfigDir, ProjectConfigFile),
			))
		})

		It("skips global config path during walk-up", func() {
			loader, homeDir, _, _ := newWalkupLoader(1)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			// Place config at the global config location - should NOT be found
			// as a project config (it would be double-loaded otherwise).
			globalDir := filepath.Dir(loader.GlobalConfigPath())
			Expect(os.MkdirAll(globalDir, 0o755)).To(Succeed())
			Expect(os.WriteFile(
				loader.GlobalConfigPath(),
				[]byte("version = 1\n"), 0o644,
			)).To(Succeed())

			Expect(loader.findProjectConfig()).To(BeEmpty())
		})

		It("finds config outside $HOME", func() {
			// Project lives in a temp dir unrelated to $HOME.
			homeDir, err := os.MkdirTemp("", "walkup-home-")
			Expect(err).NotTo(HaveOccurred())

			projectDir, err := os.MkdirTemp("", "walkup-outside-")
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() {
				os.RemoveAll(homeDir)
				os.RemoveAll(projectDir)
			})

			workDir := filepath.Join(projectDir, "src", "pkg")
			Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())

			writeConfigAt(projectDir, "version = 1\n")

			loader, err := NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			Expect(loader.findProjectConfig()).To(Equal(
				filepath.Join(projectDir, ProjectConfigDir, ProjectConfigFile),
			))
		})

		It("returns empty string when no config exists anywhere", func() {
			loader, homeDir, _, _ := newWalkupLoader(2)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			Expect(loader.findProjectConfig()).To(BeEmpty())
		})
	})

	Describe("Load integration", func() {
		It("applies settings from parent config via full Load", func() {
			loader, homeDir, projectRoot, _ := newWalkupLoader(2)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(projectRoot, `
version = 1
[validators.git.commit]
enabled = false
`)

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(*cfg.Validators.Git.Commit.Enabled).To(BeFalse())
		})

		It("LoadProjectConfigOnly picks up walked-up config", func() {
			loader, homeDir, projectRoot, _ := newWalkupLoader(2)

			DeferCleanup(func() { os.RemoveAll(homeDir) })

			writeConfigAt(projectRoot, `
version = 1
[validators.git.push]
enabled = false
`)

			cfg, path, err := loader.LoadProjectConfigOnly()
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal(
				filepath.Join(projectRoot, ProjectConfigDir, ProjectConfigFile),
			))
			Expect(cfg).NotTo(BeNil())
			Expect(*cfg.Validators.Git.Push.Enabled).To(BeFalse())
		})
	})
})
