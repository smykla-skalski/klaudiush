package parser

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// unresolvedWord stands for an argument made only of command substitutions,
// which renders empty. Keeping a stand-in holds its position, so a git or gh
// command word taken from command output is seen instead of skipped. It is
// removed before the command is recorded.
const unresolvedWord = unresolvedProgram

// globChars expand against files or into several words when unquoted.
const globChars = "*?[{"

// ghActionCommands take an action as their second word (gh pr create).
var ghActionCommands = nameSet("issue pr")

// substitutionOnly reports whether word renders empty because all of it comes
// from command substitutions.
func substitutionOnly(word *syntax.Word, rendered string) bool {
	return rendered == "" && word != nil && hasCmdSubst(word.Parts)
}

func hasCmdSubst(parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.CmdSubst:
			return true
		case *syntax.DblQuoted:
			if hasCmdSubst(p.Parts) {
				return true
			}
		}
	}

	return false
}

// withoutUnresolvedWords drops the stand-ins for substituted arguments,
// leaving the arguments as the rest of the parser knows them.
func withoutUnresolvedWords(args []string) []string {
	if !slices.Contains(args, unresolvedWord) {
		return args
	}

	return slices.DeleteFunc(slices.Clone(args), func(arg string) bool {
		return arg == unresolvedWord
	})
}

// resolveWord expands the variables in a rendered word from assignments made
// earlier on the line, then the environment. It reports false when a
// reference stays unknown or holds command output.
func (w *astWalker) resolveWord(word string) (string, bool) {
	known := true

	expanded := expandVars(word, func(name string) (string, bool) {
		if w.state.dynamicVars[name] {
			known = false

			return "", false
		}

		if value, ok := w.assignments[name]; ok {
			return value, true
		}

		return w.resolver.LookupEnv(name)
	})

	return expanded, known && !HasUnresolvedVars(expanded)
}

// commandWordDetail says why a git or gh command word cannot be known, or
// returns "" when it is literal.
func commandWordDetail(word string) string {
	switch {
	case HasUnresolvedVars(word):
		return DetailWordVariable
	case strings.Contains(word, unresolvedWord) || strings.ContainsAny(word, globChars):
		return DetailWordOutput
	default:
		return ""
	}
}

// resolveGitSubcommand substitutes variables in git's subcommand word, so
// x=status; git $x is checked as git status. A subcommand from an unknown
// variable, command output or a glob is opaque: it could be any subcommand.
// Each pass replaces one dynamic word with literal ones, so it ends.
func (w *astWalker) resolveGitSubcommand(cmd Command) (Command, bool) {
	for {
		idx := gitSubcommandIndex(cmd.Args)
		if idx < 0 {
			return cmd, true
		}

		resolved, ok := w.resolveCommandWord(cmd.Args[idx], gitProgram)
		if !ok || resolved == nil {
			return cmd, ok
		}

		cmd.Args = slices.Concat(cmd.Args[:idx], resolved, cmd.Args[idx+1:])
	}
}

// resolveGHCommand does for gh what resolveGitSubcommand does for git, for
// the command word and, for pr and issue, the action after it.
func (w *astWalker) resolveGHCommand(cmd Command) (Command, bool) {
	for pos := 0; pos < len(cmd.Args) && pos < 2; {
		if pos == 1 && !ghActionCommands[cmd.Args[0]] {
			break
		}

		resolved, ok := w.resolveCommandWord(cmd.Args[pos], ghCLI)
		if !ok {
			return cmd, false
		}

		if resolved == nil {
			pos++

			continue
		}

		cmd.Args = slices.Concat(cmd.Args[:pos], resolved, cmd.Args[pos+1:])
	}

	return cmd, true
}

// resolveCommandWord returns the words a dynamic command word expands to, nil
// for a literal word, or false after recording why it cannot be known. The
// expansion is split into words even when quoted, which only leaves more of
// it checked.
func (w *astWalker) resolveCommandWord(word, program string) ([]string, bool) {
	detail := commandWordDetail(word)
	if detail == "" {
		return nil, true
	}

	if detail == DetailWordVariable {
		if expanded, ok := w.resolveWord(word); ok {
			detail = commandWordDetail(expanded)
			if detail == "" {
				return nonNil(strings.Fields(expanded)), true
			}
		}
	}

	w.opaque(OpacityUnresolvedWord, program, detail)

	return nil, false
}

// nonNil keeps an expansion to no words distinct from a literal word.
func nonNil(words []string) []string {
	if words == nil {
		return []string{}
	}

	return words
}

// resolveEval returns the command line eval runs, with the variables it
// names substituted. A line built from an unknown variable or from command
// output is opaque.
func (w *astWalker) resolveEval(cmd Command) (string, bool) {
	line := strings.Join(cmd.Args, " ")

	if cmd.Dynamic || strings.Contains(line, unresolvedWord) {
		w.opaque(OpacityUnresolvedWord, cmd.Name, DetailWordOutput)

		return line, false
	}

	if !HasUnresolvedVars(line) {
		return line, true
	}

	expanded, ok := w.resolveWord(line)
	if !ok {
		w.opaque(OpacityUnresolvedWord, cmd.Name, DetailWordVariable)

		return line, false
	}

	return expanded, true
}

// launchedFrom returns what cmd runs. The commands it launches keep the
// stand-ins of followed, so env git $(...) is still seen; scripts and files
// are found as before. Eval runs its line with known variables substituted.
func (w *astWalker) launchedFrom(cmd, followed Command) launch {
	l := launched(cmd)

	if len(followed.Args) != len(cmd.Args) {
		l.commands = launched(followed).commands
	}

	if cmd.Name == evalBuiltin {
		if line, ok := w.resolveEval(followed); ok {
			l.scripts = []string{line}
		}
	}

	return l
}
