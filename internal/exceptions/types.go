// Package exceptions provides the exception workflow system for klaudiush.
// It allows Claude to bypass specific validation blocks when predefined
// exception rules apply, with explicit acknowledgment via command-embedded tokens.
package exceptions

import (
	"time"
)

// Time constants.
const (
	hoursPerDay = 24
)

// Token represents a parsed exception acknowledgment token.
// Token format: EXC:<ERROR_CODE>:<URL_ENCODED_REASON>
// Example: EXC:GIT022:Emergency+hotfix
type Token struct {
	// Prefix is the token prefix (e.g., "EXC").
	Prefix string

	// ErrorCode is the validator error code (e.g., "GIT022", "SEC001").
	ErrorCode string

	// Reason is the URL-decoded justification reason.
	// May be empty if no reason was provided.
	Reason string

	// Raw is the original unparsed token string.
	Raw string
}

// TokenSource indicates where the exception token was found.
type TokenSource int

const (
	// TokenSourceUnknown indicates the token source is unknown.
	TokenSourceUnknown TokenSource = iota

	// TokenSourceComment indicates the token was found in a shell comment.
	// Example: git push origin main  # EXC:GIT022:Emergency+hotfix
	TokenSourceComment

	// TokenSourceEnvVar indicates the token was found in an environment variable.
	// Example: KLAUDIUSH_ACK="EXC:SEC001:Test+fixture" git commit -sS -m "msg"
	TokenSourceEnvVar
)

// String returns a string representation of the token source.
func (s TokenSource) String() string {
	switch s {
	case TokenSourceComment:
		return "comment"
	case TokenSourceEnvVar:
		return "env_var"
	default:
		return "unknown"
	}
}

// ExceptionRequest represents a request to bypass a validation.
type ExceptionRequest struct {
	// Token is the parsed exception token.
	Token *Token

	// Source indicates where the token was found.
	Source TokenSource

	// Command is the original command being validated.
	Command string

	// ValidatorName is the name of the validator that would block.
	ValidatorName string

	// ErrorCode is the validator error code being bypassed.
	ErrorCode string

	// RequestTime is when the exception was requested.
	RequestTime time.Time
}

// ExceptionResult represents the result of evaluating an exception request.
type ExceptionResult struct {
	// Allowed indicates whether the exception was allowed.
	Allowed bool

	// Reason is the reason for allowing or denying.
	Reason string

	// AuditEntry is the audit log entry for this exception.
	// Only populated if audit logging is enabled.
	AuditEntry *AuditEntry
}

// AuditEntry represents an audit log entry for an exception.
type AuditEntry struct {
	// Timestamp is when the exception was processed.
	Timestamp time.Time `json:"timestamp"`

	// ErrorCode is the validator error code.
	ErrorCode string `json:"error_code"`

	// ValidatorName is the name of the validator.
	ValidatorName string `json:"validator_name"`

	// Allowed indicates whether the exception was allowed.
	Allowed bool `json:"allowed"`

	// Reason is the justification reason provided.
	Reason string `json:"reason,omitempty"`

	// DenialReason is why the exception was denied (if denied).
	DenialReason string `json:"denial_reason,omitempty"`

	// Source indicates where the token was found.
	Source string `json:"source"`

	// Command is the command that triggered the exception.
	// Truncated to prevent sensitive data leakage.
	Command string `json:"command,omitempty"`

	// WorkingDir is the working directory when the exception was requested.
	WorkingDir string `json:"working_dir,omitempty"`

	// Repository is the git repository path.
	Repository string `json:"repository,omitempty"`
}

// RateLimitState represents the current rate limit state.
type RateLimitState struct {
	// HourlyUsage tracks usage counts by error code for the current hour.
	// Key: error code, Value: count
	HourlyUsage map[string]int `json:"hourly_usage"`

	// DailyUsage tracks usage counts by error code for the current day.
	// Key: error code, Value: count
	DailyUsage map[string]int `json:"daily_usage"`

	// GlobalHourlyCount is the total exceptions used this hour.
	GlobalHourlyCount int `json:"global_hourly_count"`

	// GlobalDailyCount is the total exceptions used today.
	GlobalDailyCount int `json:"global_daily_count"`

	// HourStartTime is when the current hour window started.
	HourStartTime time.Time `json:"hour_start_time"`

	// DayStartTime is when the current day window started.
	DayStartTime time.Time `json:"day_start_time"`

	// LastUpdated is when this state was last modified.
	LastUpdated time.Time `json:"last_updated"`
}

// NewRateLimitState creates a new rate limit state with initialized maps.
func NewRateLimitState() *RateLimitState {
	now := time.Now()

	return &RateLimitState{
		HourlyUsage:       make(map[string]int),
		DailyUsage:        make(map[string]int),
		GlobalHourlyCount: 0,
		GlobalDailyCount:  0,
		HourStartTime:     now.Truncate(time.Hour),
		DayStartTime:      now.Truncate(hoursPerDay * time.Hour),
		LastUpdated:       now,
	}
}

// PolicyDecision represents a policy engine decision.
type PolicyDecision struct {
	// Allowed indicates whether the exception is allowed by policy.
	Allowed bool

	// Reason explains the decision.
	Reason string

	// RequiredReason indicates if a reason was required.
	RequiredReason bool

	// ProvidedReason is the reason that was provided.
	ProvidedReason string
}

// ValidationError represents an error during exception validation.
type ValidationError struct {
	// Code is the error code for this validation error.
	Code string

	// Message is the human-readable error message.
	Message string

	// Field is the field that caused the error (if applicable).
	Field string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return e.Code + ": " + e.Message + " (field: " + e.Field + ")"
	}

	return e.Code + ": " + e.Message
}
