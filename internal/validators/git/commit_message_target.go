package git

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// knownDir is a directory a path is relative to, or unknown when a cd or
// git -C names one klaudiush cannot resolve.
type knownDir struct {
	path  string
	known bool
}

// messageTarget is the message file: its resolved path when klaudiush can
// resolve it, else the path as written, which only an identical write matches.
type messageTarget struct {
	abs   string
	token string
	dir   knownDir
}

// shellDir is the directory the shell runs the commit in, before git -C.
func (src messageSource) shellDir() knownDir {
	return knownDir{
		path:  src.cmd.WorkingDirectory,
		known: !src.cmd.DirUnknown && !src.cmd.DirComputed,
	}
}

// gitDir is the directory git reads -F relative to: the shell's, moved by -C.
func (src messageSource) gitDir(gitCmd *parser.GitCommand) knownDir {
	dir := src.shellDir()

	cDir, hasC := gitCmd.GlobalOptions["-C"]
	if !hasC {
		return dir
	}

	expanded := src.expandVars(src.cmd.Vars, cDir)

	switch {
	case cDir == "", parser.HasUnresolvedVars(expanded), usesDynamicVar(src.cmd.Vars, cDir),
		src.substitutedBeforeSubcommand(gitCmd):
		return knownDir{}
	case filepath.IsAbs(expandTilde(expanded)):
		return knownDir{path: expandTilde(expanded), known: true}
	default:
		return knownDir{path: filepath.Join(dir.path, expanded), known: dir.known}
	}
}

// substitutedBeforeSubcommand reports a global option value built from a
// substitution, which renders partially.
func (src messageSource) substitutedBeforeSubcommand(gitCmd *parser.GitCommand) bool {
	idx := slices.Index(src.cmd.Args, gitCmd.Subcommand)

	return idx > 0 &&
		slices.Contains(src.cmd.SubstitutedArgs[:min(idx, len(src.cmd.SubstitutedArgs))], true)
}

// expandVars substitutes the line's variables into s, then HOME from the
// environment when the line leaves it untouched.
func (src messageSource) expandVars(vars *parser.VarScope, s string) string {
	s = vars.ExpandVars(s)

	ref := "${" + homeVar + "}"
	if !strings.Contains(s, ref) || homeAssignment.MatchString(src.text) {
		return s
	}

	if home, ok := os.LookupEnv(homeVar); ok && home != "" {
		s = strings.ReplaceAll(s, ref, home)
	}

	return s
}

// target resolves the message file, or fails when its path depends on a
// variable holding command output.
func (src messageSource) target(filePath string, dir knownDir) (messageTarget, error) {
	if usesDynamicVar(src.cmd.Vars, filePath) {
		return messageTarget{}, opaqueSource(reasonVarPath)
	}

	t := messageTarget{token: filePath, dir: dir}

	path := expandTilde(src.expandVars(src.cmd.Vars, filePath))

	switch {
	case parser.HasUnresolvedVars(path):
	case filepath.IsAbs(path):
		t.abs = filepath.Clean(path)
	case dir.known:
		t.abs = src.join(dir.path, path)
	}

	return t, nil
}

// unresolvedReason says why a target without a resolved path cannot be read.
func (t messageTarget) unresolvedReason() string {
	if !t.dir.known && !parser.HasUnresolvedVars(t.token) {
		return reasonDirectory
	}

	return reasonVarPath
}

// matchesWrite reports a write to the message file.
func (src messageSource) matchesWrite(t messageTarget, fw parser.FileWrite) bool {
	if t.abs != "" {
		target, ok := src.absolute(fw.Vars, fw.Path, fw.WorkingDirectory, fw.DirUnknown)

		return ok && sameFile(target, t.abs)
	}

	return filepath.Clean(fw.Path) == filepath.Clean(t.token) &&
		(filepath.IsAbs(t.token) || parser.HasUnresolvedVars(t.token) ||
			fw.WorkingDirectory == t.dir.path)
}

// readMessagePath returns the message file's content: what a captured write
// earlier on the line leaves in it, else the file on disk. Anything after
// that point that may change the file makes it opaque.
func (v *CommitValidator) readMessagePath(
	src messageSource,
	filePath string,
	dir knownDir,
	before parser.Location,
) (string, error) {
	t, err := src.target(filePath, dir)
	if err != nil {
		return "", err
	}

	content, from, captured, err := src.capturedWrite(t, before)
	if err != nil {
		return "", err
	}

	if changed := src.changedBetween(t, from, before); changed != nil {
		return "", changed
	}

	if captured {
		v.Logger().Debug("Reading commit message from inline file write", "path", filePath)

		return strings.TrimSpace(content), nil
	}

	if t.abs == "" {
		return "", opaqueSource(t.unresolvedReason())
	}

	v.Logger().Debug("Reading commit message from file", "path", t.abs)

	disk, err := readRegularFile(t.abs)
	if err != nil {
		v.Logger().Debug("Commit message file is unreadable", "error", err)

		if errors.Is(err, fs.ErrNotExist) {
			return "", opaqueSourceWith(reasonMissing, repairMissing)
		}

		return "", opaqueSource(reasonNotRegular)
	}

	return strings.TrimSpace(disk), nil
}

// capturedWrite finds the last write to the message file before the commit.
// It returns its content and position when the parser captured it exactly,
// and fails when it did not.
func (src messageSource) capturedWrite(
	t messageTarget,
	before parser.Location,
) (content string, from parser.Location, captured bool, err error) {
	if src.parsed == nil {
		return "", parser.Location{}, false, nil
	}

	var last *parser.FileWrite

	for _, fw := range src.parsed.WritesBefore(before) {
		if src.matchesWrite(t, fw) {
			last = &fw
		}
	}

	if last == nil {
		return "", parser.Location{}, false, nil
	}

	content, ok := last.CapturedOverwrite()
	if !ok || last.Dynamic {
		return "", parser.Location{}, false, opaqueSourceWith(reasonRewritten, repairSeparate)
	}

	return content, last.Location, true, nil
}

// changedBetween fails when something after "from" (the start of the line
// when zero) and before the commit may change the message file: a write
// whose target is unknown, or a command that names the file or a directory
// above it and is not known to only read.
func (src messageSource) changedBetween(t messageTarget, from, before parser.Location) error {
	if src.parsed == nil {
		return nil
	}

	after := func(loc parser.Location) bool {
		return from == (parser.Location{}) || from.Before(loc)
	}

	if src.parsed.DynamicWritesBetween(from, before) {
		return opaqueSourceWith(reasonUnknownWrite, repairSeparate)
	}

	for _, fw := range src.parsed.WritesBefore(before) {
		if !after(fw.Location) || src.matchesWrite(t, fw) {
			continue
		}

		if _, ok := src.absolute(
			fw.Vars,
			fw.Path,
			fw.WorkingDirectory,
			fw.DirUnknown,
		); !ok ||
			fw.Dynamic {
			return opaqueSourceWith(reasonUnknownWrite, repairSeparate)
		}
	}

	for _, cmd := range src.parsed.CommandsBefore(before) {
		if after(cmd.Location) && !readOnlyCommand(cmd) && src.namesFile(cmd, t) {
			return opaqueSourceWith(reasonChanged, repairSeparate)
		}
	}

	return nil
}

// namesFile reports a command that may place, replace or remove the message
// file: an argument naming it or a directory above it (also after = as in
// dd of=), or a file-placing program with a destination klaudiush cannot
// resolve.
func (src messageSource) namesFile(cmd parser.Command, t messageTarget) bool {
	if slices.Contains(filePlacers, cmd.Name) &&
		(cmd.Dynamic || slices.Contains(cmd.SubstitutedArgs, true) ||
			slices.ContainsFunc(cmd.Args, func(arg string) bool { return src.unresolvedArg(cmd, arg) })) {
		return true
	}

	for _, arg := range cmd.Args {
		if _, value, found := strings.Cut(arg, "="); found {
			arg = value
		}

		if t.abs == "" {
			if strings.Contains(arg, t.token) {
				return true
			}

			continue
		}

		path, ok := src.absolute(cmd.Vars, arg, cmd.WorkingDirectory, cmd.DirUnknown)
		if ok && (sameFile(path, t.abs) || containsPath(path, t.abs)) {
			return true
		}
	}

	return false
}

// unresolvedArg reports an argument the shell expands to paths klaudiush
// does not resolve: an unknown or command-output variable, ~+ and ~- (the
// current and previous directory), a glob or a brace, also after =.
func (src messageSource) unresolvedArg(cmd parser.Command, arg string) bool {
	if _, value, found := strings.Cut(arg, "="); found && src.unresolvedArg(cmd, value) {
		return true
	}

	expanded := src.expandVars(cmd.Vars, arg)
	if parser.HasUnresolvedVars(expanded) || usesDynamicVar(cmd.Vars, arg) {
		return true
	}

	return strings.HasPrefix(expanded, "~+") || strings.HasPrefix(expanded, "~-") ||
		strings.ContainsAny(varRef.ReplaceAllString(arg, ""), "*?[{")
}

// readOnlyCommand reports a command known to leave the files it names as
// they are.
func readOnlyCommand(cmd parser.Command) bool {
	if cmd.Name == gitCommand {
		gitCmd, err := parser.ParseGitCommand(cmd)

		return err == nil && slices.Contains(readOnlyGitSubcommands, gitCmd.Subcommand)
	}

	return slices.Contains(readOnlyPrograms, cmd.Name)
}

// absolute resolves path as the shell would see it from dir, or reports
// false when a variable or the directory cannot be resolved.
func (src messageSource) absolute(
	vars *parser.VarScope,
	path, dir string,
	dirUnknown bool,
) (string, bool) {
	path = expandTilde(src.expandVars(vars, path))
	if parser.HasUnresolvedVars(path) {
		return "", false
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}

	if dirUnknown {
		return "", false
	}

	return src.join(dir, path), true
}
