package evidence

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"
)

// ErrNotLiteral marks a command that is not a plain list of literal words.
var ErrNotLiteral = errors.New("command must be plain literal words")

// unquotedSpecial lists characters an unquoted word may not contain: each
// would make the shell run something other than the word as written.
const unquotedSpecial = "*?[]{}~\\"

const cdCommand = "cd"

// literalArgv parses a configured command line into its words. Only a single
// simple command of literal words is accepted, so what runs is exactly what
// the configuration says.
func literalArgv(line string) ([]string, error) {
	leaves, err := andChain(line)
	if err != nil {
		return nil, err
	}

	if len(leaves) != 1 {
		return nil, errors.Wrap(ErrNotLiteral, "expected one command")
	}

	return leaves[0], nil
}

// MatchCommand returns the check a shell command runs, or nil. The command
// counts only when its exit status is the check's own: it must be one of the
// check's commands word for word, optionally preceded by "cd <dir> &&" steps
// that end in the repository root, with nothing else chained, piped,
// backgrounded, negated or substituted.
func MatchCommand(checks []*Check, command, workDir, repoRoot string) *Check {
	leaves, err := andChain(command)
	if err != nil || len(leaves) == 0 {
		return nil
	}

	dir := workDir

	for _, argv := range leaves[:len(leaves)-1] {
		if len(argv) != 2 || argv[0] != cdCommand {
			return nil
		}

		dir = joinDir(dir, argv[1])
	}

	if !sameDir(dir, repoRoot) {
		return nil
	}

	last := leaves[len(leaves)-1]

	for _, check := range checks {
		for _, argv := range check.Commands {
			if slices.Equal(argv, last) {
				return check
			}
		}
	}

	return nil
}

func joinDir(dir, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}

	return filepath.Join(dir, target)
}

func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}

	return canonicalDir(a) == canonicalDir(b)
}

func canonicalDir(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}

	return filepath.Clean(dir)
}

// andChain parses a line made of one simple command or of simple commands
// joined by "&&", and returns the words of each. A pure && chain succeeds
// only when every command in it succeeded.
func andChain(line string) ([][]string, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).
		Parse(strings.NewReader(line), "")
	if err != nil {
		return nil, errors.Wrap(ErrNotLiteral, err.Error())
	}

	if len(file.Stmts) != 1 {
		return nil, errors.Wrap(ErrNotLiteral, "expected exactly one statement")
	}

	var leaves [][]string

	if err := collectAnd(file.Stmts[0], &leaves); err != nil {
		return nil, err
	}

	return leaves, nil
}

func collectAnd(stmt *syntax.Stmt, leaves *[][]string) error {
	if stmt.Negated || stmt.Background || stmt.Coprocess || stmt.Disown {
		return errors.Wrap(ErrNotLiteral, "negated or background command")
	}

	if err := literalRedirects(stmt.Redirs); err != nil {
		return err
	}

	switch cmd := stmt.Cmd.(type) {
	case *syntax.BinaryCmd:
		if cmd.Op != syntax.AndStmt || len(stmt.Redirs) > 0 {
			return errors.Wrapf(ErrNotLiteral, "operator %s", cmd.Op)
		}

		if err := collectAnd(cmd.X, leaves); err != nil {
			return err
		}

		return collectAnd(cmd.Y, leaves)
	case *syntax.CallExpr:
		if len(cmd.Assigns) > 0 {
			return errors.Wrap(ErrNotLiteral, "variable assignment")
		}

		argv := make([]string, 0, len(cmd.Args))

		for _, word := range cmd.Args {
			value, ok := literalWord(word)
			if !ok {
				return errors.Wrap(ErrNotLiteral, "word with expansion or special characters")
			}

			argv = append(argv, value)
		}

		if len(argv) == 0 {
			return errors.Wrap(ErrNotLiteral, "empty command")
		}

		*leaves = append(*leaves, argv)

		return nil
	default:
		return errors.Wrap(ErrNotLiteral, "compound command")
	}
}

// literalRedirects accepts output redirections to literal targets, which do
// not change the exit status. Input redirections and here-documents could
// feed the command anything, so they are refused.
func literalRedirects(redirs []*syntax.Redirect) error {
	for _, redir := range redirs {
		switch redir.Op {
		case syntax.RdrOut, syntax.AppOut, syntax.RdrAll, syntax.AppAll,
			syntax.DplOut, syntax.RdrClob:
		default:
			return errors.Wrapf(ErrNotLiteral, "redirection %s", redir.Op)
		}

		if _, ok := literalWord(redir.Word); !ok {
			return errors.Wrap(ErrNotLiteral, "redirection target with expansion")
		}
	}

	return nil
}

// literalWord returns the value of a word made only of literal text, single
// quotes and double quotes without expansions.
func literalWord(word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}

	var value strings.Builder

	for _, part := range word.Parts {
		switch typed := part.(type) {
		case *syntax.Lit:
			if strings.ContainsAny(typed.Value, unquotedSpecial) {
				return "", false
			}

			value.WriteString(typed.Value)
		case *syntax.SglQuoted:
			if typed.Dollar {
				return "", false
			}

			value.WriteString(typed.Value)
		case *syntax.DblQuoted:
			text, ok := literalDoubleQuoted(typed)
			if !ok {
				return "", false
			}

			value.WriteString(text)
		default:
			return "", false
		}
	}

	return value.String(), true
}

// literalDoubleQuoted returns the text of a double-quoted string without
// expansions or escapes.
func literalDoubleQuoted(quoted *syntax.DblQuoted) (string, bool) {
	if quoted.Dollar {
		return "", false
	}

	var value strings.Builder

	for _, inner := range quoted.Parts {
		lit, ok := inner.(*syntax.Lit)
		if !ok || strings.Contains(lit.Value, "\\") {
			return "", false
		}

		value.WriteString(lit.Value)
	}

	return value.String(), true
}
