package secrets_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/internal/validators/secrets"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// mockGitleaksChecker is a mock implementation of GitleaksChecker.
type mockGitleaksChecker struct {
	available bool
	result    *linters.LintResult
}

func (m *mockGitleaksChecker) IsAvailable() bool {
	return m.available
}

func (m *mockGitleaksChecker) Check(_ context.Context, _ string) *linters.LintResult {
	if m.result == nil {
		return &linters.LintResult{Success: true}
	}

	return m.result
}

var _ = Describe("SecretsValidator", func() {
	var (
		v        *secrets.SecretsValidator
		hookCtx  *hook.Context
		detector secrets.Detector
		gitleaks *mockGitleaksChecker
		cfg      *config.SecretsValidatorConfig
	)

	BeforeEach(func() {
		detector = secrets.NewDefaultPatternDetector()
		gitleaks = &mockGitleaksChecker{available: false}
		cfg = &config.SecretsValidatorConfig{
			ValidatorConfig: config.ValidatorConfig{Enabled: boolPtr(true)},
		}
		v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)
		hookCtx = &hook.Context{
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeWrite,
			ToolInput: hook.ToolInput{},
		}
	})

	Describe("AWS credentials detection", func() {
		It("should detect AWS Access Key ID", func() {
			hookCtx.ToolInput.Content = `aws_access_key_id = "AKIAIOSFODNN7EXAMPLE"`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsAPIKey))
			Expect(result.Message).To(ContainSubstring("AWS Access Key ID"))
		})

		It("should detect AWS Secret Access Key", func() {
			hookCtx.ToolInput.Content = `aws_secret_access_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsAPIKey))
		})
	})

	Describe("GitHub token detection", func() {
		It("should detect GitHub Personal Access Token", func() {
			hookCtx.ToolInput.Content = `GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsToken))
			Expect(result.Message).To(ContainSubstring("GitHub Personal Access Token"))
		})

		It("should detect GitHub OAuth Token", func() {
			hookCtx.ToolInput.Content = `token: gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsToken))
		})

		It("should detect GitHub App Token", func() {
			hookCtx.ToolInput.Content = `ghs_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsToken))
		})
	})

	Describe("private key detection", func() {
		It("should detect RSA private key", func() {
			hookCtx.ToolInput.Content = `-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEA...
-----END RSA PRIVATE KEY-----`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsPrivKey))
			Expect(result.Message).To(ContainSubstring("RSA Private Key"))
		})

		It("should detect OpenSSH private key", func() {
			hookCtx.ToolInput.Content = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAA...
-----END OPENSSH PRIVATE KEY-----`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsPrivKey))
		})

		It("should detect EC private key", func() {
			hookCtx.ToolInput.Content = `-----BEGIN EC PRIVATE KEY-----
MHQCAQEEIBhzUvl...
-----END EC PRIVATE KEY-----`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsPrivKey))
		})
	})

	Describe("database connection string detection", func() {
		It("should detect MongoDB connection string", func() {
			hookCtx.ToolInput.Content = `mongodb://user:password123@localhost:27017/database`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsConnString))
		})

		It("should detect PostgreSQL connection string", func() {
			hookCtx.ToolInput.Content = `postgresql://admin:secretpass@db.example.com:5432/mydb`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsConnString))
		})

		It("should detect MySQL connection string", func() {
			hookCtx.ToolInput.Content = `mysql://root:toor@localhost:3306/test`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsConnString))
		})

		It("should detect Redis connection string", func() {
			hookCtx.ToolInput.Content = `redis://default:mypassword@redis.example.com:6379`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsConnString))
		})
	})

	Describe("other service token detection", func() {
		It("should detect Slack token", func() {
			hookCtx.ToolInput.Content = `SLACK_TOKEN=xoxb-1234567890-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsToken))
		})

		It("should detect Google API Key", func() {
			// Pattern: AIza + 35 alphanumeric/underscore/hyphen chars
			hookCtx.ToolInput.Content = `GOOGLE_API_KEY=AIzaSyD-abcdefghijklmnopqrstuvwxyz12345`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsAPIKey))
		})

		It("should detect Stripe API Key", func() {
			hookCtx.ToolInput.Content = `stripe_key = "sk_live_AbCdEfGhIjKlMnOpQrStUvWxYz"`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsAPIKey))
		})

		It("should detect SendGrid API Key", func() {
			// Pattern: SG. + 22 chars + . + 43 chars
			hookCtx.ToolInput.Content = `SENDGRID_API_KEY=SG.abcdefghijklmnopqrstuv.wxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abc`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsAPIKey))
		})

		It("should detect JWT token", func() {
			//nolint:lll // JWT test data is intentionally long
			hookCtx.ToolInput.Content = `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.Gfx6VO9tcxwk6xqx9yYzSfebfeakZp5JYIgP_edcw_A`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsToken))
		})
	})

	Describe("safe content", func() {
		It("should pass for content without secrets", func() {
			hookCtx.ToolInput.Content = `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for empty content", func() {
			hookCtx.ToolInput.Content = ""
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should pass for environment variable references", func() {
			hookCtx.ToolInput.Content = `API_KEY=$GITHUB_TOKEN
password: ${DATABASE_PASSWORD}
secret = os.getenv("SECRET_KEY")
`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("configuration options", func() {
		It("should respect allow list", func() {
			cfg.AllowList = []string{`AKIA.*EXAMPLE`}
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			hookCtx.ToolInput.Content = `aws_access_key_id = "AKIAIOSFODNN7EXAMPLE"`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should respect disabled patterns", func() {
			cfg.DisabledPatterns = []string{"aws-access-key-id"}
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			hookCtx.ToolInput.Content = `aws_access_key_id = "AKIAIOSFODNN7EXAMPLE"`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should warn instead of block when configured", func() {
			cfg.BlockOnDetection = boolPtr(false)
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			hookCtx.ToolInput.Content = `GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.ShouldBlock).To(BeFalse())
		})

		It("should skip files larger than max size", func() {
			cfg.MaxFileSize = 10 // 10 bytes
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			hookCtx.ToolInput.Content = `GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Edit operations", func() {
		It("should validate new_string for Edit tool", func() {
			hookCtx.ToolName = hook.ToolTypeEdit
			hookCtx.ToolInput.Content = "" // Edit operations don't use Content
			hookCtx.ToolInput.NewString = `GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
			hookCtx.ToolInput.OldString = "placeholder"

			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Reference).To(Equal(validator.RefSecretsToken))
		})
	})

	Describe("gitleaks integration", func() {
		It("should use gitleaks when available and enabled", func() {
			cfg.UseGitleaks = boolPtr(true)
			gitleaks.available = true
			gitleaks.result = &linters.LintResult{
				Success: false,
				Findings: []linters.LintFinding{
					{Line: 1, Message: "Detected AWS credentials"},
				},
			}
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			// Content that doesn't match built-in patterns but gitleaks detects
			hookCtx.ToolInput.Content = `some obscure secret format`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeFalse())
			Expect(result.Message).To(ContainSubstring("Gitleaks"))
		})

		It("should skip gitleaks when not available", func() {
			cfg.UseGitleaks = boolPtr(true)
			gitleaks.available = false
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			hookCtx.ToolInput.Content = `some safe content`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})

		It("should skip gitleaks when disabled in config", func() {
			cfg.UseGitleaks = boolPtr(false)
			gitleaks.available = true
			gitleaks.result = &linters.LintResult{
				Success: false,
				Findings: []linters.LintFinding{
					{Line: 1, Message: "Detected something"},
				},
			}
			v = secrets.NewSecretsValidator(logger.NewNoOpLogger(), detector, gitleaks, cfg)

			hookCtx.ToolInput.Content = `some safe content`
			result := v.Validate(context.Background(), hookCtx)
			Expect(result.Passed).To(BeTrue())
		})
	})

	Describe("Category", func() {
		It("should return CategoryCPU", func() {
			Expect(v.Category()).To(Equal(validator.CategoryCPU))
		})
	})
})

func boolPtr(b bool) *bool {
	return &b
}
