package protection

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// klaudiushNames are the names the klaudiush binary runs under: its own,
// and the hook dispatcher name the installer gives it.
var klaudiushNames = []string{programKlaudiush, "dispatcher"}

// klaudiushValueFlags are root flags that take the next word.
var klaudiushValueFlags = map[string]bool{
	"-c": true, "--config": true, "--global-config": true, "--provider": true,
	"--event": true, "-T": true, "--hook-type": true, "--failure-mode": true,
	"--disable": true,
}

// Read-only second words shared by several klaudiush subcommands.
const (
	subList  = "list"
	subStats = "stats"
)

// readOnlyKlaudiush lists the klaudiush subcommands that leave policy as
// it is, by subcommand and allowed second word ("" for none, "*" for any).
// Everything else (init, update, bypass skip, disable, enable, backup
// restore, doctor --fix, ...) changes configuration, state or the binary.
var readOnlyKlaudiush = map[string][]string{
	"version":    {"*"},
	"help":       {"*"},
	"completion": {"*"},
	"suggest":    {"*"},
	"evidence":   {"*"},
	"doctor":     {"*"},
	"debug":      {"", "config", "rules", "exceptions", "overrides", "patterns", "crash"},
	"audit":      {"", subList, subStats},
	"backup":     {"", subList, wordStatus, "audit"},
	"bypass":     {"", wordStatus},
	"patterns":   {"", subList, subStats},
	"overrides":  {""},
}

// isKlaudiush reports whether cmd runs klaudiush.
func isKlaudiush(cmd parser.Command) bool {
	return slices.Contains(klaudiushNames, programName(cmd)) ||
		slices.Contains(klaudiushNames, strings.ToLower(filepath.Base(cmd.Invoked)))
}

// PolicyCommand reports a klaudiush invocation that changes policy: its
// configuration, overrides, bypass setting, state, backups or the binary.
// It returns the subcommand as written.
func PolicyCommand(cmd parser.Command) (string, bool) {
	if !isKlaudiush(cmd) {
		return "", false
	}

	return policySubcommand(cmd)
}

// policySubcommand is PolicyCommand for a command known to run klaudiush.
// Hook mode counts: a payload the agent pipes in can reset session state
// or record check results that never happened.
func policySubcommand(cmd parser.Command) (string, bool) {
	words := klaudiushWords(cmd.Args)
	sub, second := "", ""

	if len(words) > 0 {
		sub = words[0]
	}

	if len(words) > 1 {
		second = words[1]
	}

	shown := strings.TrimSpace(sub + " " + second)

	if slices.ContainsFunc(cmd.Args, isInfoFlag) {
		return "", false
	}

	if sub == "" {
		return "hook mode", true
	}

	allowed, known := readOnlyKlaudiush[sub]
	if !known {
		return shown, true
	}

	if sub == "doctor" && slices.ContainsFunc(cmd.Args, enablesFix) {
		return "doctor --fix", true
	}

	if sub == "debug" && second == "crash" && slices.Contains(words, "clean") {
		return "debug crash clean", true
	}

	if slices.Contains(allowed, "*") || slices.Contains(allowed, second) {
		return "", false
	}

	return shown, true
}

// klaudiushWords returns the positional words of a klaudiush command line,
// skipping root flags and their values.
func klaudiushWords(args []string) []string {
	var words []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case klaudiushValueFlags[arg]:
			i++
		case strings.HasPrefix(arg, "-"):
		default:
			words = append(words, arg)
		}
	}

	return words
}

func isInfoFlag(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "--version" || arg == "-v"
}

// enablesFix reports a --fix flag that is on: --fix, --fix=true, --fix=1.
func enablesFix(arg string) bool {
	value, found := strings.CutPrefix(arg, "--fix")
	if !found {
		return false
	}

	if value == "" {
		return true
	}

	enabled, err := strconv.ParseBool(strings.TrimPrefix(value, "="))

	return err != nil || enabled
}

// outputPaths returns the files a klaudiush command writes by flag.
func outputPaths(args []string) []string {
	var paths []string

	for i := 0; i < len(args); i++ {
		switch value, found := strings.CutPrefix(args[i], "--output="); {
		case found:
			paths = append(paths, value)
		case args[i] == "--output" && i+1 < len(args):
			paths = append(paths, args[i+1])
			i++
		}
	}

	return paths
}

// runsKlaudiushBinary reports whether cmd runs a copy or link of a
// klaudiush binary under another name: the same file, or a file with the
// same size and content.
func (s *Set) runsKlaudiushBinary(cmd parser.Command, dir string) bool {
	if len(s.executables) == 0 {
		return false
	}

	program := s.programPath(cmd, dir)
	if program == "" {
		return false
	}

	return s.isKlaudiushFile(program)
}

// isKlaudiushFile reports whether path is a klaudiush binary: the same file
// or a byte-identical copy.
func (s *Set) isKlaudiushFile(program string) bool {
	info, err := os.Stat(program)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	for _, exe := range s.executables {
		other, statErr := os.Stat(exe)
		if statErr != nil {
			continue
		}

		if os.SameFile(info, other) || sameContent(program, exe, info.Size(), other.Size()) {
			return true
		}
	}

	return false
}

// programPath returns the file a command runs: the word as written when it
// has a slash, else the first match on the hook's PATH.
func (s *Set) programPath(cmd parser.Command, dir string) string {
	word := cmd.Invoked
	if word == "" {
		word = cmd.Name
	}

	if strings.Contains(word, "/") {
		if strings.HasPrefix(dir, unknownPart) && !filepath.IsAbs(word) {
			return ""
		}

		return s.absolute(word, dir)
	}

	pathList, _ := s.lookupEnv("PATH")
	for _, entry := range filepath.SplitList(pathList) {
		if !filepath.IsAbs(entry) {
			continue
		}

		candidate := filepath.Join(entry, word)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}

	return ""
}

// maxCompareSize bounds the binaries compared byte for byte.
const maxCompareSize = 256 << 20

func sameContent(a, b string, sizeA, sizeB int64) bool {
	if sizeA != sizeB || sizeA > maxCompareSize {
		return false
	}

	dataA, err := os.ReadFile(filepath.Clean(a))
	if err != nil {
		return false
	}

	dataB, err := os.ReadFile(filepath.Clean(b))
	if err != nil {
		return false
	}

	return bytes.Equal(dataA, dataB)
}
