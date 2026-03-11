package main

import (
	"os"
	"path/filepath"

	gogit "github.com/go-git/go-git/v6"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func initCommandTestEnv(tempDir string) {
	GinkgoT().Setenv("HOME", filepath.Join(tempDir, "home"))
	GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg-config"))
	GinkgoT().Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "xdg-data"))
	GinkgoT().Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "xdg-state"))
	GinkgoT().Setenv("XDG_CACHE_HOME", filepath.Join(tempDir, "xdg-cache"))
}

func writeProjectConfig(projectRoot string) {
	configDir := filepath.Join(projectRoot, internalconfig.ProjectConfigDir)
	Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
	Expect(os.WriteFile(
		filepath.Join(configDir, internalconfig.ProjectConfigFile),
		[]byte("version = 1\n"),
		0o644,
	)).To(Succeed())
}

var _ = Describe("Pattern project root resolution", func() {
	It("writes project-local seed data at the walked-up config root", func() {
		tempDir := GinkgoT().TempDir()
		projectRoot := filepath.Join(tempDir, "repo")
		workDir := filepath.Join(projectRoot, "sub", "dir")
		Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())
		initCommandTestEnv(tempDir)
		writeProjectConfig(projectRoot)

		store, err := initPatternStore(
			&config.PatternsConfig{
				GlobalDataDir: filepath.Join(tempDir, "global"),
			},
			workDir,
			logger.NewNoOpLogger(),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(store).NotTo(BeNil())

		rootFile := filepath.Join(projectRoot, config.DefaultPatternsProjectDataFile)
		subdirFile := filepath.Join(workDir, config.DefaultPatternsProjectDataFile)

		Expect(rootFile).To(BeAnExistingFile())

		_, err = os.Stat(subdirFile)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("shares learned global data between subdirectory and root invocations", func() {
		tempDir := GinkgoT().TempDir()
		projectRoot := filepath.Join(tempDir, "repo")
		workDir := filepath.Join(projectRoot, "sub", "dir")
		Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())
		initCommandTestEnv(tempDir)

		_, err := gogit.PlainInit(projectRoot, false)
		Expect(err).NotTo(HaveOccurred())

		cfg := &config.PatternsConfig{
			GlobalDataDir: filepath.Join(tempDir, "global"),
		}

		storeFromSubdir, err := initPatternStore(cfg, workDir, logger.NewNoOpLogger())
		Expect(err).NotTo(HaveOccurred())
		storeFromSubdir.RecordSequence("TEST001", "TEST002")
		Expect(storeFromSubdir.Save()).To(Succeed())

		storeFromRoot, err := initPatternStore(cfg, projectRoot, logger.NewNoOpLogger())
		Expect(err).NotTo(HaveOccurred())

		followUps := storeFromRoot.GetFollowUps("TEST001", 1)
		Expect(followUps).To(HaveLen(1))
		Expect(followUps[0].TargetCode).To(Equal("TEST002"))
	})
})
