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

// fieldSeparator joins the parts of a recorded command's identity.
const fieldSeparator = "\x00"

// scriptSourceText is a script file's text and how it is walked.
type scriptSourceText struct {
	path    string
	text    string
	literal bool // interpreter code rather than shell
}

// sourceKey identifies the state a script's text is followed in from cmd:
// the directory, the variables, aliases and functions in scope, how far
// variables are trusted, the command table, the definitions being expanded
// and the latest content written on the line to each file.
func (w *astWalker) sourceKey(cmd Command, text string, literal bool) string {
	h := sha256.New()

	writeParts(h,
		text,
		cmd.Name,
		cmd.WorkingDirectory,
		strconv.FormatBool(w.dirUnknown),
		strconv.FormatBool(literal),
		strconv.FormatBool(w.distrust),
		strconv.FormatBool(w.inLoop || w.outerLoop),
		strconv.FormatBool(w.state.pathChanged),
		strconv.FormatBool(w.state.untrusted),
	)
	writeParts(h, strconv.Itoa(len(w.dirStack)))
	writeParts(h, w.dirStack...)

	for _, m := range []map[string]string{w.assignments, w.aliases, w.funcs, w.latestWrites()} {
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

// latestWrites returns what a script would read from each file written so
// far on the line.
func (w *astWalker) latestWrites() map[string]string {
	writes := make(map[string]string)

	for p := w; p != nil; p = p.parent {
		for _, fw := range p.fileWrites {
			target := resolvePath(fw.WorkingDirectory, fw.Path)
			if _, seen := writes[target]; seen {
				continue
			}

			content, found, captured := w.lastLineWrite(target)
			writes[target] = strings.Join(
				[]string{content, strconv.FormatBool(found), strconv.FormatBool(captured)},
				fieldSeparator,
			)
		}
	}

	return writes
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
	sw := scriptWalk{literal: src.literal, label: scriptName(src.path), source: key}
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
	again := w.sourceKey(cmd, src.text, src.literal)

	w.walkSource(cmd, src, depth, again)
	delete(w.state.repeated, again)

	if w.effects() != before {
		w.opaque(OpacityDepthLimit, scriptName(src.path), "")
	}
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

	writes := w.latestWrites()
	writeParts(h, strconv.Itoa(len(writes)))

	for _, k := range slices.Sorted(maps.Keys(writes)) {
		writeParts(h, k, writes[k])
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
		return slices.Contains(cmd.Args, "alias")
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
