package protection

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// klaudiushNames are the names the klaudiush binary runs under: its own,
// and the hook dispatcher name the installer gives it.
var klaudiushNames = []string{"klaudiush", "dispatcher"}

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
	"":           {"*"},
	"version":    {"*"},
	"help":       {"*"},
	"completion": {"*"},
	"suggest":    {"*"},
	"evidence":   {"*"},
	"doctor":     {"*"},
	"debug":      {"", "config", "rules", "exceptions", "overrides", "patterns", "crash"},
	"audit":      {"", subList, subStats},
	"backup":     {"", subList, "status", "audit"},
	"bypass":     {"", "status"},
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

	words := klaudiushWords(cmd.Args)
	sub, second := "", ""

	if len(words) > 0 {
		sub = words[0]
	}

	if len(words) > 1 {
		second = words[1]
	}

	shown := strings.TrimSpace(sub + " " + second)

	allowed, known := readOnlyKlaudiush[sub]
	if !known {
		return shown, true
	}

	if sub == "doctor" && slices.Contains(cmd.Args, "--fix") {
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
