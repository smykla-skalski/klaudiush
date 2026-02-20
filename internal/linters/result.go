// Package linters provides typed abstractions for external linting tools
package linters

// LintSeverity represents the severity level of a lint finding
type LintSeverity string

const (
	// SeverityError indicates a blocking error
	SeverityError LintSeverity = "error"
	// SeverityWarning indicates a non-blocking warning
	SeverityWarning LintSeverity = "warning"
	// SeverityInfo indicates an informational message
	SeverityInfo LintSeverity = "info"
)

// LintFinding represents a single lint finding
type LintFinding struct {
	File     string
	Line     int
	Column   int
	Severity LintSeverity
	Message  string
	Rule     string
}

// LintResult represents the result of running a linter
type LintResult struct {
	Success                bool
	Findings               []LintFinding
	RawOut                 string
	Err                    error
	TableSuggested         map[int]string // Line number -> suggested formatted table (blocking)
	CosmeticTableWarnings  []string       // Non-blocking cosmetic table warnings
	CosmeticTableSuggested map[int]string // Line number -> suggested table for cosmetic issues
}

// HasErrors returns true if the result contains any error-level findings
func (r *LintResult) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}

	return false
}

// HasWarnings returns true if the result contains any warning-level findings
func (r *LintResult) HasWarnings() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityWarning {
			return true
		}
	}

	return false
}
