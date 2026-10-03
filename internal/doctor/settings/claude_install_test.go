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

	It("registers PreToolUse next to a user hook that only mentions klaudiush", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")
		userHook := "/work/klaudiush-notes/log-hook.sh --quiet"
		seed := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[` +
			`{"type":"command","command":"` + userHook + `"},` +
			`{"type":"command","command":"/opt/tools/notify --tag klaudiush"}]}]}}`
		Expect(os.WriteFile(path, []byte(seed), 0o600)).To(Succeed())

		parser := settings.NewSettingsParser(path)
		Expect(parser.HasEventHookCommand(settings.ClaudeEventPreToolUse, binary)).To(BeFalse())
		Expect(parser.IsDispatcherRegistered(binary)).To(BeFalse())

		_, err := settings.InstallClaudeDispatcher(path, binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(parser.HasEventHookCommand(settings.ClaudeEventPreToolUse, binary)).To(BeTrue())

		pre, ok := readHooks(path)[settings.ClaudeEventPreToolUse].([]any)
		Expect(ok).To(BeTrue())
		Expect(pre).To(HaveLen(2), "the user hook group is kept and klaudiush is added")

		data, err := os.ReadFile(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(ContainSubstring(userHook))
	})

	DescribeTable(
		"recognizes the dispatcher behind wrappers",
		func(command string, want bool) {
			path := filepath.Join(GinkgoT().TempDir(), "settings.json")
			raw, err := json.Marshal(map[string]any{"hooks": map[string]any{"PreToolUse": []any{
				map[string]any{
					"hooks": []any{map[string]any{"type": "command", "command": command}},
				},
			}}})
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(path, raw, 0o600)).To(Succeed())

			Expect(
				settings.NewSettingsParser(path).
					HasEventHookCommand(settings.ClaudeEventPreToolUse, binary),
			).
				To(Equal(want))
		},
		Entry("sh -c", `sh -c 'klaudiush --hook-type PreToolUse'`, true),
		Entry(
			"bash -lc",
			`bash -l -c "exec /usr/local/bin/klaudiush --hook-type PreToolUse"`,
			true,
		),
		Entry("mise exec", "mise exec go@1 -- klaudiush --hook-type PreToolUse", true),
		Entry("nice", "nice -n 5 klaudiush --hook-type PreToolUse", true),
		Entry("timeout", "timeout -k 2 30 klaudiush --hook-type PreToolUse", true),
		Entry("env unset", "env -u DEBUG FOO=1 klaudiush --hook-type PreToolUse", true),
		Entry("quoted path with spaces", `"/opt/my tools/klaudiush" --hook-type PreToolUse`, true),
		Entry("script mentioning klaudiush", "sh -c 'notify --tag klaudiush'", false),
		Entry("argument mentioning klaudiush", "nice notify klaudiush", false),
		Entry("unparsable command", "klaudiush 'unterminated", true),
	)

	It("recognizes the dispatcher by program name with env assignments", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")
		seed := `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command",` +
			`"command":"KLAUDIUSH_DEBUG=1 /home/u/bin/klaudiush --hook-type PreToolUse"}]}]}}`
		Expect(os.WriteFile(path, []byte(seed), 0o600)).To(Succeed())

		parser := settings.NewSettingsParser(path)
		Expect(parser.HasEventHookCommand(settings.ClaudeEventPreToolUse, binary)).To(BeTrue())
		Expect(parser.IsDispatcherRegistered(binary)).To(BeTrue())
	})

	It("registers failed tools next to the pre- and post-tool hooks", func() {
		path := filepath.Join(GinkgoT().TempDir(), "settings.json")

		Expect(settings.NewSettingsParser(path).HasPostToolUseFailureHook()).To(BeFalse())

		already, err := settings.InstallClaudeDispatcher(path, binary)
		Expect(err).NotTo(HaveOccurred())
		Expect(already).To(BeFalse())
		Expect(settings.NewSettingsParser(path).HasPostToolUseFailureHook()).To(BeTrue())

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
