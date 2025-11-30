package configchecker_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	internalconfig "github.com/smykla-labs/klaudiush/internal/config"
	"github.com/smykla-labs/klaudiush/internal/doctor"
	configchecker "github.com/smykla-labs/klaudiush/internal/doctor/checkers/config"
	"github.com/smykla-labs/klaudiush/pkg/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Checker Suite")
}

var _ = Describe("GlobalChecker", func() {
	var (
		ctrl        *gomock.Controller
		mockLoader  *configchecker.MockConfigLoader
		checker     *configchecker.GlobalChecker
		ctx         context.Context
		validConfig *config.Config
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLoader = configchecker.NewMockConfigLoader(ctrl)
		checker = configchecker.NewGlobalCheckerWithLoader(mockLoader)
		ctx = context.Background()
		validConfig = &config.Config{}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Global config"))
		})
	})

	Describe("Category", func() {
		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		Context("when config exists and is valid", func() {
			It("should return pass", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(validConfig, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring("Loaded and validated"))
			})
		})

		Context("when config is missing", func() {
			It("should return warning with fix ID", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(false)
				mockLoader.EXPECT().GlobalConfigPath().Return("/home/user/.klaudiush/config.toml")

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityWarning))
				Expect(result.Message).To(ContainSubstring("Not found"))
				Expect(result.FixID).To(Equal("create_global_config"))
			})
		})

		Context("when config has invalid TOML", func() {
			It("should return error", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrInvalidTOML)
				mockLoader.EXPECT().GlobalConfigPath().Return("/home/user/.klaudiush/config.toml")

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Invalid TOML"))
			})
		})

		Context("when config has invalid permissions", func() {
			It("should return error with fix ID", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrInvalidPermissions)
				mockLoader.EXPECT().GlobalConfigPath().Return("/home/user/.klaudiush/config.toml")

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Insecure file permissions"))
				Expect(result.FixID).To(Equal("fix_config_permissions"))
			})
		})
	})
})

var _ = Describe("ProjectChecker", func() {
	var (
		ctrl        *gomock.Controller
		mockLoader  *configchecker.MockConfigLoader
		checker     *configchecker.ProjectChecker
		ctx         context.Context
		validConfig *config.Config
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLoader = configchecker.NewMockConfigLoader(ctrl)
		checker = configchecker.NewProjectCheckerWithLoader(mockLoader)
		ctx = context.Background()
		validConfig = &config.Config{}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Project config"))
		})
	})

	Describe("Category", func() {
		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		Context("when config exists and is valid", func() {
			It("should return pass", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(validConfig, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring("Loaded and validated"))
			})
		})

		Context("when config is missing", func() {
			It("should return skipped status", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(false)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusSkipped))
				Expect(result.Message).To(ContainSubstring("Not found"))
			})
		})

		Context("when config has invalid TOML", func() {
			It("should return error", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrInvalidTOML)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Invalid TOML"))
			})
		})

		Context("when config has invalid permissions", func() {
			It("should return error with fix ID", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrInvalidPermissions)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Insecure file permissions"))
				Expect(result.FixID).To(Equal("fix_config_permissions"))
			})
		})

		Context("when config has rules validation error", func() {
			It("should return error with fix ID for ErrEmptyMatchConditions", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrEmptyMatchConditions)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Invalid rules configuration"))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})

			It("should return error with fix ID for ErrInvalidRule", func() {
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrInvalidRule)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Invalid rules configuration"))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})

			It("should return error with fix ID for generic rule error message", func() {
				err := errors.New("rule has invalid match condition")
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, err)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Invalid rules configuration"))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})

			It("should return error with fix ID for empty rule match error", func() {
				err := errors.New("rule has empty match section")
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, err)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Message).To(ContainSubstring("Invalid rules configuration"))
				Expect(result.FixID).To(Equal("fix_invalid_rules"))
			})
		})

		Context("when config has generic load error", func() {
			It("should return error without fix ID", func() {
				err := errors.New("some generic error")
				mockLoader.EXPECT().HasProjectConfig().Return(true)
				mockLoader.EXPECT().Load(nil).Return(nil, err)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Failed to load"))
				Expect(result.FixID).To(BeEmpty())
			})
		})
	})
})

var _ = Describe("PermissionsChecker", func() {
	var (
		ctrl       *gomock.Controller
		mockLoader *configchecker.MockConfigLoader
		checker    *configchecker.PermissionsChecker
		ctx        context.Context
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockLoader = configchecker.NewMockConfigLoader(ctrl)
		checker = configchecker.NewPermissionsCheckerWithLoader(mockLoader)
		ctx = context.Background()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("Name", func() {
		It("should return the correct name", func() {
			Expect(checker.Name()).To(Equal("Config permissions"))
		})
	})

	Describe("Category", func() {
		It("should return config category", func() {
			Expect(checker.Category()).To(Equal(doctor.CategoryConfig))
		})
	})

	Describe("Check", func() {
		Context("when no config files exist", func() {
			It("should skip the check", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(false)
				mockLoader.EXPECT().HasProjectConfig().Return(false)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusSkipped))
				Expect(result.Message).To(ContainSubstring("No config files found"))
			})
		})

		Context("when config files exist with correct permissions", func() {
			It("should return pass", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(true)
				mockLoader.EXPECT().HasProjectConfig().Return(false)
				mockLoader.EXPECT().Load(nil).Return(&config.Config{}, nil)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusPass))
				Expect(result.Message).To(ContainSubstring("secured"))
			})
		})

		Context("when config files have insecure permissions", func() {
			It("should return error with fix ID", func() {
				mockLoader.EXPECT().HasGlobalConfig().Return(true)
				mockLoader.EXPECT().HasProjectConfig().Return(false)
				mockLoader.EXPECT().Load(nil).Return(nil, internalconfig.ErrInvalidPermissions)

				result := checker.Check(ctx)

				Expect(result.Status).To(Equal(doctor.StatusFail))
				Expect(result.Severity).To(Equal(doctor.SeverityError))
				Expect(result.Message).To(ContainSubstring("Insecure file permissions"))
				Expect(result.FixID).To(Equal("fix_config_permissions"))
			})
		})
	})
})
