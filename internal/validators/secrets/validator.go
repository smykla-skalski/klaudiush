package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// SecretsValidator validates file content for secrets and sensitive data.
type SecretsValidator struct {
	validator.BaseValidator
	detector        Detector
	gitleaks        linters.GitleaksChecker
	config          *config.SecretsValidatorConfig
	allowListRegex  []*regexp.Regexp
	disabledPatters map[string]bool
}

// NewSecretsValidator creates a new SecretsValidator.
func NewSecretsValidator(
	log logger.Logger,
	detector Detector,
	gitleaks linters.GitleaksChecker,
	cfg *config.SecretsValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *SecretsValidator {
	v := &SecretsValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			"validate-secrets", log, ruleAdapter,
		),
		detector:        detector,
		gitleaks:        gitleaks,
		config:          cfg,
		disabledPatters: make(map[string]bool),
	}

	// Compile allow list patterns
	if cfg != nil {
		v.allowListRegex = compileAllowList(cfg.AllowList, log)

		for _, name := range cfg.DisabledPatterns {
			v.disabledPatters[name] = true
		}
	}

	return v
}

// compileAllowList compiles allow list patterns into regular expressions.
func compileAllowList(patterns []string, log logger.Logger) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Error("invalid allow list pattern", "pattern", pattern, "error", err)

			continue
		}

		compiled = append(compiled, re)
	}

	return compiled
}

// Validate checks file content for secrets.
func (v *SecretsValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("validating content for secrets")

	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	// Get content to validate
	content, wholeFile := v.getContent(hookCtx)
	if content == "" {
		log.Debug("no content to validate")
		return inspectedIf(wholeFile, validator.Pass())
	}

	// Check file size limit
	maxSize := v.getMaxFileSize()
	if int64(len(content)) > int64(maxSize) {
		log.Debug("content exceeds max file size", "size", len(content), "max", maxSize)
		return validator.Pass()
	}

	// Run pattern detection
	findings := v.detector.Detect(content)

	// Filter findings
	findings = v.filterFindings(findings)

	if len(findings) > 0 {
		return inspectedIf(wholeFile, v.createResult(findings))
	}

	// Optionally run gitleaks as second-tier check
	if v.shouldUseGitleaks() {
		result := v.gitleaks.Check(ctx, content)
		if !result.Success && len(result.Findings) > 0 {
			return inspectedIf(wholeFile, v.createGitleaksResult(result.Findings))
		}

		if !result.Success {
			return gitleaksUnavailable(ctx, result.Err)
		}
	}

	log.Debug("no secrets detected")

	return inspectedIf(wholeFile, validator.Pass())
}

// gitleaksUnavailable reports a gitleaks run that failed without findings:
// it checked nothing, so the content is not known to be clean.
func gitleaksUnavailable(ctx context.Context, err error) *validator.Result {
	reason := validator.ReasonFromContext(ctx)
	if reason == "" {
		reason = validator.ReasonError
	}

	return validator.Unavailable(
		reason,
		fmt.Sprintf("gitleaks failed without reporting a finding: %v", err),
	)
}

// inspectedIf marks result as a check of the whole file as the tool left it
// when inspected holds.
func inspectedIf(inspected bool, result *validator.Result) *validator.Result {
	if inspected {
		return result.MarkInspected()
	}

	return result
}

// getContent extracts content to validate from the hook context. After the
// tool ran it is the whole file as the tool left it, read against the hook's
// working directory, and the second return value reports that it was read.
func (*SecretsValidator) getContent(hookCtx *hook.Context) (string, bool) {
	if hookCtx.IsAfterTool() && hookCtx.GetFilePath() != "" {
		path := hook.CanonicalFilePath(hookCtx.WorkingDir, hookCtx.GetFilePath())

		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return "", false
		}

		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return "", false
		}

		return string(data), true
	}

	// For Write operations, use the content directly
	if hookCtx.ToolName == hook.ToolTypeWrite {
		return hookCtx.GetContent(), false
	}

	// For Edit operations, validate the new content being written
	if hookCtx.ToolName == hook.ToolTypeEdit {
		return hookCtx.ToolInput.NewString, false
	}

	return "", false
}

// getMaxFileSize returns the configured max file size.
func (v *SecretsValidator) getMaxFileSize() config.ByteSize {
	if v.config != nil {
		return v.config.GetMaxFileSize()
	}

	return config.DefaultMaxFileSize
}

// shouldUseGitleaks returns whether gitleaks should be used.
func (v *SecretsValidator) shouldUseGitleaks() bool {
	if v.gitleaks == nil {
		return false
	}

	if v.config != nil && !v.config.IsUseGitleaksEnabled() {
		return false
	}

	return v.gitleaks.IsAvailable()
}

// filterFindings removes findings that match the allow list or are from disabled patterns.
func (v *SecretsValidator) filterFindings(findings []Finding) []Finding {
	filtered := make([]Finding, 0, len(findings))

	for _, finding := range findings {
		// Skip disabled patterns
		if v.disabledPatters[finding.Pattern.Name] {
			v.Logger().Debug("skipping disabled pattern", "pattern", finding.Pattern.Name)

			continue
		}

		// Skip if matches allow list
		if v.matchesAllowList(finding.Match) {
			v.Logger().Debug("skipping allowed match", "match", finding.Match)

			continue
		}

		filtered = append(filtered, finding)
	}

	return filtered
}

// matchesAllowList checks if a match should be ignored based on allow list.
func (v *SecretsValidator) matchesAllowList(match string) bool {
	for _, re := range v.allowListRegex {
		if re.MatchString(match) {
			return true
		}
	}

	return false
}

// createResult creates a validation result from findings.
func (v *SecretsValidator) createResult(findings []Finding) *validator.Result {
	// Group findings by type for better output
	messages := make([]string, 0, len(findings))
	structured := make([]validator.Finding, 0, len(findings))

	for _, finding := range findings {
		msg := fmt.Sprintf(
			"Line %d: %s (%s)",
			finding.Line,
			finding.Pattern.Description,
			finding.Pattern.Name,
		)
		messages = append(messages, msg)

		// The matched value is never copied into the finding: diagnostics
		// reach the agent, the user and logs.
		structured = append(structured, validator.Finding{
			Reference: finding.Pattern.Reference,
			Location:  fmt.Sprintf("line %d", finding.Line),
			Message:   finding.Pattern.Description + " (" + finding.Pattern.Name + ")",
			Required:  "no secret values in content",
			Repair:    validator.GetSuggestion(finding.Pattern.Reference),
		})
	}

	ref := findings[0].Pattern.Reference
	message := fmt.Sprintf(
		"Potential secrets detected (%d finding(s)):\n%s",
		len(findings),
		strings.Join(messages, "\n"),
	)

	structured = validator.SortFindings(structured, nil)

	if v.shouldBlock() {
		return validator.FailWithRef(ref, message).AddFinding(structured...)
	}

	return validator.WarnWithRef(ref, message).AddFinding(structured...)
}

// createGitleaksResult creates a validation result from gitleaks findings.
func (v *SecretsValidator) createGitleaksResult(findings []linters.LintFinding) *validator.Result {
	messages := make([]string, 0, len(findings))

	structured := make([]validator.Finding, 0, len(findings))

	for _, finding := range findings {
		msg := fmt.Sprintf("Line %d: %s", finding.Line, finding.Message)
		messages = append(messages, msg)

		structured = append(structured, validator.Finding{
			Reference: validator.RefSecretsToken,
			Location:  fmt.Sprintf("line %d", finding.Line),
			Message:   finding.Message,
			Required:  "no secret values in content",
			Repair:    validator.GetSuggestion(validator.RefSecretsToken),
		})
	}

	message := fmt.Sprintf(
		"Gitleaks detected secrets (%d finding(s)):\n%s",
		len(findings),
		strings.Join(messages, "\n"),
	)

	structured = validator.SortFindings(structured, nil)

	if v.shouldBlock() {
		return validator.FailWithRef(validator.RefSecretsToken, message).
			AddFinding(structured...)
	}

	return validator.WarnWithRef(validator.RefSecretsToken, message).AddFinding(structured...)
}

// shouldBlock returns whether detection should block the operation.
func (v *SecretsValidator) shouldBlock() bool {
	if v.config == nil {
		return true // default to blocking
	}

	return v.config.IsBlockOnDetectionEnabled()
}

// Category returns the validator category for parallel execution.
// SecretsValidator uses CategoryCPU as it primarily does regex matching.
func (*SecretsValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}
