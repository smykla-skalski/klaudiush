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
	// repeatPasses is how many passes in the same state a script must
	// already have before a repeating one is not followed.
	repeatPasses = 2
)

// scriptSourceText is a script file's text and how it is walked.
type scriptSourceText struct {
	path    string
	text    string
	literal bool // interpreter code rather than shell
}

// sourceEntry marks a script file being followed: the state it was entered
// in and how many commands the line had recorded by then.
type sourceEntry struct {
	key   string
	start int
}

// repeatsItself reports whether following a script in the state key would
// only repeat what is already being followed. A script that names itself (a
// usage line in a Python docstring, a shell script that reruns itself) would
// otherwise be followed into itself until the depth limit failed the
// command. The script must already be followed twice in the same state, and
// the pass between those two entries must have recorded exactly the commands
// recorded since the second one. A walk depends only on its state and the
// commands recorded before it, and repeating the same commands leaves every
// lookup of the latest one unchanged, so a third pass would record the same
// commands again.
func (w *astWalker) repeatsItself(key string) bool {
	var starts []int

	for _, entry := range slices.Backward(w.following) {
		if entry.key == key {
			starts = append(starts, entry.start)
		}

		if len(starts) == repeatPasses {
			break
		}
	}

	if len(starts) < repeatPasses {
		return false
	}

	events := w.state.events
	latest, earlier := starts[0], starts[1]

	return slices.Equal(events[earlier:latest], events[latest:])
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

// walkSource follows a script's text, as interpreter code when literal.
func (w *astWalker) walkSource(cmd Command, src scriptSourceText, depth int, entry sourceEntry) {
	sw := scriptWalk{literal: src.literal, label: scriptName(src.path), source: entry}
	text := src.text

	if src.literal {
		w.followCode(cmd, text, depth, sw)
	} else {
		w.walkScript(text, cmd, depth, sw)
	}
}

// followingKey reports whether a script is being followed in state key on
// the way here.
func (w *astWalker) followingKey(key string) bool {
	return slices.ContainsFunc(w.following, func(entry sourceEntry) bool {
		return entry.key == key
	})
}

// confirmRepeat checks a script whose repeating pass was not followed. The
// skipped pass would also have run whatever follows its call to itself, so
// the script is walked once more in the state the line has reached. When that
// pass records a command not seen before, writes new content or changes the
// command table or variable trust, the passes were not a fixed point and
// what further ones would run is unknown, so the command fails closed.
func (w *astWalker) confirmRepeat(cmd Command, src scriptSourceText, depth int, key string) {
	delete(w.state.repeated, key)

	before := w.effects()
	again := w.sourceKey(cmd, src.text, src.literal)

	w.walkSource(cmd, src, depth, sourceEntry{key: again, start: len(w.state.events)})

	if w.effects() != before {
		w.opaque(OpacityDepthLimit, scriptName(src.path), "")
	}
}

// effects summarizes what one walk can leave for the next: the distinct
// commands recorded, the latest content of each written file, the command
// table, variable trust and dynamic variables.
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

	return hex.EncodeToString(h.Sum(nil))
}

// noteRecorded appends cmd to the commands recorded on the line.
func (w *astWalker) noteRecorded(cmd Command) {
	w.state.events = append(w.state.events, strings.Join(
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
	))
	w.state.distinct[w.state.events[len(w.state.events)-1]] = true
}
