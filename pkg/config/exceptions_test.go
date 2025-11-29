package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// Tests are run as part of Config Rules Suite from rules_test.go.

var _ = Describe("ExceptionsConfig", func() {
	Describe("IsEnabled", func() {
		It("should return true when Enabled is nil", func() {
			cfg := &config.ExceptionsConfig{}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("should return true when Enabled is true", func() {
			enabled := true
			cfg := &config.ExceptionsConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeTrue())
		})

		It("should return false when Enabled is false", func() {
			enabled := false
			cfg := &config.ExceptionsConfig{Enabled: &enabled}
			Expect(cfg.IsEnabled()).To(BeFalse())
		})

		It("should return true for nil ExceptionsConfig", func() {
			var cfg *config.ExceptionsConfig
			Expect(cfg.IsEnabled()).To(BeTrue())
		})
	})

	Describe("GetTokenPrefix", func() {
		It("should return 'EXC' when TokenPrefix is empty", func() {
			cfg := &config.ExceptionsConfig{}
			Expect(cfg.GetTokenPrefix()).To(Equal("EXC"))
		})

		It("should return 'EXC' for nil config", func() {
			var cfg *config.ExceptionsConfig
			Expect(cfg.GetTokenPrefix()).To(Equal("EXC"))
		})

		It("should return the configured prefix", func() {
			cfg := &config.ExceptionsConfig{TokenPrefix: "ACK"}
			Expect(cfg.GetTokenPrefix()).To(Equal("ACK"))
		})
	})

	Describe("GetPolicy", func() {
		It("should return nil for nil config", func() {
			var cfg *config.ExceptionsConfig
			Expect(cfg.GetPolicy("GIT001")).To(BeNil())
		})

		It("should return nil when policies is nil", func() {
			cfg := &config.ExceptionsConfig{}
			Expect(cfg.GetPolicy("GIT001")).To(BeNil())
		})

		It("should return nil when policy doesn't exist", func() {
			cfg := &config.ExceptionsConfig{
				Policies: map[string]*config.ExceptionPolicyConfig{},
			}
			Expect(cfg.GetPolicy("GIT001")).To(BeNil())
		})

		It("should return the policy when it exists", func() {
			policy := &config.ExceptionPolicyConfig{Description: "test policy"}
			cfg := &config.ExceptionsConfig{
				Policies: map[string]*config.ExceptionPolicyConfig{
					"GIT001": policy,
				},
			}
			Expect(cfg.GetPolicy("GIT001")).To(Equal(policy))
		})
	})
})

var _ = Describe("ExceptionPolicyConfig", func() {
	Describe("IsPolicyEnabled", func() {
		It("should return true when Enabled is nil", func() {
			cfg := &config.ExceptionPolicyConfig{}
			Expect(cfg.IsPolicyEnabled()).To(BeTrue())
		})

		It("should return true when Enabled is true", func() {
			enabled := true
			cfg := &config.ExceptionPolicyConfig{Enabled: &enabled}
			Expect(cfg.IsPolicyEnabled()).To(BeTrue())
		})

		It("should return false when Enabled is false", func() {
			enabled := false
			cfg := &config.ExceptionPolicyConfig{Enabled: &enabled}
			Expect(cfg.IsPolicyEnabled()).To(BeFalse())
		})

		It("should return true for nil config", func() {
			var cfg *config.ExceptionPolicyConfig
			Expect(cfg.IsPolicyEnabled()).To(BeTrue())
		})
	})

	Describe("IsExceptionAllowed", func() {
		It("should return true when AllowException is nil", func() {
			cfg := &config.ExceptionPolicyConfig{}
			Expect(cfg.IsExceptionAllowed()).To(BeTrue())
		})

		It("should return true when AllowException is true", func() {
			allow := true
			cfg := &config.ExceptionPolicyConfig{AllowException: &allow}
			Expect(cfg.IsExceptionAllowed()).To(BeTrue())
		})

		It("should return false when AllowException is false", func() {
			allow := false
			cfg := &config.ExceptionPolicyConfig{AllowException: &allow}
			Expect(cfg.IsExceptionAllowed()).To(BeFalse())
		})

		It("should return true for nil config", func() {
			var cfg *config.ExceptionPolicyConfig
			Expect(cfg.IsExceptionAllowed()).To(BeTrue())
		})
	})

	Describe("IsReasonRequired", func() {
		It("should return false when RequireReason is nil", func() {
			cfg := &config.ExceptionPolicyConfig{}
			Expect(cfg.IsReasonRequired()).To(BeFalse())
		})

		It("should return true when RequireReason is true", func() {
			require := true
			cfg := &config.ExceptionPolicyConfig{RequireReason: &require}
			Expect(cfg.IsReasonRequired()).To(BeTrue())
		})

		It("should return false when RequireReason is false", func() {
			require := false
			cfg := &config.ExceptionPolicyConfig{RequireReason: &require}
			Expect(cfg.IsReasonRequired()).To(BeFalse())
		})

		It("should return false for nil config", func() {
			var cfg *config.ExceptionPolicyConfig
			Expect(cfg.IsReasonRequired()).To(BeFalse())
		})
	})

	Describe("GetMinReasonLength", func() {
		It("should return 10 when MinReasonLength is nil", func() {
			cfg := &config.ExceptionPolicyConfig{}
			Expect(cfg.GetMinReasonLength()).To(Equal(10))
		})

		It("should return the configured value", func() {
			length := 20
			cfg := &config.ExceptionPolicyConfig{MinReasonLength: &length}
			Expect(cfg.GetMinReasonLength()).To(Equal(20))
		})

		It("should return 10 for nil config", func() {
			var cfg *config.ExceptionPolicyConfig
			Expect(cfg.GetMinReasonLength()).To(Equal(10))
		})
	})

	Describe("GetMaxPerHour", func() {
		It("should return 0 (unlimited) when MaxPerHour is nil", func() {
			cfg := &config.ExceptionPolicyConfig{}
			Expect(cfg.GetMaxPerHour()).To(Equal(0))
		})

		It("should return the configured value", func() {
			limit := 5
			cfg := &config.ExceptionPolicyConfig{MaxPerHour: &limit}
			Expect(cfg.GetMaxPerHour()).To(Equal(5))
		})

		It("should return 0 for nil config", func() {
			var cfg *config.ExceptionPolicyConfig
			Expect(cfg.GetMaxPerHour()).To(Equal(0))
		})
	})

	Describe("GetMaxPerDay", func() {
		It("should return 0 (unlimited) when MaxPerDay is nil", func() {
			cfg := &config.ExceptionPolicyConfig{}
			Expect(cfg.GetMaxPerDay()).To(Equal(0))
		})

		It("should return the configured value", func() {
			limit := 10
			cfg := &config.ExceptionPolicyConfig{MaxPerDay: &limit}
			Expect(cfg.GetMaxPerDay()).To(Equal(10))
		})

		It("should return 0 for nil config", func() {
			var cfg *config.ExceptionPolicyConfig
			Expect(cfg.GetMaxPerDay()).To(Equal(0))
		})
	})
})

var _ = Describe("ExceptionRateLimitConfig", func() {
	Describe("IsRateLimitEnabled", func() {
		It("should return true when Enabled is nil", func() {
			cfg := &config.ExceptionRateLimitConfig{}
			Expect(cfg.IsRateLimitEnabled()).To(BeTrue())
		})

		It("should return true when Enabled is true", func() {
			enabled := true
			cfg := &config.ExceptionRateLimitConfig{Enabled: &enabled}
			Expect(cfg.IsRateLimitEnabled()).To(BeTrue())
		})

		It("should return false when Enabled is false", func() {
			enabled := false
			cfg := &config.ExceptionRateLimitConfig{Enabled: &enabled}
			Expect(cfg.IsRateLimitEnabled()).To(BeFalse())
		})

		It("should return true for nil config", func() {
			var cfg *config.ExceptionRateLimitConfig
			Expect(cfg.IsRateLimitEnabled()).To(BeTrue())
		})
	})

	Describe("GetMaxPerHour", func() {
		It("should return 10 when MaxPerHour is nil", func() {
			cfg := &config.ExceptionRateLimitConfig{}
			Expect(cfg.GetMaxPerHour()).To(Equal(10))
		})

		It("should return the configured value", func() {
			limit := 20
			cfg := &config.ExceptionRateLimitConfig{MaxPerHour: &limit}
			Expect(cfg.GetMaxPerHour()).To(Equal(20))
		})

		It("should return 10 for nil config", func() {
			var cfg *config.ExceptionRateLimitConfig
			Expect(cfg.GetMaxPerHour()).To(Equal(10))
		})
	})

	Describe("GetMaxPerDay", func() {
		It("should return 50 when MaxPerDay is nil", func() {
			cfg := &config.ExceptionRateLimitConfig{}
			Expect(cfg.GetMaxPerDay()).To(Equal(50))
		})

		It("should return the configured value", func() {
			limit := 100
			cfg := &config.ExceptionRateLimitConfig{MaxPerDay: &limit}
			Expect(cfg.GetMaxPerDay()).To(Equal(100))
		})

		It("should return 50 for nil config", func() {
			var cfg *config.ExceptionRateLimitConfig
			Expect(cfg.GetMaxPerDay()).To(Equal(50))
		})
	})

	Describe("GetStateFile", func() {
		It("should return default when StateFile is empty", func() {
			cfg := &config.ExceptionRateLimitConfig{}
			Expect(cfg.GetStateFile()).To(Equal("~/.klaudiush/exception_state.json"))
		})

		It("should return the configured value", func() {
			cfg := &config.ExceptionRateLimitConfig{StateFile: "/custom/path.json"}
			Expect(cfg.GetStateFile()).To(Equal("/custom/path.json"))
		})

		It("should return default for nil config", func() {
			var cfg *config.ExceptionRateLimitConfig
			Expect(cfg.GetStateFile()).To(Equal("~/.klaudiush/exception_state.json"))
		})
	})
})

var _ = Describe("ExceptionAuditConfig", func() {
	Describe("IsAuditEnabled", func() {
		It("should return true when Enabled is nil", func() {
			cfg := &config.ExceptionAuditConfig{}
			Expect(cfg.IsAuditEnabled()).To(BeTrue())
		})

		It("should return true when Enabled is true", func() {
			enabled := true
			cfg := &config.ExceptionAuditConfig{Enabled: &enabled}
			Expect(cfg.IsAuditEnabled()).To(BeTrue())
		})

		It("should return false when Enabled is false", func() {
			enabled := false
			cfg := &config.ExceptionAuditConfig{Enabled: &enabled}
			Expect(cfg.IsAuditEnabled()).To(BeFalse())
		})

		It("should return true for nil config", func() {
			var cfg *config.ExceptionAuditConfig
			Expect(cfg.IsAuditEnabled()).To(BeTrue())
		})
	})

	Describe("GetLogFile", func() {
		It("should return default when LogFile is empty", func() {
			cfg := &config.ExceptionAuditConfig{}
			Expect(cfg.GetLogFile()).To(Equal("~/.klaudiush/exception_audit.jsonl"))
		})

		It("should return the configured value", func() {
			cfg := &config.ExceptionAuditConfig{LogFile: "/custom/audit.jsonl"}
			Expect(cfg.GetLogFile()).To(Equal("/custom/audit.jsonl"))
		})

		It("should return default for nil config", func() {
			var cfg *config.ExceptionAuditConfig
			Expect(cfg.GetLogFile()).To(Equal("~/.klaudiush/exception_audit.jsonl"))
		})
	})

	Describe("GetMaxSizeMB", func() {
		It("should return 10 when MaxSizeMB is nil", func() {
			cfg := &config.ExceptionAuditConfig{}
			Expect(cfg.GetMaxSizeMB()).To(Equal(10))
		})

		It("should return the configured value", func() {
			size := 50
			cfg := &config.ExceptionAuditConfig{MaxSizeMB: &size}
			Expect(cfg.GetMaxSizeMB()).To(Equal(50))
		})

		It("should return 10 for nil config", func() {
			var cfg *config.ExceptionAuditConfig
			Expect(cfg.GetMaxSizeMB()).To(Equal(10))
		})
	})

	Describe("GetMaxAgeDays", func() {
		It("should return 30 when MaxAgeDays is nil", func() {
			cfg := &config.ExceptionAuditConfig{}
			Expect(cfg.GetMaxAgeDays()).To(Equal(30))
		})

		It("should return the configured value", func() {
			days := 90
			cfg := &config.ExceptionAuditConfig{MaxAgeDays: &days}
			Expect(cfg.GetMaxAgeDays()).To(Equal(90))
		})

		It("should return 30 for nil config", func() {
			var cfg *config.ExceptionAuditConfig
			Expect(cfg.GetMaxAgeDays()).To(Equal(30))
		})
	})

	Describe("GetMaxBackups", func() {
		It("should return 3 when MaxBackups is nil", func() {
			cfg := &config.ExceptionAuditConfig{}
			Expect(cfg.GetMaxBackups()).To(Equal(3))
		})

		It("should return the configured value", func() {
			backups := 5
			cfg := &config.ExceptionAuditConfig{MaxBackups: &backups}
			Expect(cfg.GetMaxBackups()).To(Equal(5))
		})

		It("should return 3 for nil config", func() {
			var cfg *config.ExceptionAuditConfig
			Expect(cfg.GetMaxBackups()).To(Equal(3))
		})
	})
})

var _ = Describe("Config GetExceptions", func() {
	Describe("GetExceptions", func() {
		It("should create exceptions config when nil", func() {
			cfg := &config.Config{}
			exceptions := cfg.GetExceptions()
			Expect(exceptions).NotTo(BeNil())
			Expect(cfg.Exceptions).NotTo(BeNil())
		})

		It("should return existing exceptions config", func() {
			enabled := true
			cfg := &config.Config{
				Exceptions: &config.ExceptionsConfig{Enabled: &enabled},
			}
			exceptions := cfg.GetExceptions()
			Expect(exceptions.IsEnabled()).To(BeTrue())
		})
	})
})
