package factory_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/config/factory"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("FileValidatorFactory", func() {
	var (
		fileFactory *factory.FileValidatorFactory
		log         logger.Logger
		cfg         *config.Config
	)

	BeforeEach(func() {
		log = logger.NewNoOpLogger()
		fileFactory = factory.NewFileValidatorFactory(log)
		cfg = &config.Config{
			Validators: &config.ValidatorsConfig{
				File: &config.FileConfig{},
			},
		}
	})

	Describe("CreateValidators", func() {
		Context("Python validator", func() {
			It("should create Python validator when enabled", func() {
				enabled := true
				cfg.Validators.File.Python = &config.PythonValidatorConfig{
					ValidatorConfig: config.ValidatorConfig{Enabled: &enabled},
				}

				validators := fileFactory.CreateValidators(cfg)
				Expect(len(validators)).To(BeNumerically(">=", 1))
			})

			It("should not create Python validator when disabled", func() {
				enabled := false
				cfg.Validators.File.Python = &config.PythonValidatorConfig{
					ValidatorConfig: config.ValidatorConfig{Enabled: &enabled},
				}

				validators := fileFactory.CreateValidators(cfg)
				// Count Python validators (should be 0)
				pythonValidatorCount := 0
				for _, v := range validators {
					if v.Validator != nil {
						pythonValidatorCount++
					}
				}
				Expect(pythonValidatorCount).To(Equal(0))
			})

			It("should create Python validator with custom config", func() {
				enabled := true
				useRuff := true
				contextLines := 5
				cfg.Validators.File.Python = &config.PythonValidatorConfig{
					ValidatorConfig: config.ValidatorConfig{Enabled: &enabled},
					UseRuff:         &useRuff,
					ContextLines:    &contextLines,
					RuffConfig:      "/path/to/ruff.toml",
					ExcludeRules:    []string{"F401"},
				}

				validators := fileFactory.CreateValidators(cfg)
				Expect(len(validators)).To(BeNumerically(">=", 1))
			})

			It("should handle nil Python config without crashing", func() {
				cfg.Validators.File.Python = nil

				validators := fileFactory.CreateValidators(cfg)
				// Should not crash, returns empty or validators for other file types
				Expect(validators).To(BeEmpty())
			})
		})

		Context("Multiple file validators", func() {
			It("should create multiple validators when enabled", func() {
				enabled := true
				cfg.Validators.File.Python = &config.PythonValidatorConfig{
					ValidatorConfig: config.ValidatorConfig{Enabled: &enabled},
				}
				cfg.Validators.File.Markdown = &config.MarkdownValidatorConfig{
					ValidatorConfig: config.ValidatorConfig{Enabled: &enabled},
				}
				cfg.Validators.File.ShellScript = &config.ShellScriptValidatorConfig{
					ValidatorConfig: config.ValidatorConfig{Enabled: &enabled},
				}

				validators := fileFactory.CreateValidators(cfg)
				Expect(len(validators)).To(BeNumerically(">=", 3))
			})
		})
	})

	Describe("SetRuleEngine", func() {
		It("should set rule engine without error", func() {
			// Should not panic
			fileFactory.SetRuleEngine(nil)
		})
	})
})
