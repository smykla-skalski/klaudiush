package tui_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-labs/klaudiush/internal/tui"
)

var _ = Describe("TUI", func() {
	Describe("IsTerminal", func() {
		It("returns a boolean", func() {
			// IsTerminal checks if stdin/stdout are connected to a terminal.
			// In CI/test environments, this will typically return false.
			result := tui.IsTerminal()
			Expect(result).To(BeAssignableToTypeOf(true))
		})
	})

	Describe("New", func() {
		It("returns a UI implementation", func() {
			ui := tui.New()
			Expect(ui).NotTo(BeNil())
		})

		It("returns a UI that implements the interface", func() {
			ui := tui.New()
			// Verify it has the IsInteractive method
			_ = ui.IsInteractive()
		})

		Context("in non-TTY environment (CI)", func() {
			It("returns FallbackUI", func() {
				// In CI/test environments stdin/stdout are not TTYs,
				// so New() should return FallbackUI
				ui := tui.New()
				Expect(ui.IsInteractive()).To(BeFalse())
			})
		})
	})

	Describe("NewWithFallback", func() {
		Context("when noTUI is true", func() {
			It("returns FallbackUI regardless of terminal state", func() {
				ui := tui.NewWithFallback(true)
				Expect(ui).NotTo(BeNil())
				Expect(ui.IsInteractive()).To(BeFalse())
			})
		})

		Context("when noTUI is false", func() {
			It("returns a UI implementation", func() {
				ui := tui.NewWithFallback(false)
				Expect(ui).NotTo(BeNil())
			})

			It("delegates to New()", func() {
				// In CI/test environments, this should behave the same as New()
				uiWithFallback := tui.NewWithFallback(false)
				uiFromNew := tui.New()
				Expect(uiWithFallback.IsInteractive()).To(Equal(uiFromNew.IsInteractive()))
			})
		})
	})

	Describe("NewHuhUI", func() {
		It("returns a HuhUI instance", func() {
			ui := tui.NewHuhUI()
			Expect(ui).NotTo(BeNil())
		})

		It("is interactive", func() {
			ui := tui.NewHuhUI()
			Expect(ui.IsInteractive()).To(BeTrue())
		})
	})

	Describe("NewFallbackUI", func() {
		It("returns a FallbackUI instance", func() {
			ui := tui.NewFallbackUI()
			Expect(ui).NotTo(BeNil())
		})

		It("is not interactive", func() {
			ui := tui.NewFallbackUI()
			Expect(ui.IsInteractive()).To(BeFalse())
		})
	})
})
