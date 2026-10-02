package shell

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

var _ = Describe("opacity explanations", func() {
	It("falls back for a cause it does not know", func() {
		o := parser.Opacity{Cause: "future", Operation: "x"}

		Expect(truncatedSummary([]parser.Opacity{o})).To(ContainSubstring("part of it is opaque"))
		Expect(opacityFinding(o)).To(SatisfyAll(
			HaveField("Message", "x cannot be inspected"),
			HaveField("Repair", validator.GetSuggestion(validator.RefShellNesting)),
		))
	})

	It("falls back when a truncated parse has no explanation", func() {
		Expect(truncatedSummary(nil)).To(Equal(truncatedText))
	})
})
