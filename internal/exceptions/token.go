// Package exceptions provides the exception workflow system for klaudiush.
package exceptions

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"mvdan.cc/sh/v3/syntax"
)

// Default constants for token parsing.
const (
	// DefaultTokenPrefix is the default prefix for exception tokens.
	DefaultTokenPrefix = "EXC"

	// DefaultEnvVarName is the default environment variable name for tokens.
	DefaultEnvVarName = "KLACK"

	// tokenParts is the expected number of parts in a token (prefix:code:reason).
	tokenParts = 3

	// minTokenParts is the minimum parts (prefix:code) without reason.
	minTokenParts = 2
)

// Error definitions.
var (
	// ErrEmptyCommand is returned when the command is empty.
	ErrEmptyCommand = errors.New("empty command")

	// ErrParseFailed is returned when command parsing fails.
	ErrParseFailed = errors.New("failed to parse command")

	// ErrInvalidToken is returned when a token has invalid format.
	ErrInvalidToken = errors.New("invalid exception token format")

	// ErrTokenNotFound is returned when no exception token is found.
	ErrTokenNotFound = errors.New("no exception token found")

	// ErrInvalidErrorCode is returned when the error code is invalid.
	ErrInvalidErrorCode = errors.New("invalid error code")
)

// Parser parses exception tokens from commands.
type Parser struct {
	shellParser   *syntax.Parser
	tokenPrefix   string
	envVarName    string
	errorCodeExpr *regexp.Regexp
}

// ParserOption configures the Parser.
type ParserOption func(*Parser)

// WithTokenPrefix sets the token prefix (default: "EXC").
func WithTokenPrefix(prefix string) ParserOption {
	return func(p *Parser) {
		if prefix != "" {
			p.tokenPrefix = prefix
		}
	}
}

// WithEnvVarName sets the environment variable name (default: "KLACK").
func WithEnvVarName(name string) ParserOption {
	return func(p *Parser) {
		if name != "" {
			p.envVarName = name
		}
	}
}

// NewParser creates a new token parser.
func NewParser(opts ...ParserOption) *Parser {
	p := &Parser{
		shellParser:   syntax.NewParser(syntax.KeepComments(true)),
		tokenPrefix:   DefaultTokenPrefix,
		envVarName:    DefaultEnvVarName,
		errorCodeExpr: regexp.MustCompile(`^[A-Z]{2,10}[0-9]{1,5}$`),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// ParseResult contains the result of parsing a command for exception tokens.
type ParseResult struct {
	// Token is the parsed exception token, if found.
	Token *Token

	// Source indicates where the token was found.
	Source TokenSource

	// Found indicates whether a token was found.
	Found bool
}

// Parse parses a command string and extracts exception tokens.
// It checks both shell comments and environment variables.
func (p *Parser) Parse(command string) (*ParseResult, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, ErrEmptyCommand
	}

	// Parse the command into an AST
	file, err := p.shellParser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, errors.Wrap(ErrParseFailed, err.Error())
	}

	// First, try to find token in environment variables
	if result := p.findTokenInEnvVars(file); result.Found {
		return result, nil
	}

	// Then, try to find token in comments
	if result := p.findTokenInComments(file); result.Found {
		return result, nil
	}

	return &ParseResult{Found: false}, nil
}

// findTokenInEnvVars searches for exception tokens in environment variable assignments.
func (p *Parser) findTokenInEnvVars(file *syntax.File) *ParseResult {
	result := &ParseResult{Found: false}

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

		// Check assignments (e.g., KLACK="EXC:GIT022:reason" git commit)
		for _, assign := range call.Assigns {
			if assign.Name == nil {
				continue
			}

			if assign.Name.Value != p.envVarName {
				continue
			}

			// Extract the value
			value := wordToString(assign.Value)
			if value == "" {
				continue
			}

			// Try to parse the token
			token, err := p.parseToken(value)
			if err != nil {
				continue
			}

			result.Token = token
			result.Source = TokenSourceEnvVar
			result.Found = true

			return false
		}

		return true
	})

	return result
}

// findTokenInComments searches for exception tokens in shell comments.
func (p *Parser) findTokenInComments(file *syntax.File) *ParseResult {
	result := &ParseResult{Found: false}

	// Walk through all statements to find comments
	syntax.Walk(file, func(node syntax.Node) bool {
		if result.Found {
			return false
		}

		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}

		// Check comments attached to this statement
		for _, comment := range stmt.Comments {
			token, err := p.parseTokenFromComment(comment.Text)
			if err != nil {
				continue
			}

			result.Token = token
			result.Source = TokenSourceComment
			result.Found = true

			return false
		}

		return true
	})

	// Also check trailing comments on the file
	if !result.Found {
		for _, comment := range file.Last {
			token, err := p.parseTokenFromComment(comment.Text)
			if err != nil {
				continue
			}

			result.Token = token
			result.Source = TokenSourceComment
			result.Found = true

			break
		}
	}

	return result
}

// parseTokenFromComment extracts a token from a comment string.
// It requires the token to appear at a word boundary (start of string or
// preceded by whitespace) to avoid matching substrings like "NOEXC:GIT019".
func (p *Parser) parseTokenFromComment(text string) (*Token, error) {
	prefix := p.tokenPrefix + ":"
	searchFrom := 0

	for {
		idx := strings.Index(text[searchFrom:], prefix)
		if idx == -1 {
			return nil, ErrTokenNotFound
		}

		tokenStart := searchFrom + idx

		// Check word boundary: must be at start of string or preceded by whitespace
		if tokenStart == 0 || text[tokenStart-1] == ' ' || text[tokenStart-1] == '\t' {
			tokenStr := text[tokenStart:]

			tokenEnd := strings.IndexAny(tokenStr, " \t\n")
			if tokenEnd != -1 {
				tokenStr = tokenStr[:tokenEnd]
			}

			return p.parseToken(tokenStr)
		}

		searchFrom = tokenStart + len(prefix)
	}
}

// parseToken parses a raw token string into a Token struct.
// Expected format: PREFIX:ERROR_CODE[:URL_ENCODED_REASON]
func (p *Parser) parseToken(raw string) (*Token, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidToken
	}

	parts := strings.SplitN(raw, ":", tokenParts)
	if len(parts) < minTokenParts {
		return nil, errors.Wrapf(
			ErrInvalidToken,
			"expected at least %d parts, got %d",
			minTokenParts,
			len(parts),
		)
	}

	prefix := parts[0]
	if prefix != p.tokenPrefix {
		return nil, errors.Wrapf(
			ErrInvalidToken,
			"expected prefix %q, got %q",
			p.tokenPrefix,
			prefix,
		)
	}

	errorCode := parts[1]
	if !p.isValidErrorCode(errorCode) {
		return nil, errors.Wrapf(ErrInvalidErrorCode, "invalid error code format: %q", errorCode)
	}

	var reason string

	if len(parts) == tokenParts {
		// URL-decode the reason
		decoded, err := url.QueryUnescape(parts[2])
		if err != nil {
			// If decoding fails, use the raw value
			reason = parts[2]
		} else {
			reason = decoded
		}
	}

	return &Token{
		Prefix:    prefix,
		ErrorCode: errorCode,
		Reason:    reason,
		Raw:       raw,
	}, nil
}

// isValidErrorCode checks if an error code matches the expected format.
// Valid format: 2-10 uppercase letters followed by 1-5 digits (e.g., GIT022, SEC001).
func (p *Parser) isValidErrorCode(code string) bool {
	return p.errorCodeExpr.MatchString(code)
}

// wordToString converts a syntax.Word to a string, returning "" if it
// contains any variable expansions or command substitutions. This prevents
// tokens like EXC:${CODE}:reason from being silently resolved to literals.
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
		case *syntax.DblQuoted:
			for _, dqPart := range p.Parts {
				switch lit := dqPart.(type) {
				case *syntax.Lit:
					result.WriteString(lit.Value)
				default:
					// Variable expansion, command substitution, etc.
					return ""
				}
			}
		default:
			// Unknown part type (parameter expansion, arithmetic, etc.)
			return ""
		}
	}

	return result.String()
}
