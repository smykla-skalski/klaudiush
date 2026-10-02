package settings_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
)

var _ = Describe("InstallClaudeDispatcher", func() {
	const binary = "/usr/local/bin/klaudiush"

	readHooks := func(path string) map[string]any {
		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())

		var raw map[string]any
		Expect(json.Unmarshal(data, &raw)).To(Succeed())

		hooks, ok := raw["hooks"].(map[string]any)
		Expect(ok).To(BeTrue())

		return hooks
	}

	It("registers failed tools next to the pre- and post-tool hooks", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")

		already, err := settings.InstallClaudeDispatcher(path, binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(already).To(BeFalse())

		hooks := readHooks(path)
		for _, event := range settings.ClaudeDispatcherEvents() {
			Expect(hooks).To(HaveKey(event))
		}

		failure, ok := hooks[settings.ClaudeEventPostToolUseFailure].([]any)
		Expect(ok).To(BeTrue())
		Expect(failure).To(HaveLen(1))

		group, ok := failure[0].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(group["matcher"]).To(Equal("Bash|Write|Edit|MultiEdit"))

		handlers, ok := group["hooks"].([]any)
		Expect(ok).To(BeTrue())

		handler, ok := handlers[0].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(handler["command"]).To(Equal(binary + " --hook-type PostToolUseFailure"))

		already, err = settings.InstallClaudeDispatcher(path, binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(already).To(BeTrue())
	})

	It("adds only the missing failure hook to an older install", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")
		raw := map[string]any{}
		settings.AddClaudeDispatcherHooks(raw, binary, map[string]bool{
			settings.ClaudeEventPreToolUse:  true,
			settings.ClaudeEventPostToolUse: true,
		})

		data, err := json.Marshal(raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(path, data, 0o600)).To(Succeed())

		already, err := settings.InstallClaudeDispatcher(path, binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(already).To(BeFalse())

		hooks := readHooks(path)
		Expect(hooks[settings.ClaudeEventPreToolUse]).To(HaveLen(1))
		Expect(hooks[settings.ClaudeEventPostToolUse]).To(HaveLen(1))
		Expect(hooks[settings.ClaudeEventPostToolUseFailure]).To(HaveLen(1))
	})
})
