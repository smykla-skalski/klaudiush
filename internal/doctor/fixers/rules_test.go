package fixers

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/prompt"
)

var _ = Describe("RulesFixer", func() {
	var (
		ctrl        *gomock.Controller
		mockPrompt  *prompt.MockPrompter
		ctx         context.Context
		tempDir     string
		originalWd  string
		originalEnv string
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockPrompt = prompt.NewMockPrompter(ctrl)
		ctx = context.Background()

		// Save original environment
		originalWd, _ = os.Getwd()
		originalEnv = os.Getenv("HOME")

		// Create temp directory for tests
		var err error
		tempDir, err = os.MkdirTemp("", "rules-fixer-test-*")
		Expect(err).NotTo(HaveOccurred())

		// Set HOME for tests
		os.Setenv("HOME", tempDir)
	})

	AfterEach(func() {
		ctrl.Finish()

		// Restore environment
		os.Setenv("HOME", originalEnv)
		os.Chdir(originalWd)

		// Clean up temp directory
		os.RemoveAll(tempDir)
	})

	Describe("ID", func() {
		It("should return the correct ID", func() {
			fixer := NewRulesFixer(mockPrompt)
			Expect(fixer.ID()).To(Equal("fix_invalid_rules"))
		})
	})

	Describe("Description", func() {
		It("should return a description", func() {
			fixer := NewRulesFixer(mockPrompt)
			Expect(fixer.Description()).To(ContainSubstring("Disable invalid rules"))
		})
	})

	Describe("CanFix", func() {
		It("should return true for matching result", func() {
			fixer := NewRulesFixer(mockPrompt)
			result := doctor.CheckResult{
				FixID:  "fix_invalid_rules",
				Status: doctor.StatusFail,
			}
			Expect(fixer.CanFix(result)).To(BeTrue())
		})

		It("should return false for different fix ID", func() {
			fixer := NewRulesFixer(mockPrompt)
			result := doctor.CheckResult{
				FixID:  "other_fix",
				Status: doctor.StatusFail,
			}
			Expect(fixer.CanFix(result)).To(BeFalse())
		})

		It("should return false for passing result", func() {
			fixer := NewRulesFixer(mockPrompt)
			result := doctor.CheckResult{
				FixID:  "fix_invalid_rules",
				Status: doctor.StatusPass,
			}
			Expect(fixer.CanFix(result)).To(BeFalse())
		})
	})

	Describe("Fix", func() {
		Context("when no issues exist", func() {
			It("should return nil when no project config exists", func() {
				// Create a working directory without config
				workDir := filepath.Join(tempDir, "project")
				Expect(os.MkdirAll(workDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, false)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when config has no rules", func() {
			It("should return nil", func() {
				// Create a project with valid config but no rules
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[validators.git.commit]
enabled = true
`
				Expect(os.WriteFile(
					filepath.Join(configDir, "config.toml"),
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, false)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when config has invalid rules", func() {
			It("should disable invalid rules in non-interactive mode", func() {
				// Create a project with invalid rules
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
[rules.rules.action]
type = "block"
`
				Expect(os.WriteFile(
					filepath.Join(configDir, "config.toml"),
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, false)
				Expect(err).NotTo(HaveOccurred())

				// Read the config back and verify the rule was disabled
				content, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("enabled = false"))
			})

			It("should prompt for confirmation in interactive mode", func() {
				// Create a project with invalid rules
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
[rules.rules.action]
type = "block"
`
				Expect(os.WriteFile(
					filepath.Join(configDir, "config.toml"),
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Mock the prompter to confirm
				mockPrompt.EXPECT().
					Confirm(gomock.Any(), true).
					Return(true, nil)

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, true)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not fix when user declines in interactive mode", func() {
				// Create a project with invalid rules
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
[rules.rules.action]
type = "block"
`
				configPath := filepath.Join(configDir, "config.toml")
				Expect(os.WriteFile(
					configPath,
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Mock the prompter to decline
				mockPrompt.EXPECT().
					Confirm(gomock.Any(), true).
					Return(false, nil)

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// Config should remain unchanged
				content, err := os.ReadFile(configPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).NotTo(ContainSubstring("enabled = false"))
			})

			It("should not fix when prompter returns error", func() {
				// Create a project with invalid rules
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
[rules.rules.action]
type = "block"
`
				configPath := filepath.Join(configDir, "config.toml")
				Expect(os.WriteFile(
					configPath,
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Mock the prompter to return an error
				mockPrompt.EXPECT().
					Confirm(gomock.Any(), true).
					Return(false, prompt.ErrInvalidInput)

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, true)
				Expect(err).NotTo(HaveOccurred())

				// Config should remain unchanged
				content, err := os.ReadFile(configPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).NotTo(ContainSubstring("enabled = false"))
			})

			It("should add disabled note to description", func() {
				// Create a project with invalid rule that has no description
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
[rules.rules.action]
type = "block"
`
				Expect(os.WriteFile(
					filepath.Join(configDir, "config.toml"),
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, false)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("DISABLED BY DOCTOR"))
			})

			It("should append suffix to existing description", func() {
				// Create a project with invalid rule that has a description
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
description = "My custom rule"
[rules.rules.action]
type = "block"
`
				Expect(os.WriteFile(
					filepath.Join(configDir, "config.toml"),
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, false)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("My custom rule"))
				Expect(string(content)).To(ContainSubstring("[DISABLED BY DOCTOR"))
			})

			It("should not duplicate disabled suffix", func() {
				// Create a project with rule that already has disabled suffix
				workDir := filepath.Join(tempDir, "project")
				configDir := filepath.Join(workDir, ".klaudiush")
				Expect(os.MkdirAll(configDir, 0o755)).To(Succeed())
				Expect(os.Chdir(workDir)).To(Succeed())

				configContent := `
[[rules.rules]]
name = "invalid-rule"
description = "My custom rule [DISABLED BY DOCTOR: fix and re-enable]"
enabled = true
[rules.rules.action]
type = "block"
`
				Expect(os.WriteFile(
					filepath.Join(configDir, "config.toml"),
					[]byte(configContent),
					0o600,
				)).To(Succeed())

				// Create fixer AFTER changing to the temp directory
				fixer := NewRulesFixer(mockPrompt)

				err := fixer.Fix(ctx, false)
				Expect(err).NotTo(HaveOccurred())

				content, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
				Expect(err).NotTo(HaveOccurred())
				// Should only have one instance of the suffix
				contentStr := string(content)
				firstIdx := indexOf(contentStr, "[DISABLED BY DOCTOR")
				lastIdx := lastIndexOf(contentStr, "[DISABLED BY DOCTOR")
				Expect(firstIdx).To(Equal(lastIdx))
			})
		})
	})
})

var _ = Describe("containsDisabledNote", func() {
	It("should return false for empty string", func() {
		Expect(containsDisabledNote("")).To(BeFalse())
	})

	It("should return true for full disabled note", func() {
		Expect(containsDisabledNote(disabledNote)).To(BeTrue())
	})

	It("should return true for description with suffix", func() {
		desc := "My rule " + disabledSuffix
		Expect(containsDisabledNote(desc)).To(BeTrue())
	})

	It("should return false for regular description", func() {
		Expect(containsDisabledNote("My custom rule")).To(BeFalse())
	})
})

// Helper functions for string manipulation
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}

func lastIndexOf(s, substr string) int {
	last := -1

	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			last = i
		}
	}

	return last
}
