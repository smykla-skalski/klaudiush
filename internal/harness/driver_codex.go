package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

const (
	codexModel  = scriptedModelName
	codexKeyEnv = "KLAUDIUSH_HARNESS_MODEL_KEY"
)

// codexTrustLimit bounds the app-server conversation that lists hooks.
var codexTrustLimit = 30 * time.Second

// codexCatalog describes the scripted model to Codex. Without it Codex falls
// back to metadata that does not offer apply_patch, so file writes could not
// be exercised.
const codexCatalog = `{"models":[{"slug":"` + codexModel + `","display_name":"klaudiush scripted",` +
	`"description":"scripted harness model","base_instructions":"Scripted test model.",` +
	`"default_reasoning_level":"low","supported_reasoning_levels":[{"effort":"low","description":"low"}],` +
	`"shell_type":"unified_exec","visibility":"list","supported_in_api":true,"priority":1,` +
	`"apply_patch_tool_type":"freeform","truncation_policy":{"mode":"tokens","limit":10000},` +
	`"experimental_supported_tools":[],"input_modalities":["text"],"context_window":200000,` +
	`"max_context_window":200000,"effective_context_window_percent":95,` +
	`"default_reasoning_summary":"none","default_verbosity":"low",` +
	`"supports_reasoning_effort_updates":false,"support_verbosity":false,` +
	`"supports_search_tool":false,"use_responses_lite":false}]}`

// CodexDriver runs `codex exec`.
type CodexDriver struct {
	*harnessBinary
}

// NewCodexDriver resolves codex from KLAUDIUSH_HARNESS_CODEX or PATH.
func NewCodexDriver() *CodexDriver {
	return &CodexDriver{harnessBinary: newHarnessBinary("KLAUDIUSH_HARNESS_CODEX", "codex")}
}

func (*CodexDriver) Name() string             { return "codex" }
func (*CodexDriver) Provider() hook.Provider  { return hook.ProviderCodex }
func (*CodexDriver) KnownGap(_ string) string { return "" }

// Supports leaves out after-tool repair (klaudiush registers no Codex
// PostToolUse), subagents (spawned through the multi-agent tool namespace,
// which the scripted model does not drive) and the permission flow (exec
// mode never asks).
func (*CodexDriver) Supports(feature Feature) bool {
	return slices.Contains([]Feature{
		FeatureWriteTool, FeatureCompletionGate, FeatureUnrelatedHook,
	}, feature)
}

func (d *CodexDriver) Prepare(sb *Sandbox, model *ScriptedModel) error {
	catalog := filepath.Join(sb.CodexHome(), "klaudiush-catalog.json")
	if err := sb.WriteFile(catalog, codexCatalog, filePerm); err != nil {
		return err
	}

	sb.SetEnv(codexKeyEnv, "unused")

	config := strings.Join([]string{
		fmt.Sprintf("model = %q", codexModel),
		`model_provider = "scripted"`,
		fmt.Sprintf("model_catalog_json = %q", catalog),
		"check_for_update_on_startup = false",
		"",
		"[model_providers.scripted]",
		`name = "scripted"`,
		fmt.Sprintf("base_url = %q", model.URL()+"/v1"),
		`wire_api = "responses"`,
		fmt.Sprintf("env_key = %q", codexKeyEnv),
		"",
	}, "\n")

	return sb.WriteFile(d.configPath(sb), config, filePerm)
}

func (*CodexDriver) configPath(sb *Sandbox) string {
	return filepath.Join(sb.CodexHome(), "config.toml")
}

func (d *CodexDriver) ProviderConfig(sb *Sandbox) string {
	return fmt.Sprintf("[providers.claude]\nenabled = false\n\n[providers.codex]\n"+
		"enabled = true\nexperimental = true\nhooks_config_path = %q\n", d.HookFile(sb))
}

func (*CodexDriver) HookFile(sb *Sandbox) string {
	return filepath.Join(sb.CodexHome(), "hooks.json")
}

func (d *CodexDriver) SeedUnrelatedHook(sb *Sandbox, logPath string) error {
	hooks := map[string]any{
		jsonHooks: map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": toolBash,
				jsonHooks: []any{map[string]any{
					jsonType: hookTypeCommand, jsonCommand: unrelatedHookCommand(logPath),
				}},
			}},
		},
	}

	return writeJSONFile(sb, d.HookFile(sb), hooks)
}

// AfterInstall trusts every hook Codex lists, the way the /hooks review
// does: Codex skips new or changed hooks until their hash is trusted.
func (d *CodexDriver) AfterInstall(ctx context.Context, sb *Sandbox, _ []Feature) error {
	hooks, err := d.listHooks(ctx, sb)
	if err != nil {
		return err
	}

	if len(hooks) == 0 {
		return errors.New("codex lists no hooks after install")
	}

	var trust strings.Builder

	for _, h := range hooks {
		fmt.Fprintf(
			&trust,
			"\n[hooks.state.%s]\ntrusted_hash = %q\n",
			tomlKey(h.Key),
			h.CurrentHash,
		)
	}

	file, err := os.OpenFile(d.configPath(sb), os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return errors.Wrap(err, "opening Codex config")
	}

	_, writeErr := file.WriteString(trust.String())

	return errors.CombineErrors(errors.Wrap(writeErr, "writing hook trust"), file.Close())
}

type codexHook struct {
	Key         string `json:"key"`
	CurrentHash string `json:"currentHash"`
	TrustStatus string `json:"trustStatus"`
}

// listHooks asks `codex app-server` for the hooks it would run in the
// sandbox project. Stdin stays open until the hooks/list reply arrives, so
// the server answers before it sees end of input and exits.
func (d *CodexDriver) listHooks(ctx context.Context, sb *Sandbox) ([]codexHook, error) {
	ctx, cancel := context.WithTimeout(ctx, codexTrustLimit)
	defer cancel()

	requests := []map[string]any{
		{jsonID: 1, jsonMethod: "initialize", "params": map[string]any{
			"clientInfo": map[string]any{jsonName: harnessIdentity, "version": "1"},
		}},
		{jsonMethod: "initialized"},
		{
			jsonID:     hookListRequest,
			jsonMethod: "hooks/list",
			"params":   map[string]any{"cwds": []string{sb.Work}},
		},
	}

	var input bytes.Buffer

	encoder := json.NewEncoder(&input)
	for _, request := range requests {
		if err := encoder.Encode(request); err != nil {
			return nil, errors.Wrap(err, "encoding app-server request")
		}
	}

	reply := &hookListReply{done: make(chan struct{})}

	opts := execpkg.RunOptions{
		Dir:    sb.Work,
		Env:    sb.Env(),
		Stdin:  &gatedReader{stop: ctx.Done(), data: &input, done: reply.done},
		Stdout: reply,
	}

	stopWatch, err := sb.track(&opts)
	if err != nil {
		return nil, err
	}

	defer stopWatch()

	result := execpkg.NewCommandRunner(0).RunWithOptions(ctx, opts, d.resolve(ctx), "app-server")

	stopWatch()

	_, _ = sb.Processes()

	if hooks, ok, err := reply.result(); ok {
		return hooks, err
	}

	if result.Err != nil {
		return nil, errors.Wrap(result.Err, "codex app-server")
	}

	return nil, errors.New("app-server closed before answering hooks/list")
}

// hookListRequest is the JSON-RPC id of the hooks/list request.
const hookListRequest = 2

// hookListReply collects app-server output until the hooks/list reply.
type hookListReply struct {
	mu      sync.Mutex
	pending []byte
	done    chan struct{}
	hooks   []codexHook
	err     error
	found   bool
}

func (r *hookListReply) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pending = append(r.pending, p...)

	for {
		line, rest, ok := bytes.Cut(r.pending, []byte("\n"))
		if !ok {
			break
		}

		r.pending = rest

		if !r.found {
			r.parse(line)
		}
	}

	return len(p), nil
}

func (r *hookListReply) parse(line []byte) {
	var reply struct {
		ID     int `json:"id"`
		Result struct {
			Data []struct {
				Hooks  []codexHook      `json:"hooks"`
				Errors []map[string]any `json:"errors"`
			} `json:"data"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}

	if err := json.Unmarshal(line, &reply); err != nil || reply.ID != hookListRequest {
		return
	}

	r.found = true

	if reply.Error != nil {
		r.err = errors.Newf("hooks/list failed: %s", reply.Error)
	}

	for _, entry := range reply.Result.Data {
		r.hooks = append(r.hooks, entry.Hooks...)

		if len(entry.Errors) > 0 && r.err == nil {
			r.err = errors.Newf("codex could not load its configuration: %s", entry.Errors)
		}
	}

	close(r.done)
}

func (r *hookListReply) result() ([]codexHook, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.hooks, r.found, r.err
}

// gatedReader yields data, then holds stdin open until done closes or the
// context ends, then reports end of input.
type gatedReader struct {
	stop <-chan struct{}
	data *bytes.Buffer
	done <-chan struct{}
}

func (g *gatedReader) Read(p []byte) (int, error) {
	if g.data.Len() > 0 {
		return g.data.Read(p)
	}

	select {
	case <-g.done:
	case <-g.stop:
	}

	return 0, io.EOF
}

func (d *CodexDriver) Run(
	ctx context.Context,
	sb *Sandbox,
	prompt string,
	_ RunOptions,
) ([]byte, error) {
	return RunIn(ctx, sb, sb.Work, d.resolve(ctx),
		"exec", "--skip-git-repo-check", "--sandbox", "workspace-write", "--color", "never", prompt)
}

func (*CodexDriver) ShellCall(command string) Call {
	return Call{Tool: "exec_command", Args: map[string]any{"cmd": command}}
}

func (*CodexDriver) WriteCall(_ *Sandbox, rel, content string) Call {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")

	var patch strings.Builder

	patch.WriteString("*** Begin Patch\n*** Add File: " + rel + "\n")

	for _, line := range lines {
		patch.WriteString("+" + line + "\n")
	}

	patch.WriteString("*** End Patch\n")

	return Call{Tool: "apply_patch", Input: patch.String()}
}

func (*CodexDriver) SubagentCall(prompt string) Call {
	return Call{Tool: "spawn_agent", Args: map[string]any{jsonMessage: prompt}}
}

func tomlKey(key string) string {
	return "'" + strings.ReplaceAll(key, "'", "") + "'"
}
