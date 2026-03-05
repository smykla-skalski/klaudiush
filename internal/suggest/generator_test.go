package suggest_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/suggest"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("Collector", func() {
	Context("with nil config", func() {
		It("returns defaults for all sections", func() {
			data := suggest.Collect(nil, "test")

			Expect(data.Version).To(Equal("test"))
			Expect(data.Commit).NotTo(BeNil())
			Expect(data.Commit.TitleMaxLength).To(Equal(50))
			Expect(data.Commit.BodyMaxLineLength).To(Equal(72))
			Expect(data.Commit.ConventionalCommit).To(BeTrue())
			Expect(data.Commit.RequireScope).To(BeTrue())
			Expect(data.Commit.RequiredFlags).To(Equal([]string{"-s", "-S"}))

			Expect(data.Push).NotTo(BeNil())
			Expect(data.Push.RequireTracking).To(BeTrue())

			Expect(data.Branch).NotTo(BeNil())
			Expect(data.Branch.RequireType).To(BeTrue())
			Expect(data.Branch.ProtectedBranches).To(ContainElements("main", "master"))

			Expect(data.Linters).NotTo(BeEmpty())
			Expect(data.Secrets).NotTo(BeNil())
			Expect(data.Shell).NotTo(BeNil())

			Expect(data.Cascades).NotTo(BeEmpty())
		})
	})

	Context("with full config", func() {
		var cfg *config.Config

		BeforeEach(func() {
			enabled := true
			disabled := false
			titleMax := 72
			bodyMax := 100

			cfg = &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							RequiredFlags: []string{"-s"},
							Message: &config.CommitMessageConfig{
								TitleMaxLength:    &titleMax,
								BodyMaxLineLength: &bodyMax,
								ValidTypes:        []string{"feat", "fix"},
							},
						},
						Push: &config.PushValidatorConfig{
							BlockedRemotes: []string{"origin"},
						},
						Branch: &config.BranchValidatorConfig{
							ValidTypes:        []string{"feat", "fix", "chore"},
							ProtectedBranches: []string{"main", "develop"},
						},
					},
					File: &config.FileConfig{
						Python: &config.PythonValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: &disabled},
						},
					},
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							UseGitleaks: &enabled,
							AllowList:   []string{"test-key-.*"},
						},
					},
					Shell: &config.ShellConfig{
						Backtick: &config.BacktickValidatorConfig{
							CheckAllCommands: true,
						},
					},
				},
				Rules: &config.RulesConfig{
					Rules: []config.RuleConfig{
						{
							Name:     "block-force-push",
							Priority: 10,
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
							Action: &config.RuleActionConfig{
								Type:    "block",
								Message: "Force push blocked",
							},
						},
					},
				},
			}
		})

		It("reflects all configured values", func() {
			data := suggest.Collect(cfg, "1.0.0")

			Expect(data.Commit.RequiredFlags).To(Equal([]string{"-s"}))
			Expect(data.Commit.TitleMaxLength).To(Equal(72))
			Expect(data.Commit.BodyMaxLineLength).To(Equal(100))
			Expect(data.Commit.ValidTypes).To(Equal([]string{"feat", "fix"}))

			Expect(data.Push.BlockedRemotes).To(Equal([]string{"origin"}))

			Expect(data.Branch.ValidTypes).To(Equal([]string{"feat", "fix", "chore"}))
			Expect(data.Branch.ProtectedBranches).To(Equal([]string{"main", "develop"}))

			// Python linter should be filtered out
			linterNames := make([]string, 0, len(data.Linters))
			for _, l := range data.Linters {
				linterNames = append(linterNames, l.Name)
			}

			Expect(linterNames).NotTo(ContainElement("Python"))
			Expect(linterNames).To(ContainElement("Markdown"))

			Expect(data.Secrets.UseGitleaks).To(BeTrue())
			Expect(data.Secrets.AllowListCount).To(Equal(1))

			Expect(data.Shell.CheckAllCommands).To(BeTrue())

			Expect(data.Rules).To(HaveLen(1))
			Expect(data.Rules[0].Name).To(Equal("block-force-push"))
			Expect(data.Rules[0].Priority).To(Equal(10))
		})
	})
})

var _ = Describe("Hasher", func() {
	Context("ComputeHash", func() {
		It("is deterministic for the same config", func() {
			cfg := &config.Config{}

			hash1, err := suggest.ComputeHash(cfg)
			Expect(err).NotTo(HaveOccurred())

			hash2, err := suggest.ComputeHash(cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(hash1).To(Equal(hash2))
		})

		It("produces different hashes for different configs", func() {
			cfg1 := &config.Config{}

			titleMax := 72

			cfg2 := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								TitleMaxLength: &titleMax,
							},
						},
					},
				},
			}

			hash1, err := suggest.ComputeHash(cfg1)
			Expect(err).NotTo(HaveOccurred())

			hash2, err := suggest.ComputeHash(cfg2)
			Expect(err).NotTo(HaveOccurred())

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("handles nil config", func() {
			hash, err := suggest.ComputeHash(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).NotTo(BeEmpty())
		})
	})

	Context("ExtractHash", func() {
		var tmpDir string

		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
		})

		It("extracts hash from valid file", func() {
			content := "<!-- klaudiush:hash:abc123def456 -->\n# KLAUDIUSH.md\n"
			filePath := filepath.Join(tmpDir, "KLAUDIUSH.md")
			Expect(os.WriteFile(filePath, []byte(content), 0o644)).To(Succeed())

			hash, err := suggest.ExtractHash(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(Equal("abc123def456"))
		})

		It("returns empty string for file without hash", func() {
			content := "# KLAUDIUSH.md\nSome content\n"
			filePath := filepath.Join(tmpDir, "KLAUDIUSH.md")
			Expect(os.WriteFile(filePath, []byte(content), 0o644)).To(Succeed())

			hash, err := suggest.ExtractHash(filePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(hash).To(BeEmpty())
		})

		It("returns error for missing file", func() {
			_, err := suggest.ExtractHash(filepath.Join(tmpDir, "nonexistent.md"))
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("Renderer", func() {
	It("produces valid markdown", func() {
		data := suggest.Collect(nil, "test")
		data.Hash = "testhash12345678"

		content, err := suggest.Render(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(ContainSubstring("<!-- klaudiush:hash:testhash12345678 -->"))
		Expect(content).To(ContainSubstring("# KLAUDIUSH.md"))
		Expect(content).To(ContainSubstring("## Commits"))
		Expect(content).To(ContainSubstring("## Push"))
		Expect(content).To(ContainSubstring("## Branches"))
		Expect(content).To(ContainSubstring("## Linters"))
	})

	It("includes cascades section", func() {
		data := suggest.Collect(nil, "test")
		data.Hash = "abc"

		content, err := suggest.Render(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(ContainSubstring("## Cascades"))
		Expect(content).To(ContainSubstring("GIT013"))
	})

	It("excludes custom rules when none configured", func() {
		data := suggest.Collect(nil, "test")
		data.Hash = "abc"

		content, err := suggest.Render(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).NotTo(ContainSubstring("## Custom Rules"))
	})

	It("includes custom rules when configured", func() {
		cfg := &config.Config{
			Rules: &config.RulesConfig{
				Rules: []config.RuleConfig{
					{
						Name: "test-rule",
						Match: &config.RuleMatchConfig{
							ValidatorType: "git.push",
						},
						Action: &config.RuleActionConfig{
							Type: "block",
						},
					},
				},
			},
		}

		data := suggest.Collect(cfg, "test")
		data.Hash = "abc"

		content, err := suggest.Render(data)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(ContainSubstring("## Custom Rules"))
		Expect(content).To(ContainSubstring("test-rule"))
	})
})

var _ = Describe("Generator", func() {
	var (
		tmpDir string
		gen    *suggest.Generator
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		gen = suggest.NewGenerator(&config.Config{}, "test")
	})

	Context("Generate", func() {
		It("produces valid content", func() {
			content, err := gen.Generate()
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(ContainSubstring("<!-- klaudiush:hash:"))
			Expect(content).To(ContainSubstring("# KLAUDIUSH.md"))
		})
	})

	Context("Check", func() {
		It("returns false for missing file", func() {
			upToDate, err := gen.Check(filepath.Join(tmpDir, "nonexistent.md"))
			Expect(err).NotTo(HaveOccurred())
			Expect(upToDate).To(BeFalse())
		})

		It("returns true for up-to-date file", func() {
			outputPath := filepath.Join(tmpDir, "KLAUDIUSH.md")

			content, err := gen.Generate()
			Expect(err).NotTo(HaveOccurred())
			Expect(gen.WriteFile(outputPath, content)).To(Succeed())

			upToDate, err := gen.Check(outputPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(upToDate).To(BeTrue())
		})

		It("returns false for stale file", func() {
			outputPath := filepath.Join(tmpDir, "KLAUDIUSH.md")

			content := "<!-- klaudiush:hash:stale_hash_value -->\n# KLAUDIUSH.md\n"
			Expect(os.WriteFile(outputPath, []byte(content), 0o644)).To(Succeed())

			upToDate, err := gen.Check(outputPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(upToDate).To(BeFalse())
		})
	})

	Context("WriteFile", func() {
		It("writes file atomically", func() {
			outputPath := filepath.Join(tmpDir, "output", "KLAUDIUSH.md")

			err := gen.WriteFile(outputPath, "test content")
			Expect(err).NotTo(HaveOccurred())

			content, err := os.ReadFile(outputPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal("test content"))
		})

		It("creates parent directories", func() {
			outputPath := filepath.Join(tmpDir, "deep", "nested", "KLAUDIUSH.md")

			err := gen.WriteFile(outputPath, "content")
			Expect(err).NotTo(HaveOccurred())

			Expect(outputPath).To(BeARegularFile())
		})

		It("does not leave tmp file on success", func() {
			outputPath := filepath.Join(tmpDir, "KLAUDIUSH.md")

			err := gen.WriteFile(outputPath, "content")
			Expect(err).NotTo(HaveOccurred())

			tmpPath := outputPath + ".tmp"
			Expect(tmpPath).NotTo(BeAnExistingFile())
		})
	})
})
