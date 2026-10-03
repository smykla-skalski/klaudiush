package harness

import (
	"bytes"
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Feature is a harness behavior a scenario depends on.
type Feature string

const (
	FeatureWriteTool       Feature = "write_tool"
	FeatureAfterToolRepair Feature = "after_tool_repair"
	FeatureCompletionGate  Feature = "completion_gate"
	FeatureSubagent        Feature = "subagent"
	FeaturePermissionFlow  Feature = "permission_flow"
	FeatureUnrelatedHook   Feature = "unrelated_hook"
)

// RunOptions tunes one harness run. AllowedTools overrides the tools a
// harness runs without asking: nil keeps the driver default, an empty slice
// allows none.
type RunOptions struct {
	AllowedTools []string
}

// Driver runs one harness against the scripted model.
//
// Binary is the resolved executable ("" when missing), and BinaryError
// says why a harness on PATH could not be resolved. Prepare points the
// harness at the model. ProviderConfig is the klaudiush [providers] TOML that
// makes `klaudiush init --install-hooks` register the harness, and HookFile
// is where those hooks land. SeedUnrelatedHook registers a user hook that has
// nothing to do with klaudiush before install. AfterInstall does what a user
// does after installing (Codex hook trust review) plus what a feature needs.
// KnownGap explains why klaudiush cannot enforce on a harness version, or
// returns "" when it should.
type Driver interface {
	Name() string
	Provider() hook.Provider
	Binary() string
	BinaryError() error
	Prepare(sb *Sandbox, model *ScriptedModel) error
	ProviderConfig(sb *Sandbox) string
	HookFile(sb *Sandbox) string
	SeedUnrelatedHook(sb *Sandbox, logPath string) error
	AfterInstall(ctx context.Context, sb *Sandbox, features []Feature) error
	Run(ctx context.Context, sb *Sandbox, prompt string, opts RunOptions) ([]byte, error)
	Supports(feature Feature) bool
	ShellCall(command string) Call
	WriteCall(sb *Sandbox, rel, content string) Call
	SubagentCall(prompt string) Call
	KnownGap(version string) string
}

const decimalBase = 10

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?`)

// Version runs `<binary> --version` in the sandbox and returns the version
// number it prints.
func Version(ctx context.Context, sb *Sandbox, binary string) (string, error) {
	out, err := RunIn(ctx, sb, sb.Work, binary, "--version")
	if err != nil {
		return "", errors.Wrapf(err, "%s --version failed in the sandbox (a version manager shim "+
			"needs its config; point the override variable at the real binary): %s",
			binary, strings.TrimSpace(string(out)))
	}

	version := versionPattern.FindString(string(out))
	if version == "" {
		return "", errors.Newf("no version in %q", strings.TrimSpace(string(out)))
	}

	return version, nil
}

// MajorVersion returns the leading number of a version string.
func MajorVersion(version string) int {
	major := 0

	for _, r := range version {
		if r < '0' || r > '9' {
			break
		}

		major = major*decimalBase + int(r-'0')
	}

	return major
}

// RunIn runs a command in the sandbox environment with stdin closed and
// returns its combined output. The command runs in a session the sandbox
// tracks, so StopProcesses finds what it leaves running; the session is
// dropped again once nothing runs in it.
func RunIn(ctx context.Context, sb *Sandbox, dir, name string, args ...string) ([]byte, error) {
	var out bytes.Buffer

	opts := execpkg.RunOptions{
		Dir:    dir,
		Env:    sb.Env(),
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &out,
	}

	stopWatch, err := sb.track(&opts)
	if err != nil {
		return nil, err
	}

	defer stopWatch()

	result := execpkg.NewCommandRunner(0).RunWithOptions(ctx, opts, name, args...)

	stopWatch()

	_, _ = sb.Processes()

	return out.Bytes(), errors.Wrapf(
		result.Err,
		"%s %s",
		filepath.Base(name),
		strings.Join(args, " "),
	)
}
