package config

import (
	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// Tests are run as part of Koanf Rules Suite from koanf_rules_test.go.

var _ = Describe("Validator", func() {
	var validator *Validator

	BeforeEach(func() {
		validator = NewValidator()
	})

	Describe("NewValidator", func() {
		It("should create a new validator", func() {
			v := NewValidator()
			Expect(v).NotTo(BeNil())
		})
	})

	Describe("Validate", func() {
		It("should return error when config is nil", func() {
			err := validator.Validate(nil)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("config is nil"))
		})

		It("should pass validation for empty config", func() {
			cfg := &config.Config{}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass validation for valid config", func() {
			cfg := DefaultConfig()
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should collect multiple validation errors", func() {
			negativeLength := -1
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								TitleMaxLength:    &negativeLength,
								BodyMaxLineLength: &negativeLength,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})
	})

	Describe("validateGitConfig", func() {
		It("should pass with nil config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: nil,
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass with empty git config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate push config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Push: &config.PushValidatorConfig{},
					},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate add config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Add: &config.AddValidatorConfig{},
					},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate branch config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Branch: &config.BranchValidatorConfig{},
					},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should validate no_verify config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						NoVerify: &config.NoVerifyValidatorConfig{},
					},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateCommitMessageConfig", func() {
		It("should reject negative title_max_length", func() {
			negativeLength := -5
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								TitleMaxLength: &negativeLength,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should reject zero title_max_length", func() {
			zeroLength := 0
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								TitleMaxLength: &zeroLength,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should reject negative body_max_line_length", func() {
			negativeLength := -1
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								BodyMaxLineLength: &negativeLength,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})

		It("should reject negative body_line_tolerance", func() {
			negativeTolerance := -1
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								BodyLineTolerance: &negativeTolerance,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})

		It("should allow zero body_line_tolerance", func() {
			zeroTolerance := 0
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								BodyLineTolerance: &zeroTolerance,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject empty string in valid_types", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								ValidTypes: []string{"feat", "", "fix"},
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should accept valid_types without empty strings", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							Message: &config.CommitMessageConfig{
								ValidTypes: []string{"feat", "fix", "chore"},
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validatePRConfig", func() {
		It("should reject negative title_max_length", func() {
			negativeLength := -10
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						PR: &config.PRValidatorConfig{
							TitleMaxLength: &negativeLength,
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})

		It("should reject empty string in valid_types", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						PR: &config.PRValidatorConfig{
							ValidTypes: []string{"feat", ""},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("validateBranchConfig", func() {
		It("should reject empty string in valid_types", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Branch: &config.BranchValidatorConfig{
							ValidTypes: []string{"feature", ""},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should accept valid_types without empty strings", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Branch: &config.BranchValidatorConfig{
							ValidTypes: []string{"feature", "bugfix", "hotfix"},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateFileConfig", func() {
		It("should pass with nil file config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: nil,
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass with empty file config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateMarkdownConfig", func() {
		It("should reject negative context_lines", func() {
			negativeContext := -1
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ContextLines: &negativeContext,
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})

		It("should allow zero context_lines", func() {
			zeroContext := 0
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Markdown: &config.MarkdownValidatorConfig{
							ContextLines: &zeroContext,
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateShellScriptConfig", func() {
		It("should reject negative context_lines", func() {
			negativeContext := -1
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						ShellScript: &config.ShellScriptValidatorConfig{
							ContextLines: &negativeContext,
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})

		It("should reject invalid shellcheck_severity", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						ShellScript: &config.ShellScriptValidatorConfig{
							ShellcheckSeverity: "invalid",
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should accept valid shellcheck_severity values", func() {
			for _, severity := range []string{"error", "warning", "info", "style"} {
				cfg := &config.Config{
					Validators: &config.ValidatorsConfig{
						File: &config.FileConfig{
							ShellScript: &config.ShellScriptValidatorConfig{
								ShellcheckSeverity: severity,
							},
						},
					},
				}

				err := validator.Validate(cfg)
				Expect(err).NotTo(HaveOccurred(), "severity %q should be valid", severity)
			}
		})
	})

	Describe("validateTerraformConfig", func() {
		It("should reject negative context_lines", func() {
			negativeContext := -1
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Terraform: &config.TerraformValidatorConfig{
							ContextLines: &negativeContext,
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
		})

		It("should reject invalid tool_preference", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Terraform: &config.TerraformValidatorConfig{
							ToolPreference: "invalid",
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should accept valid tool_preference values", func() {
			for _, pref := range []string{"tofu", "terraform", "auto"} {
				cfg := &config.Config{
					Validators: &config.ValidatorsConfig{
						File: &config.FileConfig{
							Terraform: &config.TerraformValidatorConfig{
								ToolPreference: pref,
							},
						},
					},
				}

				err := validator.Validate(cfg)
				Expect(err).NotTo(HaveOccurred(), "tool_preference %q should be valid", pref)
			}
		})
	})

	Describe("validateWorkflowConfig", func() {
		It("should pass with valid workflow config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					File: &config.FileConfig{
						Workflow: &config.WorkflowValidatorConfig{},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateNotificationConfig", func() {
		It("should pass with nil notification config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Notification: nil,
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass with empty notification config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Notification: &config.NotificationConfig{},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass with valid bell config", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Notification: &config.NotificationConfig{
						Bell: &config.BellValidatorConfig{},
					},
				},
			}
			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("validateBaseConfig", func() {
		It("should reject invalid severity", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{
								Severity: config.Severity(99),
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrInvalidConfig)).To(BeTrue())
		})

		It("should accept SeverityError", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{
								Severity: config.SeverityError,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept SeverityWarning", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{
								Severity: config.SeverityWarning,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept SeverityUnknown (default)", func() {
			cfg := &config.Config{
				Validators: &config.ValidatorsConfig{
					Git: &config.GitConfig{
						Commit: &config.CommitValidatorConfig{
							ValidatorConfig: config.ValidatorConfig{
								Severity: config.SeverityUnknown,
							},
						},
					},
				},
			}

			err := validator.Validate(cfg)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("combineErrors", func() {
		It("should return nil for empty slice", func() {
			err := combineErrors(nil)
			Expect(err).To(BeNil())
		})

		It("should return single error unchanged", func() {
			singleErr := errors.New("single error")
			err := combineErrors([]error{singleErr})
			Expect(err).To(Equal(singleErr))
		})

		It("should join multiple errors", func() {
			err1 := errors.New("error 1")
			err2 := errors.New("error 2")
			err := combineErrors([]error{err1, err2})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, err1)).To(BeTrue())
			Expect(errors.Is(err, err2)).To(BeTrue())
		})
	})
})
