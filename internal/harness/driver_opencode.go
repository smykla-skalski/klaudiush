package harness

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	openCodeModel = "scripted/klaudiush"

	// openCodeToolRename is the major version that renamed bash to shell,
	// write's filePath to path, and replaced the plugin API.
	openCodeToolRename = 2
)

// openCode2Gap is why klaudiush does not enforce on opencode 2.x: the
// generated bridge plugin uses the 1.x plugin API (a module of named plugin
// functions returning tool.execute.* hooks). opencode 2.x loads only a
// default export {id, setup|effect} and registers tool hooks through
// ctx.tool.hook("execute.before"), so it skips the bridge with a warning and
// runs every tool unchecked.
const openCode2Gap = "opencode 2.x rejects the klaudiush bridge plugin (1.x plugin API), " +
	"so no tool call is checked"

// OpenCodeDriver runs `opencode run` with a private server.
type OpenCodeDriver struct {
	binary    string
	binaryErr error
	version   string
}

// NewOpenCodeDriver resolves opencode from KLAUDIUSH_HARNESS_OPENCODE or PATH.
func NewOpenCodeDriver() *OpenCodeDriver {
	binary, err := ResolveBinary("KLAUDIUSH_HARNESS_OPENCODE", "opencode")

	return &OpenCodeDriver{binary: binary, binaryErr: err}
}

func (*OpenCodeDriver) Name() string            { return "opencode" }
func (*OpenCodeDriver) Provider() hook.Provider { return hook.ProviderOpenCode }
func (d *OpenCodeDriver) Binary() string        { return d.binary }
func (d *OpenCodeDriver) BinaryError() error    { return d.binaryErr }

// SetVersion selects the tool names of the opencode major version.
func (d *OpenCodeDriver) SetVersion(version string) { d.version = version }

func (*OpenCodeDriver) KnownGap(version string) string {
	if MajorVersion(version) >= openCodeToolRename {
		return openCode2Gap
	}

	return ""
}

// Supports leaves out the completion gate (session.idle cannot keep the
// agent working), after-tool repair and subagents (not driven yet), the
// permission flow (run --auto) and unrelated hooks (opencode has no
// declarative hook config).
func (*OpenCodeDriver) Supports(feature Feature) bool {
	return slices.Contains([]Feature{FeatureWriteTool}, feature)
}

func (*OpenCodeDriver) Prepare(sb *Sandbox, model *ScriptedModel) error {
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
	return RunIn(ctx, sb, sb.Work, d.binary,
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
