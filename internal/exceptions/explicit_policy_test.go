package exceptions_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/exceptions"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("codes that guard policy", func() {
	It("need an explicit policy before a token bypasses them", func() {
		for _, code := range []string{"POL001", "POL002", "POL003", "MCP004", "MCP005"} {
			Expect(exceptions.RequiresExplicitPolicy(code)).To(BeTrue())

			decision := exceptions.NewPolicyMatcher(nil).Match(&exceptions.ExceptionRequest{
				Token: &exceptions.Token{ErrorCode: code, Reason: "please let me"},
			})
			Expect(decision.Allowed).To(BeFalse(), code)
			Expect(decision.Reason).To(ContainSubstring("no explicit policy"))
		}

		Expect(exceptions.RequiresExplicitPolicy("GIT019")).To(BeFalse())
	})

	It("can be bypassed when the user writes a policy for the code", func() {
		allow := true
		matcher := exceptions.NewPolicyMatcher(&config.ExceptionsConfig{
			Policies: map[string]*config.ExceptionPolicyConfig{
				"POL001": {AllowException: &allow},
			},
		})

		decision := matcher.Match(&exceptions.ExceptionRequest{
			Token: &exceptions.Token{ErrorCode: "POL001", Reason: "approved maintenance"},
		})
		Expect(decision.Allowed).To(BeTrue())
	})
})
