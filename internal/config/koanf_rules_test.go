package config

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

func TestKoanfRules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Koanf Rules Suite")
}

var _ = Describe("mergeRules", func() {
	Describe("basic merge behavior", func() {
		It("should return project rules when global is empty", func() {
			projectRules := []config.RuleConfig{
				{Name: "project-rule", Priority: 100},
			}

			merged := mergeRules(nil, projectRules)
			Expect(merged).To(HaveLen(1))
			Expect(merged[0].Name).To(Equal("project-rule"))
		})

		It("should return global rules when project is empty", func() {
			globalRules := []config.RuleConfig{
				{Name: "global-rule", Priority: 100},
			}

			merged := mergeRules(globalRules, nil)
			Expect(merged).To(HaveLen(1))
			Expect(merged[0].Name).To(Equal("global-rule"))
		})

		It("should return empty when both are empty", func() {
			merged := mergeRules(nil, nil)
			Expect(merged).To(BeEmpty())
		})
	})

	Describe("project overrides global", func() {
		It("should override global rule with same name", func() {
			globalRules := []config.RuleConfig{
				{Name: "shared-rule", Priority: 50, Description: "global version"},
			}
			projectRules := []config.RuleConfig{
				{Name: "shared-rule", Priority: 100, Description: "project version"},
			}

			merged := mergeRules(globalRules, projectRules)
			Expect(merged).To(HaveLen(1))
			Expect(merged[0].Priority).To(Equal(100))
			Expect(merged[0].Description).To(Equal("project version"))
		})

		It("should combine rules with different names", func() {
			globalRules := []config.RuleConfig{
				{Name: "global-rule", Priority: 50},
			}
			projectRules := []config.RuleConfig{
				{Name: "project-rule", Priority: 100},
			}

			merged := mergeRules(globalRules, projectRules)
			Expect(merged).To(HaveLen(2))
		})

		It("should handle mixed override and combine", func() {
			globalRules := []config.RuleConfig{
				{Name: "shared-rule", Priority: 50, Description: "global"},
				{Name: "global-only", Priority: 40, Description: "global only"},
			}
			projectRules := []config.RuleConfig{
				{Name: "shared-rule", Priority: 100, Description: "project"},
				{Name: "project-only", Priority: 60, Description: "project only"},
			}

			merged := mergeRules(globalRules, projectRules)
			Expect(merged).To(HaveLen(3))

			// Find each rule and verify
			rulesByName := make(map[string]config.RuleConfig)

			for _, rule := range merged {
				rulesByName[rule.Name] = rule
			}

			Expect(rulesByName["shared-rule"].Description).To(Equal("project"))
			Expect(rulesByName["global-only"].Description).To(Equal("global only"))
			Expect(rulesByName["project-only"].Description).To(Equal("project only"))
		})
	})

	Describe("rules without names", func() {
		It("should include rules without names from both sources", func() {
			globalRules := []config.RuleConfig{
				{Name: "", Priority: 50, Description: "unnamed global"},
			}
			projectRules := []config.RuleConfig{
				{Name: "", Priority: 100, Description: "unnamed project"},
			}

			merged := mergeRules(globalRules, projectRules)
			Expect(merged).To(HaveLen(2))
		})

		It("should mix named and unnamed rules", func() {
			globalRules := []config.RuleConfig{
				{Name: "named-global", Priority: 50},
				{Name: "", Priority: 40, Description: "unnamed global"},
			}
			projectRules := []config.RuleConfig{
				{Name: "named-project", Priority: 100},
				{Name: "", Priority: 60, Description: "unnamed project"},
			}

			merged := mergeRules(globalRules, projectRules)
			Expect(merged).To(HaveLen(4))
		})
	})
})

var _ = Describe("KoanfLoader rules loading", func() {
	var (
		loader   *KoanfLoader
		homeDir  string
		workDir  string
		cleanups []string
	)

	BeforeEach(func() {
		var err error

		// Use separate directories for home and work
		homeDir, err = os.MkdirTemp("", "koanf-rules-test-home")
		Expect(err).NotTo(HaveOccurred())
		cleanups = append(cleanups, homeDir)

		workDir, err = os.MkdirTemp("", "koanf-rules-test-work")
		Expect(err).NotTo(HaveOccurred())
		cleanups = append(cleanups, workDir)

		loader, err = NewKoanfLoaderWithDirs(homeDir, workDir)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		for _, dir := range cleanups {
			os.RemoveAll(dir)
		}

		cleanups = nil
	})

	Describe("loading rules from TOML", func() {
		It("should load rules from global config", func() {
			globalDir := filepath.Join(homeDir, GlobalConfigDir)
			Expect(os.MkdirAll(globalDir, 0o755)).To(Succeed())

			globalConfig := `
[rules]
enabled = true
stop_on_first_match = true

[[rules.rules]]
name = "block-origin"
priority = 100
[rules.rules.match]
validator_type = "git.push"
remote = "origin"
[rules.rules.action]
type = "block"
message = "Don't push to origin"
`
			err := os.WriteFile(
				filepath.Join(globalDir, GlobalConfigFile),
				[]byte(globalConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules).NotTo(BeNil())
			Expect(cfg.Rules.Rules).To(HaveLen(1))
			Expect(cfg.Rules.Rules[0].Name).To(Equal("block-origin"))
			Expect(cfg.Rules.Rules[0].Priority).To(Equal(100))
			Expect(cfg.Rules.Rules[0].Match.Remote).To(Equal("origin"))
			Expect(cfg.Rules.Rules[0].Action.Type).To(Equal("block"))
		})

		It("should load rules from project config", func() {
			projectDir := filepath.Join(workDir, ProjectConfigDir)
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[[rules.rules]]
name = "project-rule"
priority = 200
[rules.rules.match]
validator_type = "git.commit"
[rules.rules.action]
type = "warn"
message = "Project warning"
`
			err := os.WriteFile(
				filepath.Join(projectDir, ProjectConfigFile),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules).NotTo(BeNil())
			Expect(cfg.Rules.Rules).To(HaveLen(1))
			Expect(cfg.Rules.Rules[0].Name).To(Equal("project-rule"))
		})

		It("should merge global and project rules", func() {
			// Create global config in homeDir
			globalDir := filepath.Join(homeDir, GlobalConfigDir)
			Expect(os.MkdirAll(globalDir, 0o755)).To(Succeed())

			globalConfig := `
[[rules.rules]]
name = "global-rule"
priority = 100
[rules.rules.match]
validator_type = "git.push"
[rules.rules.action]
type = "block"

[[rules.rules]]
name = "shared-rule"
priority = 50
description = "global version"
[rules.rules.match]
validator_type = "git.commit"
[rules.rules.action]
type = "block"
message = "global message"
`
			err := os.WriteFile(
				filepath.Join(globalDir, GlobalConfigFile),
				[]byte(globalConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create project config in workDir
			projectDir := filepath.Join(workDir, ProjectConfigDir)
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[[rules.rules]]
name = "project-rule"
priority = 200
[rules.rules.match]
validator_type = "file.markdown"
[rules.rules.action]
type = "warn"

[[rules.rules]]
name = "shared-rule"
priority = 150
description = "project version"
[rules.rules.match]
validator_type = "git.commit"
[rules.rules.action]
type = "allow"
message = "project message"
`
			err = os.WriteFile(
				filepath.Join(projectDir, ProjectConfigFile),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules).NotTo(BeNil())
			Expect(cfg.Rules.Rules).To(HaveLen(3))

			// Build map for easier verification
			rulesByName := make(map[string]config.RuleConfig)

			for _, rule := range cfg.Rules.Rules {
				rulesByName[rule.Name] = rule
			}

			// Global rule should be present
			Expect(rulesByName["global-rule"].Priority).To(Equal(100))

			// Project rule should be present
			Expect(rulesByName["project-rule"].Priority).To(Equal(200))

			// Shared rule should be project version (override)
			Expect(rulesByName["shared-rule"].Priority).To(Equal(150))
			Expect(rulesByName["shared-rule"].Description).To(Equal("project version"))
			Expect(rulesByName["shared-rule"].Action.Type).To(Equal("allow"))
		})

		It("should respect rules.enabled setting", func() {
			projectDir := filepath.Join(workDir, ProjectConfigDir)
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[rules]
enabled = false

[[rules.rules]]
name = "should-not-run"
[rules.rules.match]
validator_type = "git.push"
[rules.rules.action]
type = "block"
`
			err := os.WriteFile(
				filepath.Join(projectDir, ProjectConfigFile),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules).NotTo(BeNil())
			Expect(cfg.Rules.IsEnabled()).To(BeFalse())
		})
	})
})
