package color_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/color"
)

func TestColor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Color Suite")
}

var _ = Describe("Profile", func() {
	// Ginkgo's T().Setenv handles save/restore automatically.
	// We just need to unset vars we want absent.
	clearColorEnv := func() {
		os.Unsetenv("NO_COLOR")
		os.Unsetenv("CLICOLOR")
		os.Unsetenv("TERM")
	}

	BeforeEach(func() {
		clearColorEnv()
	})

	It("returns true when no env vars disable color and flag is false", func() {
		Expect(color.Profile(false)).To(BeTrue())
	})

	It("returns false when --no-color flag is true", func() {
		Expect(color.Profile(true)).To(BeFalse())
	})

	It("returns false when NO_COLOR is set to empty string", func() {
		GinkgoT().Setenv("NO_COLOR", "")
		Expect(color.Profile(false)).To(BeFalse())
	})

	It("returns false when NO_COLOR is set to any value", func() {
		GinkgoT().Setenv("NO_COLOR", "1")
		Expect(color.Profile(false)).To(BeFalse())
	})

	It("returns false when CLICOLOR=0", func() {
		GinkgoT().Setenv("CLICOLOR", "0")
		Expect(color.Profile(false)).To(BeFalse())
	})

	It("returns true when CLICOLOR=1", func() {
		GinkgoT().Setenv("CLICOLOR", "1")
		Expect(color.Profile(false)).To(BeTrue())
	})

	It("returns false when TERM=dumb", func() {
		GinkgoT().Setenv("TERM", "dumb")
		Expect(color.Profile(false)).To(BeFalse())
	})

	It("returns true when TERM is xterm-256color", func() {
		GinkgoT().Setenv("TERM", "xterm-256color")
		Expect(color.Profile(false)).To(BeTrue())
	})

	It("flag takes precedence over CLICOLOR=1", func() {
		GinkgoT().Setenv("CLICOLOR", "1")
		Expect(color.Profile(true)).To(BeFalse())
	})
})

var _ = Describe("IsTerminal", func() {
	It("returns false for a pipe", func() {
		r, w, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		defer r.Close()
		defer w.Close()

		Expect(color.IsTerminal(r)).To(BeFalse())
	})

	It("returns false for a regular file", func() {
		f, err := os.CreateTemp("", "color-test-*")
		Expect(err).NotTo(HaveOccurred())

		defer os.Remove(f.Name())
		defer f.Close()

		Expect(color.IsTerminal(f)).To(BeFalse())
	})
})

var _ = Describe("NewTheme", func() {
	It("creates a theme with color styles that have foreground set", func() {
		theme := color.NewTheme(true)
		// Verify styles have color configured (lipgloss may strip ANSI in non-TTY)
		Expect(theme.Pass.GetForeground()).NotTo(BeNil())
		Expect(theme.Fail.GetForeground()).NotTo(BeNil())
		Expect(theme.Warning.GetForeground()).NotTo(BeNil())
		Expect(theme.Header.GetBold()).To(BeTrue())
		Expect(theme.CheckName.GetBold()).To(BeTrue())
		Expect(theme.Fail.GetBold()).To(BeTrue())
	})

	It("creates empty styles when color is disabled", func() {
		theme := color.NewTheme(false)
		rendered := theme.Pass.Render("ok")
		Expect(rendered).To(Equal("ok"))
		Expect(theme.Header.GetBold()).To(BeFalse())
		Expect(theme.CheckName.GetBold()).To(BeFalse())
	})
})
