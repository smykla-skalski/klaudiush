// Package exceptions provides the exception workflow system for klaudiush.
package exceptions

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// File permission constants.
const (
	// stateFilePermissions is the permission mode for the state file.
	stateFilePermissions = 0o600

	// stateDirPermissions is the permission mode for the state directory.
	stateDirPermissions = 0o700
)

// RateLimiter manages rate limiting for exception usage.
// It tracks usage counts per error code and globally, with
// configurable hourly and daily limits.
type RateLimiter struct {
	mu     sync.RWMutex
	state  *RateLimitState
	config *config.ExceptionRateLimitConfig
	policy *config.ExceptionsConfig
	logger logger.Logger

	// stateFile is the resolved path for state persistence.
	stateFile string

	// now is a function that returns the current time.
	// Used for testing to control time.
	now func() time.Time
}

// RateLimiterOption configures the RateLimiter.
type RateLimiterOption func(*RateLimiter)

// WithRateLimiterLogger sets the logger.
func WithRateLimiterLogger(log logger.Logger) RateLimiterOption {
	return func(r *RateLimiter) {
		if log != nil {
			r.logger = log
		}
	}
}

// WithStateFile sets a custom state file path.
func WithStateFile(path string) RateLimiterOption {
	return func(r *RateLimiter) {
		r.stateFile = path
	}
}

// WithTimeFunc sets a custom time function for testing.
func WithTimeFunc(fn func() time.Time) RateLimiterOption {
	return func(r *RateLimiter) {
		if fn != nil {
			r.now = fn
		}
	}
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(
	rateCfg *config.ExceptionRateLimitConfig,
	policyCfg *config.ExceptionsConfig,
	opts ...RateLimiterOption,
) *RateLimiter {
	r := &RateLimiter{
		state:  NewRateLimitState(),
		config: rateCfg,
		policy: policyCfg,
		logger: logger.NewNoOpLogger(),
		now:    time.Now,
	}

	// Set default state file from config
	if rateCfg != nil {
		r.stateFile = rateCfg.GetStateFile()
	} else {
		r.stateFile = (&config.ExceptionRateLimitConfig{}).GetStateFile()
	}

	for _, opt := range opts {
		opt(r)
	}

	// Reinitialize state window times using the (possibly custom) time function.
	// This ensures tests with custom time functions get correct initial windows.
	now := r.now()
	r.state.HourStartTime = now.Truncate(time.Hour)
	r.state.DayStartTime = now.Truncate(hoursPerDay * time.Hour)
	r.state.LastUpdated = now

	return r
}

// CheckResult represents the result of a rate limit check.
type CheckResult struct {
	// Allowed indicates whether the rate limit allows this exception.
	Allowed bool

	// Reason explains why the check passed or failed.
	Reason string

	// GlobalHourlyRemaining is remaining global hourly quota.
	GlobalHourlyRemaining int

	// GlobalDailyRemaining is remaining global daily quota.
	GlobalDailyRemaining int

	// ErrorCodeHourlyRemaining is remaining quota for this error code hourly.
	ErrorCodeHourlyRemaining int

	// ErrorCodeDailyRemaining is remaining quota for this error code daily.
	ErrorCodeDailyRemaining int
}

// Check verifies if an exception can be allowed under current rate limits.
// It does NOT record the usage - call Record after a successful exception.
func (r *RateLimiter) Check(errorCode string) *CheckResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	// First, ensure windows are current
	r.resetIfExpiredLocked()

	// Check if rate limiting is enabled
	if r.config != nil && !r.config.IsRateLimitEnabled() {
		return &CheckResult{
			Allowed:                  true,
			Reason:                   "rate limiting disabled",
			GlobalHourlyRemaining:    -1, // -1 indicates unlimited
			GlobalDailyRemaining:     -1,
			ErrorCodeHourlyRemaining: -1,
			ErrorCodeDailyRemaining:  -1,
		}
	}

	// Get global limits
	globalMaxHour := r.getGlobalMaxPerHour()
	globalMaxDay := r.getGlobalMaxPerDay()

	// Check global hourly limit
	if globalMaxHour > 0 && r.state.GlobalHourlyCount >= globalMaxHour {
		return &CheckResult{
			Allowed:               false,
			Reason:                "global hourly limit exceeded",
			GlobalHourlyRemaining: 0,
			GlobalDailyRemaining:  max(0, globalMaxDay-r.state.GlobalDailyCount),
		}
	}

	// Check global daily limit
	if globalMaxDay > 0 && r.state.GlobalDailyCount >= globalMaxDay {
		return &CheckResult{
			Allowed:               false,
			Reason:                "global daily limit exceeded",
			GlobalHourlyRemaining: max(0, globalMaxHour-r.state.GlobalHourlyCount),
			GlobalDailyRemaining:  0,
		}
	}

	// Get per-error-code limits from policy
	codeMaxHour, codeMaxDay := r.getPolicyLimits(errorCode)

	// Check per-error-code hourly limit
	codeHourlyUsage := r.state.HourlyUsage[errorCode]
	if codeMaxHour > 0 && codeHourlyUsage >= codeMaxHour {
		return &CheckResult{
			Allowed:                  false,
			Reason:                   "hourly limit exceeded for " + errorCode,
			GlobalHourlyRemaining:    max(0, globalMaxHour-r.state.GlobalHourlyCount),
			GlobalDailyRemaining:     max(0, globalMaxDay-r.state.GlobalDailyCount),
			ErrorCodeHourlyRemaining: 0,
			ErrorCodeDailyRemaining:  max(0, codeMaxDay-r.state.DailyUsage[errorCode]),
		}
	}

	// Check per-error-code daily limit
	codeDailyUsage := r.state.DailyUsage[errorCode]
	if codeMaxDay > 0 && codeDailyUsage >= codeMaxDay {
		return &CheckResult{
			Allowed:                  false,
			Reason:                   "daily limit exceeded for " + errorCode,
			GlobalHourlyRemaining:    max(0, globalMaxHour-r.state.GlobalHourlyCount),
			GlobalDailyRemaining:     max(0, globalMaxDay-r.state.GlobalDailyCount),
			ErrorCodeHourlyRemaining: max(0, codeMaxHour-codeHourlyUsage),
			ErrorCodeDailyRemaining:  0,
		}
	}

	// Calculate remaining quotas
	globalHourlyRemaining := -1
	if globalMaxHour > 0 {
		globalHourlyRemaining = globalMaxHour - r.state.GlobalHourlyCount
	}

	globalDailyRemaining := -1
	if globalMaxDay > 0 {
		globalDailyRemaining = globalMaxDay - r.state.GlobalDailyCount
	}

	codeHourlyRemaining := -1
	if codeMaxHour > 0 {
		codeHourlyRemaining = codeMaxHour - codeHourlyUsage
	}

	codeDailyRemaining := -1
	if codeMaxDay > 0 {
		codeDailyRemaining = codeMaxDay - codeDailyUsage
	}

	return &CheckResult{
		Allowed:                  true,
		Reason:                   "within rate limits",
		GlobalHourlyRemaining:    globalHourlyRemaining,
		GlobalDailyRemaining:     globalDailyRemaining,
		ErrorCodeHourlyRemaining: codeHourlyRemaining,
		ErrorCodeDailyRemaining:  codeDailyRemaining,
	}
}

// Record records an exception usage for the given error code.
// Should be called after an exception has been allowed.
func (r *RateLimiter) Record(errorCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Ensure windows are current
	r.resetIfExpiredLocked()

	// Increment counters
	r.state.GlobalHourlyCount++
	r.state.GlobalDailyCount++
	r.state.HourlyUsage[errorCode]++
	r.state.DailyUsage[errorCode]++
	r.state.LastUpdated = r.now()

	r.logger.Debug("recorded exception usage",
		"error_code", errorCode,
		"global_hourly", r.state.GlobalHourlyCount,
		"global_daily", r.state.GlobalDailyCount,
	)

	return nil
}

// Load loads the rate limit state from the configured state file.
func (r *RateLimiter) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := r.resolveStatePath()

	// Path comes from trusted configuration, not user input.
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			r.logger.Debug("state file does not exist, using fresh state",
				"path", path,
			)

			return nil
		}

		return errors.Wrap(err, "reading state file")
	}

	var state RateLimitState
	if err := json.Unmarshal(data, &state); err != nil {
		r.logger.Debug("failed to parse state file, using fresh state",
			"path", path,
			"error", err.Error(),
		)

		return nil
	}

	// Initialize maps if nil (could happen with corrupted/old state files)
	if state.HourlyUsage == nil {
		state.HourlyUsage = make(map[string]int)
	}

	if state.DailyUsage == nil {
		state.DailyUsage = make(map[string]int)
	}

	r.state = &state
	r.resetIfExpiredLocked()

	r.logger.Debug("loaded state from file",
		"path", path,
		"global_hourly", r.state.GlobalHourlyCount,
		"global_daily", r.state.GlobalDailyCount,
	)

	return nil
}

// Save persists the current rate limit state to the configured state file.
func (r *RateLimiter) Save() error {
	r.mu.RLock()
	state := r.state
	path := r.resolveStatePath()
	r.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, stateDirPermissions); err != nil {
		return errors.Wrap(err, "creating state directory")
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshaling state")
	}

	// Write to temp file first for atomic operation
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, stateFilePermissions); err != nil {
		return errors.Wrap(err, "writing temp state file")
	}

	// Rename for atomic replace
	if err := os.Rename(tmpPath, path); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, "renaming state file")
	}

	r.logger.Debug("saved state to file",
		"path", path,
	)

	return nil
}

// Reset clears all rate limit state.
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.state = NewRateLimitState()

	// Update times with the current time function
	now := r.now()
	r.state.HourStartTime = now.Truncate(time.Hour)
	r.state.DayStartTime = now.Truncate(hoursPerDay * time.Hour)
	r.state.LastUpdated = now

	r.logger.Debug("rate limit state reset")
}

// GetState returns a copy of the current rate limit state.
func (r *RateLimiter) GetState() RateLimitState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a deep copy
	state := *r.state
	state.HourlyUsage = make(map[string]int, len(r.state.HourlyUsage))
	state.DailyUsage = make(map[string]int, len(r.state.DailyUsage))
	maps.Copy(state.HourlyUsage, r.state.HourlyUsage)
	maps.Copy(state.DailyUsage, r.state.DailyUsage)

	return state
}

// resetIfExpiredLocked resets counters if time windows have expired.
// Must be called with mu held.
func (r *RateLimiter) resetIfExpiredLocked() {
	now := r.now()
	currentHour := now.Truncate(time.Hour)
	currentDay := now.Truncate(hoursPerDay * time.Hour)

	// Reset hourly counters if hour has changed
	if currentHour.After(r.state.HourStartTime) {
		r.logger.Debug("resetting hourly counters",
			"old_hour", r.state.HourStartTime.Format(time.RFC3339),
			"new_hour", currentHour.Format(time.RFC3339),
		)

		r.state.GlobalHourlyCount = 0
		r.state.HourlyUsage = make(map[string]int)
		r.state.HourStartTime = currentHour
	}

	// Reset daily counters if day has changed
	if currentDay.After(r.state.DayStartTime) {
		r.logger.Debug("resetting daily counters",
			"old_day", r.state.DayStartTime.Format(time.RFC3339),
			"new_day", currentDay.Format(time.RFC3339),
		)

		r.state.GlobalDailyCount = 0
		r.state.DailyUsage = make(map[string]int)
		r.state.DayStartTime = currentDay
	}
}

// getGlobalMaxPerHour returns the global hourly limit.
func (r *RateLimiter) getGlobalMaxPerHour() int {
	if r.config == nil {
		return config.DefaultRateLimitPerHour
	}

	return r.config.GetMaxPerHour()
}

// getGlobalMaxPerDay returns the global daily limit.
func (r *RateLimiter) getGlobalMaxPerDay() int {
	if r.config == nil {
		return config.DefaultRateLimitPerDay
	}

	return r.config.GetMaxPerDay()
}

// getPolicyLimits returns the per-error-code limits from policy config.
// Returns (maxPerHour, maxPerDay) where 0 means unlimited.
func (r *RateLimiter) getPolicyLimits(errorCode string) (int, int) {
	if r.policy == nil {
		return 0, 0
	}

	policy := r.policy.GetPolicy(errorCode)
	if policy == nil {
		return 0, 0
	}

	return policy.GetMaxPerHour(), policy.GetMaxPerDay()
}

// resolveStatePath expands ~ in the state file path.
func (r *RateLimiter) resolveStatePath() string {
	path := r.stateFile
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	return path
}
