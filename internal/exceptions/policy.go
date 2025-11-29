// Package exceptions provides the exception workflow system for klaudiush.
package exceptions

import (
	"slices"
	"strconv"
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/config"
)

// DefaultPolicy provides default policy settings when no explicit policy exists.
var DefaultPolicy = &config.ExceptionPolicyConfig{}

// PolicyMatcher evaluates exception policies against requests.
type PolicyMatcher struct {
	config *config.ExceptionsConfig
}

// NewPolicyMatcher creates a new policy matcher.
func NewPolicyMatcher(cfg *config.ExceptionsConfig) *PolicyMatcher {
	return &PolicyMatcher{
		config: cfg,
	}
}

// Match evaluates a request against the configured policies.
// Returns a PolicyDecision indicating whether the exception is allowed.
func (m *PolicyMatcher) Match(req *ExceptionRequest) *PolicyDecision {
	if req == nil || req.Token == nil {
		return &PolicyDecision{
			Allowed: false,
			Reason:  "no exception token provided",
		}
	}

	// Check if exceptions are enabled globally
	if m.config != nil && !m.config.IsEnabled() {
		return &PolicyDecision{
			Allowed: false,
			Reason:  "exception system is disabled",
		}
	}

	// Get the policy for this error code
	policy := m.getPolicy(req.Token.ErrorCode)

	// Check if policy is enabled
	if !policy.IsPolicyEnabled() {
		return &PolicyDecision{
			Allowed: false,
			Reason:  "policy for " + req.Token.ErrorCode + " is disabled",
		}
	}

	// Check if exceptions are allowed for this error code
	if !policy.IsExceptionAllowed() {
		return &PolicyDecision{
			Allowed: false,
			Reason:  "exceptions not allowed for " + req.Token.ErrorCode,
		}
	}

	// Validate reason if required
	if policy.IsReasonRequired() {
		decision := m.validateReason(policy, req.Token.Reason)
		if !decision.Allowed {
			return decision
		}
	}

	return &PolicyDecision{
		Allowed:        true,
		Reason:         "policy allows exception for " + req.Token.ErrorCode,
		RequiredReason: policy.IsReasonRequired(),
		ProvidedReason: req.Token.Reason,
	}
}

// getPolicy returns the policy for an error code, or the default policy.
func (m *PolicyMatcher) getPolicy(errorCode string) *config.ExceptionPolicyConfig {
	if m.config == nil {
		return DefaultPolicy
	}

	policy := m.config.GetPolicy(errorCode)
	if policy == nil {
		return DefaultPolicy
	}

	return policy
}

// validateReason validates the provided reason against policy requirements.
func (m *PolicyMatcher) validateReason(
	policy *config.ExceptionPolicyConfig,
	reason string,
) *PolicyDecision {
	reason = strings.TrimSpace(reason)

	// Check if reason is provided
	if reason == "" {
		return &PolicyDecision{
			Allowed:        false,
			Reason:         "reason is required but not provided",
			RequiredReason: true,
		}
	}

	// Check minimum length
	minLength := policy.GetMinReasonLength()
	if len(reason) < minLength {
		return &PolicyDecision{
			Allowed:        false,
			Reason:         "reason too short (minimum " + strconv.Itoa(minLength) + " characters)",
			RequiredReason: true,
			ProvidedReason: reason,
		}
	}

	// Check against valid reasons if specified
	if len(policy.ValidReasons) > 0 {
		if !m.isValidReason(policy.ValidReasons, reason) {
			return &PolicyDecision{
				Allowed:        false,
				Reason:         "reason not in approved list",
				RequiredReason: true,
				ProvidedReason: reason,
			}
		}
	}

	return &PolicyDecision{
		Allowed:        true,
		RequiredReason: true,
		ProvidedReason: reason,
	}
}

// isValidReason checks if the reason matches any of the valid reasons.
// Comparison is case-insensitive and supports prefix matching.
func (*PolicyMatcher) isValidReason(validReasons []string, reason string) bool {
	reasonLower := strings.ToLower(strings.TrimSpace(reason))

	return slices.ContainsFunc(validReasons, func(valid string) bool {
		validLower := strings.ToLower(strings.TrimSpace(valid))

		// Exact match (case-insensitive)
		if reasonLower == validLower {
			return true
		}

		// Prefix match for approved reasons
		if strings.HasPrefix(reasonLower, validLower) {
			return true
		}

		return false
	})
}

// GetPolicyLimits returns the rate limits for a specific error code.
// Returns (maxPerHour, maxPerDay) where 0 means unlimited.
func (m *PolicyMatcher) GetPolicyLimits(errorCode string) (maxPerHour, maxPerDay int) {
	policy := m.getPolicy(errorCode)

	return policy.GetMaxPerHour(), policy.GetMaxPerDay()
}

// HasExplicitPolicy returns true if there is an explicit policy for the error code.
func (m *PolicyMatcher) HasExplicitPolicy(errorCode string) bool {
	if m.config == nil {
		return false
	}

	return m.config.GetPolicy(errorCode) != nil
}
