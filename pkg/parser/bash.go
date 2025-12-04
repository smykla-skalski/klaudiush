package parser

import (
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
	Commands      []Command   // All commands found
	FileWrites    []FileWrite // All file write operations
	GitOperations []Command   // Git commands only
}

// BashParser parses Bash commands using mvdan.cc/sh.
type BashParser struct {
	parser *syntax.Parser
}

// NewBashParser creates a new BashParser instance.
func NewBashParser() *BashParser {
	return &BashParser{
		parser: syntax.NewParser(),
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
	walker := &astWalker{
		commands:   make([]Command, 0),
		fileWrites: make([]FileWrite, 0),
	}

	syntax.Walk(file, walker.visit)

	// Extract git operations
	gitOps := make([]Command, 0)

	for _, cmd := range walker.commands {
		if cmd.Name == "git" {
			gitOps = append(gitOps, cmd)
		}
	}

	return &ParseResult{
		Commands:      walker.commands,
		FileWrites:    walker.fileWrites,
		GitOperations: gitOps,
	}, nil
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
