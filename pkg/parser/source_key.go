package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strconv"
)

// sourceKeyPrefix keeps script keys apart from alias and function names in
// the expanding set.
const sourceKeyPrefix = "file:"

// sourceKey identifies following a script's text from cmd in this walker's
// state: the directory, the variables, aliases and functions in scope and
// how far variables are trusted. A script that names itself (a usage line
// in a Python docstring, a shell script that reruns itself) would otherwise
// be followed into itself until the depth limit failed the command. Walking
// the same text again in the same state records the same commands, so the
// repeat adds nothing; a change of state (a cd, a new assignment) gets a new
// key and is still followed.
func (w *astWalker) sourceKey(cmd Command, text string, literal bool) string {
	h := sha256.New()

	write := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(strconv.Itoa(len(p))))
			h.Write([]byte{':'})
			h.Write([]byte(p))
		}
	}

	write(
		text,
		cmd.Name,
		cmd.WorkingDirectory,
		strconv.FormatBool(w.dirUnknown),
		strconv.FormatBool(literal),
		strconv.FormatBool(w.distrust),
		strconv.FormatBool(w.inLoop || w.outerLoop),
	)

	for _, m := range []map[string]string{w.assignments, w.aliases, w.funcs} {
		write(strconv.Itoa(len(m)))

		for _, k := range slices.Sorted(maps.Keys(m)) {
			write(k, m[k])
		}
	}

	write(strconv.Itoa(len(w.unknownVars)))

	for _, k := range slices.Sorted(maps.Keys(w.unknownVars)) {
		write(k, strconv.FormatBool(w.unknownVars[k]))
	}

	return sourceKeyPrefix + hex.EncodeToString(h.Sum(nil))
}
