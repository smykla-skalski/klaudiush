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

		Expect(
			truncatedSummary([]parser.Opacity{o}, false),
		).To(ContainSubstring("part of it is opaque"))
		Expect(opacityFinding(o)).To(SatisfyAll(
			HaveField("Message", "x cannot be inspected"),
			HaveField("Repair", validator.GetSuggestion(validator.RefShellNesting)),
		))
	})

	It("has a repair for every eval setup tool the parser names", func() {
		tools := make([]string, 0, len(evalSetupRepairs))
		for tool := range evalSetupRepairs {
			tools = append(tools, tool)
		}

		Expect(tools).To(ConsistOf(parser.EvalSetupTools()))
	})

	It("keeps the generic eval repair for a tool it does not know", func() {
		o := parser.Opacity{
			Cause:     parser.OpacityUnresolvedWord,
			Operation: "eval",
			Detail:    parser.DetailWordOutput,
			Tool:      "future",
		}

		Expect(opacityFinding(o)).To(
			HaveField("Repair", "Run the commands directly instead of through eval"),
		)
	})

	It("explains a push argument that hides a secret", func() {
		o := parser.Opacity{
			Cause:     parser.OpacityUnresolvedWord,
			Operation: parser.ArgumentOperation("push"),
			Detail:    parser.DetailWordSecret,
		}

		Expect(opacityFinding(o)).To(SatisfyAll(
			HaveField("Message", ContainSubstring("looks like a secret")),
			HaveField("Repair", ContainSubstring("configured remote by name")),
		))
	})

	It("falls back when a truncated parse has no explanation", func() {
		Expect(truncatedSummary(nil, false)).To(Equal(truncatedText))
	})
})
