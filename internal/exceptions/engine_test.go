package exceptions_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/exceptions"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("Engine", func() {
	var engine *exceptions.Engine

	Describe("NewEngine", func() {
		It("creates engine with nil config", func() {
			e := exceptions.NewEngine(nil)
			Expect(e).NotTo(BeNil())
		})

		It("creates engine with empty config", func() {
			e := exceptions.NewEngine(&config.ExceptionsConfig{})
			Expect(e).NotTo(BeNil())
		})

		It("accepts custom logger", func() {
			buf := &bytes.Buffer{}
			log := logger.NewFileLoggerWithWriter(buf, true, false)
			e := exceptions.NewEngine(nil, exceptions.WithLogger(log))
			Expect(e).NotTo(BeNil())
		})

		It("uses custom token prefix from config", func() {
			e := exceptions.NewEngine(&config.ExceptionsConfig{
				TokenPrefix: "ACK",
			})
			result := e.Evaluate(&exceptions.EvaluateRequest{
				Command: "git push # ACK:GIT022:reason",
			})
			Expect(result.Allowed).To(BeTrue())
		})
	})

	Describe("Evaluate", func() {
		Context("with nil request", func() {
			BeforeEach(func() {
				engine = exceptions.NewEngine(nil)
			})

			It("returns not allowed", func() {
				result := engine.Evaluate(nil)
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("no request"))
			})
		})

		Context("when exceptions are disabled", func() {
			BeforeEach(func() {
				enabled := false
				engine = exceptions.NewEngine(&config.ExceptionsConfig{
					Enabled: &enabled,
				})
			})

			It("returns not allowed", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command: "git push # EXC:GIT022:reason",
				})
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("disabled"))
			})
		})

		Context("with no exception token in command", func() {
			BeforeEach(func() {
				engine = exceptions.NewEngine(nil)
			})

			It("returns not allowed", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command: "git push origin main",
				})
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("no exception token"))
			})
		})

		Context("with parse error", func() {
			BeforeEach(func() {
				engine = exceptions.NewEngine(nil)
			})

			It("returns not allowed for empty command", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command: "",
				})
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("parse"))
			})
		})

		Context("with error code mismatch", func() {
			BeforeEach(func() {
				engine = exceptions.NewEngine(nil)
			})

			It("denies when error codes don't match", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:   "git push # EXC:GIT022:reason",
					ErrorCode: "SEC001",
				})
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("does not match"))
			})

			It("allows when error codes match", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:   "git push # EXC:GIT022:reason",
					ErrorCode: "GIT022",
				})
				Expect(result.Allowed).To(BeTrue())
			})

			It("allows when no expected error code specified", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command: "git push # EXC:GIT022:reason",
				})
				Expect(result.Allowed).To(BeTrue())
			})
		})

		Context("with valid exception token", func() {
			BeforeEach(func() {
				engine = exceptions.NewEngine(nil)
			})

			It("allows exception from comment", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:       "git push origin main # EXC:GIT022:Emergency+hotfix",
					ValidatorName: "git.push",
					ErrorCode:     "GIT022",
				})
				Expect(result.Allowed).To(BeTrue())
				Expect(result.AuditEntry).NotTo(BeNil())
				Expect(result.AuditEntry.ErrorCode).To(Equal("GIT022"))
				Expect(result.AuditEntry.ValidatorName).To(Equal("git.push"))
				Expect(result.AuditEntry.Reason).To(Equal("Emergency hotfix"))
				Expect(result.AuditEntry.Source).To(Equal("comment"))
			})

			It("allows exception from env var", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:       `KLACK="EXC:SEC001:Test+fixture" git commit -sS -m "msg"`,
					ValidatorName: "git.commit",
					ErrorCode:     "SEC001",
				})
				Expect(result.Allowed).To(BeTrue())
				Expect(result.AuditEntry.Source).To(Equal("env_var"))
			})
		})

		Context("with audit entry", func() {
			BeforeEach(func() {
				engine = exceptions.NewEngine(nil)
			})

			It("populates audit entry fields", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:       "git push # EXC:GIT022:reason",
					ValidatorName: "git.push",
					ErrorCode:     "GIT022",
					WorkingDir:    "/path/to/repo",
					Repository:    "my-repo",
				})
				Expect(result.AuditEntry).NotTo(BeNil())
				Expect(result.AuditEntry.Timestamp).NotTo(BeZero())
				Expect(result.AuditEntry.WorkingDir).To(Equal("/path/to/repo"))
				Expect(result.AuditEntry.Repository).To(Equal("my-repo"))
				Expect(result.AuditEntry.Allowed).To(BeTrue())
				Expect(result.AuditEntry.DenialReason).To(BeEmpty())
			})

			It("includes denial reason when denied", func() {
				enabled := false
				engine = exceptions.NewEngine(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"GIT022": {AllowException: &enabled},
					},
				})
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:   "git push # EXC:GIT022:reason",
					ErrorCode: "GIT022",
				})
				Expect(result.AuditEntry.Allowed).To(BeFalse())
				Expect(result.AuditEntry.DenialReason).NotTo(BeEmpty())
			})

			It("truncates long commands", func() {
				longCommand := "git push " + string(make([]byte, 300)) + " # EXC:GIT022:reason"
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command: longCommand,
				})
				Expect(len(result.AuditEntry.Command)).To(BeNumerically("<=", 205))
			})
		})

		Context("with policy requirements", func() {
			BeforeEach(func() {
				required := true
				minLen := 15
				engine = exceptions.NewEngine(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"SEC001": {
							RequireReason:   &required,
							MinReasonLength: &minLen,
						},
					},
				})
			})

			It("denies when reason requirement not met", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:   "git push # EXC:SEC001:short",
					ErrorCode: "SEC001",
				})
				Expect(result.Allowed).To(BeFalse())
				Expect(result.Reason).To(ContainSubstring("too short"))
			})

			It("allows when reason requirement met", func() {
				result := engine.Evaluate(&exceptions.EvaluateRequest{
					Command:   "git push # EXC:SEC001:Long+enough+reason+here",
					ErrorCode: "SEC001",
				})
				Expect(result.Allowed).To(BeTrue())
			})
		})
	})

	Describe("EvaluateForErrorCode", func() {
		BeforeEach(func() {
			engine = exceptions.NewEngine(nil)
		})

		It("is a convenience wrapper for Evaluate", func() {
			result := engine.EvaluateForErrorCode(
				"git push # EXC:GIT022:reason",
				"git.push",
				"GIT022",
			)
			Expect(result.Allowed).To(BeTrue())
			Expect(result.AuditEntry.ValidatorName).To(Equal("git.push"))
		})
	})

	Describe("HasToken", func() {
		BeforeEach(func() {
			engine = exceptions.NewEngine(nil)
		})

		It("returns true when token present", func() {
			Expect(engine.HasToken("git push # EXC:GIT022:reason")).To(BeTrue())
		})

		It("returns false when no token", func() {
			Expect(engine.HasToken("git push origin main")).To(BeFalse())
		})

		It("returns false for invalid command", func() {
			Expect(engine.HasToken("")).To(BeFalse())
		})
	})

	Describe("GetTokenErrorCode", func() {
		BeforeEach(func() {
			engine = exceptions.NewEngine(nil)
		})

		It("returns error code when token present", func() {
			code := engine.GetTokenErrorCode("git push # EXC:GIT022:reason")
			Expect(code).To(Equal("GIT022"))
		})

		It("returns empty when no token", func() {
			code := engine.GetTokenErrorCode("git push origin main")
			Expect(code).To(BeEmpty())
		})

		It("returns empty for invalid command", func() {
			code := engine.GetTokenErrorCode("")
			Expect(code).To(BeEmpty())
		})
	})

	Describe("IsEnabled", func() {
		It("returns true with nil config", func() {
			e := exceptions.NewEngine(nil)
			Expect(e.IsEnabled()).To(BeTrue())
		})

		It("returns true when enabled is nil", func() {
			e := exceptions.NewEngine(&config.ExceptionsConfig{})
			Expect(e.IsEnabled()).To(BeTrue())
		})

		It("returns false when disabled", func() {
			enabled := false
			e := exceptions.NewEngine(&config.ExceptionsConfig{
				Enabled: &enabled,
			})
			Expect(e.IsEnabled()).To(BeFalse())
		})

		It("returns true when explicitly enabled", func() {
			enabled := true
			e := exceptions.NewEngine(&config.ExceptionsConfig{
				Enabled: &enabled,
			})
			Expect(e.IsEnabled()).To(BeTrue())
		})
	})
})
