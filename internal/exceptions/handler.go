// Package exceptions provides the exception workflow system for klaudiush.
package exceptions

import (
	"os"
	"strconv"
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// Handler coordinates exception evaluation, rate limiting, and audit logging.
// It provides a single entry point for the dispatcher to check if a validation
// error should be bypassed due to an exception token.
type Handler struct {
	engine      *Engine
	rateLimiter *RateLimiter
	auditLogger *AuditLogger
	config      *config.ExceptionsConfig
	logger      logger.Logger
}

// HandlerOption configures the Handler.
type HandlerOption func(*Handler)

// WithHandlerLogger sets the logger for the handler.
func WithHandlerLogger(log logger.Logger) HandlerOption {
	return func(h *Handler) {
		if log != nil {
			h.logger = log
		}
	}
}

// WithEngine sets a custom engine.
func WithEngine(e *Engine) HandlerOption {
	return func(h *Handler) {
		if e != nil {
			h.engine = e
		}
	}
}

// WithRateLimiter sets a custom rate limiter.
func WithRateLimiter(r *RateLimiter) HandlerOption {
	return func(h *Handler) {
		if r != nil {
			h.rateLimiter = r
		}
	}
}

// WithAuditLogger sets a custom audit logger.
func WithAuditLogger(a *AuditLogger) HandlerOption {
	return func(h *Handler) {
		if a != nil {
			h.auditLogger = a
		}
	}
}

// NewHandler creates a new exception handler.
func NewHandler(cfg *config.ExceptionsConfig, opts ...HandlerOption) *Handler {
	log := logger.NewNoOpLogger()

	h := &Handler{
		config: cfg,
		logger: log,
	}

	// Apply options first to allow custom logger to be set
	for _, opt := range opts {
		opt(h)
	}

	// Initialize components with the handler's logger if not already set
	if h.engine == nil {
		h.engine = NewEngine(cfg, WithLogger(h.logger))
	}

	if h.rateLimiter == nil {
		var rateCfg *config.ExceptionRateLimitConfig
		if cfg != nil {
			rateCfg = cfg.RateLimit
		}

		h.rateLimiter = NewRateLimiter(rateCfg, cfg, WithRateLimiterLogger(h.logger))
	}

	if h.auditLogger == nil {
		var auditCfg *config.ExceptionAuditConfig
		if cfg != nil {
			auditCfg = cfg.Audit
		}

		h.auditLogger = NewAuditLogger(auditCfg, WithAuditLoggerLogger(h.logger))
	}

	return h
}

// CheckRequest represents a request to check for exception bypass.
type CheckRequest struct {
	// HookContext is the hook context being validated.
	HookContext *hook.Context

	// ValidatorName is the name of the validator that failed.
	ValidatorName string

	// ErrorCode is the validator error code (e.g., "GIT022", "SEC001").
	ErrorCode string

	// ErrorMessage is the original validation error message.
	ErrorMessage string
}

// CheckResponse represents the result of checking for exception bypass.
type CheckResponse struct {
	// Bypassed indicates whether the validation was bypassed.
	Bypassed bool

	// Reason explains why the exception was allowed or denied.
	Reason string

	// ErrorCode is the error code from the exception token.
	ErrorCode string

	// TokenReason is the justification reason provided in the token.
	TokenReason string

	// RateLimitInfo contains rate limit quota information.
	RateLimitInfo *CheckResult
}

// Check evaluates whether a validation error should be bypassed due to an
// exception token in the command.
func (h *Handler) Check(req *CheckRequest) *CheckResponse {
	// Validate request
	if resp := h.validateRequest(req); resp != nil {
		return resp
	}

	// Get command from hook context
	command := h.getCommand(req.HookContext)
	if command == "" {
		return &CheckResponse{Bypassed: false, Reason: "no command to parse"}
	}

	// Evaluate exception token
	evalResult := h.evaluateToken(req, command)

	// Check policy evaluation result
	if !evalResult.Allowed {
		return h.handlePolicyDenial(req, evalResult)
	}

	// Check rate limits
	rateLimitResult := h.rateLimiter.Check(evalResult.AuditEntry.ErrorCode)
	if !rateLimitResult.Allowed {
		return h.handleRateLimitDenial(evalResult, rateLimitResult)
	}

	// Record and log successful exception
	return h.handleAllowedExeption(req, evalResult, rateLimitResult)
}

// validateRequest validates the check request and returns an error response if invalid.
func (h *Handler) validateRequest(req *CheckRequest) *CheckResponse {
	if req == nil {
		return &CheckResponse{Bypassed: false, Reason: "no request provided"}
	}

	if !h.IsEnabled() {
		h.logger.Debug("exception system disabled")

		return &CheckResponse{Bypassed: false, Reason: "exception system is disabled"}
	}

	return nil
}

// evaluateToken evaluates the exception token in the command.
func (h *Handler) evaluateToken(req *CheckRequest, command string) *ExceptionResult {
	return h.engine.Evaluate(&EvaluateRequest{
		Command:       command,
		ValidatorName: req.ValidatorName,
		ErrorCode:     req.ErrorCode,
		WorkingDir:    h.getWorkingDir(),
		Repository:    h.getRepository(req.HookContext),
	})
}

// handlePolicyDenial handles when the policy denies the exception.
func (h *Handler) handlePolicyDenial(
	req *CheckRequest,
	evalResult *ExceptionResult,
) *CheckResponse {
	h.logger.Debug("exception not allowed by policy",
		"error_code", req.ErrorCode,
		"reason", evalResult.Reason,
	)

	h.logAuditEntry(evalResult.AuditEntry, "denied exception")

	return &CheckResponse{
		Bypassed:  false,
		Reason:    evalResult.Reason,
		ErrorCode: req.ErrorCode,
	}
}

// handleRateLimitDenial handles when rate limit denies the exception.
func (h *Handler) handleRateLimitDenial(
	evalResult *ExceptionResult,
	rateLimitResult *CheckResult,
) *CheckResponse {
	h.logger.Debug("exception denied by rate limit",
		"error_code", evalResult.AuditEntry.ErrorCode,
		"reason", rateLimitResult.Reason,
	)

	if evalResult.AuditEntry != nil {
		evalResult.AuditEntry.Allowed = false
		evalResult.AuditEntry.DenialReason = rateLimitResult.Reason
		h.logAuditEntry(evalResult.AuditEntry, "rate-limited exception")
	}

	return &CheckResponse{
		Bypassed:      false,
		Reason:        rateLimitResult.Reason,
		ErrorCode:     evalResult.AuditEntry.ErrorCode,
		RateLimitInfo: rateLimitResult,
	}
}

// handleAllowedExeption handles a successful exception bypass.
func (h *Handler) handleAllowedExeption(
	req *CheckRequest,
	evalResult *ExceptionResult,
	rateLimitResult *CheckResult,
) *CheckResponse {
	if err := h.rateLimiter.Record(evalResult.AuditEntry.ErrorCode); err != nil {
		h.logger.Error("failed to record exception usage", "error", err.Error())
	}

	h.logAuditEntry(evalResult.AuditEntry, "exception")

	h.logger.Info("exception allowed",
		"error_code", evalResult.AuditEntry.ErrorCode,
		"validator", req.ValidatorName,
		"reason", evalResult.AuditEntry.Reason,
	)

	return &CheckResponse{
		Bypassed:      true,
		Reason:        "exception allowed",
		ErrorCode:     evalResult.AuditEntry.ErrorCode,
		TokenReason:   evalResult.AuditEntry.Reason,
		RateLimitInfo: rateLimitResult,
	}
}

// logAuditEntry logs an audit entry with error handling.
func (h *Handler) logAuditEntry(entry *AuditEntry, context string) {
	if entry == nil {
		return
	}

	if err := h.auditLogger.Log(entry); err != nil {
		h.logger.Error("failed to log "+context+" to audit", "error", err.Error())
	}
}

// IsEnabled returns whether the exception system is enabled.
func (h *Handler) IsEnabled() bool {
	if h.config == nil {
		return true
	}

	return h.config.IsEnabled()
}

// LoadState loads persisted rate limit state.
func (h *Handler) LoadState() error {
	return h.rateLimiter.Load()
}

// SaveState persists rate limit state.
func (h *Handler) SaveState() error {
	return h.rateLimiter.Save()
}

// GetAuditStats returns audit log statistics.
func (h *Handler) GetAuditStats() (*AuditStats, error) {
	return h.auditLogger.Stats()
}

// GetRateLimitState returns the current rate limit state.
func (h *Handler) GetRateLimitState() RateLimitState {
	return h.rateLimiter.GetState()
}

// CleanupAudit performs audit log cleanup.
func (h *Handler) CleanupAudit() error {
	return h.auditLogger.Cleanup()
}

// getCommand extracts the command from hook context.
func (*Handler) getCommand(ctx *hook.Context) string {
	if ctx == nil {
		return ""
	}

	return ctx.GetCommand()
}

// getWorkingDir returns the current working directory.
func (*Handler) getWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return wd
}

// getRepository attempts to get the repository path from hook context.
func (*Handler) getRepository(ctx *hook.Context) string {
	if ctx == nil {
		return ""
	}

	// For now, use working directory as repository
	// In future, could extract from git remote URL
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return wd
}

// FormatBypassMessage formats a message explaining the exception bypass.
func FormatBypassMessage(resp *CheckResponse) string {
	if resp == nil || !resp.Bypassed {
		return ""
	}

	var builder strings.Builder

	builder.WriteString("✅ Exception bypass allowed for ")
	builder.WriteString(resp.ErrorCode)

	if resp.TokenReason != "" {
		builder.WriteString("\n   Reason: ")
		builder.WriteString(resp.TokenReason)
	}

	if resp.RateLimitInfo != nil {
		builder.WriteString("\n   ")
		builder.WriteString(formatRemainingQuota(resp.RateLimitInfo))
	}

	return builder.String()
}

// FormatDenialMessage formats a message explaining why exception was denied.
func FormatDenialMessage(resp *CheckResponse) string {
	if resp == nil || resp.Bypassed {
		return ""
	}

	var builder strings.Builder

	builder.WriteString("❌ Exception denied")

	if resp.ErrorCode != "" {
		builder.WriteString(" for ")
		builder.WriteString(resp.ErrorCode)
	}

	builder.WriteString(": ")
	builder.WriteString(resp.Reason)

	return builder.String()
}

// formatRemainingQuota formats the remaining rate limit quota.
func formatRemainingQuota(info *CheckResult) string {
	if info == nil {
		return ""
	}

	var parts []string

	if info.GlobalHourlyRemaining >= 0 {
		parts = append(parts, formatQuotaPart("hourly", info.GlobalHourlyRemaining))
	}

	if info.GlobalDailyRemaining >= 0 {
		parts = append(parts, formatQuotaPart("daily", info.GlobalDailyRemaining))
	}

	if len(parts) == 0 {
		return "Quota: unlimited"
	}

	return "Remaining: " + strings.Join(parts, ", ")
}

// formatQuotaPart formats a single quota part.
func formatQuotaPart(period string, remaining int) string {
	return period + "=" + formatInt(remaining)
}

// formatInt formats an integer as a string.
func formatInt(n int) string {
	if n < 0 {
		return "unlimited"
	}

	return strconv.Itoa(n)
}
