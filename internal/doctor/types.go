// Package doctor provides health check and diagnostics functionality for klaudiush
package doctor

//go:generate mockgen -source=types.go -destination=types_mock.go -package=doctor

import "context"

// Severity represents the severity level of a check result
type Severity string

const (
	// SeverityError indicates a blocking error that must be fixed
	SeverityError Severity = "error"
	// SeverityWarning indicates a non-blocking warning that should be fixed
	SeverityWarning Severity = "warning"
	// SeverityInfo indicates informational output
	SeverityInfo Severity = "info"
)

// Status represents the status of a health check
type Status string

const (
	// StatusPass indicates the check passed
	StatusPass Status = "pass"
	// StatusFail indicates the check failed
	StatusFail Status = "fail"
	// StatusSkipped indicates the check was skipped
	StatusSkipped Status = "skipped"
)

// Category represents the category of a health check
type Category string

const (
	// CategoryBinary checks for binary availability and permissions
	CategoryBinary Category = "binary"
	// CategoryHook checks for hook registration in Claude settings
	CategoryHook Category = "hook"
	// CategoryConfig checks for configuration file validity
	CategoryConfig Category = "config"
	// CategoryTools checks for optional tool dependencies
	CategoryTools Category = "tools"
	// CategoryBackup checks for backup system health
	CategoryBackup Category = "backup"
)

// CheckResult represents the result of a health check
type CheckResult struct {
	// Name is the human-readable name of the check
	Name string

	// Category indicates the category this check belongs to
	Category Category

	// Severity indicates the severity level
	Severity Severity

	// Status indicates whether the check passed, failed, or was skipped
	Status Status

	// Message is the primary message describing the result
	Message string

	// Details contains additional context about the result
	Details []string

	// FixID links to a Fixer that can fix this issue, if available
	FixID string
}

// HealthChecker performs a health check and returns a result
type HealthChecker interface {
	// Name returns the human-readable name of the check
	Name() string

	// Category returns the category this check belongs to
	Category() Category

	// Check performs the health check and returns a result
	Check(ctx context.Context) CheckResult
}

// Fixer can automatically fix issues identified by health checks
type Fixer interface {
	// ID returns the unique identifier for this fixer
	ID() string

	// Description returns a human-readable description of what this fixer does
	Description() string

	// CanFix determines if this fixer can fix the given check result
	CanFix(result CheckResult) bool

	// Fix attempts to fix the issue. If interactive is true, it may prompt the user.
	Fix(ctx context.Context, interactive bool) error
}

// Reporter formats and outputs check results
type Reporter interface {
	// Report outputs the results of health checks
	Report(results []CheckResult, verbose bool)
}

// NewCheckResult creates a new CheckResult with the given parameters
func NewCheckResult(name string, severity Severity, status Status, message string) CheckResult {
	return CheckResult{
		Name:     name,
		Severity: severity,
		Status:   status,
		Message:  message,
		Details:  []string{},
	}
}

// WithDetails adds details to a CheckResult
func (r CheckResult) WithDetails(details ...string) CheckResult {
	r.Details = append(r.Details, details...)
	return r
}

// WithFixID sets the fix ID for a CheckResult
func (r CheckResult) WithFixID(fixID string) CheckResult {
	r.FixID = fixID
	return r
}

// Pass creates a passing check result
func Pass(name, message string) CheckResult {
	return NewCheckResult(name, SeverityInfo, StatusPass, message)
}

// FailError creates a failing check result with error severity
func FailError(name, message string) CheckResult {
	return NewCheckResult(name, SeverityError, StatusFail, message)
}

// FailWarning creates a failing check result with warning severity
func FailWarning(name, message string) CheckResult {
	return NewCheckResult(name, SeverityWarning, StatusFail, message)
}

// Skip creates a skipped check result
func Skip(name, message string) CheckResult {
	return NewCheckResult(name, SeverityInfo, StatusSkipped, message)
}

// IsError returns true if the result is an error
func (r CheckResult) IsError() bool {
	return r.Status == StatusFail && r.Severity == SeverityError
}

// IsWarning returns true if the result is a warning
func (r CheckResult) IsWarning() bool {
	return r.Status == StatusFail && r.Severity == SeverityWarning
}

// IsPassed returns true if the check passed
func (r CheckResult) IsPassed() bool {
	return r.Status == StatusPass
}

// IsSkipped returns true if the check was skipped
func (r CheckResult) IsSkipped() bool {
	return r.Status == StatusSkipped
}

// HasFix returns true if the result has a fix available
func (r CheckResult) HasFix() bool {
	return r.FixID != ""
}
