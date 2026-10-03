package protection_test

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

func toolContext(name string, input map[string]any) *hook.Context {
	toolType, family := hook.ResolveToolMetadata(name)
	ctx := &hook.Context{RawToolName: name, ToolName: toolType, ToolFamily: family}
	ctx.ToolInput.Additional = map[string]json.RawMessage{}

	for key, value := range input {
		raw, err := json.Marshal(value)
		Expect(err).NotTo(HaveOccurred())

		switch key {
		case "file_path":
			ctx.ToolInput.FilePath = value.(string)
		case "path":
			ctx.ToolInput.Path = value.(string)
		case "command":
			ctx.ToolInput.Command = value.(string)
		default:
			ctx.ToolInput.Additional[key] = raw
		}
	}

	return ctx
}

var _ = Describe("ToolTargets", func() {
	It("returns the file of writes and edits", func() {
		ctx := toolContext("Write", map[string]any{"file_path": "a.go"})
		ctx.AffectedPaths = []string{"a.go"}

		Expect(protection.ToolTargets(ctx)).To(ContainElement("a.go"))
	})

	It("returns notebook and opencode paths", func() {
		ctx := toolContext("NotebookEdit", map[string]any{"notebook_path": "n.ipynb"})
		Expect(protection.ToolTargets(ctx)).To(ContainElement("n.ipynb"))

		ctx = toolContext("edit", map[string]any{"filePath": ".claude/settings.json"})
		Expect(protection.ToolTargets(ctx)).To(ContainElement(".claude/settings.json"))
	})

	It("reads every file of a patch, however it is spaced", func() {
		patch := strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: a.go",
			"***   add file :  .klaudiush/config.toml",
			"*** Delete File: .claude/settings.json",
			"*** Move to: b.go",
			"*** End Patch",
		}, "\n")

		ctx := toolContext("apply_patch", map[string]any{"command": patch})
		Expect(protection.ToolTargets(ctx)).To(ContainElements(
			"a.go", ".klaudiush/config.toml", ".claude/settings.json", "b.go",
		))

		ctx = toolContext("apply_patch", map[string]any{"input": patch})
		Expect(protection.ToolTargets(ctx)).To(ContainElement(".klaudiush/config.toml"))
	})

	It("ignores read-only tools", func() {
		for _, name := range []string{"Read", "Grep", "Glob", "mcp__fs__read_file", "mcp__fs__listDirectory", "WebFetch", "TodoWrite"} {
			ctx := toolContext(
				name,
				map[string]any{"path": ".claude/settings.json", "file_path": "x/y"},
			)
			Expect(protection.ToolTargets(ctx)).To(BeEmpty(), name)
		}
	})

	It("ignores shell tools", func() {
		ctx := toolContext("Bash", map[string]any{"command": "rm .claude/settings.json"})
		Expect(protection.ToolTargets(ctx)).To(BeEmpty())
		Expect(protection.ToolTargets(nil)).To(BeEmpty())
	})

	It("searches the input of other tools for paths", func() {
		ctx := toolContext("mcp__fs__move_file", map[string]any{
			"source":      "a.txt",
			"destination": ".claude/settings.json",
			"options":     map[string]any{"backup": []any{"~/.codex/hooks.json", "plain words", 3}},
			"url":         "https://example.com/x.json",
		})

		targets := protection.ToolTargets(ctx)
		Expect(targets).To(ContainElements("a.txt", ".claude/settings.json", "~/.codex/hooks.json"))
		Expect(targets).NotTo(ContainElement("plain words"))
		Expect(targets).NotTo(ContainElement("https://example.com/x.json"))
	})
})

var _ = Describe("PolicyCommand", func() {
	It("ignores other programs", func() {
		_, ok := protection.PolicyCommand(commandNamed("git", "status"))
		Expect(ok).To(BeFalse())
	})

	DescribeTable("allows read-only subcommands",
		func(args ...string) {
			_, ok := protection.PolicyCommand(commandNamed("klaudiush", args...))
			Expect(ok).To(BeFalse())
		},
		Entry("hook mode", "--provider", "codex", "--event", "PreToolUse"),
		Entry("version", "version"),
		Entry("evidence run", "evidence", "run", "tests"),
		Entry("doctor", "doctor", "--category", "protection"),
		Entry("debug rules", "debug", "rules"),
		Entry("crash view", "debug", "crash", "view", "x"),
		Entry("audit list", "audit", "list"),
		Entry("bypass status", "bypass", "status"),
		Entry("backup list", "backup", "list"),
	)
})
