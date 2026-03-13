package fixers_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/doctor/fixers"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("PatternsFixer", func() {
	It("writes seed data to the project root when started from a subdirectory", func() {
		tempDir := GinkgoT().TempDir()
		projectRoot := filepath.Join(tempDir, "repo")
		workDir := filepath.Join(projectRoot, "sub", "dir")
		Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())
		GinkgoT().Setenv("HOME", filepath.Join(tempDir, "home"))
		GinkgoT().Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "xdg-config"))
		GinkgoT().Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "xdg-data"))
		GinkgoT().Setenv("XDG_STATE_HOME", filepath.Join(tempDir, "xdg-state"))
		GinkgoT().Setenv("XDG_CACHE_HOME", filepath.Join(tempDir, "xdg-cache"))

		configDir := filepath.Join(projectRoot, internalconfig.ProjectConfigDir)
		Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(
			filepath.Join(configDir, internalconfig.ProjectConfigFile),
			[]byte("version = 1\n"),
			0o644,
		)).To(Succeed())

		originalWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(os.Chdir(workDir)).To(Succeed())
		DeferCleanup(func() {
			Expect(os.Chdir(originalWD)).To(Succeed())
		})

		fixer := fixers.NewPatternsFixer(nil)
		Expect(fixer.Fix(context.Background(), false)).To(Succeed())

		rootFile := filepath.Join(projectRoot, config.DefaultPatternsProjectDataFile)
		subdirFile := filepath.Join(workDir, config.DefaultPatternsProjectDataFile)

		Expect(rootFile).To(BeAnExistingFile())

		_, err = os.Stat(subdirFile)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
