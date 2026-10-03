package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// stopHookTimeout matches the timeout klaudiush init gives the other hooks.
const stopHookTimeout = 30

// claudeFakeKey is sent to the scripted model only. It is not a credential.
const claudeFakeKey = "klaudiush-harness-scripted-model"

// ClaudeDriver runs Claude Code in print mode.
type ClaudeDriver struct {
	binary    string
	binaryErr error
}

// NewClaudeDriver resolves claude from KLAUDIUSH_HARNESS_CLAUDE or PATH.
func NewClaudeDriver() *ClaudeDriver {
	binary, err := ResolveBinary("KLAUDIUSH_HARNESS_CLAUDE", "claude")

	return &ClaudeDriver{binary: binary, binaryErr: err}
}

func (*ClaudeDriver) Name() string             { return "claude" }
func (*ClaudeDriver) Provider() hook.Provider  { return hook.ProviderClaude }
func (d *ClaudeDriver) Binary() string         { return d.binary }
func (d *ClaudeDriver) BinaryError() error     { return d.binaryErr }
func (*ClaudeDriver) KnownGap(_ string) string { return "" }

func (*ClaudeDriver) Supports(feature Feature) bool {
	return slices.Contains([]Feature{
		FeatureWriteTool, FeatureAfterToolRepair, FeatureCompletionGate,
		FeatureSubagent, FeaturePermissionFlow, FeatureUnrelatedHook,
	}, feature)
}

func (*ClaudeDriver) Prepare(sb *Sandbox, model *ScriptedModel) error {
	sb.SetEnv("ANTHROPIC_BASE_URL", model.URL())
	sb.SetEnv("ANTHROPIC_API_KEY", claudeFakeKey)
	sb.SetEnv("DISABLE_AUTOUPDATER", "1")
	sb.SetEnv("DISABLE_TELEMETRY", "1")
	sb.SetEnv("DISABLE_ERROR_REPORTING", "1")
	sb.SetEnv("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")

	return nil
}

func (*ClaudeDriver) ProviderConfig(_ *Sandbox) string {
	return "[providers.claude]\nenabled = true\n"
}

func (*ClaudeDriver) HookFile(sb *Sandbox) string {
	return filepath.Join(sb.Work, ".claude", "settings.json")
}

func (d *ClaudeDriver) SeedUnrelatedHook(sb *Sandbox, logPath string) error {
	settings := map[string]any{
		jsonHooks: map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": toolBash,
				jsonHooks: []any{map[string]any{
					jsonType: hookTypeCommand, jsonCommand: unrelatedHookCommand(logPath),
				}},
			}},
		},
	}

	return writeJSONFile(sb, d.HookFile(sb), settings)
}

// AfterInstall registers Stop for the completion gate: `klaudiush init`
// leaves Claude Stop to the user (see the evidence guide).
func (d *ClaudeDriver) AfterInstall(_ context.Context, sb *Sandbox, features []Feature) error {
	if !slices.Contains(features, FeatureCompletionGate) {
		return nil
	}

	data, err := os.ReadFile(d.HookFile(sb))
	if err != nil {
		return errors.Wrap(err, "reading Claude settings")
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return errors.Wrap(err, "parsing Claude settings")
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return errors.New("Claude settings have no hooks")
	}

	hooks["Stop"] = []any{map[string]any{jsonHooks: []any{
		map[string]any{
			jsonType:    "command",
			jsonCommand: "klaudiush --provider claude --event Stop",
			"timeout":   stopHookTimeout,
		},
	}}}

	return writeJSONFile(sb, d.HookFile(sb), settings)
}

func (d *ClaudeDriver) Run(
	ctx context.Context,
	sb *Sandbox,
	prompt string,
	opts RunOptions,
) ([]byte, error) {
	allowed := opts.AllowedTools
	if allowed == nil {
		allowed = []string{toolBash, toolWrite, "Agent"}
	}

	args := []string{
		"-p", prompt, "--output-format", "json", "--model", "claude-haiku-4-5",
		"--max-turns", "12", "--permission-mode", "default",
	}

	if len(allowed) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowed, ","))
	}

	return RunIn(ctx, sb, sb.Work, d.binary, args...)
}

func (*ClaudeDriver) ShellCall(command string) Call {
	return Call{
		Tool: toolBash,
		Args: map[string]any{jsonCommand: command, jsonDescription: stepDescription},
	}
}

func (*ClaudeDriver) WriteCall(sb *Sandbox, rel, content string) Call {
	return Call{Tool: toolWrite, Args: map[string]any{
		"file_path": filepath.Join(sb.Work, rel), jsonContent: content,
	}}
}

func (*ClaudeDriver) SubagentCall(prompt string) Call {
	return Call{Tool: "Agent", Args: map[string]any{
		jsonDescription: "harness subagent", "prompt": prompt, "subagent_type": "general-purpose",
	}}
}

func unrelatedHookCommand(logPath string) string {
	return "printf 'unrelated hook ran\\n' >> " + shellQuote(logPath)
}

func writeJSONFile(sb *Sandbox, path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return errors.Wrap(err, "encoding JSON")
	}

	return sb.WriteFile(path, string(data)+"\n", filePerm)
}
