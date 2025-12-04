// Package session provides session state tracking for Claude Code hooks.
package session

import (
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"
)

// Default constants for unpoison token parsing.
const (
	// DefaultUnpoisonPrefix is the prefix for unpoison tokens.
	DefaultUnpoisonPrefix = "SESS"

	// DefaultUnpoisonEnvVar is the environment variable name for unpoison tokens.
	DefaultUnpoisonEnvVar = "KLACK"

	// tokenParts is the expected number of parts in an unpoison token (prefix:codes).
	unpoisonTokenParts = 2
)

// Unpoison token errors.
var (
	// ErrUnpoisonEmptyCommand is returned when the command is empty.
	ErrUnpoisonEmptyCommand = errors.New("empty command")

	// ErrUnpoisonParseFailed is returned when command parsing fails.
	ErrUnpoisonParseFailed = errors.New("failed to parse command")

	// ErrUnpoisonTokenNotFound is returned when no unpoison token is found.
	ErrUnpoisonTokenNotFound = errors.New("no unpoison token found")

	// ErrUnpoisonInvalidToken is returned when a token has invalid format.
	ErrUnpoisonInvalidToken = errors.New("invalid unpoison token format")

	// ErrUnpoisonInvalidCode is returned when an error code is invalid.
	ErrUnpoisonInvalidCode = errors.New("invalid error code")
)

// UnpoisonToken represents a parsed unpoison token.
type UnpoisonToken struct {
	// Codes contains the acknowledged error codes (e.g., ["GIT001", "GIT002"]).
	Codes []string

	// Raw is the original token string.
	Raw string
}

// UnpoisonTokenSource indicates where the token was found.
type UnpoisonTokenSource int

const (
	// UnpoisonTokenSourceEnvVar indicates the token was found in an environment variable.
	UnpoisonTokenSourceEnvVar UnpoisonTokenSource = iota

	// UnpoisonTokenSourceComment indicates the token was found in a shell comment.
	UnpoisonTokenSourceComment
)

// String returns a string representation of the token source.
func (s UnpoisonTokenSource) String() string {
	switch s {
	case UnpoisonTokenSourceEnvVar:
		return "env_var"
	case UnpoisonTokenSourceComment:
		return "comment"
	default:
		return "unknown"
	}
}

// UnpoisonParseResult contains the result of parsing a command for unpoison tokens.
type UnpoisonParseResult struct {
	// Token is the parsed unpoison token, if found.
	Token *UnpoisonToken

	// Source indicates where the token was found.
	Source UnpoisonTokenSource

	// Found indicates whether a token was found.
	Found bool
}

// UnpoisonParser parses unpoison tokens from commands.
type UnpoisonParser struct {
	shellParser   *syntax.Parser
	tokenPrefix   string
	envVarName    string
	errorCodeExpr *regexp.Regexp
}

// UnpoisonParserOption configures the UnpoisonParser.
type UnpoisonParserOption func(*UnpoisonParser)

// WithUnpoisonPrefix sets the token prefix (default: "SESS").
func WithUnpoisonPrefix(prefix string) UnpoisonParserOption {
	return func(p *UnpoisonParser) {
		if prefix != "" {
			p.tokenPrefix = prefix
		}
	}
}

// WithUnpoisonEnvVar sets the environment variable name (default: "KLACK").
func WithUnpoisonEnvVar(name string) UnpoisonParserOption {
	return func(p *UnpoisonParser) {
		if name != "" {
			p.envVarName = name
		}
	}
}

// NewUnpoisonParser creates a new unpoison token parser.
func NewUnpoisonParser(opts ...UnpoisonParserOption) *UnpoisonParser {
	p := &UnpoisonParser{
		shellParser:   syntax.NewParser(syntax.KeepComments(true)),
		tokenPrefix:   DefaultUnpoisonPrefix,
		envVarName:    DefaultUnpoisonEnvVar,
		errorCodeExpr: regexp.MustCompile(`^[A-Z]{2,10}[0-9]{1,5}$`),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Parse parses a command string and extracts unpoison tokens.
// Token format: SESS:<CODE1>[,<CODE2>,...] (comma-separated error codes)
// Can be found in:
// - Environment variable: KLACK="SESS:GIT001,GIT002" command
// - Shell comment: command # SESS:GIT001,GIT002
func (p *UnpoisonParser) Parse(command string) (*UnpoisonParseResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, ErrUnpoisonEmptyCommand
	}

	file, err := p.shellParser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.Wrap(ErrUnpoisonParseFailed, err.Error())
	}

	if result := p.findTokenInEnvVars(file); result.Found {
		return result, nil
	}

	if result := p.findTokenInComments(file); result.Found {
		return result, nil
	}

	return &UnpoisonParseResult{Found: false}, nil
}

// findTokenInEnvVars searches for unpoison tokens in environment variable assignments.
func (p *UnpoisonParser) findTokenInEnvVars(file *syntax.File) *UnpoisonParseResult {
	result := &UnpoisonParseResult{Found: false}

	syntax.Walk(file, func(node syntax.Node) bool {
		if result.Found {
			return false
		}

		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}

		call, ok := stmt.Cmd.(*syntax.CallExpr)
		if !ok {
			return true
		}

		for _, assign := range call.Assigns {
			if assign.Name == nil {
				continue
			}

			if assign.Name.Value != p.envVarName {
				continue
			}

			value := unpoisonWordToString(assign.Value)
			if value == "" {
				continue
			}

			token, err := p.parseToken(value)
			if err != nil {
				continue
			}

			result.Token = token
			result.Source = UnpoisonTokenSourceEnvVar
			result.Found = true

			return false
		}

		return true
	})

	return result
}

// findTokenInComments searches for unpoison tokens in shell comments.
func (p *UnpoisonParser) findTokenInComments(file *syntax.File) *UnpoisonParseResult {
	result := &UnpoisonParseResult{Found: false}

	syntax.Walk(file, func(node syntax.Node) bool {
		if result.Found {
			return false
		}

		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}

		for _, comment := range stmt.Comments {
			token, err := p.parseTokenFromComment(comment.Text)
			if err != nil {
				continue
			}

			result.Token = token
			result.Source = UnpoisonTokenSourceComment
			result.Found = true

			return false
		}

		return true
	})

	if !result.Found {
		for _, comment := range file.Last {
			token, err := p.parseTokenFromComment(comment.Text)
			if err != nil {
				continue
			}

			result.Token = token
			result.Source = UnpoisonTokenSourceComment
			result.Found = true

			break
		}
	}

	return result
}

// parseTokenFromComment extracts a token from a comment string.
func (p *UnpoisonParser) parseTokenFromComment(text string) (*UnpoisonToken, error) {
	tokenStart := strings.Index(text, p.tokenPrefix+":")
	if tokenStart == -1 {
		return nil, ErrUnpoisonTokenNotFound
	}

	tokenStr := text[tokenStart:]

	tokenEnd := strings.IndexAny(tokenStr, " \t\n")
	if tokenEnd != -1 {
		tokenStr = tokenStr[:tokenEnd]
	}

	return p.parseToken(tokenStr)
}

// parseToken parses a raw token string into an UnpoisonToken struct.
// Expected format: SESS:CODE1[,CODE2,...]
func (p *UnpoisonParser) parseToken(raw string) (*UnpoisonToken, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrUnpoisonInvalidToken
	}

	parts := strings.SplitN(raw, ":", unpoisonTokenParts)
	if len(parts) != unpoisonTokenParts {
		return nil, errors.Wrap(ErrUnpoisonInvalidToken, "expected format PREFIX:CODES")
	}

	prefix := parts[0]
	if prefix != p.tokenPrefix {
		return nil, errors.Wrapf(
			ErrUnpoisonInvalidToken,
			"expected prefix %q, got %q",
			p.tokenPrefix,
			prefix,
		)
	}

	codesStr := parts[1]
	if codesStr == "" {
		return nil, errors.Wrap(ErrUnpoisonInvalidToken, "no error codes provided")
	}

	codes := strings.Split(codesStr, ",")
	validCodes := make([]string, 0, len(codes))

	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}

		if !p.errorCodeExpr.MatchString(code) {
			return nil, errors.Wrapf(ErrUnpoisonInvalidCode, "invalid error code format: %q", code)
		}

		validCodes = append(validCodes, code)
	}

	if len(validCodes) == 0 {
		return nil, errors.Wrap(ErrUnpoisonInvalidToken, "no valid error codes found")
	}

	return &UnpoisonToken{
		Codes: validCodes,
		Raw:   raw,
	}, nil
}

// unpoisonWordToString converts a syntax.Word to a string.
func unpoisonWordToString(word *syntax.Word) string {
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
		case *syntax.DblQuoted:
			for _, dqPart := range p.Parts {
				if lit, ok := dqPart.(*syntax.Lit); ok {
					result.WriteString(lit.Value)
				}
			}
		}
	}

	return result.String()
}

// CheckUnpoisonAcknowledgment checks if the given command contains an unpoison token
// that acknowledges all the specified poison codes.
// Returns true if all codes are acknowledged, false otherwise.
// Also returns the list of unacknowledged codes for error messaging.
func CheckUnpoisonAcknowledgment(
	command string,
	poisonCodes []string,
) (acknowledged bool, unacknowledgedCodes []string, err error) {
	if len(poisonCodes) == 0 {
		return true, nil, nil
	}

	parser := NewUnpoisonParser()

	result, err := parser.Parse(command)
	if err != nil {
		return false, poisonCodes, err
	}

	if !result.Found {
		return false, poisonCodes, nil
	}

	ackedSet := make(map[string]struct{}, len(result.Token.Codes))
	for _, code := range result.Token.Codes {
		ackedSet[code] = struct{}{}
	}

	unacked := make([]string, 0)

	for _, code := range poisonCodes {
		if _, ok := ackedSet[code]; !ok {
			unacked = append(unacked, code)
		}
	}

	if len(unacked) > 0 {
		return false, unacked, nil
	}

	return true, nil, nil
}
