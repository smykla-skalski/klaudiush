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

// sourceKeyPrefix keeps script keys apart from alias and function names in
// the expanding set.
const sourceKeyPrefix = "file:"

// fieldSeparator joins the parts of a recorded command's identity.
const fieldSeparator = "\x00"

// sourceKey identifies following a script's text from cmd in the current
// state of the parse. A script that names itself (a usage line in a Python
// docstring, a shell script that reruns itself) would otherwise be followed
// into itself until the depth limit failed the command. The key covers
// everything a walk depends on: the text, the directory, the variables,
// aliases and functions in scope, how far variables are trusted, the shared
// command table and dynamic variables, and how many distinct commands the
// line has recorded. When the key repeats on the way down, the pass in
// between recorded nothing new and changed no state, so walking again would
// record the same commands; any change (a cd, an assignment, hash -p, a new
// git alias or gh alias command) gives a new key and is followed.
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
		strconv.Itoa(w.state.novel),
	)

	for _, m := range []map[string]string{w.assignments, w.aliases, w.funcs} {
		writeParts(h, strconv.Itoa(len(m)))

		for _, k := range slices.Sorted(maps.Keys(m)) {
			writeParts(h, k, m[k])
		}
	}

	for _, m := range []map[string]bool{w.unknownVars, w.state.dynamicVars} {
		writeParts(h, strconv.Itoa(len(m)))

		for _, k := range slices.Sorted(maps.Keys(m)) {
			writeParts(h, k, strconv.FormatBool(m[k]))
		}
	}

	return sourceKeyPrefix + hex.EncodeToString(h.Sum(nil))
}

// writeParts hashes each part with its length, so parts cannot run together.
func writeParts(h hash.Hash, parts ...string) {
	for _, p := range parts {
		h.Write([]byte(strconv.Itoa(len(p)) + ":" + p))
	}
}

// noteRecorded counts cmd when the line has not recorded the same command in
// the same directory before.
func (w *astWalker) noteRecorded(cmd Command) {
	id := strings.Join(
		slices.Concat(
			[]string{
				cmd.Name,
				cmd.Invoked,
				cmd.WorkingDirectory,
				strconv.FormatBool(cmd.DirUnknown),
			},
			cmd.Args,
		),
		fieldSeparator,
	)

	if w.state.recorded == nil {
		w.state.recorded = make(map[string]bool)
	}

	if !w.state.recorded[id] {
		w.state.recorded[id] = true
		w.state.novel++
	}
}
