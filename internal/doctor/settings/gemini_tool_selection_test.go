package settings_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
)

var _ = Describe("Gemini tool selection", func() {
	It("registers BeforeToolSelection once, next to the dispatcher hooks", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")
		Expect(os.WriteFile(path, []byte(`{"ui":{"theme":"x"}}`), 0o600)).To(Succeed())

		_, err := settings.InstallGeminiDispatcher(path, "/bin/klaudiush")
		Expect(err).NotTo(HaveOccurred())

		present, err := settings.InstallGeminiToolSelection(path, "/bin/klaudiush")
		Expect(err).NotTo(HaveOccurred())
		Expect(present).To(BeFalse())

		present, err = settings.InstallGeminiToolSelection(path, "/bin/klaudiush")
		Expect(err).NotTo(HaveOccurred())
		Expect(present).To(BeTrue())

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(
			settings.GeminiToolSelectionCommand("/bin/klaudiush")))
		Expect(string(data)).To(ContainSubstring(`"theme"`))

		parser := settings.NewGeminiSettingsParser(path)
		registered, err := parser.HasEventHook("BeforeToolSelection", "/bin/klaudiush")
		Expect(err).NotTo(HaveOccurred())
		Expect(registered).To(BeTrue())

		matchers, err := parser.GeminiBeforeToolMatchers("klaudiush")
		Expect(err).NotTo(HaveOccurred())
		Expect(matchers).To(HaveLen(1))
	})

	It("reports a settings file it cannot read", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")
		Expect(os.WriteFile(path, []byte(`{`), 0o600)).To(Succeed())

		_, err := settings.InstallGeminiToolSelection(path, "/bin/klaudiush")
		Expect(err).To(HaveOccurred())

		_, err = settings.NewGeminiSettingsParser(path).GeminiBeforeToolMatchers("klaudiush")
		Expect(err).To(HaveOccurred())

		matchers, err := settings.NewGeminiSettingsParser(path + ".missing").
			GeminiBeforeToolMatchers("klaudiush")
		Expect(err).NotTo(HaveOccurred())
		Expect(matchers).To(BeEmpty())
	})

	DescribeTable("GeminiMatcherSelects tests an unanchored regex",
		func(matcher, tool string, selected bool) {
			Expect(settings.GeminiMatcherSelects(matcher, tool)).To(Equal(selected))
		},
		Entry("empty", "", "save_memory", true),
		Entry("star", " * ", "save_memory", true),
		Entry("substring", "grep", "grep_search", true),
		Entry("alternation", "write_file|replace", "replace", true),
		Entry("no match", "read_file", "write_file", false),
		Entry("invalid regex compared literally", "[x", "[x", true),
		Entry("invalid regex not matching", "[x", "write_file", false),
	)
})
