package validator_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/validator"
)

var _ = Describe("Unavailable", func() {
	It("neither passes nor blocks and carries the reason", func() {
		result := validator.Unavailable(validator.ReasonTimeout, "shellcheck timed out")

		Expect(result.Passed).To(BeFalse())
		Expect(result.ShouldBlock).To(BeFalse())
		Expect(result.Unavailable).To(BeTrue())
		Expect(result.ReasonOf()).To(Equal(validator.ReasonTimeout))
		Expect(result.Reference.Code()).To(Equal("HOOK001"))
		Expect(result.FixHint).NotTo(BeEmpty())
	})

	It("defaults the reason of an unavailable result to error", func() {
		Expect(validator.Fail("x").MarkUnavailable().ReasonOf()).To(Equal(validator.ReasonError))
		Expect(validator.Pass().ReasonOf()).To(BeEmpty())
	})

	DescribeTable("describes every reason",
		func(reason validator.UnavailableReason, want string) {
			Expect(reason.Describe()).To(Equal(want))
		},
		Entry("missing tool", validator.ReasonMissingTool, "required tool not installed"),
		Entry("timeout", validator.ReasonTimeout, "timed out"),
		Entry("canceled", validator.ReasonCanceled, "canceled"),
		Entry("panic", validator.ReasonPanic, "crashed"),
		Entry("malformed output", validator.ReasonMalformedOutput, "unreadable output"),
		Entry("malformed input", validator.ReasonMalformedInput, "unreadable hook input"),
		Entry("config", validator.ReasonConfig, "configuration error"),
		Entry("state", validator.ReasonState, "session state unavailable"),
		Entry("error", validator.ReasonError, "failed to run"),
		Entry("unknown", validator.UnavailableReason("other"), "failed to run"),
	)

	Describe("ReasonFromContext", func() {
		It("is empty while the context is live", func() {
			Expect(validator.ReasonFromContext(context.Background())).To(BeEmpty())
		})

		It("reports a passed deadline as a timeout", func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
			defer cancel()

			<-ctx.Done()
			Expect(validator.ReasonFromContext(ctx)).To(Equal(validator.ReasonTimeout))
		})

		It("reports a cancellation as canceled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			Expect(validator.ReasonFromContext(ctx)).To(Equal(validator.ReasonCanceled))
		})
	})
})
