package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"maps"
	"slices"
	"strconv"
	"strings"
)

const (
	// fieldSeparator joins the parts of a recorded command's identity.
	fieldSeparator = "\x00"
	// maxKeyWrites and maxKeyWriteBytes cap the file writes a script's state
	// covers, since hashing more on every followed script could outlast the
	// hook timeout. Past them no repeat is cut, and a script that names
	// itself fails closed at the depth limit as any deep nesting does.
	maxKeyWrites     = 256
	maxKeyWriteBytes = 64 << 10
	maxKeyScopeBytes = 64 << 10
	ghAliasCommand   = "alias"
)

// scriptSourceText is a script file's text and how it is walked.
type scriptSourceText struct {
	path    string
	text    string
	literal bool // interpreter code rather than shell
	run     scriptRun
	prelude []startupScript
}

// sourceKey identifies the state a script's text is followed in from cmd:
// the directory, the variables, aliases and functions in scope, how far
// variables are trusted, the command table, the definitions being expanded
// and the latest content written on the line to each file. A walker whose
// directory is unknown or computed gets a key that matches nothing: relative
// reads and writes there cannot be told apart, so its repeats are never cut.
// So does one with more definitions in scope than maxKeyScopeBytes, which
// would cost too much to hash on every followed script.
func (w *astWalker) sourceKey(cmd Command, src scriptSourceText) string {
	if w.dirUnknown || w.dirComputed || cmd.DirUnknown || w.scopeBytes() > maxKeyScopeBytes {
		return w.uniqueKey()
	}

	h := sha256.New()

	writeParts(h,
		src.text,
		cmd.Name,
		cmd.WorkingDirectory,
		strconv.FormatBool(src.literal),
		strconv.FormatBool(w.distrust),
		strconv.FormatBool(w.inLoop || w.outerLoop),
		strconv.FormatBool(w.state.pathChanged),
		strconv.FormatBool(w.state.untrusted),
	)
	writeParts(
		h,
		src.run.zero,
		src.run.file,
		strconv.FormatBool(src.run.withArgs),
		strconv.Itoa(len(src.run.args)),
	)
	writeParts(h, src.run.args...)
	writeParts(h, strconv.Itoa(len(w.dirStack)))
	writeParts(h, w.dirStack...)

	if !w.writeWrites(h) {
		return w.uniqueKey()
	}

	for _, m := range []map[string]string{w.assignments, w.aliases, w.funcs} {
		writeParts(h, strconv.Itoa(len(m)))

		for _, k := range slices.Sorted(maps.Keys(m)) {
			writeParts(h, k, m[k])
		}
	}

	for _, m := range []map[string]bool{w.unknownVars, w.state.dynamicVars, w.expanding} {
		writeParts(h, strconv.Itoa(len(m)))

		for _, k := range slices.Sorted(maps.Keys(m)) {
			writeParts(h, k, strconv.FormatBool(m[k]))
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// lineWrites returns, for each file written so far on the line, every
// version a later script could read from it, oldest first, each once. A
// version is the captured content or a marker for content that cannot be
// reconstructed. ok is false past maxKeyWrites writes or maxKeyWriteBytes,
// where summarizing them on every followed script would cost more than the
// hook's timeout allows, and after a write under an unknown directory, whose
// target cannot be told.
func (w *astWalker) lineWrites() (versions map[string][]string, ok bool) {
	var chain []*astWalker

	total := 0

	for p := w; p != nil; p = p.parent {
		chain = append(chain, p)
		total += len(p.fileWrites)
	}

	if total > maxKeyWrites || writtenBytes(chain) > maxKeyWriteBytes {
		return nil, false
	}

	versions = make(map[string][]string)

	for _, p := range slices.Backward(chain) {
		for _, fw := range p.fileWrites {
			if fw.DirUnknown || fw.TargetUnknown {
				return nil, false
			}

			target, known := w.writtenPath(fw)
			if !known {
				return nil, false
			}

			version := writeVersion(fw)

			if !slices.Contains(versions[target], version) {
				versions[target] = append(versions[target], version)
			}
		}
	}

	return versions, true
}

// writeVersion is what a script reads from a file after fw, as lastWrite
// sees it.
func writeVersion(fw FileWrite) string {
	switch fw.Operation {
	case WriteOpRedirect, WriteOpHeredoc:
		if content, captured := fw.CapturedOverwrite(); captured {
			return "captured" + fieldSeparator + content
		}
	default:
	}

	return "unknown"
}

// writeWrites hashes the write versions, or reports false when they are
// past the cap.
func (w *astWalker) writeWrites(h hash.Hash) bool {
	versions, ok := w.lineWrites()
	if !ok {
		return false
	}

	writeParts(h, strconv.Itoa(len(versions)))

	for _, target := range slices.Sorted(maps.Keys(versions)) {
		writeParts(h, target, strconv.Itoa(len(versions[target])))
		writeParts(h, versions[target]...)
	}

	return true
}

// uniqueKey returns a key no other call returns, so nothing matches it.
func (w *astWalker) uniqueKey() string {
	w.state.uniqueKeys++

	return "unique" + fieldSeparator + strconv.Itoa(w.state.uniqueKeys)
}

// writeParts hashes each part with its length, so parts cannot run together.
func writeParts(h hash.Hash, parts ...string) {
	for _, p := range parts {
		h.Write([]byte(strconv.Itoa(len(p)) + ":" + p))
	}
}

// walkSource follows a script's text in the state key, as interpreter code
// when literal.
func (w *astWalker) walkSource(cmd Command, src scriptSourceText, depth int, key string) {
	sw := scriptWalk{
		literal: src.literal,
		label:   scriptName(src.path),
		source:  key,
		prelude: src.prelude,
		run:     src.run,
		file:    !src.literal,
	}
	text := src.text

	if src.literal {
		w.followCode(cmd, text, depth, sw)
	} else {
		w.walkScript(text, cmd, depth, sw)
	}
}

// followingKey reports whether a script is already being followed in state
// key on the way here. A script that names itself (a usage line in a Python
// docstring, a shell script that reruns itself) would otherwise be followed
// into itself until the depth limit failed the command, so that repeat is
// not followed and confirmRepeat checks it once the outer pass is done.
func (w *astWalker) followingKey(key string) bool {
	return slices.Contains(w.following, key)
}

// confirmRepeat checks a script whose repeat was not followed. The skipped
// pass would have run the script again with what the outer pass had
// recorded by then, including whatever follows its call to itself, so the
// script is walked once more in the state the line has reached. When that
// pass records a command not seen before, writes new content, redefines a
// git or gh alias, or changes the command table or variable trust, the
// passes were not a fixed point and what further ones would run is unknown,
// so the command fails closed.
func (w *astWalker) confirmRepeat(cmd Command, src scriptSourceText, depth int, key string) {
	delete(w.state.repeated, key)

	before := w.effects()
	again := w.sourceKey(cmd, src)

	w.walkSource(cmd, src, depth, again)
	delete(w.state.repeated, again)

	if w.effects() != before || w.ambiguousWrites() || w.ambiguousAliases() {
		w.opaque(OpacityDepthLimit, scriptName(src.path), "")
	}
}

// ambiguousWrites reports whether some file was written more than one way on
// the line. A script read from it sees only the last version, but a pass that
// was cut may have run while an earlier one was in place, for instance when
// the later write is conditional.
func (w *astWalker) ambiguousWrites() bool {
	versions, ok := w.lineWrites()
	if !ok {
		return true
	}

	for _, v := range versions {
		if len(v) > 1 {
			return true
		}
	}

	return false
}

// effects summarizes what one walk can leave for the next: the distinct
// commands recorded, the latest content of each written file, the order of
// the latest git and gh alias definitions, the command table, variable trust
// and dynamic variables.
func (w *astWalker) effects() string {
	h := sha256.New()

	writeParts(h,
		strconv.Itoa(len(w.state.distinct)),
		strconv.FormatBool(w.state.pathChanged),
		strconv.FormatBool(w.state.untrusted),
	)

	if !w.writeWrites(h) {
		return w.uniqueKey()
	}

	writeParts(h, slices.Sorted(maps.Keys(w.state.dynamicVars))...)
	writeParts(h, w.aliasDefinitions()...)

	return hex.EncodeToString(h.Sum(nil))
}

// aliasDefinitions lists the git config and gh alias set commands recorded
// so far, latest first, each once. Alias lookups take the latest definition,
// so repeating an older one changes what a later alias runs.
func (w *astWalker) aliasDefinitions() []string {
	var defs []string

	seen := make(map[string]bool)

	for cmd := range w.earlierCommands() {
		if !definesAlias(cmd) {
			continue
		}

		id := commandIdentity(cmd)
		if !seen[id] {
			seen[id] = true
			defs = append(defs, id)
		}
	}

	return defs
}

// definesAlias reports whether cmd may define a git or gh alias.
func definesAlias(cmd Command) bool {
	switch cmd.Name {
	case gitProgram:
		return slices.Contains(cmd.Args, "config")
	case ghCLI:
		return slices.Contains(cmd.Args, ghAliasCommand)
	default:
		return false
	}
}

// noteRecorded adds cmd to the distinct commands recorded on the line.
func (w *astWalker) noteRecorded(cmd Command) {
	w.state.distinct[commandIdentity(cmd)] = true
}

// commandIdentity is what tells two recorded commands apart.
func commandIdentity(cmd Command) string {
	return strings.Join(
		slices.Concat(
			[]string{
				cmd.Name,
				cmd.Invoked,
				cmd.WorkingDirectory,
				strconv.FormatBool(cmd.DirUnknown),
				cmd.Stdin,
				cmd.StdinFile,
			},
			cmd.Args,
		),
		fieldSeparator,
	)
}

// writtenBytes counts the content the walkers' file writes hold, which a
// script's state would have to hash on every followed script.
func writtenBytes(chain []*astWalker) int {
	size := 0

	for _, p := range chain {
		for _, fw := range p.fileWrites {
			size += len(
				fw.Path,
			) + len(
				fw.WorkingDirectory,
			) + len(
				fw.Content,
			) + len(
				fw.RedirectContent,
			)
		}
	}

	return size
}

// ambiguousAliases reports whether the line holds more than one distinct
// command that may define a git or gh alias. A lookup sees only the latest
// definition, but a pass that was cut may have run while an earlier one was
// in place. Telling which alias each command touches would mean matching
// every flag form git and gh accept, so any two different ones count.
func (w *astWalker) ambiguousAliases() bool {
	first := ""

	for cmd := range w.earlierCommands() {
		if !definesAlias(cmd) {
			continue
		}

		id := commandIdentity(cmd)
		if first == "" {
			first = id
		} else if id != first {
			return true
		}
	}

	return false
}

// scopeBytes counts the saved directories, variables, aliases and functions
// in scope, which a script's state would have to hash on every followed
// script.
func (w *astWalker) scopeBytes() int {
	size := 0

	for _, dir := range w.dirStack {
		size += len(dir)
	}

	for _, m := range []map[string]string{w.assignments, w.aliases, w.funcs} {
		for k, v := range m {
			size += len(k) + len(v)
		}
	}

	return size
}
