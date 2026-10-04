package parser

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// OpacityCause says why part of a command could not be inspected.
type OpacityCause string

// OpacityDepthLimit means launchers, scripts or aliases nest deeper than the
// parser follows.
const OpacityDepthLimit OpacityCause = "depth-limit"

// OpacityWorkBudget means the command fans out into more commands and scripts
// than one parse follows.
const OpacityWorkBudget OpacityCause = "work-budget"

// OpacityUnreadableScript means a script file the command runs cannot be read
// in full.
const OpacityUnreadableScript OpacityCause = "unreadable-script"

// OpacityScriptSyntax means a script the command runs does not parse.
const OpacityScriptSyntax OpacityCause = "script-syntax"

// OpacityUnresolvedProgram means a git subcommand is neither built in,
// installed, nor an alias the parser can see.
const OpacityUnresolvedProgram OpacityCause = "unresolved-program"

// OpacityUnresolvedArgs means a function forwards its arguments in a form the
// parser does not substitute.
const OpacityUnresolvedArgs OpacityCause = "unresolved-args"

// OpacityUnresolvedWord means the line eval runs, or the command word of git
// or gh, comes from a variable or command output the parser cannot resolve.
const OpacityUnresolvedWord OpacityCause = "unresolved-word"

// Opacity describes one operation the parser could not see through: why
// (Cause), what (Operation), the programs that led to it, outermost first
// (Origin), for some causes a fixed explanation (Detail), and for a known
// tool's printed shell setup that tool, one of EvalSetupTools (Tool), which
// has its own bound (see MaxSetupOpacities). It names programs, scripts and
// subcommands only, never their arguments, so it is safe to show.
type Opacity struct {
	Cause     OpacityCause
	Operation string
	Origin    []string
	Detail    string
	Tool      string
}

// MaxOpacities bounds the opacities one parse keeps that name no setup tool.
// One slot is held for an exhausted work budget, so it is reported however
// many came before. Opacities naming a setup tool are bounded apart, so many
// setup evals never hide another finding: up to MaxOpacities of them, then
// only tools not yet listed, up to MaxSetupOpacities, so every tool's repair
// is shown.
const MaxOpacities = 8

// MaxSetupOpacities bounds the opacities one parse keeps that name a setup
// tool. It leaves room for every EvalSetupTools entry after MaxOpacities-1
// repeats of one tool.
const MaxSetupOpacities = 3 * MaxOpacities

// maxShownNameLen bounds a name shown in an opacity.
const maxShownNameLen = 32

// minTokenLen is the shortest name checked for looking like a secret.
const minTokenLen = 6

// digits are the characters that make a mixed name look like a secret.
const digits = "0123456789"

// hiddenName stands in for a name that is not safe to show.
const hiddenName = "<hidden>"

// Fixed reasons, set as Opacity.Detail, that a script file is opaque.
const (
	DetailScriptVariable  = "its path depends on a variable klaudiush cannot resolve"
	DetailScriptDirectory = "it is a relative path after a cd to a directory klaudiush cannot resolve"
	DetailScriptWritten   = "it is written earlier on the line with content klaudiush cannot see"
	DetailScriptRead      = "the file cannot be read in full (too large, unreadable or not a regular file)"
)

// Fixed reasons, set as Opacity.Detail, that eval or a git or gh command
// word is opaque.
const (
	DetailWordVariable = "it comes from a variable klaudiush cannot resolve"
	DetailWordOutput   = "it comes from command output, arithmetic or a glob"
	DetailWordLoop     = "it comes from a variable, and klaudiush resolves no variable inside a loop"
	DetailWordNewShell = "it comes from a variable, and klaudiush resolves no variable " +
		"inside a new shell or script"
	DetailWordUntrusted = "it comes from a variable, and klaudiush resolves no variable " +
		"after an earlier command changed how variables expand"
)

// shownName matches a name plain enough to show: no expansions, quotes,
// spaces or path separators.
var shownName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+@-]*$`)

// safeName returns name when it is plain and short enough to show, and a
// placeholder otherwise, so a diagnostic never carries command text.
func safeName(name string) string {
	if len(name) > maxShownNameLen || !shownName.MatchString(name) || tokenLike(name) {
		return hiddenName
	}

	return name
}

// tokenLike reports a name that mixes letters and digits like a key or
// password would, unless it is a program or git command klaudiush knows.
func tokenLike(name string) bool {
	if len(name) < minTokenLen || gitBuiltins[name] {
		return false
	}

	if _, ok := launchers[name]; ok {
		return false
	}

	if _, ok := interpreters[name]; ok {
		return false
	}

	return strings.ContainsAny(name, digits) &&
		strings.ContainsFunc(name, func(r rune) bool {
			return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		})
}

// scriptName returns the shown name of a script file.
func scriptName(path string) string {
	switch {
	case path == "-" || path == devStdin:
		return "stdin"
	case strings.HasPrefix(path, procSubstPrefix):
		return "process substitution"
	default:
		return safeName(filepath.Base(path))
	}
}

// opaque records that something could not be inspected, so the parse fails
// closed. Only the first opacities are kept; the parse is marked truncated
// regardless.
func (w *astWalker) opaque(cause OpacityCause, operation, detail string) {
	w.addOpacity(Opacity{Cause: cause, Operation: operation, Detail: detail})
}

// addOpacity records o, reached through the programs being walked.
func (w *astWalker) addOpacity(o Opacity) {
	w.state.truncated = true

	if o.Cause == OpacityWorkBudget {
		if w.state.budgetReported {
			return
		}

		w.state.budgetReported = true
	}

	o.Origin = slices.Clone(w.via)

	if slices.ContainsFunc(w.state.opacities, o.equal) {
		return
	}

	if o.Tool != "" {
		w.addSetupOpacity(o)

		return
	}

	limit := MaxOpacities - 1
	if w.state.budgetReported {
		limit = MaxOpacities
	}

	if len(w.state.opacities)-w.state.setupOpacities >= limit {
		w.state.moreOpacities = true

		return
	}

	w.state.opacities = append(w.state.opacities, o)
}

// addSetupOpacity keeps an opacity naming a setup tool outside the limit on
// other opacities. A tool not yet listed is kept up to MaxSetupOpacities, so
// its repair is shown.
func (w *astWalker) addSetupOpacity(o Opacity) {
	shown := slices.ContainsFunc(w.state.opacities, func(kept Opacity) bool {
		return kept.Tool == o.Tool
	})

	if w.state.setupOpacities >= MaxSetupOpacities ||
		shown && w.state.setupOpacities >= MaxOpacities {
		w.state.moreOpacities = true

		return
	}

	w.state.setupOpacities++
	w.state.opacities = append(w.state.opacities, o)
}

func (o Opacity) equal(other Opacity) bool {
	return o.Cause == other.Cause && o.Operation == other.Operation &&
		o.Detail == other.Detail && o.Tool == other.Tool &&
		slices.Equal(o.Origin, other.Origin)
}

// enter adds cmd to the origin of what it launches until the returned
// function runs.
func (w *astWalker) enter(cmd Command) func() {
	w.via = append(w.via, w.shownWord(cmd.Name))

	return func() { w.via = w.via[:len(w.via)-1] }
}

// operation names the script sw walks.
func (sw scriptWalk) operation() string {
	switch {
	case sw.label != "":
		return sw.label
	case strings.HasPrefix(sw.name, "git:"):
		return "git alias " + safeName(strings.TrimPrefix(sw.name, "git:"))
	case strings.HasPrefix(sw.name, "gh:"):
		return "gh alias " + safeName(strings.TrimPrefix(sw.name, "gh:"))
	case sw.name != "":
		return safeName(sw.name)
	default:
		return "inline script"
	}
}

// MaxLaunchDepth is how many launchers, scripts and aliases the parser follows
// from one command.
const MaxLaunchDepth = maxLaunchDepth

// MaxParseWork is how many commands and scripts one parse follows.
const MaxParseWork = maxParseWork

// MaxScriptBytes is the largest script file the parser reads.
const MaxScriptBytes = maxScriptBytes
