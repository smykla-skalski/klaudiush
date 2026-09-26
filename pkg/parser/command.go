// Package parser provides Bash command parsing capabilities using mvdan.cc/sh
package parser

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// CmdType represents the type of command.
type CmdType int

const (
	// CmdTypeSimple represents a simple command (e.g., "git add file.txt").
	CmdTypeSimple CmdType = iota
	// CmdTypePipe represents a pipeline (e.g., "ls | grep foo").
	CmdTypePipe
	// CmdTypeSubshell represents a subshell (e.g., "(cd dir && git commit)").
	CmdTypeSubshell
	// CmdTypeCmdSubst represents command substitution (e.g., "$(git log)").
	CmdTypeCmdSubst
	// CmdTypeChain represents chained commands (&&, ||, ;).
	CmdTypeChain
)

// String returns string representation of CmdType.
func (t CmdType) String() string {
	switch t {
	case CmdTypeSimple:
		return "Simple"
	case CmdTypePipe:
		return "Pipe"
	case CmdTypeSubshell:
		return "Subshell"
	case CmdTypeCmdSubst:
		return "CmdSubst"
	case CmdTypeChain:
		return "Chain"
	default:
		return "Unknown"
	}
}

// Location represents position in source code.
type Location struct {
	Line   uint
	Column uint
}

// Command represents a parsed command with metadata.
type Command struct {
	Name             string   // Command name (e.g., "git")
	Args             []string // Command arguments
	Location         Location // Position in source
	Type             CmdType  // Command type
	Raw              string   // Raw command string
	WorkingDirectory string   // Effective working directory from preceding cd commands
	Stdin            string   // Content fed to stdin via heredoc or a piped echo/printf
	StdinFile        string   // File redirected to stdin (<)
	Invoked          string   // Program word as written, before resolving it to Name
}

// String returns a string representation of the command.
func (c *Command) String() string {
	if len(c.Args) == 0 {
		return c.Name
	}

	return fmt.Sprintf("%s %s", c.Name, strings.Join(c.Args, " "))
}

// FullCommand returns the complete command as a string slice.
func (c *Command) FullCommand() []string {
	result := make([]string, 0, 1+len(c.Args))
	result = append(result, c.Name)
	result = append(result, c.Args...)

	return result
}

// wordToString converts syntax.Word to string, handling quotes and expansions.
func wordToString(word *syntax.Word) string {
	if word == nil {
		return ""
	}

	var result strings.Builder

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			result.WriteString(p.Value)
		case *syntax.SglQuoted:
			result.WriteString(p.Value)
		case *syntax.ParamExp:
			// Render a variable reference as a stable braced token ("${MSG}")
			// instead of dropping it. Keeping the token preserves argument
			// positions - so "git commit -F \"$MSG\" -- file" does not misalign -F
			// onto "--" - and lets a redirect target and a -F path that name the
			// same variable match by token equality. The braced form also keeps a
			// real expansion distinct from a single-quoted literal like '$MSG'.
			result.WriteString(paramExpToString(p))
		case *syntax.DblQuoted:
			for _, dqPart := range p.Parts {
				switch dqp := dqPart.(type) {
				case *syntax.Lit:
					result.WriteString(dqp.Value)
				case *syntax.ParamExp:
					result.WriteString(paramExpToString(dqp))
				case *syntax.CmdSubst:
					// Handle command substitution (e.g., "$(cat <<'EOF' ... EOF)")
					if heredoc := extractHeredocFromCmdSubst(dqp); heredoc != "" {
						result.WriteString(heredoc)
					}
				}
			}
		case *syntax.CmdSubst:
			// Handle unquoted command substitution
			if heredoc := extractHeredocFromCmdSubst(p); heredoc != "" {
				result.WriteString(heredoc)
			}
		}
	}

	return result.String()
}

// paramExpToString renders a parameter expansion as a stable token. A simple
// reference - "$MSG" or "${MSG}" - canonicalizes to the braced form "${MSG}" so
// a write target and a consumer that name the same variable in different forms
// match, and so a real expansion stays distinct from a single-quoted literal like
// '$MSG' (which keeps its source form and is still validated). An expansion
// carrying an operator or modifier - "${#MSG}", "${MSG:-default}", "${!MSG}" - is
// printed verbatim so distinct expansions are not collapsed onto "${MSG}".
func paramExpToString(pe *syntax.ParamExp) string {
	if pe == nil || pe.Param == nil {
		return ""
	}

	canonical := "${" + pe.Param.Value + "}"

	// A short reference ("$NAME", "$@") has no braces and so no operator; use the
	// canonical braced form directly.
	if pe.Short {
		return canonical
	}

	// A braced reference may carry an operator/modifier. Print it and keep that
	// form only when it differs from the plain "${NAME}".
	var b strings.Builder
	if err := syntax.NewPrinter().Print(&b, pe); err != nil {
		return canonical
	}

	if printed := b.String(); printed != canonical {
		return printed
	}

	return canonical
}

// extractHeredocFromCmdSubst extracts heredoc content from command substitution.
// It looks for patterns like "$(cat <<'EOF' ... EOF)" or "$(cat <<EOF ... EOF)".
func extractHeredocFromCmdSubst(cmdSubst *syntax.CmdSubst) string {
	if cmdSubst == nil || len(cmdSubst.Stmts) == 0 {
		return ""
	}

	// Walk through statements looking for heredoc redirections
	for _, stmt := range cmdSubst.Stmts {
		if stmt.Redirs == nil {
			continue
		}

		for _, redir := range stmt.Redirs {
			// Check if this is a heredoc redirection
			if redir.Op == syntax.Hdoc || redir.Op == syntax.DashHdoc {
				// Extract heredoc content from Hdoc field
				if redir.Hdoc != nil {
					return wordToString(redir.Hdoc)
				}
			}
		}
	}

	return ""
}

// wordsToStrings converts a slice of syntax.Word to string slice.
func wordsToStrings(words []*syntax.Word) []string {
	result := make([]string, 0, len(words))

	for _, word := range words {
		if s := wordToString(word); s != "" {
			result = append(result, s)
		}
	}

	return result
}

// hasDoubleQuotedBackticks checks if a word contains backticks within double quotes.
// Backticks in double quotes are parsed as CmdSubst nodes by the shell parser.
func hasDoubleQuotedBackticks(word *syntax.Word) bool {
	if word == nil {
		return false
	}

	for _, part := range word.Parts {
		if dq, ok := part.(*syntax.DblQuoted); ok {
			// Check if any part within the double quotes is a command substitution
			for _, dqPart := range dq.Parts {
				if _, isCmdSubst := dqPart.(*syntax.CmdSubst); isCmdSubst {
					return true
				}
			}
		}
	}

	return false
}

// QuotingContext represents the quoting context of a backtick or variable.
type QuotingContext int

const (
	// QuotingContextUnquoted means the content is not quoted.
	QuotingContextUnquoted QuotingContext = iota
	// QuotingContextSingleQuoted means the content is in single quotes.
	QuotingContextSingleQuoted
	// QuotingContextDoubleQuoted means the content is in double quotes.
	QuotingContextDoubleQuoted
)

// BacktickLocation represents the location and context of a backtick.
type BacktickLocation struct {
	ArgIndex      int            // Index of the argument
	Context       QuotingContext // Quoting context
	HasVariables  bool           // Whether the string contains variables
	IsEscaped     bool           // Whether backticks are escaped
	RawValue      string         // Raw value of the argument
	SuggestSingle bool           // Whether single quotes should be suggested
}

// hasUnquotedBackticks checks if a word contains unquoted backticks.
func hasUnquotedBackticks(word *syntax.Word) bool {
	if word == nil {
		return false
	}

	for _, part := range word.Parts {
		// Check for command substitution that's not inside quotes
		if _, isCmdSubst := part.(*syntax.CmdSubst); isCmdSubst {
			return true
		}
	}

	return false
}

// analyzeBacktickContext analyzes the quoting context and variables in a word.
// It finds the first CmdSubst (backtick or $()) and returns its quoting context.
func analyzeBacktickContext(word *syntax.Word) *BacktickLocation {
	if word == nil {
		return nil
	}

	location := &BacktickLocation{
		RawValue: wordToString(word),
	}

	for _, part := range word.Parts {
		if result := analyzeWordPart(part, location); result != nil {
			return result
		}
	}

	return location
}

// analyzeWordPart analyzes a single part of a word for backtick context.
// Returns a BacktickLocation if a CmdSubst was found, nil otherwise.
func analyzeWordPart(part syntax.WordPart, location *BacktickLocation) *BacktickLocation {
	switch p := part.(type) {
	case *syntax.SglQuoted:
		// Single quotes prevent command substitution
		location.Context = QuotingContextSingleQuoted

		return location

	case *syntax.DblQuoted:
		return analyzeDoubleQuoted(p, location)

	case *syntax.CmdSubst:
		// Unquoted command substitution
		location.Context = QuotingContextUnquoted
		location.IsEscaped = false

		return location

	case *syntax.ParamExp, *syntax.ArithmExp:
		// Track variables at the top level (unquoted context)
		location.HasVariables = true
	}

	return nil
}

// analyzeDoubleQuoted analyzes a double-quoted section for backticks and variables.
func analyzeDoubleQuoted(dq *syntax.DblQuoted, location *BacktickLocation) *BacktickLocation {
	hasCmdSubst := false
	hasVars := false

	for _, dqPart := range dq.Parts {
		switch dqPart.(type) {
		case *syntax.CmdSubst:
			hasCmdSubst = true

		case *syntax.ParamExp, *syntax.ArithmExp:
			hasVars = true
		}
	}

	if !hasCmdSubst {
		return nil
	}

	// Found backticks in double quotes
	location.Context = QuotingContextDoubleQuoted
	location.HasVariables = hasVars
	location.IsEscaped = false
	location.SuggestSingle = !hasVars

	return location
}
