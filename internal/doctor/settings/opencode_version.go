package settings

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	execpkg "github.com/smykla-skalski/klaudiush/internal/exec"
)

// OpenCodeAPI is the opencode plugin API a bridge plugin targets. 1.x loads
// named plugin functions returning hook keys; 2.x loads a default export
// {id, setup} that registers hooks through the context it receives.
type OpenCodeAPI string

// Known opencode plugin APIs.
const (
	OpenCodeAPIUnknown OpenCodeAPI = ""
	OpenCodeAPIV1      OpenCodeAPI = "1.x"
	OpenCodeAPIV2      OpenCodeAPI = "2.x"
)

const (
	openCodeBinaryName     = "opencode"
	openCodeVersionTimeout = 30 * time.Second
	openCodeMajorV2        = 2
)

// ErrOpenCodeNotInstalled is returned when no opencode binary is on PATH.
var ErrOpenCodeNotInstalled = errors.New("opencode not found in PATH")

var openCodeVersionPattern = regexp.MustCompile(`\d+\.\d+\.\d+[0-9A-Za-z.+-]*`)

// openCodeV2Registrations maps a forwarded event to the 2.x subscription that
// feeds it. The 2.x API registers hooks per domain and renamed the turn-end
// and compaction bus events, so the forwarded name never appears as a hook key
// or case label there.
var openCodeV2Registrations = map[string]string{
	"tool.execute.before": `ctx.tool.hook("execute.before",`,
	"tool.execute.after":  `ctx.tool.hook("execute.after",`,
	"chat.message":        `ctx.session.hook("prompt",`,
	"session.compacting":  `ctx.session.hook("compaction",`,
	"session.idle":        `case "session.execution.succeeded":`,
	"session.compacted":   `case "session.compaction.ended":`,
}

// OpenCodeVersionDetector reports the installed opencode version, or
// ErrOpenCodeNotInstalled.
type OpenCodeVersionDetector interface {
	Detect(ctx context.Context) (string, error)
}

// CommandOpenCodeVersionDetector runs `opencode --version` from PATH.
type CommandOpenCodeVersionDetector struct {
	tools  execpkg.ToolChecker
	runner execpkg.CommandRunner
}

// NewOpenCodeVersionDetector creates a detector backed by the real PATH.
func NewOpenCodeVersionDetector() *CommandOpenCodeVersionDetector {
	return NewOpenCodeVersionDetectorWith(
		execpkg.NewToolChecker(),
		execpkg.NewCommandRunner(openCodeVersionTimeout),
	)
}

// NewOpenCodeVersionDetectorWith creates a detector with injected execution.
func NewOpenCodeVersionDetectorWith(
	tools execpkg.ToolChecker,
	runner execpkg.CommandRunner,
) *CommandOpenCodeVersionDetector {
	return &CommandOpenCodeVersionDetector{tools: tools, runner: runner}
}

// Detect runs `opencode --version` and extracts the version number.
func (d *CommandOpenCodeVersionDetector) Detect(ctx context.Context) (string, error) {
	if !d.tools.IsAvailable(openCodeBinaryName) {
		return "", ErrOpenCodeNotInstalled
	}

	result := d.runner.Run(ctx, openCodeBinaryName, "--version")
	if result.Failed() {
		return "", errors.Wrapf(result.Err, "opencode --version failed: %s",
			strings.TrimSpace(result.Stderr))
	}

	version := openCodeVersionPattern.FindString(result.Stdout)
	if version == "" {
		return "", errors.Newf("no version in opencode --version output %q",
			strings.TrimSpace(result.Stdout))
	}

	return version, nil
}

// OpenCodeAPIForVersion returns the plugin API an opencode version loads.
func OpenCodeAPIForVersion(version string) OpenCodeAPI {
	majorText, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")

	major, err := strconv.Atoi(majorText)
	if err != nil {
		return OpenCodeAPIUnknown
	}

	if major >= openCodeMajorV2 {
		return OpenCodeAPIV2
	}

	return OpenCodeAPIV1
}

// DetectOpenCodePluginAPI classifies a bridge plugin source by the API its
// exports follow. 2.x validates only the default export as {id, setup}; 1.x
// calls every export as a plugin function, so a default object breaks it.
func DetectOpenCodePluginAPI(source string) OpenCodeAPI {
	hasDefault := strings.Contains(source, "export default")
	hasNamed := strings.Contains(source, "export const ") ||
		strings.Contains(source, "export async function ") ||
		strings.Contains(source, "export function ")

	switch {
	case hasDefault && !hasNamed && strings.Contains(source, "setup"):
		return OpenCodeAPIV2
	case hasNamed && !hasDefault:
		return OpenCodeAPIV1
	default:
		return OpenCodeAPIUnknown
	}
}

// OpenCodeTarget is the bridge plugin API to install. Version is the detected
// opencode version, empty when DetectErr explains why it is unknown.
type OpenCodeTarget struct {
	API       OpenCodeAPI
	Version   string
	DetectErr error
}

// Detected reports whether the target comes from the installed opencode.
func (t OpenCodeTarget) Detected() bool {
	return t.Version != ""
}

// Describe names the target for user-facing messages.
func (t OpenCodeTarget) Describe() string {
	if t.Detected() {
		return "opencode " + t.Version
	}

	return "opencode " + string(t.API) + " (version not detected)"
}

// ResolveOpenCodeTarget picks the bridge plugin API to install.
//
// The installed opencode decides when it can be detected. Otherwise the API of
// the plugin already installed is kept, so running doctor where opencode is not
// on PATH does not flip a working bridge, and a fresh install falls back to
// 1.x, which is what earlier releases installed.
func ResolveOpenCodeTarget(
	ctx context.Context,
	detector OpenCodeVersionDetector,
	pluginPath string,
) OpenCodeTarget {
	version, err := detector.Detect(ctx)
	if err == nil {
		if api := OpenCodeAPIForVersion(version); api != OpenCodeAPIUnknown {
			return OpenCodeTarget{API: api, Version: version}
		}

		err = errors.Newf("unrecognized opencode version %q", version)
	}

	target := OpenCodeTarget{API: OpenCodeAPIV1, DetectErr: err}

	source, readErr := NewOpenCodePluginParser(pluginPath).Read()
	if readErr == nil {
		if api := DetectOpenCodePluginAPI(source); api != OpenCodeAPIUnknown {
			target.API = api
		}
	}

	return target
}
