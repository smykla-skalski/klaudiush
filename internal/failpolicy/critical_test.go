package failpolicy_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var _ = Describe("WithCritical", func() {
	It("adds critical validators without changing the original", func() {
		base := failpolicy.New(&config.FailurePolicyConfig{Critical: []string{"git.commit"}})
		extended := base.WithCritical("policy.protection", "mcp-trust")

		Expect(extended.IsCritical("protection")).To(BeTrue())
		Expect(extended.IsCritical("mcp-trust")).To(BeTrue())
		Expect(extended.IsCritical("commit")).To(BeTrue())
		Expect(base.IsCritical("protection")).To(BeFalse())
		Expect(extended.Deadline()).To(Equal(base.Deadline()))
	})

	It("works on a nil policy", func() {
		var policy *failpolicy.Policy

		Expect(policy.WithCritical("protection").IsCritical("protection")).To(BeTrue())
	})

	It("knows the policy validator names", func() {
		Expect(failpolicy.IsKnownName(failpolicy.NormalizeName("policy.protection"))).To(BeTrue())
		Expect(failpolicy.IsKnownName(failpolicy.NormalizeName("policy.mcp_trust"))).To(BeTrue())
	})
})
