package config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

func TestConfigRules(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Rules Suite")
}

var _ = Describe("RulesConfig", func() {
	Describe("IsEnabled", func() {
		It("should return true when Enabled is nil", func() {
			cfg := &config.RulesConfig{}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("should return true when Enabled is true", func() {
			enabled := true
			cfg := &config.RulesConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("should return false when Enabled is false", func() {
			enabled := false
			cfg := &config.RulesConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeFalse())
		})

		It("should return true for nil RulesConfig", func() {
			var cfg *config.RulesConfig
			Expect(cfg.IsEnabled()).To(BeTrue())
		})
	})

	Describe("ShouldStopOnFirstMatch", func() {
		It("should return true when StopOnFirstMatch is nil", func() {
			cfg := &config.RulesConfig{}
			Expect(cfg.ShouldStopOnFirstMatch()).To(BeTrue())
		})

		It("should return true when StopOnFirstMatch is true", func() {
			stop := true
			cfg := &config.RulesConfig{StopOnFirstMatch: &stop}
			Expect(cfg.ShouldStopOnFirstMatch()).To(BeTrue())
		})

		It("should return false when StopOnFirstMatch is false", func() {
			stop := false
			cfg := &config.RulesConfig{StopOnFirstMatch: &stop}
			Expect(cfg.ShouldStopOnFirstMatch()).To(BeFalse())
		})

		It("should return true for nil RulesConfig", func() {
			var cfg *config.RulesConfig
			Expect(cfg.ShouldStopOnFirstMatch()).To(BeTrue())
		})
	})
})

var _ = Describe("RuleConfig", func() {
	Describe("IsRuleEnabled", func() {
		It("should return true when Enabled is nil", func() {
			cfg := config.RuleConfig{}
			Expect(cfg.IsRuleEnabled()).To(BeTrue())
		})

		It("should return true when Enabled is true", func() {
			enabled := true
			cfg := config.RuleConfig{Enabled: &enabled}
			Expect(cfg.IsRuleEnabled()).To(BeTrue())
		})

		It("should return false when Enabled is false", func() {
			enabled := false
			cfg := config.RuleConfig{Enabled: &enabled}
			Expect(cfg.IsRuleEnabled()).To(BeFalse())
		})
	})
})

var _ = Describe("RuleActionConfig", func() {
	Describe("GetActionType", func() {
		It("should return 'block' when Type is empty", func() {
			cfg := &config.RuleActionConfig{}
			Expect(cfg.GetActionType()).To(Equal("block"))
		})

		It("should return 'block' for nil config", func() {
			var cfg *config.RuleActionConfig
			Expect(cfg.GetActionType()).To(Equal("block"))
		})

		It("should return the configured type", func() {
			cfg := &config.RuleActionConfig{Type: "warn"}
			Expect(cfg.GetActionType()).To(Equal("warn"))
		})

		It("should return 'allow' when configured", func() {
			cfg := &config.RuleActionConfig{Type: "allow"}
			Expect(cfg.GetActionType()).To(Equal("allow"))
		})
	})
})

var _ = Describe("Config", func() {
	Describe("GetValidators", func() {
		It("should create validators config when nil", func() {
			cfg := &config.Config{}
			validators := cfg.GetValidators()
			Expect(validators).NotTo(BeNil())
			Expect(cfg.Validators).NotTo(BeNil())
		})

		It("should return existing validators config", func() {
			gitCfg := &config.GitConfig{}
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{Git: gitCfg},
			}
			validators := cfg.GetValidators()
			Expect(validators.Git).To(Equal(gitCfg))
		})
	})

	Describe("GetGlobal", func() {
		It("should create global config when nil", func() {
			cfg := &config.Config{}
			global := cfg.GetGlobal()
			Expect(global).NotTo(BeNil())
			Expect(cfg.Global).NotTo(BeNil())
		})

		It("should return existing global config", func() {
			useSDK := true
			cfg := &config.Config{
				Global: &config.GlobalConfig{UseSDKGit: &useSDK},
			}
			global := cfg.GetGlobal()
			Expect(*global.UseSDKGit).To(BeTrue())
		})
	})

	Describe("GetPlugins", func() {
		It("should create plugins config when nil", func() {
			cfg := &config.Config{}
			plugins := cfg.GetPlugins()
			Expect(plugins).NotTo(BeNil())
			Expect(cfg.Plugins).NotTo(BeNil())
		})

		It("should return existing plugins config", func() {
			cfg := &config.Config{
				Plugins: &config.PluginConfig{},
			}
			plugins := cfg.GetPlugins()
			Expect(plugins).NotTo(BeNil())
		})
	})

	Describe("GetRules", func() {
		It("should create rules config when nil", func() {
			cfg := &config.Config{}
			rules := cfg.GetRules()
			Expect(rules).NotTo(BeNil())
			Expect(cfg.Rules).NotTo(BeNil())
		})

		It("should return existing rules config", func() {
			enabled := true
			cfg := &config.Config{
				Rules: &config.RulesConfig{Enabled: &enabled},
			}
			rules := cfg.GetRules()
			Expect(rules.IsEnabled()).To(BeTrue())
		})
	})
})

var _ = Describe("ValidatorsConfig", func() {
	Describe("GetGit", func() {
		It("should create git config when nil", func() {
			cfg := &config.ValidatorsConfig{}
			git := cfg.GetGit()
			Expect(git).NotTo(BeNil())
			Expect(cfg.Git).NotTo(BeNil())
		})

		It("should return existing git config", func() {
			commit := &config.CommitValidatorConfig{}
			cfg := &config.ValidatorsConfig{
				Git: &config.GitConfig{Commit: commit},
			}
			git := cfg.GetGit()
			Expect(git.Commit).To(Equal(commit))
		})
	})

	Describe("GetGitHub", func() {
		It("should create GitHub config when nil", func() {
			cfg := &config.ValidatorsConfig{}
			github := cfg.GetGitHub()
			Expect(github).NotTo(BeNil())
			Expect(cfg.GitHub).NotTo(BeNil())
		})

		It("should return existing GitHub config", func() {
			issue := &config.IssueValidatorConfig{}
			cfg := &config.ValidatorsConfig{
				GitHub: &config.GitHubConfig{Issue: issue},
			}
			github := cfg.GetGitHub()
			Expect(github.Issue).To(Equal(issue))
		})
	})

	Describe("GetFile", func() {
		It("should create file config when nil", func() {
			cfg := &config.ValidatorsConfig{}
			file := cfg.GetFile()
			Expect(file).NotTo(BeNil())
			Expect(cfg.File).NotTo(BeNil())
		})

		It("should return existing file config", func() {
			markdown := &config.MarkdownValidatorConfig{}
			cfg := &config.ValidatorsConfig{
				File: &config.FileConfig{Markdown: markdown},
			}
			file := cfg.GetFile()
			Expect(file.Markdown).To(Equal(markdown))
		})
	})

	Describe("GetNotification", func() {
		It("should create notification config when nil", func() {
			cfg := &config.ValidatorsConfig{}
			notification := cfg.GetNotification()
			Expect(notification).NotTo(BeNil())
			Expect(cfg.Notification).NotTo(BeNil())
		})

		It("should return existing notification config", func() {
			bell := &config.BellValidatorConfig{}
			cfg := &config.ValidatorsConfig{
				Notification: &config.NotificationConfig{Bell: bell},
			}
			notification := cfg.GetNotification()
			Expect(notification.Bell).To(Equal(bell))
		})
	})

	Describe("GetSecrets", func() {
		It("should create secrets config when nil", func() {
			cfg := &config.ValidatorsConfig{}
			secrets := cfg.GetSecrets()
			Expect(secrets).NotTo(BeNil())
			Expect(cfg.Secrets).NotTo(BeNil())
		})

		It("should return existing secrets config", func() {
			secretsValidator := &config.SecretsValidatorConfig{}
			cfg := &config.ValidatorsConfig{
				Secrets: &config.SecretsConfig{Secrets: secretsValidator},
			}
			secrets := cfg.GetSecrets()
			Expect(secrets.Secrets).To(Equal(secretsValidator))
		})
	})
})

var _ = Describe("GlobalConfig", func() {
	Describe("IsParallelExecutionEnabled", func() {
		It("should return false when GlobalConfig is nil", func() {
			var cfg *config.GlobalConfig
			Expect(cfg.IsParallelExecutionEnabled()).To(BeFalse())
		})

		It("should return false when ParallelExecution is nil", func() {
			cfg := &config.GlobalConfig{}
			Expect(cfg.IsParallelExecutionEnabled()).To(BeFalse())
		})

		It("should return true when ParallelExecution is true", func() {
			enabled := true
			cfg := &config.GlobalConfig{ParallelExecution: &enabled}
			Expect(cfg.IsParallelExecutionEnabled()).To(BeTrue())
		})

		It("should return false when ParallelExecution is false", func() {
			enabled := false
			cfg := &config.GlobalConfig{ParallelExecution: &enabled}
			Expect(cfg.IsParallelExecutionEnabled()).To(BeFalse())
		})
	})
})
