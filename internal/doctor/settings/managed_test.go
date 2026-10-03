package settings_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
)

var _ = Describe("HookCommandsInFile", func() {
	write := func(name, content string) string {
		path := filepath.Join(GinkgoT().TempDir(), name)
		Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

		return path
	}

	It("reads JSON hook commands", func() {
		path := write(
			"s.json",
			`{"hooks":{"PreToolUse":[{"hooks":[{"command":"a"},{"command":"b"}]}]},"command":"x"}`,
		)
		Expect(settings.HookCommandsInFile(path)).To(ConsistOf("a", "b"))
	})

	It("reads TOML hook commands", func() {
		path := write(
			"r.toml",
			"[[hooks.PreToolUse]]\n[[hooks.PreToolUse.hooks]]\ncommand = \"c\"\n",
		)
		Expect(settings.HookCommandsInFile(path)).To(ConsistOf("c"))
	})

	It("returns nothing for missing or invalid files", func() {
		Expect(
			settings.HookCommandsInFile(filepath.Join(GinkgoT().TempDir(), "none.json")),
		).To(BeEmpty())
		Expect(settings.HookCommandsInFile(write("bad.json", "{"))).To(BeEmpty())
	})

	It("knows where managed hooks live", func() {
		Expect(settings.ManagedHookFiles()).NotTo(BeEmpty())
	})
})
