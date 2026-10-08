package factory_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/config/factory"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// overrunTimeout is the timeout of the plugin that never answers. It bounds
// the plugin's --info at load too, so it must outlast a process start on a
// busy machine. The plugin execs sleep so no child keeps the output pipe open
// after the kill.
const overrunTimeout = 2 * time.Second

// writePluginScript writes an exec plugin that answers validation with body.
// It runs the plugin once before returning: macOS checks a new file the first
// time it runs, and under load that check alone outlasts the 5s the loader
// gives --version.
func writePluginScript(dir, name, body string) string {
	path := filepath.Join(dir, name)
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  --info) echo '{"name":"%s","version":"1.0.0"}'; exit 0 ;;
  --version) echo "1.0.0"; exit 0 ;;
esac
cat >/dev/null
%s
`, name, body)

	Expect(os.WriteFile(path, []byte(script), 0o755)).To(Succeed())
	Expect(exec.Command(path, "--version").Run()).To(Succeed())

	return path
}

var _ = Describe("Plugin failures", func() {
	var (
		root      string
		pluginDir string
		hookCtx   *hook.Context
	)

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		pluginDir = filepath.Join(root, ".klaudiush", "plugins")
		Expect(os.MkdirAll(pluginDir, 0o755)).To(Succeed())

		hookCtx = &hook.Context{
			Provider:  hook.ProviderClaude,
			Event:     hook.CanonicalEventBeforeTool,
			EventType: hook.EventTypePreToolUse,
			ToolName:  hook.ToolTypeBash,
			ToolInput: hook.ToolInput{Command: "ls"},
		}
	})

	instance := func(name, path string) *config.PluginInstanceConfig {
		return &config.PluginInstanceConfig{
			Name:        name,
			Type:        config.PluginTypeExec,
			Path:        path,
			ProjectRoot: root,
			Timeout:     config.Duration(5 * time.Second),
		}
	}

	validate := func(plugins ...*config.PluginInstanceConfig) *validator.Result {
		cfg := &config.Config{Plugins: &config.PluginConfig{Enabled: new(true), Plugins: plugins}}

		validators := factory.NewPluginValidatorFactory(logger.NewNoOpLogger()).
			CreateValidators(cfg)
		Expect(validators).To(HaveLen(1))

		return validators[0].Validator.Validate(context.Background(), hookCtx)
	}

	pass := `echo '{"passed":true,"should_block":false,"message":"ok"}'`
	warn := `echo '{"passed":false,"should_block":false,"message":"careful"}'`
	block := `echo '{"passed":false,"should_block":true,"message":"denied"}'`

	It("keeps loaded plugins and reports one that failed to load", func() {
		broken := instance("broken", "/nonexistent/plugin")
		broken.Type = config.PluginType("invalid")

		result := validate(instance("ok", writePluginScript(pluginDir, "ok", pass)), broken)

		Expect(result.Unavailable).To(BeTrue())
		Expect(result.ShouldBlock).To(BeFalse())
		Expect(result.Message).To(ContainSubstring("Plugin broken failed to load"))
	})

	It("does not report a failed plugin for contexts it would not check", func() {
		broken := instance("codex-only", "/nonexistent/plugin")
		broken.Type = config.PluginType("invalid")
		broken.Predicate = &config.PluginPredicate{Providers: []string{"codex"}}

		Expect(validate(broken).Passed).To(BeTrue())
	})

	It("blocks on a plugin that exits non-zero", func() {
		result := validate(instance("crash", writePluginScript(pluginDir, "crash", "exit 1")))

		Expect(result.Unavailable).To(BeTrue())
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.UnavailableReason).To(Equal(validator.ReasonError))
	})

	It("reports unreadable plugin output", func() {
		result := validate(
			instance("garbage", writePluginScript(pluginDir, "garbage", "echo nope")),
		)

		Expect(result.UnavailableReason).To(Equal(validator.ReasonMalformedOutput))
	})

	It("reports a response without a verdict", func() {
		for _, body := range []string{"echo '{}'", "echo null"} {
			result := validate(instance("empty", writePluginScript(pluginDir, "empty", body)))

			Expect(result.UnavailableReason).To(Equal(validator.ReasonMalformedOutput), body)
			Expect(result.ShouldBlock).To(BeTrue())
		}
	})

	It("reports a plugin that ran past its timeout", func() {
		slow := instance("slow", writePluginScript(pluginDir, "slow", "exec sleep 60"))
		slow.Timeout = config.Duration(overrunTimeout)

		result := validate(slow)

		Expect(result.UnavailableReason).To(Equal(validator.ReasonTimeout))
	})

	It("lets a real block win and lists the unavailable plugin", func() {
		result := validate(
			instance("deny", writePluginScript(pluginDir, "deny", block)),
			instance("crash", writePluginScript(pluginDir, "crash", "exit 1")),
		)

		Expect(result.Unavailable).To(BeFalse())
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Message).To(ContainSubstring("denied"))
		Expect(result.Message).To(ContainSubstring("Plugin error (crash)"))
	})

	It("keeps warnings next to an unavailable plugin", func() {
		result := validate(
			instance("warn", writePluginScript(pluginDir, "warn", warn)),
			instance("crash", writePluginScript(pluginDir, "crash", "exit 1")),
		)

		Expect(result.Unavailable).To(BeTrue())
		Expect(result.ShouldBlock).To(BeTrue())
		Expect(result.Message).To(ContainSubstring("careful"))
	})
})
