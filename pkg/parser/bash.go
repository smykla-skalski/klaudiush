package parser

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"
)

var (
	// ErrEmptyCommand is returned when trying to parse an empty command.
	ErrEmptyCommand = errors.New("empty command")
	// ErrParseFailed is returned when parsing fails.
	ErrParseFailed = errors.New("failed to parse command")
)

// ParseResult contains the results of parsing a Bash command.
type ParseResult struct {
	Commands      []Command         // All commands found
	FileWrites    []FileWrite       // All file write operations
	GitOperations []Command         // Git commands only
	Assignments   map[string]string // Literal NAME=value assignments
	// Truncated reports that the command launches something nested deeper than
	// the parser follows, so what it finally runs is unknown.
	Truncated bool
}

// BashParser parses Bash commands using mvdan.cc/sh.
type BashParser struct {
	parser   *syntax.Parser
	resolver Resolver
}

// NewBashParser creates a BashParser that resolves programs, scripts,
// environment variables and git aliases against the running system.
func NewBashParser() *BashParser {
	return NewBashParserWithResolver(OSResolver{})
}

// NewBashParserWithResolver creates a BashParser that asks resolver what the
// command text alone cannot tell.
func NewBashParserWithResolver(resolver Resolver) *BashParser {
	if resolver == nil {
		resolver = OSResolver{}
	}

	return &BashParser{
		parser:   syntax.NewParser(),
		resolver: resolver,
	}
}

// Parse parses a Bash command string and extracts all commands and operations.
func (p *BashParser) Parse(command string) (*ParseResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, ErrEmptyCommand
	}

	// Parse the command into an AST
	file, err := p.parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.Wrap(ErrParseFailed, err.Error())
	}

	// Walk the AST to extract commands and file operations
	walker := newAstWalker(p.resolver)

	syntax.Walk(file, walker.visit)

	// Extract git operations
	gitOps := make([]Command, 0)

	for _, cmd := range walker.commands {
		if cmd.Name == gitProgram {
			gitOps = append(gitOps, cmd)
		}
	}

	return &ParseResult{
		Commands:      walker.commands,
		FileWrites:    walker.fileWrites,
		GitOperations: gitOps,
		Assignments:   walker.assignments,
		Truncated:     walker.truncated,
	}, nil
}

// maxExpandPasses bounds variable expansion so a self-referential assignment
// cannot loop.
const maxExpandPasses = 5

// varRefPattern matches a canonical ${NAME} reference, the form wordToString
// produces for both $NAME and ${NAME}, and ${NAME[@]} or ${NAME[*]}, which
// expand to every element of an array.
var varRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:\[[@*]\])?\}`)

// ExpandVars substitutes assignments captured from the same command line into
// s. References with no known assignment are left as they are, so callers can
// tell a resolved value from one they still cannot see.
func (r *ParseResult) ExpandVars(s string) string {
	return expandVars(s, func(name string) (string, bool) {
		value, ok := r.Assignments[name]

		return value, ok
	})
}

// expandVars substitutes the values lookup knows into s, leaving unknown
// references as they are.
func expandVars(s string, lookup func(name string) (string, bool)) string {
	for range maxExpandPasses {
		if !strings.Contains(s, "${") {
			break
		}

		expanded := varRefPattern.ReplaceAllStringFunc(s, func(ref string) string {
			// ref is "${NAME}", or "${NAME[@]}" for every element of an array.
			if value, ok := lookup(varRefPattern.FindStringSubmatch(ref)[1]); ok {
				return value
			}

			return ref
		})

		if expanded == s {
			break
		}

		s = expanded
	}

	return s
}

// HasUnresolvedVars reports whether s still carries a variable reference that
// could not be expanded.
func HasUnresolvedVars(s string) bool {
	return strings.Contains(s, "${")
}

// HasCommand checks if the parse result contains a command with the given name.
func (r *ParseResult) HasCommand(name string) bool {
	for _, cmd := range r.Commands {
		if cmd.Name == name {
			return true
		}
	}

	return false
}

// HasGitCommand checks if the parse result contains any git commands.
func (r *ParseResult) HasGitCommand() bool {
	return len(r.GitOperations) > 0
}

// GetCommands returns all commands with the given name.
func (r *ParseResult) GetCommands(name string) []Command {
	result := make([]Command, 0)

	for _, cmd := range r.Commands {
		if cmd.Name == name {
			result = append(result, cmd)
		}
	}

	return result
}

// GetFirstGitWorkingDir returns the effective working directory for the first
// git command, as set by a preceding cd command in the same command chain.
// Returns "" if no cd command preceded the git operation.
//
// Example: "cd /path/to/repo && git commit -m 'msg'" returns "/path/to/repo".
func (r *ParseResult) GetFirstGitWorkingDir() string {
	for _, op := range r.GitOperations {
		if op.WorkingDirectory != "" {
			return op.WorkingDirectory
		}
	}

	return ""
}

// InlineFileContent returns the content written to path before the consumer at
// source position "before", and whether that content could be reconstructed.
// workDir is the consumer's effective working directory (from cd or git -C),
// used to resolve a relative path so writes in a different directory don't match.
//
// Only writes preceding that position (in source order, which models shell
// execution order for sequential commands) are considered, so a write that
// happens after the consumer - e.g. "git commit -F f && cat > f <<EOF" - is
// ignored. The parser reconstructs two overwrite forms: a heredoc fed to a
// verbatim copier ("cat > f <<EOF ... EOF"), captured exactly, and a literal
// echo/printf redirect ("printf '%s' msg > f"), captured best-effort
// (normalized). The last such overwrite to the resolved path wins. Appends
// (">>"), heredocs on transforming commands, and tee/cp/mv leave the result
// uncertain, so ok is false and callers should fall back to reading from disk.
//
// This lets validators inspect "git commit -F f" messages when f is created
// inline (e.g. "cat > f <<EOF ... EOF; git commit -F f"), since f does not
// exist on disk yet when the PreToolUse hook runs.
func (r *ParseResult) InlineFileContent(path, workDir string, before Location) (string, bool) {
	return lastCapturedWrite(r.FileWrites, resolvePath(workDir, path), &before)
}

// lastCapturedWrite returns what the writes leave in target, when the last of
// them captured it exactly. A nil before considers every write.
func lastCapturedWrite(writes []FileWrite, target string, before *Location) (string, bool) {
	var (
		content string
		ok      bool
	)

	for _, fw := range writes {
		if before != nil && !locationBefore(fw.Location, *before) {
			continue
		}

		if resolvePath(fw.WorkingDirectory, fw.Path) != target {
			continue
		}

		switch fw.Operation {
		case WriteOpRedirect, WriteOpHeredoc:
			// Overwrite: the last write wins, discarding earlier content. Only
			// captured content (an exact reconstruction) counts - a heredoc body
			// fed to cat, or literal echo/printf output - otherwise ok stays
			// false so callers fall back to reading the file from disk.
			content, ok = fw.CapturedOverwrite()
		default:
			// Append, tee, cp, mv: the resulting bytes can't be reconstructed
			// from the command alone (prior content or trailing newlines are
			// unknown), so the capture is no longer exact.
			content, ok = "", false
		}
	}

	return content, ok
}

// resolvePath cleans path, joining it onto workDir when path is relative and a
// working directory is known, so writes and consumers in different directories
// compare unequal. A leading ~ is left unjoined: the shell expands it to a home
// directory independent of the current directory, so a ~ path must compare the
// same regardless of the consumer's working directory.
func resolvePath(workDir, path string) string {
	if workDir == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}

	return filepath.Clean(filepath.Join(workDir, path))
}

// locationBefore reports whether a occurs strictly before b in source order.
func locationBefore(a, b Location) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}

	return a.Column < b.Column
}

// BacktickIssue represents a problematic use of backticks in double quotes.
type BacktickIssue struct {
	ArgIndex int    // Index of the argument containing backticks
	ArgValue string // Value of the argument
}

// FindDoubleQuotedBackticks detects backticks in double-quoted command arguments.
// It returns a list of arguments that contain backticks within double quotes.
func (p *BashParser) FindDoubleQuotedBackticks(command string) ([]BacktickIssue, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, ErrEmptyCommand
	}

	// Parse the command into an AST
	file, err := p.parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.Wrap(ErrParseFailed, err.Error())
	}

	var issues []BacktickIssue

	// Walk the AST looking for CallExpr nodes
	syntax.Walk(file, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			// Check each argument (index 0 is command name)
			for i, arg := range call.Args {
				if hasDoubleQuotedBackticks(arg) {
					issues = append(issues, BacktickIssue{
						ArgIndex: i,
						ArgValue: wordToString(arg),
					})
				}
			}
		}

		return true
	})

	return issues, nil
}

// FindAllBacktickIssues performs comprehensive analysis of backticks in all contexts.
// It detects unquoted backticks, backticks in double quotes, and analyzes whether
// single quotes should be suggested (when no variables are present).
func (p *BashParser) FindAllBacktickIssues(command string) ([]BacktickLocation, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, ErrEmptyCommand
	}

	// Parse the command into an AST
	file, err := p.parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.Wrap(ErrParseFailed, err.Error())
	}

	var locations []BacktickLocation

	// Walk the AST looking for CallExpr nodes
	syntax.Walk(file, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok {
			// Check each argument (index 0 is command name)
			for i, arg := range call.Args {
				// Check for any backticks (quoted or unquoted)
				if hasDoubleQuotedBackticks(arg) || hasUnquotedBackticks(arg) {
					if analysis := analyzeBacktickContext(arg); analysis != nil {
						analysis.ArgIndex = i
						locations = append(locations, *analysis)
					}
				}
			}
		}

		return true
	})

	return locations, nil
}
