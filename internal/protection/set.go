// Package protection decides which files enforce policy on the agent and
// whether a tool call or shell command would change one of them.
package protection

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// Reasons name the kind of policy file a path holds.
const (
	ReasonKlaudiushConfig = "klaudiush configuration"
	ReasonKlaudiushState  = "klaudiush state"
	ReasonKlaudiushBinary = "klaudiush binary"
	ReasonClaudeSettings  = "Claude Code settings"
	ReasonCodexHooks      = "Codex hook configuration"
	ReasonGeminiSettings  = "Gemini CLI settings"
	ReasonOpenCodePlugin  = "opencode bridge plugin"
	ReasonMCPConfig       = "MCP server configuration"
	ReasonHookScript      = "hook script"
	ReasonHookConfig      = "hook registration file"
	ReasonEvidenceScript  = "evidence check script"
	ReasonPlugin          = "klaudiush plugin"
	ReasonConfigured      = "protection.paths entry"
)

// maxEnumeratedFiles bounds how many files inside protected directories are
// listed for hard link and pattern checks.
const maxEnumeratedFiles = 512

// ErrBadPattern is returned for a protection pattern that does not compile.
var ErrBadPattern = errors.New("invalid protection pattern")

// ruleKind says how a rule matches: ruleAbs protects one absolute path (and
// what is below it for a tree), ruleSuffix protects a sequence of names
// wherever it appears (as the last names of a path, or anywhere in it for a
// tree), and ruleGlob protects what a pattern matches and what is below it.
type ruleKind int

const (
	ruleAbs ruleKind = iota
	ruleSuffix
	ruleGlob
)

type rule struct {
	kind    ruleKind
	path    string
	display string
	comps   []string
	names   []string
	glob    *regexp.Regexp
	tree    bool
	reason  string
}

// entry is a concrete protected path, used to tell whether a directory or
// a pattern takes in a protected file.
type entry struct {
	key    string
	path   string
	reason string
}

// Options says where the policy files of this hook live. WorkDir is the
// absolute directory the hook runs for and ProjectRoot the repository root
// (WorkDir outside a repository). ConfigDir, StateDir, DataDir and LegacyDir
// are the klaudiush directories; XDGConfigHome is where opencode keeps its
// plugins. HookFiles are configured hook registration files (Codex
// hooks_config_path, Gemini settings_path). Executables are the klaudiush
// binaries hooks run, OpenCodePlugin
// the configured bridge plugin, EvidenceCommands the commands of the
// evidence checks and PluginPaths the klaudiush plugin executables.
// LookupEnv defaults to os.LookupEnv and GOOS to runtime.GOOS.
type Options struct {
	WorkDir          string
	ProjectRoot      string
	Home             string
	ConfigDir        string
	StateDir         string
	DataDir          string
	LegacyDir        string
	XDGConfigHome    string
	HookFiles        []string
	Executables      []string
	OpenCodePlugin   string
	EvidenceCommands []string
	PluginPaths      []string
	Config           *config.ProtectionConfig
	LookupEnv        func(string) (string, bool)
	GOOS             string
}

// Set is the compiled set of protected paths for one hook.
type Set struct {
	rules       []rule
	allow       []rule
	entries     []entry
	foldCase    bool
	workDir     string
	projectRoot string
	home        string
	executables []string
	lookupEnv   func(string) (string, bool)
}

// NewSet builds the protected set. A configured pattern that does not
// compile is an error; Validate reports the same errors when the
// configuration loads.
func NewSet(opts Options) (*Set, error) {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}

	if opts.LookupEnv == nil {
		opts.LookupEnv = os.LookupEnv
	}

	if opts.ProjectRoot == "" {
		opts.ProjectRoot = opts.WorkDir
	}

	s := &Set{
		foldCase:    foldsCase(opts.GOOS),
		workDir:     filepath.Clean(opts.WorkDir),
		projectRoot: filepath.Clean(opts.ProjectRoot),
		home:        filepath.Clean(opts.Home),
		lookupEnv:   opts.LookupEnv,
	}

	s.addBuiltins(opts)
	s.addScripts(opts)

	for _, pattern := range opts.Config.GetPaths() {
		r, err := s.compileUserPattern(pattern, ReasonConfigured)
		if err != nil {
			return nil, err
		}

		s.rules = append(s.rules, s.withCanonical(r, true)...)
	}

	for _, pattern := range opts.Config.GetAllow() {
		r, err := s.compileUserPattern(pattern, "")
		if err != nil {
			return nil, err
		}

		s.allow = append(s.allow, s.withCanonical(r, false)...)
	}

	s.materialize()

	return s, nil
}

// Validate reports protection patterns that do not compile and unknown
// ConfigChange sources.
func Validate(cfg *config.ProtectionConfig) error {
	s := &Set{workDir: "/", projectRoot: "/", home: "/"}

	var errs []error

	patterns := append(append([]string{}, cfg.GetPaths()...), cfg.GetAllow()...)
	for _, pattern := range patterns {
		if _, err := s.compileUserPattern(pattern, ""); err != nil {
			errs = append(errs, err)
		}
	}

	for _, source := range cfg.GetConfigChangeSources() {
		switch source {
		case config.ConfigSourceUserSettings, config.ConfigSourceProjectSettings,
			config.ConfigSourceLocalSettings, config.ConfigSourcePolicySettings,
			config.ConfigSourceSkills:
		default:
			errs = append(errs, errors.Newf("unknown config_change_sources entry %q", source))
		}
	}

	return errors.Join(errs...)
}

func (s *Set) addAbs(path string, tree bool, reason string) {
	if path == "" || !filepath.IsAbs(path) {
		return
	}

	clean := filepath.Clean(path)
	s.rules = append(s.rules, rule{
		kind: ruleAbs, path: s.key(clean), display: clean, tree: tree, reason: reason,
	})

	if canon := canonical(clean); canon != clean {
		s.rules = append(s.rules, rule{
			kind: ruleAbs, path: s.key(canon), display: canon, tree: tree, reason: reason,
		})
	}
}

func (s *Set) addSuffix(reason string, tree bool, names ...string) {
	comps := make([]string, 0, len(names))
	for _, name := range names {
		comps = append(comps, s.key(name))
	}

	s.rules = append(s.rules, rule{
		kind: ruleSuffix, comps: comps, names: names, tree: tree, reason: reason,
	})
}

// addBuiltins protects klaudiush configuration and state, the binary, and
// the hook registration files of every supported harness. Any .klaudiush
// directory or klaudiush.toml counts, not only the loaded one: the loader
// walks up from the working directory, so a new one in a subdirectory
// would take precedence.
func (s *Set) addBuiltins(opts Options) {
	s.addSuffix(ReasonKlaudiushConfig, true, ".klaudiush")
	s.addSuffix(ReasonKlaudiushConfig, false, "klaudiush.toml")
	s.addAbs(opts.ConfigDir, true, ReasonKlaudiushConfig)
	s.addAbs(opts.LegacyDir, true, ReasonKlaudiushConfig)
	s.addAbs(opts.StateDir, true, ReasonKlaudiushState)
	s.addAbs(opts.DataDir, true, ReasonKlaudiushState)

	for _, name := range klaudiushNames {
		pathList, _ := opts.LookupEnv("PATH")
		for _, dir := range filepath.SplitList(pathList) {
			s.addAbs(filepath.Join(dir, name), false, ReasonKlaudiushBinary)
		}
	}

	for _, exe := range opts.Executables {
		s.addAbs(exe, false, ReasonKlaudiushBinary)

		if filepath.IsAbs(exe) {
			s.executables = append(s.executables, exe)
		}
	}

	for _, file := range opts.HookFiles {
		s.addAbs(s.absolute(file, s.projectRoot), false, ReasonHookConfig)
	}

	s.addClaude(opts)
	s.addCodex(opts)
	s.addSuffix(ReasonGeminiSettings, false, ".gemini", "settings.json")

	for _, managed := range geminiSystemPaths(opts.GOOS, opts.LookupEnv) {
		s.addAbs(managed, false, ReasonGeminiSettings)
	}

	pluginDir := opts.XDGConfigHome
	if pluginDir == "" {
		pluginDir = filepath.Join(opts.Home, ".config")
	}

	s.addAbs(
		filepath.Join(pluginDir, "opencode", "plugin", "klaudiush.ts"),
		false,
		ReasonOpenCodePlugin,
	)
	s.addAbs(opts.OpenCodePlugin, false, ReasonOpenCodePlugin)

	for _, plugin := range opts.PluginPaths {
		s.addAbs(s.absolute(plugin, s.projectRoot), false, ReasonPlugin)
	}
}

func (s *Set) addClaude(opts Options) {
	s.addSuffix(ReasonClaudeSettings, false, ".claude", "settings.json")
	s.addSuffix(ReasonClaudeSettings, false, ".claude", "settings.local.json")
	s.addSuffix(ReasonHookScript, true, ".claude", "hooks")
	s.addSuffix(ReasonMCPConfig, false, ".mcp.json")
	s.addAbs(filepath.Join(opts.Home, ".claude.json"), false, ReasonMCPConfig)

	for _, managed := range claudeManagedPaths(opts.GOOS) {
		s.addAbs(managed, true, ReasonClaudeSettings)
	}
}

func (s *Set) addCodex(opts Options) {
	codexHome := filepath.Join(opts.Home, ".codex")
	if value, ok := opts.LookupEnv("CODEX_HOME"); ok && filepath.IsAbs(value) {
		codexHome = value
	}

	for _, name := range []string{"hooks.json", "config.toml", "requirements.toml", "managed_config.toml"} {
		s.addAbs(filepath.Join(codexHome, name), false, ReasonCodexHooks)
	}

	s.addSuffix(ReasonCodexHooks, false, ".codex", "hooks.json")
	s.addSuffix(ReasonCodexHooks, false, ".codex", "config.toml")

	for _, managed := range codexManagedPaths(opts.GOOS, opts.LookupEnv) {
		s.addAbs(managed, false, ReasonCodexHooks)
	}
}

// claudeManagedPaths lists the Claude Code managed settings directories.
// Source: https://code.claude.com/docs/en/settings#settings-files.
func claudeManagedPaths(goos string) []string {
	switch goos {
	case goosDarwin:
		return []string{"/Library/Application Support/ClaudeCode"}
	case goosLinux:
		return []string{"/etc/claude-code"}
	case goosWindows:
		return []string{`C:\Program Files\ClaudeCode`, `C:\ProgramData\ClaudeCode`}
	default:
		return nil
	}
}

// codexManagedPaths lists the Codex managed configuration files.
// Source: https://developers.openai.com/codex/enterprise/managed-configuration.
func codexManagedPaths(goos string, lookupEnv func(string) (string, bool)) []string {
	if goos == goosWindows {
		programData, ok := lookupEnv("ProgramData")
		if !ok || programData == "" {
			programData = `C:\ProgramData`
		}

		return []string{filepath.Join(programData, "OpenAI", "Codex", "requirements.toml")}
	}

	return []string{"/etc/codex/requirements.toml", "/etc/codex/managed_config.toml"}
}

// geminiSystemPaths lists the Gemini CLI system settings files.
// Source: https://geminicli.com/docs/reference/configuration.
func geminiSystemPaths(goos string, lookupEnv func(string) (string, bool)) []string {
	var paths []string

	for _, name := range []string{"GEMINI_CLI_SYSTEM_SETTINGS_PATH", "GEMINI_CLI_SYSTEM_DEFAULTS_PATH"} {
		if value, ok := lookupEnv(name); ok && filepath.IsAbs(value) {
			paths = append(paths, value)
		}
	}

	var dir string

	switch goos {
	case goosDarwin:
		dir = "/Library/Application Support/GeminiCli"
	case goosLinux:
		dir = "/etc/gemini-cli"
	case goosWindows:
		dir = `C:\ProgramData\gemini-cli`
	default:
		return paths
	}

	return append(
		paths,
		filepath.Join(dir, "settings.json"),
		filepath.Join(dir, "system-defaults.json"),
	)
}

// absolute resolves path against base, expanding ~.
func (s *Set) absolute(path, base string) string {
	switch {
	case path == "~":
		path = s.home
	case strings.HasPrefix(path, "~/"):
		path = filepath.Join(s.home, path[2:])
	case !filepath.IsAbs(path):
		path = filepath.Join(base, path)
	}

	return filepath.Clean(path)
}

// compileUserPattern compiles a protection.paths or protection.allow entry.
// A name without a slash matches that name in any directory; anything else
// is relative to the project root. Entries take in what is below them too.
func (s *Set) compileUserPattern(pattern, reason string) (rule, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return rule{}, errors.Wrap(ErrBadPattern, "empty pattern")
	}

	anywhere := !strings.Contains(trimmed, "/") && !strings.HasPrefix(trimmed, "~")
	trimmed = strings.TrimSuffix(trimmed, "/")

	if !hasGlobMeta(trimmed) {
		if anywhere {
			return rule{
				kind:   ruleSuffix,
				comps:  []string{s.key(trimmed)},
				names:  []string{trimmed},
				tree:   true,
				reason: reason,
			}, nil
		}

		abs := s.absolute(trimmed, s.projectRoot)

		return rule{kind: ruleAbs, path: s.key(abs), display: abs, tree: true, reason: reason}, nil
	}

	prefix := "^"
	body := trimmed

	if anywhere {
		prefix = "^(?:.*/)?"
	} else {
		body = s.absolute(trimmed, s.projectRoot)
	}

	expr, err := globRegexp(s.key(filepath.ToSlash(body)))
	if err != nil {
		return rule{}, errors.Wrapf(ErrBadPattern, "%q: %v", pattern, err)
	}

	re, err := regexp.Compile(prefix + expr + "(?:/.*)?$")
	if err != nil {
		return rule{}, errors.Wrapf(ErrBadPattern, "%q: %v", pattern, err)
	}

	return rule{kind: ruleGlob, glob: re, tree: true, reason: reason}, nil
}

// withCanonical returns r, and for an absolute rule also the rule for the
// path with symlinks resolved, so both spellings match. An allow entry
// resolves only its directories: an allowed name that is a symlink to a
// protected file must not allow that file.
func (s *Set) withCanonical(r rule, followLast bool) []rule {
	if r.kind != ruleAbs {
		return []rule{r}
	}

	canon := canonical(r.display)
	if !followLast {
		canon = filepath.Join(canonical(filepath.Dir(r.display)), filepath.Base(r.display))
	}

	if canon == r.display {
		return []rule{r}
	}

	resolved := r
	resolved.display = canon
	resolved.path = s.key(canon)

	return []rule{r, resolved}
}

// materialize lists concrete protected paths: absolute rules, the
// anywhere-rules placed in the project root, working directory and home,
// and the files inside protected directories.
func (s *Set) materialize() {
	s.resolveSuffixTargets()

	seen := make(map[string]bool)
	add := func(path, reason string) {
		k := s.key(path)
		if seen[k] {
			return
		}

		seen[k] = true

		s.entries = append(s.entries, entry{key: k, path: path, reason: reason})
	}

	var trees []string

	for _, r := range s.rules {
		switch r.kind {
		case ruleAbs:
			add(r.display, r.reason)

			if r.tree {
				trees = append(trees, r.display)
			}
		case ruleSuffix:
			for _, base := range []string{s.projectRoot, s.workDir, s.home} {
				if base == "" || base == "." {
					continue
				}

				path := filepath.Join(append([]string{base}, r.names...)...)
				add(path, r.reason)

				if r.tree {
					trees = append(trees, path)
				}
			}
		case ruleGlob:
		}
	}

	s.enumerate(trees, add)
}

// resolveSuffixTargets protects, by absolute path, the real files behind
// the anywhere-rules placed in the project root, working directory and
// home. When ~/.claude/settings.json is a symlink into a dotfiles
// repository, the file there is the one that takes effect.
func (s *Set) resolveSuffixTargets() {
	var extra []rule

	for _, r := range s.rules {
		if r.kind != ruleSuffix {
			continue
		}

		for _, base := range []string{s.projectRoot, s.workDir, s.home} {
			if base == "" || base == "." {
				continue
			}

			path := filepath.Join(append([]string{base}, r.names...)...)
			if canon := canonical(path); canon != path {
				extra = append(extra, rule{
					kind:    ruleAbs,
					path:    s.key(canon),
					display: canon,
					tree:    r.tree,
					reason:  r.reason,
				})
			}
		}
	}

	s.rules = append(s.rules, extra...)
}

func (s *Set) enumerate(trees []string, add func(path, reason string)) {
	budget := maxEnumeratedFiles

	for _, tree := range trees {
		if budget <= 0 {
			return
		}

		reason := s.reasonFor(tree)
		_ = filepath.WalkDir(tree, func(path string, d os.DirEntry, err error) error {
			if budget <= 0 {
				return filepath.SkipAll
			}

			if err != nil {
				return filepath.SkipDir
			}

			if !d.IsDir() {
				add(path, reason)

				budget--
			}

			return nil
		})
	}
}

func (s *Set) reasonFor(path string) string {
	if r, ok := s.matchRule(s.key(path)); ok {
		return r.reason
	}

	return ReasonConfigured
}

// canonical resolves symlinks in the longest existing prefix of path, so a
// file reached through a symlinked directory, or a symlink itself, maps to
// the file it really is. The rest of the path is kept as written.
func canonical(path string) string {
	path = filepath.Clean(path)
	rest := ""
	dir := path

	for range 64 {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			if rest == "" {
				return resolved
			}

			return filepath.Join(resolved, rest)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return path
		}

		if rest == "" {
			rest = filepath.Base(dir)
		} else {
			rest = filepath.Join(filepath.Base(dir), rest)
		}

		dir = parent
	}

	return path
}
