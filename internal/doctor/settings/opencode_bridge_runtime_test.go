package settings_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
)

const fakeKlaudiush = `#!/bin/sh
cat >/dev/null
case "$4" in
tool.execute.before) echo '{"decision":"deny","reason":"DENIED-BY-TEST"}' ;;
*) echo '{"hookSpecificOutput":{"additionalContext":"FIX-IT"}}' ;;
esac
`

const bridgeDriver = `import { pathToFileURL } from "node:url"

const [bridgePath, directory] = process.argv.slice(2)
const mod = await import(pathToFileURL(bridgePath).href)
const hooks = {}
const register = (domain) => async (name, callback) => {
  hooks[domain + "." + name] = callback
  return { dispose() {} }
}
const ctx = {
  location: { directory },
  tool: { hook: register("tool") },
  session: { hook: register("session") },
  event: {
    subscribe: () => ({
      [Symbol.asyncIterator]() {
        return { next: () => new Promise(() => {}) }
      },
    }),
  },
}
const cleanup = await mod.default.setup(ctx)

const after = {}
for (const [name, result] of [
  ["string", { content: "done" }],
  ["array", { content: [{ type: "text", text: "done" }] }],
  ["absent", {}],
  ["missing", undefined],
]) {
  const event = { tool: "shell", sessionID: "s", id: "c", input: {}, status: "completed", result }
  await hooks["tool.execute.after"](event)
  after[name] = event.result.content
}

const refusal = async (event) => {
  try {
    await hooks["tool.execute.before"](event)
    return ""
  } catch (error) {
    return error.message
  }
}

const denied = await refusal({ tool: "shell", sessionID: "s", id: "c", input: { command: "x" } })
const moved = await refusal({ tool: "opencode_session_move", input: { directory: "sub" } })

await cleanup()
console.log(JSON.stringify({ after, denied, moved }))
process.exit(0)
`

type textPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type bridgeRun struct {
	After  map[string][]textPart `json:"after"`
	Denied string                `json:"denied"`
	Moved  string                `json:"moved"`
}

var _ = Describe("the rendered 2.x bridge at runtime", func() {
	It("delivers findings and refusals through the 2.x hooks", func() {
		node, err := exec.LookPath("node")
		if err != nil {
			Skip("node is not installed")
		}

		dir := GinkgoT().TempDir()
		binary := filepath.Join(dir, "klaudiush")
		Expect(os.WriteFile(binary, []byte(fakeKlaudiush), 0o700)).To(Succeed())

		rendered, err := settings.RenderOpenCodePlugin(binary, settings.OpenCodeAPIV2)
		Expect(err).NotTo(HaveOccurred())

		bridge := filepath.Join(dir, "bridge.mts")
		Expect(os.WriteFile(bridge, rendered, 0o600)).To(Succeed())

		driver := filepath.Join(dir, "driver.mjs")
		Expect(os.WriteFile(driver, []byte(bridgeDriver), 0o600)).To(Succeed())

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		probe := exec.CommandContext(ctx, node, "--experimental-strip-types", "-e", "0")
		if probe.Run() != nil {
			Skip("node cannot strip TypeScript types")
		}

		cmd := exec.CommandContext(
			ctx, node, "--experimental-strip-types", "--no-warnings", driver, bridge, dir,
		)
		cmd.Dir = dir

		out, err := cmd.Output()
		Expect(err).NotTo(HaveOccurred(), string(out))

		var run bridgeRun
		Expect(json.Unmarshal(out, &run)).To(Succeed(), string(out))

		finding := textPart{Type: "text", Text: "FIX-IT"}
		done := textPart{Type: "text", Text: "done"}

		Expect(run.After["string"]).To(Equal([]textPart{done, finding}))
		Expect(run.After["array"]).To(Equal([]textPart{done, finding}))
		Expect(run.After["absent"]).To(Equal([]textPart{finding}))
		Expect(run.After["missing"]).To(Equal([]textPart{finding}))
		Expect(run.Denied).To(Equal("DENIED-BY-TEST"))
		Expect(run.Moved).To(ContainSubstring("cannot move"))
	})
})
