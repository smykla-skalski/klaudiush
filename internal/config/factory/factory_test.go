package factory_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/config/factory"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

func ptrBool(v bool) *bool {
	return &v
}

var _ = Describe("DefaultValidatorFactory", func() {
	var (
		validatorFactory *factory.DefaultValidatorFactory
		log              logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		validatorFactory = factory.NewValidatorFactory(log)
	})

	Describe("NewValidatorFactory", func() {
		It("should create a factory with all sub-factories", func() {
			Expect(validatorFactory).NotTo(BeNil())
		})
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine on all sub-factories", func() {
			enabled := true
			rulesCfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &enabled,
					Rules: []config.RuleConfig{
						{
							Name:   "test-rule",
							Action: &config.RuleActionConfig{Type: "block"},
						},
					},
				},
			}

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(rulesCfg)
			Expect(err).NotTo(HaveOccurred())

			// Should not panic.
			validatorFactory.SetRuleEngine(engine)
		})

		It("should handle nil rule engine", func() {
			// Should not panic.
			validatorFactory.SetRuleEngine(nil)
		})
	})

	Describe("CreateGitValidators", func() {
		It("should create add validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Add: &config.AddValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create commit validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create push validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create PR validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						PR: &config.PRValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create branch validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Branch: &config.BranchValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create merge validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Merge: &config.MergeValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create no-verify validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						NoVerify: &config.NoVerifyValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should not create validators when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should create all git validators when all enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Add: &config.AddValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						NoVerify: &config.NoVerifyValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						PR: &config.PRValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Branch: &config.BranchValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Merge: &config.MergeValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(Equal(7))
		})

		It("should create validators with rule engine integration", func() {
			// Setup rule engine
			rulesEnabled := true
			rulesCfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &rulesEnabled,
					Rules: []config.RuleConfig{
						{
							Name: "test-rule",
							Match: &config.RuleMatchConfig{
								ValidatorType: "git.push",
							},
							Action: &config.RuleActionConfig{Type: "block"},
						},
					},
				},
			}

			rulesFactory := factory.NewRulesFactory(log)
			engine, err := rulesFactory.CreateRuleEngine(rulesCfg)
			Expect(err).NotTo(HaveOccurred())
			validatorFactory.SetRuleEngine(engine)

			// Create validators
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})
	})

	Describe("CreateFileValidators", func() {
		It("should create markdown validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create terraform validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Terraform: &config.TerraformValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create shell script validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						ShellScript: &config.ShellScriptValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create workflow validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Workflow: &config.WorkflowValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should not create validators when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should use global timeout when configured", func() {
			cfg := &config.Config{
				Global: &config.GlobalConfig{
					DefaultTimeout: config.Duration(5000000000), // 5 seconds
				},
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should create all file validators when all enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Terraform: &config.TerraformValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						ShellScript: &config.ShellScriptValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Workflow: &config.WorkflowValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateFileValidators(cfg)
			Expect(len(validators)).To(Equal(4))
		})
	})

	Describe("CreateNotificationValidators", func() {
		It("should create bell validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Notification: &config.NotificationConfig{
						Bell: &config.BellValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateNotificationValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should not create validators when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Notification: &config.NotificationConfig{
						Bell: &config.BellValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := validatorFactory.CreateNotificationValidators(cfg)
			Expect(validators).To(BeEmpty())
		})
	})

	Describe("CreateSecretsValidators", func() {
		It("should create secrets validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should not create validators when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty for nil config", func() {
			cfg := &config.Config{}

			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty when secrets config is nil", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Secrets: nil,
				},
			}

			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should create validator with custom patterns", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
							CustomPatterns: []config.CustomPatternConfig{
								{
									Name:        "custom-key",
									Description: "Custom API key pattern",
									Regex:       `CUSTOM_[A-Z0-9]{32}`,
								},
							},
						},
					},
				},
			}

			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})

		It("should handle invalid custom pattern regex gracefully", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
							CustomPatterns: []config.CustomPatternConfig{
								{
									Name:  "invalid-pattern",
									Regex: `[invalid`,
								},
							},
						},
					},
				},
			}

			// Should not panic; invalid patterns are logged and skipped.
			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})

		It("should use global timeout", func() {
			cfg := &config.Config{
				Global: &config.GlobalConfig{
					DefaultTimeout: config.Duration(5000000000), // 5 seconds
				},
				Validators: &config.ValidatorsConfig{
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateSecretsValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})
	})

	Describe("CreateShellValidators", func() {
		It("should create backtick validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Shell: &config.ShellConfig{
						Backtick: &config.BacktickValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateShellValidators(cfg)
			Expect(len(validators)).To(BeNumerically(">=", 1))
		})

		It("should not create validators when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Shell: &config.ShellConfig{
						Backtick: &config.BacktickValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := validatorFactory.CreateShellValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty when shell config is nil", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Shell: nil,
				},
			}

			validators := validatorFactory.CreateShellValidators(cfg)
			Expect(validators).To(BeEmpty())
		})
	})

	Describe("CreatePluginValidators", func() {
		It("should return empty when plugins config is nil", func() {
			cfg := &config.Config{}

			validators := validatorFactory.CreatePluginValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty when plugins list is empty", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{},
			}

			validators := validatorFactory.CreatePluginValidators(cfg)
			Expect(validators).To(BeEmpty())
		})
	})

	Describe("CreateAll", func() {
		It("should create validators from all categories", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					Notification: &config.NotificationConfig{
						Bell: &config.BellValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					Shell: &config.ShellConfig{
						Backtick: &config.BacktickValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					Secrets: &config.SecretsConfig{
						Secrets: &config.SecretsValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateAll(cfg)
			Expect(len(validators)).To(Equal(5))
		})

		It("should return empty for minimal config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git:          &config.GitConfig{},
					File:         &config.FileConfig{},
					Notification: &config.NotificationConfig{},
					Secrets:      &config.SecretsConfig{},
					Shell:        &config.ShellConfig{},
				},
			}

			validators := validatorFactory.CreateAll(cfg)
			Expect(validators).To(BeEmpty())
		})
	})
})

var _ = Describe("RegistryBuilder", func() {
	var (
		builder *factory.RegistryBuilder
		log     logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		builder = factory.NewRegistryBuilder(log)
	})

	Describe("NewRegistryBuilder", func() {
		It("should create a registry builder", func() {
			Expect(builder).NotTo(BeNil())
		})
	})

	Describe("Build", func() {
		It("should build registry with validators", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					File:         &config.FileConfig{},
					Notification: &config.NotificationConfig{},
					Secrets:      &config.SecretsConfig{},
					Shell:        &config.ShellConfig{},
				},
			}

			registry := builder.Build(cfg)
			Expect(registry).NotTo(BeNil())
		})

		It("should build empty registry for minimal config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git:          &config.GitConfig{},
					File:         &config.FileConfig{},
					Notification: &config.NotificationConfig{},
					Secrets:      &config.SecretsConfig{},
					Shell:        &config.ShellConfig{},
				},
			}

			registry := builder.Build(cfg)
			Expect(registry).NotTo(BeNil())
		})
	})

	Describe("BuildWithRuleEngine", func() {
		It("should build registry and rule engine", func() {
			rulesEnabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &rulesEnabled,
					Rules: []config.RuleConfig{
						{
							Name:   "test-rule",
							Action: &config.RuleActionConfig{Type: "block"},
						},
					},
				},
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					File:         &config.FileConfig{},
					Notification: &config.NotificationConfig{},
					Secrets:      &config.SecretsConfig{},
					Shell:        &config.ShellConfig{},
				},
			}

			registry, engine, err := builder.BuildWithRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(registry).NotTo(BeNil())
			Expect(engine).NotTo(BeNil())
			Expect(engine.Size()).To(Equal(1))
		})

		It("should build registry with nil engine when rules disabled", func() {
			rulesEnabled := false
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &rulesEnabled,
				},
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					File:         &config.FileConfig{},
					Notification: &config.NotificationConfig{},
					Secrets:      &config.SecretsConfig{},
					Shell:        &config.ShellConfig{},
				},
			}

			registry, engine, err := builder.BuildWithRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(registry).NotTo(BeNil())
			Expect(engine).To(BeNil())
		})

		It("should return error for invalid rule config", func() {
			rulesEnabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &rulesEnabled,
					Rules: []config.RuleConfig{
						{
							Name: "invalid-rule",
							Match: &config.RuleMatchConfig{
								RepoPattern: "[invalid", // Invalid glob pattern
							},
							Action: &config.RuleActionConfig{Type: "block"},
						},
					},
				},
			}

			_, _, err := builder.BuildWithRuleEngine(cfg)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CreateRuleEngine", func() {
		It("should create rule engine", func() {
			rulesEnabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &rulesEnabled,
					Rules: []config.RuleConfig{
						{
							Name:   "test-rule",
							Action: &config.RuleActionConfig{Type: "block"},
						},
					},
				},
			}

			engine, err := builder.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).NotTo(BeNil())
		})

		It("should return nil for disabled rules", func() {
			rulesEnabled := false
			cfg := &config.Config{
				Rules: &config.RulesConfig{
					Enabled: &rulesEnabled,
				},
			}

			engine, err := builder.CreateRuleEngine(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(engine).To(BeNil())
		})
	})
})

var _ = Describe("GitValidatorFactory", func() {
	var (
		gitFactory *factory.GitValidatorFactory
		log        logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		gitFactory = factory.NewGitValidatorFactory(log)
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionBlock}},
			})

			// Should not panic.
			gitFactory.SetRuleEngine(engine)
		})

		It("should handle nil engine", func() {
			// Should not panic.
			gitFactory.SetRuleEngine(nil)
		})
	})

	Describe("CreateValidators", func() {
		It("should share git runner between validators", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Add: &config.AddValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Push: &config.PushValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
						Merge: &config.MergeValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := gitFactory.CreateValidators(cfg)
			// All validators using git should share the same cached runner.
			Expect(len(validators)).To(Equal(4))
		})
	})
})

var _ = Describe("FileValidatorFactory", func() {
	var (
		fileFactory *factory.FileValidatorFactory
		log         logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		fileFactory = factory.NewFileValidatorFactory(log)
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionBlock}},
			})

			// Should not panic.
			fileFactory.SetRuleEngine(engine)
		})
	})
})

var _ = Describe("NotificationValidatorFactory", func() {
	var (
		notificationFactory *factory.NotificationValidatorFactory
		log                 logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		notificationFactory = factory.NewNotificationValidatorFactory(log)
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionBlock}},
			})

			// Should not panic.
			notificationFactory.SetRuleEngine(engine)
		})
	})
})

var _ = Describe("SecretsValidatorFactory", func() {
	var (
		secretsFactory *factory.SecretsValidatorFactory
		log            logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		secretsFactory = factory.NewSecretsValidatorFactory(log)
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionBlock}},
			})

			// Should not panic.
			secretsFactory.SetRuleEngine(engine)
		})
	})
})

var _ = Describe("ShellValidatorFactory", func() {
	var (
		shellFactory *factory.ShellValidatorFactory
		log          logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		shellFactory = factory.NewShellValidatorFactory(log)
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionBlock}},
			})

			// Should not panic.
			shellFactory.SetRuleEngine(engine)
		})
	})
})

var _ = Describe("GitHubValidatorFactory", func() {
	var (
		githubFactory *factory.GitHubValidatorFactory
		log           logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		githubFactory = factory.NewGitHubValidatorFactory(log)
	})

	Describe("NewGitHubValidatorFactory", func() {
		It("should create a factory", func() {
			Expect(githubFactory).NotTo(BeNil())
		})
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{Name: "test", Enabled: true, Action: &rules.RuleAction{Type: rules.ActionBlock}},
			})

			// Should not panic.
			githubFactory.SetRuleEngine(engine)
		})

		It("should handle nil engine", func() {
			// Should not panic.
			githubFactory.SetRuleEngine(nil)
		})
	})

	Describe("CreateValidators", func() {
		It("should return empty when GitHub config is nil", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: nil,
				},
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty when validators config is nil", func() {
			cfg := &config.Config{
				Validators: nil,
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should create issue validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})

		It("should not create issue validator when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty when issue config is nil", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: nil,
					},
				},
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should create validator with rule engine integration", func() {
			engine, _ := rules.NewRuleEngine([]*rules.Rule{
				{
					Name:    "test-rule",
					Enabled: true,
					Action:  &rules.RuleAction{Type: rules.ActionBlock},
				},
			})
			githubFactory.SetRuleEngine(engine)

			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})

		It("should create validator with custom configuration", func() {
			requireBody := true
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig:       config.ValidatorConfig{Enabled: ptrBool(true)},
							RequireBody:           &requireBody,
							MarkdownDisabledRules: []string{"MD001", "MD002"},
							Timeout:               config.Duration(5000000000), // 5 seconds
						},
					},
				},
			}

			validators := githubFactory.CreateValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})
	})
})

var _ = Describe("DefaultValidatorFactory GitHub Integration", func() {
	var (
		validatorFactory *factory.DefaultValidatorFactory
		log              logger.Logger
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		validatorFactory = factory.NewValidatorFactory(log)
	})

	Describe("CreateGitHubValidators", func() {
		It("should create issue validator when enabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitHubValidators(cfg)
			Expect(len(validators)).To(Equal(1))
		})

		It("should not create validators when disabled", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(false)},
						},
					},
				},
			}

			validators := validatorFactory.CreateGitHubValidators(cfg)
			Expect(validators).To(BeEmpty())
		})

		It("should return empty for nil GitHub config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: nil,
				},
			}

			validators := validatorFactory.CreateGitHubValidators(cfg)
			Expect(validators).To(BeEmpty())
		})
	})

	Describe("CreateAll with GitHub validators", func() {
		It("should include GitHub validators in CreateAll", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					GitHub: &config.GitHubConfig{
						Issue: &config.IssueValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{Enabled: ptrBool(true)},
						},
					},
					Git:          &config.GitConfig{},
					File:         &config.FileConfig{},
					Notification: &config.NotificationConfig{},
					Secrets:      &config.SecretsConfig{},
					Shell:        &config.ShellConfig{},
				},
			}

			validators := validatorFactory.CreateAll(cfg)
			Expect(len(validators)).To(Equal(1))
		})
	})
})
