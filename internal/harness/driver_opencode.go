package harness

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const openCodeModel = "scripted/klaudiush"

// openCodeToolRename is the major version that renamed bash to shell, write's
// filePath to path, and replaced the plugin API.
const openCodeToolRename = 2

// OpenCodeDriver runs `opencode run` with a private server.
type OpenCodeDriver struct {
	*harnessBinary
	version string
}

// NewOpenCodeDriver resolves opencode from KLAUDIUSH_HARNESS_OPENCODE or PATH.
func NewOpenCodeDriver() *OpenCodeDriver {
	return &OpenCodeDriver{
		harnessBinary: newHarnessBinary("KLAUDIUSH_HARNESS_OPENCODE", "opencode"),
	}
}

func (*OpenCodeDriver) Name() string            { return "opencode" }
func (*OpenCodeDriver) Provider() hook.Provider { return hook.ProviderOpenCode }

// SetVersion selects the tool names of the opencode major version.
func (d *OpenCodeDriver) SetVersion(version string) { d.version = version }

func (*OpenCodeDriver) KnownGap(_ string) string { return "" }

// Supports leaves out the completion gate (session.idle cannot keep the
// agent working), subagents (not driven yet), the permission flow (run
// --auto) and unrelated hooks (opencode has no declarative hook config).
// After-tool repair is driven on 2.x, whose bridge appends findings to the
// tool result.
func (d *OpenCodeDriver) Supports(feature Feature) bool {
	if feature == FeatureAfterToolRepair {
		return MajorVersion(d.version) >= openCodeToolRename
	}

	return slices.Contains([]Feature{FeatureWriteTool}, feature)
}

// Prepare puts opencode on the sandbox PATH, where `klaudiush init` looks for
// it to pick the bridge plugin API, the same way it finds it for a user.
func (d *OpenCodeDriver) Prepare(sb *Sandbox, model *ScriptedModel) error {
	if d.binary != "" {
		if err := os.MkdirAll(sb.Bin, dirPerm); err != nil {
			return errors.Wrap(err, "creating sandbox bin")
		}

		link := filepath.Join(sb.Bin, "opencode")
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "replacing the sandbox opencode link")
		}

		if err := os.Symlink(d.binary, link); err != nil {
			return errors.Wrap(err, "linking opencode into the sandbox")
		}
	}

	config := map[string]any{
		"$schema":    "https://opencode.ai/config.json",
		"autoupdate": false,
		"share":      "disabled",
		jsonModel:    openCodeModel,
		"provider": map[string]any{
			"scripted": map[string]any{
				"npm":    "@ai-sdk/openai-compatible",
				jsonName: "scripted",
				"options": map[string]any{
					"baseURL": model.URL() + "/v1",
					"apiKey":  "unused",
				},
				"models": map[string]any{
					"klaudiush": map[string]any{jsonName: "klaudiush", "tool_call": true},
				},
			},
		},
	}

	return writeJSONFile(sb, filepath.Join(sb.ConfigHome(), "opencode", "opencode.json"), config)
}

func (*OpenCodeDriver) ProviderConfig(_ *Sandbox) string {
	return "[providers.claude]\nenabled = false\n\n[providers.opencode]\nenabled = true\n"
}

func (*OpenCodeDriver) HookFile(sb *Sandbox) string {
	return filepath.Join(sb.ConfigHome(), "opencode", "plugin", "klaudiush.ts")
}

func (*OpenCodeDriver) SeedUnrelatedHook(_ *Sandbox, _ string) error { return nil }

func (*OpenCodeDriver) AfterInstall(_ context.Context, _ *Sandbox, _ []Feature) error {
	return nil
}

// Run uses a private server: without --standalone, opencode 2.x attaches to
// the user's background service, which runs with the real configuration.
func (d *OpenCodeDriver) Run(
	ctx context.Context,
	sb *Sandbox,
	prompt string,
	_ RunOptions,
) ([]byte, error) {
	return RunIn(ctx, sb, sb.Work, d.resolve(ctx),
		"run", "--standalone", "--auto", "--model", openCodeModel, prompt)
}

func (d *OpenCodeDriver) ShellCall(command string) Call {
	if MajorVersion(d.version) >= openCodeToolRename {
		return Call{Tool: "shell", Args: map[string]any{jsonCommand: command}}
	}

	return Call{
		Tool: "bash",
		Args: map[string]any{jsonCommand: command, jsonDescription: stepDescription},
	}
}

func (d *OpenCodeDriver) WriteCall(sb *Sandbox, rel, content string) Call {
	path := filepath.Join(sb.Work, rel)

	if MajorVersion(d.version) >= openCodeToolRename {
		return Call{Tool: "write", Args: map[string]any{"path": path, jsonContent: content}}
	}

	return Call{Tool: "write", Args: map[string]any{"filePath": path, jsonContent: content}}
}

func (*OpenCodeDriver) SubagentCall(prompt string) Call {
	return Call{Tool: "task", Args: map[string]any{
		jsonDescription: "harness subagent", "prompt": prompt, "subagent_type": "general",
	}}
}
