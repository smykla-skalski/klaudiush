package dispatcher_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/dispatcher"
	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

var _ = Describe("ExceptionChecker", func() {
	var (
		checker *dispatcher.DefaultExceptionChecker
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "exception-checker-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if tempDir != "" {
			os.RemoveAll(tempDir)
		}
	})

	Describe("NewExceptionChecker", func() {
		It("creates checker with nil handler", func() {
			c := dispatcher.NewExceptionChecker(nil)
			Expect(c).NotTo(BeNil())
		})

		It("creates checker with handler", func() {
			handler := exceptions.NewHandler(nil)
			c := dispatcher.NewExceptionChecker(handler)
			Expect(c).NotTo(BeNil())
		})
	})

	Describe("CheckException", func() {
		Context("with nil handler", func() {
			BeforeEach(func() {
				checker = dispatcher.NewExceptionChecker(nil)
			})

			It("returns error unchanged", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "test",
					Message:     "test error",
					ShouldBlock: true,
				}
				result, bypassed := checker.CheckException(nil, verr)
				Expect(bypassed).To(BeFalse())
				Expect(result).To(Equal(verr))
			})
		})

		Context("with disabled handler", func() {
			BeforeEach(func() {
				enabled := false
				handler := exceptions.NewHandler(&config.ExceptionsConfig{
					Enabled: &enabled,
				})
				checker = dispatcher.NewExceptionChecker(handler)
			})

			It("returns error unchanged", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "test",
					Message:     "test error",
					ShouldBlock: true,
				}
				result, bypassed := checker.CheckException(nil, verr)
				Expect(bypassed).To(BeFalse())
				Expect(result).To(Equal(verr))
			})
		})

		Context("with non-blocking error", func() {
			BeforeEach(func() {
				handler := exceptions.NewHandler(nil)
				checker = dispatcher.NewExceptionChecker(handler)
			})

			It("returns error unchanged", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "test",
					Message:     "test warning",
					ShouldBlock: false, // Non-blocking
				}
				hookCtx := &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push # EXC:GIT022:reason",
					},
				}

				result, bypassed := checker.CheckException(hookCtx, verr)
				Expect(bypassed).To(BeFalse())
				Expect(result).To(Equal(verr))
			})
		})

		Context("with no reference URL", func() {
			BeforeEach(func() {
				handler := exceptions.NewHandler(nil)
				checker = dispatcher.NewExceptionChecker(handler)
			})

			It("returns error unchanged when no reference", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "test",
					Message:     "test error",
					ShouldBlock: true,
					Reference:   "", // No reference
				}
				hookCtx := &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push # EXC:GIT022:reason",
					},
				}

				result, bypassed := checker.CheckException(hookCtx, verr)
				Expect(bypassed).To(BeFalse())
				Expect(result).To(Equal(verr))
			})
		})

		Context("with valid exception token", func() {
			BeforeEach(func() {
				stateFile := filepath.Join(tempDir, "state.json")
				auditFile := filepath.Join(tempDir, "audit.jsonl")

				handler := exceptions.NewHandler(&config.ExceptionsConfig{
					RateLimit: &config.ExceptionRateLimitConfig{
						StateFile: stateFile,
					},
					Audit: &config.ExceptionAuditConfig{
						LogFile: auditFile,
					},
				})
				checker = dispatcher.NewExceptionChecker(handler)
			})

			It("converts blocking error to warning when exception matches", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "git.push",
					Message:     "cannot push to protected branch",
					ShouldBlock: true,
					Reference:   "https://klaudiu.sh/GIT022",
				}
				hookCtx := &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
					},
				}

				result, bypassed := checker.CheckException(hookCtx, verr)
				Expect(bypassed).To(BeTrue())
				Expect(result).NotTo(BeNil())
				Expect(result.ShouldBlock).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("BYPASSED"))
				Expect(result.Message).To(ContainSubstring("Emergency hotfix"))
			})

			It("preserves error details when bypassed", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "git.push",
					Message:     "cannot push",
					ShouldBlock: true,
					Reference:   "https://klaudiu.sh/GIT022",
					Details:     map[string]string{"branch": "main"},
					FixHint:     "use a feature branch",
				}
				hookCtx := &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push # EXC:GIT022:reason",
					},
				}

				result, bypassed := checker.CheckException(hookCtx, verr)
				Expect(bypassed).To(BeTrue())
				Expect(result.Details).To(HaveKey("branch"))
				Expect(result.FixHint).To(Equal("use a feature branch"))
				Expect(result.Reference).To(Equal(validator.Reference("https://klaudiu.sh/GIT022")))
			})

			It("handles reference URL with trailing slash", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "git.push",
					Message:     "cannot push to protected branch",
					ShouldBlock: true,
					Reference:   "https://klaudiu.sh/GIT022/", // Trailing slash
				}
				hookCtx := &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push origin main # EXC:GIT022:Emergency+hotfix",
					},
				}

				result, bypassed := checker.CheckException(hookCtx, verr)
				Expect(bypassed).To(BeTrue())
				Expect(result).NotTo(BeNil())
				Expect(result.ShouldBlock).To(BeFalse())
				Expect(result.Message).To(ContainSubstring("BYPASSED"))
			})
		})

		Context("with error code mismatch", func() {
			BeforeEach(func() {
				handler := exceptions.NewHandler(nil)
				checker = dispatcher.NewExceptionChecker(handler)
			})

			It("returns error unchanged when codes don't match", func() {
				verr := &dispatcher.ValidationError{
					Validator:   "git.push",
					Message:     "cannot push",
					ShouldBlock: true,
					Reference:   "https://klaudiu.sh/GIT022",
				}
				hookCtx := &hook.Context{
					ToolInput: hook.ToolInput{
						Command: "git push # EXC:SEC001:wrong+code", // Different code
					},
				}

				result, bypassed := checker.CheckException(hookCtx, verr)
				Expect(bypassed).To(BeFalse())
				Expect(result).To(Equal(verr))
			})
		})
	})

	Describe("IsEnabled", func() {
		It("returns false with nil handler", func() {
			c := dispatcher.NewExceptionChecker(nil)
			Expect(c.IsEnabled()).To(BeFalse())
		})

		It("returns true with enabled handler", func() {
			handler := exceptions.NewHandler(nil)
			c := dispatcher.NewExceptionChecker(handler)
			Expect(c.IsEnabled()).To(BeTrue())
		})

		It("returns false with disabled handler", func() {
			enabled := false
			handler := exceptions.NewHandler(&config.ExceptionsConfig{
				Enabled: &enabled,
			})
			c := dispatcher.NewExceptionChecker(handler)
			Expect(c.IsEnabled()).To(BeFalse())
		})
	})
})

var _ = Describe("NoOpExceptionChecker", func() {
	var checker *dispatcher.NoOpExceptionChecker

	BeforeEach(func() {
		checker = &dispatcher.NoOpExceptionChecker{}
	})

	Describe("CheckException", func() {
		It("always returns error unchanged", func() {
			verr := &dispatcher.ValidationError{
				Validator:   "test",
				Message:     "test error",
				ShouldBlock: true,
			}
			result, bypassed := checker.CheckException(nil, verr)
			Expect(bypassed).To(BeFalse())
			Expect(result).To(Equal(verr))
		})
	})

	Describe("IsEnabled", func() {
		It("always returns false", func() {
			Expect(checker.IsEnabled()).To(BeFalse())
		})
	})
})
