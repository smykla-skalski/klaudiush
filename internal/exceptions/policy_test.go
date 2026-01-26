package exceptions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/exceptions"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("PolicyMatcher", func() {
	var matcher *exceptions.PolicyMatcher

	Describe("NewPolicyMatcher", func() {
		It("creates matcher with nil config", func() {
			m := exceptions.NewPolicyMatcher(nil)
			Expect(m).NotTo(BeNil())
		})

		It("creates matcher with empty config", func() {
			m := exceptions.NewPolicyMatcher(&config.ExceptionsConfig{})
			Expect(m).NotTo(BeNil())
		})
	})

	Describe("Match", func() {
		Context("with nil request", func() {
			BeforeEach(func() {
				matcher = exceptions.NewPolicyMatcher(nil)
			})

			It("returns denied for nil request", func() {
				decision := matcher.Match(nil)
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("no exception token"))
			})

			It("returns denied for request with nil token", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("no exception token"))
			})
		})

		Context("when exceptions are disabled globally", func() {
			BeforeEach(func() {
				enabled := false
				matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
					Enabled: &enabled,
				})
			})

			It("denies all exceptions", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{ErrorCode: "GIT022"},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("disabled"))
			})
		})

		Context("with default policy (no explicit config)", func() {
			BeforeEach(func() {
				matcher = exceptions.NewPolicyMatcher(nil)
			})

			It("allows exception with token", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "test reason",
					},
				})
				Expect(decision.Allowed).To(BeTrue())
				Expect(decision.RequiredReason).To(BeFalse())
			})

			It("allows exception without reason by default", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
					},
				})
				Expect(decision.Allowed).To(BeTrue())
			})
		})

		Context("with policy disabled for specific error code", func() {
			BeforeEach(func() {
				enabled := false
				matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"GIT022": {Enabled: &enabled},
					},
				})
			})

			It("denies exception for disabled policy", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{ErrorCode: "GIT022"},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("GIT022"))
				Expect(decision.Reason).To(ContainSubstring("disabled"))
			})

			It("allows other error codes", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{ErrorCode: "SEC001"},
				})
				Expect(decision.Allowed).To(BeTrue())
			})
		})

		Context("with AllowException set to false", func() {
			BeforeEach(func() {
				allow := false
				matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"SEC001": {AllowException: &allow},
					},
				})
			})

			It("denies exception for disallowed error code", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{ErrorCode: "SEC001"},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("not allowed"))
			})
		})

		Context("with reason required", func() {
			BeforeEach(func() {
				required := true
				minLen := 10
				matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"GIT022": {
							RequireReason:   &required,
							MinReasonLength: &minLen,
						},
					},
				})
			})

			It("denies exception without reason", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{ErrorCode: "GIT022"},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("required"))
				Expect(decision.RequiredReason).To(BeTrue())
			})

			It("denies exception with too short reason", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "short",
					},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("too short"))
			})

			It("allows exception with sufficient reason", func() {
				reason := "This is a long enough reason for the policy"
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    reason,
					},
				})
				Expect(decision.Allowed).To(BeTrue())
				Expect(decision.RequiredReason).To(BeTrue())
				Expect(decision.ProvidedReason).To(Equal(reason))
			})

			It("trims whitespace from reason", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "  ",
					},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("required"))
			})
		})

		Context("with valid reasons list", func() {
			BeforeEach(func() {
				required := true
				matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"GIT022": {
							RequireReason: &required,
							ValidReasons: []string{
								"Emergency hotfix",
								"Test fixture",
								"Approved by",
							},
						},
					},
				})
			})

			It("allows exact match", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "Emergency hotfix",
					},
				})
				Expect(decision.Allowed).To(BeTrue())
			})

			It("allows case-insensitive match", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "EMERGENCY HOTFIX",
					},
				})
				Expect(decision.Allowed).To(BeTrue())
			})

			It("allows prefix match", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "Approved by @manager",
					},
				})
				Expect(decision.Allowed).To(BeTrue())
			})

			It("denies non-matching reason", func() {
				decision := matcher.Match(&exceptions.ExceptionRequest{
					Token: &exceptions.Token{
						ErrorCode: "GIT022",
						Reason:    "Random reason",
					},
				})
				Expect(decision.Allowed).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("not in approved list"))
			})
		})
	})

	Describe("GetPolicyLimits", func() {
		Context("with no config", func() {
			BeforeEach(func() {
				matcher = exceptions.NewPolicyMatcher(nil)
			})

			It("returns zero (unlimited) for unknown error code", func() {
				maxHour, maxDay := matcher.GetPolicyLimits("GIT001")
				Expect(maxHour).To(Equal(0))
				Expect(maxDay).To(Equal(0))
			})
		})

		Context("with configured limits", func() {
			BeforeEach(func() {
				maxHour := 5
				maxDay := 20
				matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
					Policies: map[string]*config.ExceptionPolicyConfig{
						"SEC001": {
							MaxPerHour: &maxHour,
							MaxPerDay:  &maxDay,
						},
					},
				})
			})

			It("returns configured limits", func() {
				maxHour, maxDay := matcher.GetPolicyLimits("SEC001")
				Expect(maxHour).To(Equal(5))
				Expect(maxDay).To(Equal(20))
			})

			It("returns zero for unconfigured error code", func() {
				maxHour, maxDay := matcher.GetPolicyLimits("GIT022")
				Expect(maxHour).To(Equal(0))
				Expect(maxDay).To(Equal(0))
			})
		})
	})

	Describe("HasExplicitPolicy", func() {
		BeforeEach(func() {
			matcher = exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
				Policies: map[string]*config.ExceptionPolicyConfig{
					"GIT022": {},
				},
			})
		})

		It("returns true for configured error code", func() {
			Expect(matcher.HasExplicitPolicy("GIT022")).To(BeTrue())
		})

		It("returns false for unconfigured error code", func() {
			Expect(matcher.HasExplicitPolicy("SEC001")).To(BeFalse())
		})

		It("returns false with nil config", func() {
			m := exceptions.NewPolicyMatcher(nil)
			Expect(m.HasExplicitPolicy("GIT022")).To(BeFalse())
		})
	})
})
