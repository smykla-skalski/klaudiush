package config

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgconfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("KoanfLoader patterns defaults", func() {
	var (
		tempDir string
		loader  *KoanfLoader
	)

	BeforeEach(func() {
		var err error

		tempDir, err = os.MkdirTemp("", "koanf-patterns-test")
		Expect(err).NotTo(HaveOccurred())

		GinkgoT().Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "xdg-data"))

		loader, err = NewKoanfLoaderWithDirs(tempDir, tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	It("loads top-level patterns defaults from koanf", func() {
		cfg, err := loader.Load(nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.Patterns).NotTo(BeNil())
		Expect(cfg.Patterns.Enabled).NotTo(BeNil())
		Expect(*cfg.Patterns.Enabled).To(BeTrue())
		Expect(cfg.Patterns.MinCount).To(Equal(pkgconfig.DefaultPatternsMinCount))
		Expect(cfg.Patterns.MaxWarningsPerError).To(
			Equal(pkgconfig.DefaultPatternsMaxWarningsPerError),
		)
		Expect(cfg.Patterns.MaxWarningsTotal).To(Equal(pkgconfig.DefaultPatternsMaxWarningsTotal))
		Expect(cfg.Patterns.ProjectDataFile).To(Equal(pkgconfig.DefaultPatternsProjectDataFile))
		Expect(cfg.Patterns.GlobalDataDir).To(Equal(
			filepath.Join(os.Getenv("XDG_DATA_HOME"), "klaudiush", "patterns"),
		))
		Expect(cfg.Patterns.UseSeedData).NotTo(BeNil())
		Expect(*cfg.Patterns.UseSeedData).To(BeTrue())
		Expect(cfg.Patterns.MaxPatterns).To(Equal(pkgconfig.DefaultPatternsMaxPatterns))
		Expect(cfg.Patterns.MaxSessions).To(Equal(pkgconfig.DefaultPatternsMaxSessions))
	})
})
