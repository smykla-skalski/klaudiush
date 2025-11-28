package notification_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/validators/notification"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

var _ = Describe("BellValidator", func() {
	var (
		validator *notification.BellValidator
		ctx       *hook.Context
	)

	BeforeEach(func() {
		validator = notification.NewBellValidator(logger.NewNoOpLogger(), nil, nil)
		ctx = &hook.Context{
			EventType: hook.EventTypeNotification,
		}
	})

	Describe("Validate", func() {
		Context("when notification type is permission_prompt", func() {
			BeforeEach(func() {
				ctx.NotificationType = "permission_prompt"
			})

			It("should pass", func() {
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})

			It("should not block", func() {
				result := validator.Validate(context.Background(), ctx)
				Expect(result.ShouldBlock).To(BeFalse())
			})
		})

		Context("when notification type is bell", func() {
			BeforeEach(func() {
				ctx.NotificationType = "bell"
			})

			It("should pass", func() {
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})

		Context("when notification type is empty", func() {
			It("should pass", func() {
				result := validator.Validate(context.Background(), ctx)
				Expect(result.Passed).To(BeTrue())
			})
		})
	})
})
