package rules_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/config/factory"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("Rule Engine Integration Tests", func() {
	var (
		ctx context.Context
		log logger.Logger
	)

	BeforeEach(func() {
		ctx = context.Background()
		log = logger.NewNoOpLogger()
	})

	Describe("End-to-End: Config to RuleEngine to Validator", func() {
		var (
			homeDir  string
			workDir  string
			cleanups []string
		)

		BeforeEach(func() {
			var err error

			homeDir, err = os.MkdirTemp("", "rules-integration-home")
			Expect(err).NotTo(HaveOccurred())
			cleanups = append(cleanups, homeDir)

			workDir, err = os.MkdirTemp("", "rules-integration-work")
			Expect(err).NotTo(HaveOccurred())
			cleanups = append(cleanups, workDir)
		})

		AfterEach(func() {
			for _, dir := range cleanups {
				os.RemoveAll(dir)
			}

			cleanups = nil
		})

		It("should block git push to origin with rule from config", func() {
			// Create project config with blocking rule
			projectDir := filepath.Join(workDir, ".klaudiush")
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[rules]
enabled = true

[[rules.rules]]
name = "block-origin-push"
priority = 100
description = "Block pushes to origin remote"

[rules.rules.match]
validator_type = "git.push"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Pushing to origin is not allowed"
reference = "GIT019"
`
			err := os.WriteFile(
				filepath.Join(projectDir, "config.toml"),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Load config
			loader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules).NotTo(BeNil())
			Expect(cfg.Rules.Rules).To(HaveLen(1))

			// Create rule engine from config using factory
			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).NotTo(BeNil())

			// Create adapter for git.push validator
			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "origin",
					}
				}),
			)

			// Verify rule blocks the push
			hookCtx := &hook.Context{
				EventType: hook.EventTypePreToolUse,
				ToolName:  hook.ToolTypeBash,
				ToolInput: hook.ToolInput{
					Command: "git push origin main",
				},
			}

			result := adapter.CheckRules(ctx, hookCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("Pushing to origin is not allowed"))
			Expect(string(result.Reference)).To(Equal("GIT019"))
		})

		It("should allow git push to upstream with allow rule", func() {
			projectDir := filepath.Join(workDir, ".klaudiush")
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[rules]
enabled = true

[[rules.rules]]
name = "allow-upstream-push"
priority = 100

[rules.rules.match]
validator_type = "git.push"
remote = "upstream"

[rules.rules.action]
type = "allow"
`
			err := os.WriteFile(
				filepath.Join(projectDir, "config.toml"),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			loader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "upstream",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeTrue())
		})

		It("should warn on push to specific branch", func() {
			projectDir := filepath.Join(workDir, ".klaudiush")
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[rules]
enabled = true

[[rules.rules]]
name = "warn-main-push"
priority = 100

[rules.rules.match]
validator_type = "git.push"
branch_pattern = "main"

[rules.rules.action]
type = "warn"
message = "Warning: pushing to main branch"
`
			err := os.WriteFile(
				filepath.Join(projectDir, "config.toml"),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			loader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Branch: "main",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(Equal("Warning: pushing to main branch"))
		})
	})

	Describe("Config Precedence: Project Overrides Global", func() {
		var (
			homeDir  string
			workDir  string
			cleanups []string
		)

		BeforeEach(func() {
			var err error

			homeDir, err = os.MkdirTemp("", "rules-precedence-home")
			Expect(err).NotTo(HaveOccurred())
			cleanups = append(cleanups, homeDir)

			workDir, err = os.MkdirTemp("", "rules-precedence-work")
			Expect(err).NotTo(HaveOccurred())
			cleanups = append(cleanups, workDir)
		})

		AfterEach(func() {
			for _, dir := range cleanups {
				os.RemoveAll(dir)
			}

			cleanups = nil
		})

		It("should override global rule with same name from project config", func() {
			// Create global config with blocking rule
			globalDir := filepath.Join(homeDir, ".klaudiush")
			Expect(os.MkdirAll(globalDir, 0o755)).To(Succeed())

			globalConfig := `
[rules]
enabled = true

[[rules.rules]]
name = "origin-push-rule"
priority = 100

[rules.rules.match]
validator_type = "git.push"
remote = "origin"

[rules.rules.action]
type = "block"
message = "Global: block origin push"
`
			err := os.WriteFile(
				filepath.Join(globalDir, "config.toml"),
				[]byte(globalConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create project config that overrides to allow
			projectDir := filepath.Join(workDir, ".klaudiush")
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[[rules.rules]]
name = "origin-push-rule"
priority = 200

[rules.rules.match]
validator_type = "git.push"
remote = "origin"

[rules.rules.action]
type = "allow"
`
			err = os.WriteFile(
				filepath.Join(projectDir, "config.toml"),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Load config - project should override global
			loader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules.Rules).To(HaveLen(1))
			Expect(cfg.Rules.Rules[0].Action.Type).To(Equal("allow"))

			// Create engine and verify behavior
			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "origin",
					}
				}),
			)

			// Should allow (project override), not block (global default)
			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeTrue())
		})

		It("should combine rules with different names from global and project", func() {
			// Create global config
			globalDir := filepath.Join(homeDir, ".klaudiush")
			Expect(os.MkdirAll(globalDir, 0o755)).To(Succeed())

			globalConfig := `
[[rules.rules]]
name = "global-only-rule"
priority = 50

[rules.rules.match]
validator_type = "git.push"
remote = "origin"

[rules.rules.action]
type = "block"
message = "global block"
`
			err := os.WriteFile(
				filepath.Join(globalDir, "config.toml"),
				[]byte(globalConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Create project config with different rule name
			projectDir := filepath.Join(workDir, ".klaudiush")
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[[rules.rules]]
name = "project-only-rule"
priority = 100

[rules.rules.match]
validator_type = "git.push"
remote = "upstream"

[rules.rules.action]
type = "warn"
message = "project warn"
`
			err = os.WriteFile(
				filepath.Join(projectDir, "config.toml"),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			// Both rules should be loaded
			loader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Rules.Rules).To(HaveLen(2))

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			// Test origin remote - should block (global rule)
			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "origin",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("global block"))

			// Test upstream remote - should warn (project rule)
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Remote: "upstream",
					}
				}),
			)

			result = adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(Equal("project warn"))
		})

		It("should use higher priority rule when multiple rules match", func() {
			projectDir := filepath.Join(workDir, ".klaudiush")
			Expect(os.MkdirAll(projectDir, 0o755)).To(Succeed())

			projectConfig := `
[rules]
stop_on_first_match = true

[[rules.rules]]
name = "low-priority"
priority = 10

[rules.rules.match]
validator_type = "git.push"

[rules.rules.action]
type = "warn"
message = "low priority warning"

[[rules.rules]]
name = "high-priority"
priority = 100

[rules.rules.match]
validator_type = "git.push"

[rules.rules.action]
type = "block"
message = "high priority block"
`
			err := os.WriteFile(
				filepath.Join(projectDir, "config.toml"),
				[]byte(projectConfig),
				0o600,
			)
			Expect(err).NotTo(HaveOccurred())

			loader, err := internalconfig.NewKoanfLoaderWithDirs(homeDir, workDir)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.Load(nil)
			Expect(err).NotTo(HaveOccurred())

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
			)

			// High priority rule should be evaluated first
			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("high priority block"))
		})
	})

	Describe("Multi-Validator Interactions", func() {
		It("should apply wildcard validator rules to multiple validators", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "block-all-git",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitAll,
						Remote:        "forbidden",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "all git operations blocked for forbidden remote",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			gitCtxProvider := rules.WithGitContextProvider(func() *rules.GitContext {
				return &rules.GitContext{
					Remote: "forbidden",
				}
			})

			// Test with git.push
			pushAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				gitCtxProvider,
			)
			result := pushAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())

			// Test with git.commit
			commitAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitCommit,
				gitCtxProvider,
			)
			result = commitAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())

			// Test with git.branch
			branchAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitBranch,
				gitCtxProvider,
			)
			result = branchAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
		})

		It("should apply rules based on repository pattern across validators", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "block-secrets-repo",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						RepoPattern: "**/sensitive-repo/**",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "operations blocked for sensitive repo",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			gitCtxProvider := rules.WithGitContextProvider(func() *rules.GitContext {
				return &rules.GitContext{
					RepoRoot: "/home/user/projects/sensitive-repo/src",
				}
			})

			// Test with git.push
			pushAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				gitCtxProvider,
			)
			result := pushAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())

			// Test with secrets validator
			secretsAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorSecrets,
				gitCtxProvider,
			)
			result = secretsAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
		})

		It("should apply different rules to different validators in same engine", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "push-specific",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "push blocked",
					},
				},
				{
					Name:     "commit-specific",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitCommit,
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionWarn,
						Message: "commit warning",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			// Push should be blocked
			pushAdapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)
			result := pushAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("push blocked"))

			// Commit should warn
			commitAdapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitCommit)
			result = commitAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeFalse())
			Expect(result.Message).To(Equal("commit warning"))

			// Branch should have no matching rule
			branchAdapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitBranch)
			result = branchAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})
	})

	Describe("File Validator Rules", func() {
		It("should allow test files to bypass secrets validation", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "allow-test-secrets",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorSecrets,
						FilePattern:   "**/*_test.go",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionAllow,
						Message: "test files allowed",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			// Test file should be allowed
			testAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorSecrets,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Path: "internal/secrets/validator_test.go",
					}
				}),
			)

			result := testAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeTrue())

			// Non-test file should not match
			prodAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorSecrets,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Path: "internal/secrets/validator.go",
					}
				}),
			)

			result = prodAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})

		It("should block markdown validation for specific directories", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "skip-vendor-markdown",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorFileMarkdown,
						FilePattern:   "vendor/**/*.md",
					},
					Action: &rules.RuleAction{
						Type: rules.ActionAllow,
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			// Vendor markdown should be allowed (skipped)
			vendorAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorFileMarkdown,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Path: "vendor/github.com/pkg/README.md",
					}
				}),
			)

			result := vendorAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.Passed).To(BeTrue())

			// Non-vendor markdown should not match
			docsAdapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorFileMarkdown,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Path: "docs/README.md",
					}
				}),
			)

			result = docsAdapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})
	})

	Describe("Complex Pattern Matching", func() {
		It("should match repository patterns with glob wildcards", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "org-repo-rule",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						RepoPattern: "**/github.com/myorg/**",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked myorg repo",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			// Matching repo
			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						RepoRoot: "/home/user/github.com/myorg/project",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())

			// Non-matching repo
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						RepoRoot: "/home/user/github.com/otherorg/project",
					}
				}),
			)

			result = adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})

		It("should match branch patterns with wildcards", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "protect-release-branches",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						BranchPattern: "release/*",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "release branches protected",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			// Matching branch
			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Branch: "release/v1.0.0",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())

			// Non-matching branch
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorGitPush,
				rules.WithGitContextProvider(func() *rules.GitContext {
					return &rules.GitContext{
						Branch: "feat/new-feature",
					}
				}),
			)

			result = adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})

		It("should match command patterns", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "block-force-push",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						CommandPattern: "*--force*",
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "force push not allowed",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)

			// Matching command
			hookCtx := &hook.Context{
				ToolInput: hook.ToolInput{
					Command: "git push --force origin main",
				},
			}

			result := adapter.CheckRules(ctx, hookCtx)
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())

			// Non-matching command
			hookCtx = &hook.Context{
				ToolInput: hook.ToolInput{
					Command: "git push origin main",
				},
			}

			result = adapter.CheckRules(ctx, hookCtx)
			Expect(result).To(BeNil())
		})

		It("should match content patterns with regex", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "block-aws-keys",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ContentPattern: `AKIA[0-9A-Z]{16}`,
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "AWS access key detected",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			// Matching content
			adapter := rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorSecrets,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Content: "aws_key = \"AKIAIOSFODNN7EXAMPLE\"",
					}
				}),
			)

			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Message).To(Equal("AWS access key detected"))

			// Non-matching content
			adapter = rules.NewRuleValidatorAdapter(
				engine,
				rules.ValidatorSecrets,
				rules.WithFileContextProvider(func() *rules.FileContext {
					return &rules.FileContext{
						Content: "no secrets here",
					}
				}),
			)

			result = adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())
		})
	})

	Describe("Disabled Rules", func() {
		It("should skip disabled rules", func() {
			enabled := false

			ruleList := []*rules.Rule{
				{
					Name:     "disabled-rule",
					Priority: 100,
					Enabled:  false,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "should not see this",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)
			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).To(BeNil())

			// Verify using config
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:     "config-disabled-rule",
							Enabled:  &enabled,
							Priority: 100,
						},
					},
				},
			}

			rulesFactory := factory.NewRulesFactory(log)
			engine, err = rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})

		It("should skip rules when engine is disabled globally", func() {
			disabled := false

			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &disabled,
					Rules: []config.RuleConfig{
						{
							Name:     "some-rule",
							Priority: 100,
							Action: &config.RuleActionConfig{
								Type:    "block",
								Message: "blocked",
							},
						},
					},
				},
			}

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})
	})

	Describe("Reference Codes", func() {
		It("should propagate reference codes to validator result", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "rule-with-ref",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{
						Type:      rules.ActionBlock,
						Message:   "blocked with reference",
						Reference: "CUSTOM001",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)
			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(string(result.Reference)).To(Equal("CUSTOM001"))
		})

		It("should handle rules without reference codes", func() {
			ruleList := []*rules.Rule{
				{
					Name:     "rule-no-ref",
					Priority: 100,
					Enabled:  true,
					Match: &rules.RuleMatch{
						ValidatorType: rules.ValidatorGitPush,
					},
					Action: &rules.RuleAction{
						Type:    rules.ActionBlock,
						Message: "blocked without reference",
					},
				},
			}

			engine, err := rules.NewRuleEngine(ruleList)
			Expect(err).NotTo(HaveOccurred())

			adapter := rules.NewRuleValidatorAdapter(engine, rules.ValidatorGitPush)
			result := adapter.CheckRules(ctx, &hook.Context{})
			Expect(result).NotTo(BeNil())
			Expect(result.ShouldBlock).To(BeTrue())
			Expect(result.Reference).To(BeEmpty())
		})
	})
})
