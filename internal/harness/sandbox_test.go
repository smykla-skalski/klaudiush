package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/harness"
)

func newSandbox() *harness.Sandbox {
	sb, err := harness.NewSandbox(GinkgoT().TempDir())
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() { Expect(sb.Close()).To(Succeed()) })

	return sb
}

func envMap(env []string) map[string]string {
	out := map[string]string{}

	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		out[key] = value
	}

	return out
}

var _ = Describe("Sandbox", func() {
	It("builds the environment from scratch inside the sandbox", func() {
		GinkgoT().Setenv("KLAUDIUSH_LEAK_PROBE", "leaked")
		GinkgoT().Setenv("OPENCODE_CONFIG_DIR", "/real/opencode")

		sb := newSandbox()
		sb.SetEnv("EXTRA", "1")

		env := envMap(sb.Env())
		Expect(env).NotTo(HaveKey("KLAUDIUSH_LEAK_PROBE"))
		Expect(env).NotTo(HaveKey("OPENCODE_CONFIG_DIR"))
		Expect(env).To(HaveKeyWithValue("EXTRA", "1"))
		Expect(env["HOME"]).To(Equal(sb.Home))
		Expect(env["CODEX_HOME"]).To(Equal(sb.CodexHome()))
		Expect(env["CLAUDE_CONFIG_DIR"]).To(Equal(sb.ClaudeHome()))

		for _, key := range []string{
			"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "TMPDIR", "CLAUDE_CODE_TMPDIR",
		} {
			Expect(env[key]).To(HavePrefix(sb.Root), key)
		}

		path := strings.Split(env["PATH"], string(os.PathListSeparator))
		Expect(path[0]).To(Equal(sb.Bin))
		Expect(path).NotTo(ContainElement(ContainSubstring(os.Getenv("HOME"))))
	})

	It("records hook invocations through the shim and answers like the real binary", func() {
		sb := newSandbox()
		binary := filepath.Join(sb.Root, "real-klaudiush")
		Expect(sb.WriteFile(binary, "#!/bin/sh\ncat > /dev/null\necho '{\"decision\":\"block\"}'\n"+
			"echo warn >&2\nexit 3\n", 0o700)).To(Succeed())
		Expect(sb.InstallShim(binary)).To(Succeed())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		out, err := harness.RunIn(ctx, sb, sb.Work, "sh", "-c",
			`echo '{"hook_event_name":"Stop"}' | klaudiush --provider codex --event Stop`)
		Expect(err).To(HaveOccurred(), "the shim keeps the exit status")
		Expect(string(out)).To(ContainSubstring(`{"decision":"block"}`))
		Expect(string(out)).To(ContainSubstring("warn"))

		captures, err := sb.ReadCaptures()
		Expect(err).NotTo(HaveOccurred())
		Expect(captures).To(HaveLen(1))
		Expect(captures[0].Event()).To(Equal("Stop"))
		Expect(captures[0].Status).To(Equal(3))
		Expect(string(captures[0].Input)).To(ContainSubstring(`"Stop"`))
		Expect(string(captures[0].Stderr)).To(ContainSubstring("warn"))
	})

	It("skips hooks that are still running and reports broken captures", func() {
		sb := newSandbox()
		Expect(
			sb.WriteFile(filepath.Join(sb.Captures, "hook.running.in"), "{}", 0o600),
		).To(Succeed())
		Expect(sb.WriteFile(filepath.Join(sb.Captures, "hook.running"), "", 0o600)).To(Succeed())

		captures, err := sb.ReadCaptures()
		Expect(err).NotTo(HaveOccurred())
		Expect(captures).To(BeEmpty())

		Expect(
			sb.WriteFile(filepath.Join(sb.Captures, "hook.running.status"), "x", 0o600),
		).To(Succeed())
		_, err = sb.ReadCaptures()
		Expect(err).To(MatchError(ContainSubstring("parsing status")))

		Expect(
			sb.WriteFile(filepath.Join(sb.Captures, "hook.running.status"), "0", 0o600),
		).To(Succeed())
		_, err = sb.ReadCaptures()
		Expect(err).To(MatchError(ContainSubstring("reading")))
	})

	It("names no event for a capture without one", func() {
		Expect(harness.Capture{Args: []string{"init"}}.Event()).To(BeEmpty())
		Expect(
			harness.Capture{Args: []string{"--hook-type", "PreToolUse"}}.Event(),
		).To(Equal("PreToolUse"))
	})

	It("redacts sandbox paths, slugs and extra strings", func() {
		sb := newSandbox()
		sb.AddRedaction("/opt/build/klaudiush", "klaudiush")
		sb.AddRedaction("", "ignored")

		slug := strings.NewReplacer("/", "-", ".", "-", "_", "-").Replace(sb.Work)
		text := sb.Work + "/a " + sb.Home + "/.codex " + sb.Root + "/captures " +
			"projects/" + slug + "/x /opt/build/klaudiush evidence run"

		Expect(sb.Redact(text)).To(Equal(
			"{{WORK}}/a {{HOME}}/.codex {{ROOT}}/captures projects/{{WORK_SLUG}}/x klaudiush evidence run",
		))
	})

	It("fails when the base directory does not exist", func() {
		_, err := harness.NewSandbox(filepath.Join(GinkgoT().TempDir(), "missing"))
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("HomeGuard", func() {
	It("reports created, changed and removed guarded files only", func() {
		home := GinkgoT().TempDir()
		config := filepath.Join(home, "xdg")

		settings := filepath.Join(home, ".claude", "settings.json")
		Expect(os.MkdirAll(filepath.Dir(settings), 0o750)).To(Succeed())
		Expect(os.WriteFile(settings, []byte("{}"), 0o600)).To(Succeed())

		guard, err := harness.GuardHome(home, config)
		Expect(err).NotTo(HaveOccurred())
		Expect(guard.Paths()).To(ContainElement(filepath.Join(config, "klaudiush", "config.toml")))
		Expect(guard.Changed()).To(BeEmpty())

		Expect(os.WriteFile(filepath.Join(home, "unguarded.txt"), []byte("x"), 0o600)).To(Succeed())
		Expect(guard.Changed()).To(BeEmpty())

		Expect(os.WriteFile(settings, []byte(`{"hooks":{}}`), 0o600)).To(Succeed())

		hooks := filepath.Join(home, ".codex", "hooks.json")
		Expect(os.MkdirAll(filepath.Dir(hooks), 0o750)).To(Succeed())
		Expect(os.WriteFile(hooks, []byte("{}"), 0o600)).To(Succeed())

		Expect(guard.Changed()).To(ConsistOf(settings, hooks))
	})

	It("defaults the config home to ~/.config", func() {
		home := GinkgoT().TempDir()
		guard, err := harness.GuardHome(home, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(
			guard.Paths(),
		).To(ContainElement(filepath.Join(home, ".config", "opencode", "opencode.json")))
	})
})
