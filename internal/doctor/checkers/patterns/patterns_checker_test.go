package patternschecker_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	patternschecker "github.com/smykla-skalski/klaudiush/internal/doctor/checkers/patterns"
	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

func TestPatternsChecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Patterns Checker Suite")
}

// testPathProvider implements PathProvider for testing.
type testPathProvider struct {
	projectFile string
	globalDir   string
	enabled     bool
}

func (p *testPathProvider) ProjectDataFile() string { return p.projectFile }
func (p *testPathProvider) GlobalDataDir() string   { return p.globalDir }
func (p *testPathProvider) IsEnabled() bool         { return p.enabled }

var _ = Describe("SeedDataChecker", func() {
	var (
		ctx      context.Context
		tmpDir   string
		provider *testPathProvider
		checker  *patternschecker.SeedDataChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir = GinkgoT().TempDir()
		provider = &testPathProvider{
			projectFile: filepath.Join(tmpDir, "patterns.json"),
			globalDir:   filepath.Join(tmpDir, "global"),
			enabled:     true,
		}
		checker = patternschecker.NewSeedDataCheckerWithProvider(provider)
	})

	It("returns category patterns", func() {
		Expect(checker.Category()).To(Equal(doctor.CategoryPatterns))
	})

	Context("when patterns disabled", func() {
		BeforeEach(func() {
			provider.enabled = false
		})

		It("skips", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})
	})

	Context("when seed file does not exist", func() {
		It("returns warning with fix ID", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
			Expect(result.FixID).To(Equal("seed_patterns"))
		})
	})

	Context("when seed file is empty", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(provider.projectFile, []byte{}, 0o600)).To(Succeed())
		})

		It("returns warning with fix ID", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.FixID).To(Equal("seed_patterns"))
		})
	})

	Context("when seed file has invalid JSON", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(provider.projectFile, []byte(`{bad`), 0o600)).To(Succeed())
		})

		It("returns error with fix ID", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityError))
			Expect(result.FixID).To(Equal("seed_patterns"))
		})
	})

	Context("when seed file is valid", func() {
		BeforeEach(func() {
			cfg := &config.PatternsConfig{
				ProjectDataFile: "patterns.json",
				GlobalDataDir:   filepath.Join(tmpDir, "global"),
			}
			store := patterns.NewFilePatternStore(cfg, tmpDir)
			store.SetProjectData(patterns.SeedPatterns())
			Expect(store.SaveProject()).To(Succeed())
		})

		It("returns pass with pattern count", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("patterns"))
		})
	})
})

var _ = Describe("DataFileChecker", func() {
	var (
		ctx      context.Context
		tmpDir   string
		provider *testPathProvider
		checker  *patternschecker.DataFileChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir = GinkgoT().TempDir()
		provider = &testPathProvider{
			projectFile: filepath.Join(tmpDir, "patterns.json"),
			globalDir:   filepath.Join(tmpDir, "global"),
			enabled:     true,
		}
		checker = patternschecker.NewDataFileCheckerWithProvider(provider)
	})

	Context("when patterns disabled", func() {
		BeforeEach(func() {
			provider.enabled = false
		})

		It("skips", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})
	})

	Context("when global dir does not exist", func() {
		It("returns warning", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
		})
	})

	Context("when global dir exists but is empty", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(provider.globalDir, 0o700)).To(Succeed())
		})

		It("returns warning about no data", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
		})
	})

	Context("when global dir has valid JSON files", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(provider.globalDir, 0o700)).To(Succeed())

			pd := &patterns.PatternData{
				Patterns: map[string]*patterns.FailurePattern{},
				Version:  1,
			}

			data, err := json.Marshal(pd)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(
				filepath.Join(provider.globalDir, "abc123.json"),
				data,
				0o600,
			)).To(Succeed())
		})

		It("returns pass", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("1 file(s) valid"))
		})
	})

	Context("when global dir has corrupt JSON", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(provider.globalDir, 0o700)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(provider.globalDir, "bad.json"),
				[]byte(`{corrupt`),
				0o600,
			)).To(Succeed())
		})

		It("returns error", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityError))
			Expect(result.Message).To(ContainSubstring("corrupt"))
		})
	})
})

var _ = Describe("DescriptionChecker", func() {
	var (
		ctx      context.Context
		tmpDir   string
		provider *testPathProvider
		checker  *patternschecker.DescriptionChecker
	)

	BeforeEach(func() {
		ctx = context.Background()
		tmpDir = GinkgoT().TempDir()
		provider = &testPathProvider{
			projectFile: filepath.Join(tmpDir, "patterns.json"),
			globalDir:   filepath.Join(tmpDir, "global"),
			enabled:     true,
		}
		checker = patternschecker.NewDescriptionCheckerWithProvider(provider)
	})

	Context("when patterns disabled", func() {
		BeforeEach(func() {
			provider.enabled = false
		})

		It("skips", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})
	})

	Context("when no pattern data exists", func() {
		It("skips", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusSkipped))
		})
	})

	Context("when all codes have descriptions", func() {
		BeforeEach(func() {
			pd := &patterns.PatternData{
				Patterns: map[string]*patterns.FailurePattern{
					"GIT013->GIT004": {
						SourceCode: "GIT013",
						TargetCode: "GIT004",
						Count:      5,
					},
				},
				Version: 1,
			}

			data, err := json.Marshal(pd)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(provider.projectFile, data, 0o600)).To(Succeed())
		})

		It("returns pass", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusPass))
			Expect(result.Message).To(ContainSubstring("descriptions"))
		})
	})

	Context("when unknown codes exist in data", func() {
		BeforeEach(func() {
			pd := &patterns.PatternData{
				Patterns: map[string]*patterns.FailurePattern{
					"CUSTOM001->CUSTOM002": {
						SourceCode: "CUSTOM001",
						TargetCode: "CUSTOM002",
						Count:      5,
					},
				},
				Version: 1,
			}

			data, err := json.Marshal(pd)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(provider.projectFile, data, 0o600)).To(Succeed())
		})

		It("returns warning with unknown codes", func() {
			result := checker.Check(ctx)
			Expect(result.Status).To(Equal(doctor.StatusFail))
			Expect(result.Severity).To(Equal(doctor.SeverityWarning))
			Expect(result.Message).To(ContainSubstring("missing descriptions"))
		})
	})
})
