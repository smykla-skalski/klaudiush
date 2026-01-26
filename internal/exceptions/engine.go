// Package exceptions provides the exception workflow system for klaudiush.
package exceptions

import (
	"time"

	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// Engine is the main entry point for the exception workflow.
// It coordinates token parsing, policy evaluation, rate limiting, and audit logging.
type Engine struct {
	parser  *Parser
	matcher *PolicyMatcher
	logger  logger.Logger
	config  *config.ExceptionsConfig
}

// EngineOption configures the Engine.
type EngineOption func(*Engine)

// WithLogger sets the logger for the engine.
func WithLogger(log logger.Logger) EngineOption {
	return func(e *Engine) {
		if log != nil {
			e.logger = log
		}
	}
}

// WithParser sets a custom token parser.
func WithParser(p *Parser) EngineOption {
	return func(e *Engine) {
		if p != nil {
			e.parser = p
		}
	}
}

// WithMatcher sets a custom policy matcher.
func WithMatcher(m *PolicyMatcher) EngineOption {
	return func(e *Engine) {
		if m != nil {
			e.matcher = m
		}
	}
}

// NewEngine creates a new exception engine.
func NewEngine(cfg *config.ExceptionsConfig, opts ...EngineOption) *Engine {
	// Create parser with config-driven prefix
	parserOpts := []ParserOption{}
	if cfg != nil && cfg.GetTokenPrefix() != "" {
		parserOpts = append(parserOpts, WithTokenPrefix(cfg.GetTokenPrefix()))
	}

	e := &Engine{
		parser:  NewParser(parserOpts...),
		matcher: NewPolicyMatcher(cfg),
		logger:  logger.NewNoOpLogger(),
		config:  cfg,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// EvaluateRequest represents a request to evaluate an exception.
type EvaluateRequest struct {
	// Command is the shell command to parse for exception tokens.
	Command string

	// ValidatorName is the name of the validator that would block.
	ValidatorName string

	// ErrorCode is the validator error code being bypassed.
	// If empty, the error code from the token will be used.
	ErrorCode string

	// WorkingDir is the current working directory (for audit).
	WorkingDir string

	// Repository is the git repository path (for audit).
	Repository string
}

// Evaluate evaluates a command for exception tokens and returns the result.
// This is the main entry point for exception evaluation.
func (e *Engine) Evaluate(req *EvaluateRequest) *ExceptionResult {
	if req == nil {
		return &ExceptionResult{
			Allowed: false,
			Reason:  "no request provided",
		}
	}

	// Check if exceptions are enabled
	if e.config != nil && !e.config.IsEnabled() {
		e.logger.Debug("exception system disabled")

		return &ExceptionResult{
			Allowed: false,
			Reason:  "exception system is disabled",
		}
	}

	// Parse the command for exception tokens
	parseResult, err := e.parser.Parse(req.Command)
	if err != nil {
		e.logger.Debug("failed to parse command for exception tokens",
			"error", err.Error(),
			"command_length", len(req.Command),
		)

		return &ExceptionResult{
			Allowed: false,
			Reason:  "failed to parse command: " + err.Error(),
		}
	}

	// Check if a token was found
	if !parseResult.Found {
		return &ExceptionResult{
			Allowed: false,
			Reason:  "no exception token found",
		}
	}

	token := parseResult.Token

	// Validate error code matches if specified
	if req.ErrorCode != "" && token.ErrorCode != req.ErrorCode {
		e.logger.Debug("token error code mismatch",
			"expected", req.ErrorCode,
			"found", token.ErrorCode,
		)

		return &ExceptionResult{
			Allowed: false,
			Reason:  "token error code " + token.ErrorCode + " does not match expected " + req.ErrorCode,
		}
	}

	// Create exception request for policy evaluation
	exceptionReq := &ExceptionRequest{
		Token:         token,
		Source:        parseResult.Source,
		Command:       req.Command,
		ValidatorName: req.ValidatorName,
		ErrorCode:     token.ErrorCode,
		RequestTime:   time.Now(),
	}

	// Evaluate against policy
	decision := e.matcher.Match(exceptionReq)

	e.logger.Debug("policy evaluation complete",
		"error_code", token.ErrorCode,
		"allowed", decision.Allowed,
		"reason", decision.Reason,
	)

	// Build result with audit entry
	result := &ExceptionResult{
		Allowed: decision.Allowed,
		Reason:  decision.Reason,
		AuditEntry: &AuditEntry{
			Timestamp:     time.Now(),
			ErrorCode:     token.ErrorCode,
			ValidatorName: req.ValidatorName,
			Allowed:       decision.Allowed,
			Reason:        token.Reason,
			Source:        parseResult.Source.String(),
			Command:       truncateCommand(req.Command),
			WorkingDir:    req.WorkingDir,
			Repository:    req.Repository,
		},
	}

	if !decision.Allowed {
		result.AuditEntry.DenialReason = decision.Reason
	}

	return result
}

// EvaluateForErrorCode is a convenience method that evaluates a command
// for a specific error code.
func (e *Engine) EvaluateForErrorCode(
	command, validatorName, errorCode string,
) *ExceptionResult {
	return e.Evaluate(&EvaluateRequest{
		Command:       command,
		ValidatorName: validatorName,
		ErrorCode:     errorCode,
	})
}

// HasToken checks if a command contains an exception token.
// This is useful for quick checks without full evaluation.
func (e *Engine) HasToken(command string) bool {
	result, err := e.parser.Parse(command)
	if err != nil {
		return false
	}

	return result.Found
}

// GetTokenErrorCode extracts the error code from a command's exception token.
// Returns empty string if no token is found.
func (e *Engine) GetTokenErrorCode(command string) string {
	result, err := e.parser.Parse(command)
	if err != nil || !result.Found {
		return ""
	}

	return result.Token.ErrorCode
}

// IsEnabled returns whether the exception system is enabled.
func (e *Engine) IsEnabled() bool {
	if e.config == nil {
		return true
	}

	return e.config.IsEnabled()
}

// truncateCommand truncates a command to prevent sensitive data leakage in logs.
const maxCommandLength = 200

func truncateCommand(cmd string) string {
	if len(cmd) <= maxCommandLength {
		return cmd
	}

	return cmd[:maxCommandLength] + "..."
}
