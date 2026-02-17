package exceptions_test

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/exceptions"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

var _ = Describe("Handler", func() {
	var (
		handler *exceptions.Handler
		tempDir string
	)

	BeforeEach(func() {
		var err error

		tempDir, err = os.MkdirTemp("", "handler-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("NewHandler", func() {
		It("creates handler with nil config", func() {
			h := exceptions.NewHandler(nil)
			Expect(h).NotTo(BeNil())
		})

		It("creates handler with empty config", func() {
			h := exceptions.NewHandler(&config.ExceptionsConfig{})
			Expect(h).NotTo(BeNil())
		})

		It("accepts custom logger", func() {
			buf := &bytes.Buffer{}
			log := logger.NewFileLoggerWithWriter(buf, true, false)
			h := exceptions.NewHandler(nil, exceptions.WithHandlerLogger(log))
			Expect(h).NotTo(BeNil())
		})

		It("accepts custom engine", func() {
			engine := exceptions.NewEngine(nil)
			h := exceptions.NewHandler(nil, exceptions.WithEngine(engine))
			Expect(h).NotTo(BeNil())
		})

		It("accepts custom rate limiter", func() {
			rateLimiter := exceptions.NewRateLimiter(nil, nil)
			h := exceptions.NewHandler(nil, exceptions.WithRateLimiter(rateLimiter))
			Expect(h).NotTo(BeNil())
		})

		It("accepts custom audit logger", func() {
			auditCfg := &config.ExceptionAuditConfig{
				LogFile: filepath.Join(tempDir, "audit.jsonl"),
			}
			auditLogger := exceptions.NewAuditLogger(auditCfg)
			h := exceptions.NewHandler(nil, exceptions.WithAuditLogger(auditLogger))
			Expect(h).NotTo(BeNil())
		})
	})

	Describe("Check", func() {
		Context("with nil request", func() {
			BeforeEach(func() {
				handler = exceptions.NewHandler(nil)
			})

			It("returns not bypassed", func() {
				result := handler.Check(nil)
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("no request"))
			})
		})

		Context("when exceptions are disabled", func() {
			BeforeEach(func() {
				enabled := false
				handler = exceptions.NewHandler(&config.ExceptionsConfig{
					Enabled: &enabled,
				})
			})

			It("returns not bypassed", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push # EXC:GIT022:reason",
						},
					},
					ErrorCode: "GIT022",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("disabled"))
			})
		})

		Context("with no command", func() {
			BeforeEach(func() {
				handler = exceptions.NewHandler(nil)
			})

			It("returns not bypassed when hook context is nil", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: nil,
					ErrorCode:   "GIT022",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("no command"))
			})

			It("returns not bypassed when command is empty", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "",
						},
					},
					ErrorCode: "GIT022",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("no command"))
			})
		})

		Context("with no exception token", func() {
			BeforeEach(func() {
				handler = exceptions.NewHandler(nil)
			})

			It("returns not bypassed", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push origin main",
						},
					},
					ErrorCode: "GIT022",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("no exception token"))
			})
		})

		Context("with error code mismatch", func() {
			BeforeEach(func() {
				handler = exceptions.NewHandler(nil)
			})

			It("returns not bypassed", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push # EXC:GIT022:reason",
						},
					},
					ErrorCode: "SEC001",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("does not match"))
			})
		})

		Context("with valid exception token", func() {
			var auditFile string

			BeforeEach(func() {
				auditFile = filepath.Join(tempDir, "audit.jsonl")
				stateFile := filepath.Join(tempDir, "state.json")

				handler = exceptions.NewHandler(&config.ExceptionsConfig{
					RateLimit: &config.ExceptionRateLimitConfig{
						StateFile: stateFile,
					},
					Audit: &config.ExceptionAuditConfig{
						LogFile: auditFile,
					},
				})
			})

			It("allows exception and returns bypassed", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
						},
					},
					ValidatorName: "git.push",
					ErrorCode:     "GIT022",
				})
				Expect(result.Bypassed).To(BeTrue())
				Expect(result.ErrorCode).To(Equal("GIT022"))
				Expect(result.TokenReason).To(Equal("Emergency hotfix"))
			})

			It("logs exception to audit file", func() {
				_ = handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push # EXC:GIT022:reason",
						},
					},
					ValidatorName: "git.push",
					ErrorCode:     "GIT022",
				})

				// Verify audit file exists and has content
				content, err := os.ReadFile(auditFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(content)).To(ContainSubstring("GIT022"))
			})

			It("includes rate limit info in response", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push # EXC:GIT022:reason",
						},
					},
					ErrorCode: "GIT022",
				})
				Expect(result.RateLimitInfo).NotTo(BeNil())
				Expect(result.RateLimitInfo.Allowed).To(BeTrue())
			})
		})

		Context("with rate limit exceeded", func() {
			BeforeEach(func() {
				maxHour := 1
				stateFile := filepath.Join(tempDir, "state.json")

				rateLimiter := exceptions.NewRateLimiter(
					&config.ExceptionRateLimitConfig{
						MaxPerHour: &maxHour,
						StateFile:  stateFile,
					},
					nil,
				)

				// Record one exception to exhaust the limit
				err := rateLimiter.Record("GIT022")
				Expect(err).NotTo(HaveOccurred())

				handler = exceptions.NewHandler(nil,
					exceptions.WithRateLimiter(rateLimiter),
				)
			})

			It("returns not bypassed with rate limit reason", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push # EXC:GIT022:reason",
						},
					},
					ErrorCode: "GIT022",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("limit"))
			})
		})

		Context("with policy not allowing exception", func() {
			BeforeEach(func() {
				allowException := false
				handler = exceptions.NewHandler(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"SEC001": {AllowException: &allowException},
					},
				})
			})

			It("returns not bypassed", func() {
				result := handler.Check(&exceptions.CheckRequest{
					HookContext: &hook.Context{
						ToolInput: hook.ToolInput{
							Command: "git push # EXC:SEC001:reason",
						},
					},
					ErrorCode: "SEC001",
				})
				Expect(result.Bypassed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("not allowed"))
			})
		})
	})

	Describe("IsEnabled", func() {
		It("returns true with nil config", func() {
			h := exceptions.NewHandler(nil)
			Expect(h.IsEnabled()).To(BeTrue())
		})

		It("returns true when enabled is nil", func() {
			h := exceptions.NewHandler(&config.ExceptionsConfig{})
			Expect(h.IsEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			h := exceptions.NewHandler(&config.ExceptionsConfig{
				Enabled: &enabled,
			})
			Expect(h.IsEnabled()).To(BeFalse())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			h := exceptions.NewHandler(&config.ExceptionsConfig{
				Enabled: &enabled,
			})
			Expect(h.IsEnabled()).To(BeTrue())
		})
	})

	Describe("LoadState/SaveState", func() {
		var stateFile string

		BeforeEach(func() {
			stateFile = filepath.Join(tempDir, "state.json")
			handler = exceptions.NewHandler(&config.ExceptionsConfig{
				RateLimit: &config.ExceptionRateLimitConfig{
					StateFile: stateFile,
				},
			})
		})

		It("saves and loads state", func() {
			// Record an exception to modify state
			_ = handler.Check(&exceptions.CheckRequest{
				HookContext: &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push # EXC:GIT022:reason",
					},
				},
				ErrorCode: "GIT022",
			})

			// Save state
			err := handler.SaveState()
			Expect(err).NotTo(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(stateFile)
			Expect(err).NotTo(HaveOccurred())

			// Create new handler and load state
			newHandler := exceptions.NewHandler(&config.ExceptionsConfig{
				RateLimit: &config.ExceptionRateLimitConfig{
					StateFile: stateFile,
				},
			})
			err = newHandler.LoadState()
			Expect(err).NotTo(HaveOccurred())

			// Verify state was loaded
			state := newHandler.GetRateLimitState()
			Expect(state.GlobalHourlyCount).To(Equal(1))
		})
	})

	Describe("GetAuditStats", func() {
		BeforeEach(func() {
			auditFile := filepath.Join(tempDir, "audit.jsonl")
			handler = exceptions.NewHandler(&config.ExceptionsConfig{
				Audit: &config.ExceptionAuditConfig{
					LogFile: auditFile,
				},
			})
		})

		It("returns stats with no entries", func() {
			stats, err := handler.GetAuditStats()
			Expect(err).NotTo(HaveOccurred())
			Expect(stats).NotTo(BeNil())
			Expect(stats.EntryCount).To(Equal(0))
		})

		It("returns stats after logging", func() {
			// Log an exception
			_ = handler.Check(&exceptions.CheckRequest{
				HookContext: &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push # EXC:GIT022:reason",
					},
				},
				ErrorCode: "GIT022",
			})

			stats, err := handler.GetAuditStats()
			Expect(err).NotTo(HaveOccurred())
			Expect(stats.EntryCount).To(Equal(1))
		})
	})

	Describe("CleanupAudit", func() {
		BeforeEach(func() {
			auditFile := filepath.Join(tempDir, "audit.jsonl")
			now := time.Now()
			nowFunc := func() time.Time { return now }

			auditLogger := exceptions.NewAuditLogger(
				&config.ExceptionAuditConfig{
					LogFile:    auditFile,
					MaxAgeDays: intPtr(1),
				},
				exceptions.WithAuditTimeFunc(nowFunc),
			)

			handler = exceptions.NewHandler(nil, exceptions.WithAuditLogger(auditLogger))
		})

		It("runs cleanup without error", func() {
			err := handler.CleanupAudit()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("FormatBypassMessage", func() {
	It("returns empty for nil response", func() {
		msg := exceptions.FormatBypassMessage(nil)
		Expect(msg).To(BeEmpty())
	})

	It("returns empty when not bypassed", func() {
		msg := exceptions.FormatBypassMessage(&exceptions.CheckResponse{
			Bypassed: false,
		})
		Expect(msg).To(BeEmpty())
	})

	It("formats bypass message with error code", func() {
		msg := exceptions.FormatBypassMessage(&exceptions.CheckResponse{
			Bypassed:  true,
			ErrorCode: "GIT022",
		})
		Expect(msg).To(ContainSubstring("GIT022"))
		Expect(msg).To(ContainSubstring("✅"))
	})

	It("includes reason when provided", func() {
		msg := exceptions.FormatBypassMessage(&exceptions.CheckResponse{
			Bypassed:    true,
			ErrorCode:   "GIT022",
			TokenReason: "Emergency hotfix",
		})
		Expect(msg).To(ContainSubstring("Emergency hotfix"))
	})

	It("includes rate limit info when provided", func() {
		msg := exceptions.FormatBypassMessage(&exceptions.CheckResponse{
			Bypassed:  true,
			ErrorCode: "GIT022",
			RateLimitInfo: &exceptions.CheckResult{
				Allowed:               true,
				GlobalHourlyRemaining: 5,
				GlobalDailyRemaining:  20,
			},
		})
		Expect(msg).To(ContainSubstring("Remaining"))
	})
})

var _ = Describe("FormatDenialMessage", func() {
	It("returns empty for nil response", func() {
		msg := exceptions.FormatDenialMessage(nil)
		Expect(msg).To(BeEmpty())
	})

	It("returns empty when bypassed", func() {
		msg := exceptions.FormatDenialMessage(&exceptions.CheckResponse{
			Bypassed: true,
		})
		Expect(msg).To(BeEmpty())
	})

	It("formats denial message with reason", func() {
		msg := exceptions.FormatDenialMessage(&exceptions.CheckResponse{
			Bypassed: false,
			Reason:   "rate limit exceeded",
		})
		Expect(msg).To(ContainSubstring("❌"))
		Expect(msg).To(ContainSubstring("rate limit exceeded"))
	})

	It("includes error code when provided", func() {
		msg := exceptions.FormatDenialMessage(&exceptions.CheckResponse{
			Bypassed:  false,
			ErrorCode: "SEC001",
			Reason:    "not allowed by policy",
		})
		Expect(msg).To(ContainSubstring("SEC001"))
	})
})

// intPtr returns a pointer to an int.
func intPtr(i int) *int {
	return &i
}
