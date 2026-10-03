package evidence

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

var (
	// ErrInvalidPhase marks a tool phase configuration that cannot be used.
	ErrInvalidPhase = errors.New("invalid evidence tool phase")

	// ErrPhaseDisabled marks a configuration whose tool phase is off.
	ErrPhaseDisabled = errors.New("evidence tool phase is disabled")
)

// Gemini tool names the tool phase keeps or withholds by itself.
const (
	GeminiShellTool = "run_shell_command"
	GeminiWriteTool = "write_file"
	GeminiEditTool  = "replace"
)

// Verifier subcommands a restricted phase lets through the shell tool.
const (
	verifierCommand = "evidence"
	verifierRun     = "run"
	verifierStatus  = "status"
)

// Phase is a compiled tool phase: mutation tools stay withheld until every
// prerequisite check has a passing result on the current content.
type Phase struct {
	Requires      []*Check
	ReadOnlyTools []string
	WritablePaths []string
}

// CompilePhase validates the tool phase against the compiled checks. A
// disabled phase returns ErrPhaseDisabled.
func CompilePhase(cfg *config.EvidenceConfig, checks []*Check) (*Phase, error) {
	phaseCfg := cfg.GetToolPhase()
	if !phaseCfg.IsEnabled() {
		return nil, ErrPhaseDisabled
	}

	if !cfg.IsEnabled() {
		return nil, errors.Wrap(ErrInvalidPhase, "tool_phase needs evidence.enabled = true")
	}

	if len(phaseCfg.Requires) == 0 {
		return nil, errors.Wrap(ErrInvalidPhase, "tool_phase.requires must name at least one check")
	}

	phase := &Phase{WritablePaths: slices.Clone(phaseCfg.WritablePaths)}

	for _, name := range phaseCfg.Requires {
		check := Find(checks, name)
		if check == nil {
			return nil, errors.Wrapf(ErrInvalidPhase,
				"tool_phase.requires names %q, which is not a configured check", name)
		}

		if !slices.Contains(phase.Requires, check) {
			phase.Requires = append(phase.Requires, check)
		}
	}

	for _, tool := range phaseCfg.GetReadOnlyTools() {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return nil, errors.Wrap(ErrInvalidPhase, "tool_phase.read_only_tools has an empty name")
		}

		if isMutationTool(tool) {
			return nil, errors.Wrapf(ErrInvalidPhase,
				"tool_phase.read_only_tools lists %q, which changes files", tool)
		}

		phase.ReadOnlyTools = append(phase.ReadOnlyTools, tool)
	}

	for _, pattern := range phase.WritablePaths {
		if pattern == "" || filepath.IsAbs(pattern) || !doublestar.ValidatePattern(pattern) {
			return nil, errors.Wrapf(ErrInvalidPhase,
				"tool_phase.writable_paths has invalid pattern %q", pattern)
		}
	}

	return phase, nil
}

// isMutationTool reports the Gemini tools the phase governs itself: listing
// them as read-only would offer them without the phase's per-call limits.
func isMutationTool(tool string) bool {
	switch tool {
	case GeminiShellTool, GeminiWriteTool, GeminiEditTool:
		return true
	default:
		return false
	}
}

// RequiredNames lists the prerequisite check names.
func (p *Phase) RequiredNames() []string {
	names := make([]string, 0, len(p.Requires))
	for _, check := range p.Requires {
		names = append(names, check.Name)
	}

	return names
}

// AllowedTools lists the tools offered while the phase is restricted: the
// read-only tools, the shell for the verifier, and the file tools when some
// paths stay writable. Sorted, without duplicates.
func (p *Phase) AllowedTools() []string {
	tools := slices.Concat(p.ReadOnlyTools, []string{GeminiShellTool})
	if len(p.WritablePaths) > 0 {
		tools = append(tools, GeminiWriteTool, GeminiEditTool)
	}

	slices.Sort(tools)

	return slices.Compact(tools)
}

// AllowsReadOnly reports whether a tool is offered as read-only.
func (p *Phase) AllowsReadOnly(tool string) bool {
	return slices.Contains(p.ReadOnlyTools, tool)
}

// Writable reports whether a file tool may change path while the phase is
// restricted. The path is resolved through existing symbolic links, so a
// link inside a writable directory cannot reach a file outside it. Files
// that define or run the prerequisites are never writable: klaudiush and
// harness configuration in any directory (a nested config would change what
// the verifier runs), git state, and the files the prerequisite commands
// name.
func (p *Phase) Writable(repoRoot, path string) bool {
	if len(p.WritablePaths) == 0 || path == "" || repoRoot == "" {
		return false
	}

	if danglingLink(path) {
		return false
	}

	root := resolveExisting(repoRoot)
	target := resolveExisting(path)

	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	rel = filepath.ToSlash(rel)
	if definesPolicy(rel) || p.runsPrerequisite(rel) {
		return false
	}

	return matchAny(p.WritablePaths, rel)
}

// policyNames are path components that hold klaudiush or harness
// configuration, compared without case since macOS and Windows file systems
// ignore it.
var policyNames = []string{
	".klaudiush", "klaudiush.toml", ".git", ".gemini", ".claude", ".codex", ".mcp.json",
}

func definesPolicy(rel string) bool {
	for part := range strings.SplitSeq(rel, "/") {
		for _, name := range policyNames {
			if strings.EqualFold(part, name) {
				return true
			}
		}
	}

	return false
}

// interpreters run the script their first operand names.
var interpreters = []string{
	"bash", "dash", "node", "perl", "python", "python3", "ruby", "sh", "zsh",
}

// runsPrerequisite reports a repository-relative path that a prerequisite
// command runs: its program, or the script an interpreter is given. Files a
// check only reads, such as the plan it tests, stay writable.
func (p *Phase) runsPrerequisite(rel string) bool {
	for _, check := range p.Requires {
		for _, argv := range check.Commands {
			for _, word := range programWords(argv) {
				if !filepath.IsAbs(word) &&
					strings.EqualFold(filepath.ToSlash(filepath.Clean(word)), rel) {
					return true
				}
			}
		}
	}

	return false
}

func programWords(argv []string) []string {
	words := []string{argv[0]}
	if !slices.Contains(interpreters, filepath.Base(argv[0])) {
		return words
	}

	for _, arg := range argv[1:] {
		if !strings.HasPrefix(arg, "-") {
			return append(words, arg)
		}
	}

	return words
}

// danglingLink reports a symbolic link whose target does not exist yet:
// writing through it would create a file wherever it points.
func danglingLink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}

	_, err = filepath.EvalSymlinks(path)

	return err != nil
}

// resolveExisting resolves the symbolic links of the longest existing prefix
// of an absolute path and keeps the rest as written.
func resolveExisting(path string) string {
	path = filepath.Clean(path)

	var rest []string

	for current := path; ; current = filepath.Dir(current) {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...)
		}

		if current == filepath.Dir(current) {
			return path
		}

		rest = append([]string{filepath.Base(current)}, rest...)
	}
}

// AllowsVerifier reports whether a shell command only runs the klaudiush
// verifier for a prerequisite check, or shows evidence status, so a
// restricted phase can let it through. The command must be one command of
// literal words with no redirections, run where the hook runs: a cd would
// let the verifier load another directory's configuration. Its program
// must be binary itself, named by absolute path or found on PATH.
func (p *Phase) AllowsVerifier(command, binary string) bool {
	if binary == "" || hasRedirect(command) {
		return false
	}

	argv, err := literalArgv(command)
	if err != nil {
		return false
	}

	if len(argv) < 2 || argv[1] != verifierCommand || !sameProgram(argv[0], binary) {
		return false
	}

	switch {
	case len(argv) == 3 && argv[2] == verifierStatus:
		return true
	case len(argv) == 4 && argv[2] == verifierRun:
		return slices.Contains(p.RequiredNames(), argv[3])
	default:
		return false
	}
}

func hasRedirect(command string) bool {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(command), "")
	if err != nil {
		return true
	}

	found := false

	syntax.Walk(file, func(node syntax.Node) bool {
		if _, ok := node.(*syntax.Redirect); ok {
			found = true
		}

		return !found
	})

	return found
}

// sameProgram reports whether name runs binary. A relative path depends on
// a working directory the hook cannot see, so only an absolute path or a
// bare name looked up on PATH counts.
func sameProgram(name, binary string) bool {
	switch {
	case filepath.IsAbs(name):
	case strings.ContainsAny(name, `/\`):
		return false
	default:
		found, err := exec.LookPath(name)
		if err != nil {
			return false
		}

		name = found
	}

	if !filepath.IsAbs(binary) {
		found, err := exec.LookPath(binary)
		if err != nil {
			return false
		}

		binary = found
	}

	want, err := os.Stat(binary)
	if err != nil {
		return false
	}

	got, err := os.Stat(name)
	if err != nil {
		return false
	}

	return os.SameFile(want, got)
}
