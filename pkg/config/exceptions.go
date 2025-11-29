// Package config provides configuration schema types for klaudiush validators.
package config

// Default values for exception configuration.
const (
	// DefaultMinReasonLength is the minimum reason length when required.
	DefaultMinReasonLength = 10

	// DefaultRateLimitPerHour is the global max exceptions per hour.
	DefaultRateLimitPerHour = 10

	// DefaultRateLimitPerDay is the global max exceptions per day.
	DefaultRateLimitPerDay = 50

	// DefaultAuditMaxSizeMB is the max audit log size before rotation.
	DefaultAuditMaxSizeMB = 10

	// DefaultAuditMaxAgeDays is the max age of audit entries.
	DefaultAuditMaxAgeDays = 30

	// DefaultAuditMaxBackups is the number of backup files to keep.
	DefaultAuditMaxBackups = 3
)

// ExceptionsConfig contains configuration for the exception workflow.
// Exceptions allow Claude to bypass specific validation blocks when predefined
// exception rules apply, with explicit acknowledgment via command-embedded tokens.
type ExceptionsConfig struct {
	// Enabled controls whether the exception system is active.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// Policies defines exception policies by error code.
	// Key is the error code (e.g., "GIT022", "SEC001").
	Policies map[string]*ExceptionPolicyConfig `json:"policies,omitempty" koanf:"policies" toml:"policies"`

	// RateLimit configures global rate limiting for exceptions.
	RateLimit *ExceptionRateLimitConfig `json:"rate_limit,omitempty" koanf:"rate_limit" toml:"rate_limit"`

	// Audit configures exception audit logging.
	Audit *ExceptionAuditConfig `json:"audit,omitempty" koanf:"audit" toml:"audit"`

	// TokenPrefix is the prefix used for exception tokens.
	// Default: "EXC"
	TokenPrefix string `json:"token_prefix,omitempty" koanf:"token_prefix" toml:"token_prefix"`
}

// ExceptionPolicyConfig defines policy for a specific error code.
type ExceptionPolicyConfig struct {
	// Enabled controls whether this exception policy is active.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// AllowException controls whether exceptions are allowed for this error code.
	// Default: true
	AllowException *bool `json:"allow_exception,omitempty" koanf:"allow_exception" toml:"allow_exception"`

	// RequireReason requires a justification reason in the exception token.
	// Default: false
	RequireReason *bool `json:"require_reason,omitempty" koanf:"require_reason" toml:"require_reason"`

	// MinReasonLength is the minimum length for the reason when required.
	// Default: 10
	MinReasonLength *int `json:"min_reason_length,omitempty" koanf:"min_reason_length" toml:"min_reason_length"`

	// ValidReasons is a list of pre-approved reasons.
	// If non-empty, only these reasons are accepted.
	ValidReasons []string `json:"valid_reasons,omitempty" koanf:"valid_reasons" toml:"valid_reasons"`

	// MaxPerHour limits how many times this exception can be used per hour.
	// Default: 0 (unlimited)
	MaxPerHour *int `json:"max_per_hour,omitempty" koanf:"max_per_hour" toml:"max_per_hour"`

	// MaxPerDay limits how many times this exception can be used per day.
	// Default: 0 (unlimited)
	MaxPerDay *int `json:"max_per_day,omitempty" koanf:"max_per_day" toml:"max_per_day"`

	// Description is a human-readable description of the policy.
	Description string `json:"description,omitempty" koanf:"description" toml:"description"`
}

// ExceptionRateLimitConfig configures global rate limiting for exceptions.
type ExceptionRateLimitConfig struct {
	// Enabled controls whether rate limiting is active.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// MaxPerHour is the global limit for all exceptions per hour.
	// Default: 10
	MaxPerHour *int `json:"max_per_hour,omitempty" koanf:"max_per_hour" toml:"max_per_hour"`

	// MaxPerDay is the global limit for all exceptions per day.
	// Default: 50
	MaxPerDay *int `json:"max_per_day,omitempty" koanf:"max_per_day" toml:"max_per_day"`

	// StateFile is the path to the rate limit state file.
	// Default: "~/.klaudiush/exception_state.json"
	StateFile string `json:"state_file,omitempty" koanf:"state_file" toml:"state_file"`
}

// ExceptionAuditConfig configures exception audit logging.
type ExceptionAuditConfig struct {
	// Enabled controls whether audit logging is active.
	// Default: true
	Enabled *bool `json:"enabled,omitempty" koanf:"enabled" toml:"enabled"`

	// LogFile is the path to the audit log file.
	// Default: "~/.klaudiush/exception_audit.jsonl"
	LogFile string `json:"log_file,omitempty" koanf:"log_file" toml:"log_file"`

	// MaxSizeMB is the maximum size of the audit log file before rotation.
	// Default: 10
	MaxSizeMB *int `json:"max_size_mb,omitempty" koanf:"max_size_mb" toml:"max_size_mb"`

	// MaxAgeDays is the maximum age of audit entries before deletion.
	// Default: 30
	MaxAgeDays *int `json:"max_age_days,omitempty" koanf:"max_age_days" toml:"max_age_days"`

	// MaxBackups is the number of rotated log files to keep.
	// Default: 3
	MaxBackups *int `json:"max_backups,omitempty" koanf:"max_backups" toml:"max_backups"`
}

// IsEnabled returns true if the exceptions system is enabled.
// Returns true if Enabled is nil (default behavior).
func (e *ExceptionsConfig) IsEnabled() bool {
	if e == nil || e.Enabled == nil {
		return true
	}

	return *e.Enabled
}

// GetTokenPrefix returns the token prefix, defaulting to "EXC".
func (e *ExceptionsConfig) GetTokenPrefix() string {
	if e == nil || e.TokenPrefix == "" {
		return "EXC"
	}

	return e.TokenPrefix
}

// GetPolicy returns the policy for the given error code.
// Returns nil if no policy is defined.
func (e *ExceptionsConfig) GetPolicy(errorCode string) *ExceptionPolicyConfig {
	if e == nil || e.Policies == nil {
		return nil
	}

	return e.Policies[errorCode]
}

// IsPolicyEnabled returns true if the policy is enabled.
// Returns true if Enabled is nil (default behavior).
func (p *ExceptionPolicyConfig) IsPolicyEnabled() bool {
	if p == nil || p.Enabled == nil {
		return true
	}

	return *p.Enabled
}

// IsExceptionAllowed returns true if exceptions are allowed.
// Returns true if AllowException is nil (default behavior).
func (p *ExceptionPolicyConfig) IsExceptionAllowed() bool {
	if p == nil || p.AllowException == nil {
		return true
	}

	return *p.AllowException
}

// IsReasonRequired returns true if a reason is required.
// Returns false if RequireReason is nil (default behavior).
func (p *ExceptionPolicyConfig) IsReasonRequired() bool {
	if p == nil || p.RequireReason == nil {
		return false
	}

	return *p.RequireReason
}

// GetMinReasonLength returns the minimum reason length.
// Returns DefaultMinReasonLength if MinReasonLength is nil (default).
func (p *ExceptionPolicyConfig) GetMinReasonLength() int {
	if p == nil || p.MinReasonLength == nil {
		return DefaultMinReasonLength
	}

	return *p.MinReasonLength
}

// GetMaxPerHour returns the max per hour limit for this policy.
// Returns 0 if MaxPerHour is nil (unlimited).
func (p *ExceptionPolicyConfig) GetMaxPerHour() int {
	if p == nil || p.MaxPerHour == nil {
		return 0
	}

	return *p.MaxPerHour
}

// GetMaxPerDay returns the max per day limit for this policy.
// Returns 0 if MaxPerDay is nil (unlimited).
func (p *ExceptionPolicyConfig) GetMaxPerDay() int {
	if p == nil || p.MaxPerDay == nil {
		return 0
	}

	return *p.MaxPerDay
}

// IsRateLimitEnabled returns true if rate limiting is enabled.
// Returns true if Enabled is nil (default behavior).
func (r *ExceptionRateLimitConfig) IsRateLimitEnabled() bool {
	if r == nil || r.Enabled == nil {
		return true
	}

	return *r.Enabled
}

// GetMaxPerHour returns the global max per hour limit.
// Returns DefaultRateLimitPerHour if MaxPerHour is nil (default).
func (r *ExceptionRateLimitConfig) GetMaxPerHour() int {
	if r == nil || r.MaxPerHour == nil {
		return DefaultRateLimitPerHour
	}

	return *r.MaxPerHour
}

// GetMaxPerDay returns the global max per day limit.
// Returns DefaultRateLimitPerDay if MaxPerDay is nil (default).
func (r *ExceptionRateLimitConfig) GetMaxPerDay() int {
	if r == nil || r.MaxPerDay == nil {
		return DefaultRateLimitPerDay
	}

	return *r.MaxPerDay
}

// GetStateFile returns the state file path.
// Returns "~/.klaudiush/exception_state.json" if StateFile is empty.
func (r *ExceptionRateLimitConfig) GetStateFile() string {
	if r == nil || r.StateFile == "" {
		return "~/.klaudiush/exception_state.json"
	}

	return r.StateFile
}

// IsAuditEnabled returns true if audit logging is enabled.
// Returns true if Enabled is nil (default behavior).
func (a *ExceptionAuditConfig) IsAuditEnabled() bool {
	if a == nil || a.Enabled == nil {
		return true
	}

	return *a.Enabled
}

// GetLogFile returns the audit log file path.
// Returns "~/.klaudiush/exception_audit.jsonl" if LogFile is empty.
func (a *ExceptionAuditConfig) GetLogFile() string {
	if a == nil || a.LogFile == "" {
		return "~/.klaudiush/exception_audit.jsonl"
	}

	return a.LogFile
}

// GetMaxSizeMB returns the maximum log file size in MB.
// Returns DefaultAuditMaxSizeMB if MaxSizeMB is nil (default).
func (a *ExceptionAuditConfig) GetMaxSizeMB() int {
	if a == nil || a.MaxSizeMB == nil {
		return DefaultAuditMaxSizeMB
	}

	return *a.MaxSizeMB
}

// GetMaxAgeDays returns the maximum age of audit entries in days.
// Returns DefaultAuditMaxAgeDays if MaxAgeDays is nil (default).
func (a *ExceptionAuditConfig) GetMaxAgeDays() int {
	if a == nil || a.MaxAgeDays == nil {
		return DefaultAuditMaxAgeDays
	}

	return *a.MaxAgeDays
}

// GetMaxBackups returns the number of backup files to keep.
// Returns DefaultAuditMaxBackups if MaxBackups is nil (default).
func (a *ExceptionAuditConfig) GetMaxBackups() int {
	if a == nil || a.MaxBackups == nil {
		return DefaultAuditMaxBackups
	}

	return *a.MaxBackups
}
