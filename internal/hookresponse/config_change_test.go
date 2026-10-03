package hookresponse_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

var _ = Describe("Claude ConfigChange", func() {
	It("blocks the change with a top-level decision", func() {
		fields := responseFields(hookresponse.BuildForContext(
			eventCtx(hook.ProviderClaude, "ConfigChange"), semanticsBlocking(), nil,
		))

		Expect(fields).To(HaveKeyWithValue("decision", "block"))
		Expect(fields).To(HaveKey("reason"))
		Expect(fields).NotTo(HaveKey("hookSpecificOutput"))
	})

	It("does not block on warnings", func() {
		fields := responseFields(hookresponse.BuildForContext(
			eventCtx(hook.ProviderClaude, "ConfigChange"), semanticsWarning(), nil,
		))

		Expect(fields).NotTo(HaveKey("decision"))
	})
})
