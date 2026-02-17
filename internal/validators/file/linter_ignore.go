package file

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// defaultIgnorePatterns are common linter ignore directives to block.
// These patterns are compiled into regexes that match ignore directives.
var defaultIgnorePatterns = []string{
	// Python
	`#\s*noqa`,              // # noqa, # noqa: E501
	`#\s*type:\s*ignore`,    // # type: ignore
	`#\s*pylint:\s*disable`, // # pylint: disable=...
	`#\s*pyright:\s*ignore`, // # pyright: ignore
	`#\s*mypy:\s*ignore`,    // mypy suppress directive
	`#\s*pyrefly:\s*ignore`, // pyrefly suppress directive

	// JavaScript/TypeScript
	`//\s*eslint-disable`,   // // eslint-disable
	`//\s*@ts-ignore`,       // // @ts-ignore
	`//\s*@ts-nocheck`,      // // @ts-nocheck
	`//\s*@ts-expect-error`, // // @ts-expect-error
	`/\*\s*eslint-disable`,  // /* eslint-disable */

	// Go
	`//nolint`,    // //nolint, //nolint:errcheck
	`//\s*nolint`, // // nolint

	// Rust
	`#\[allow\(`,  // #[allow(dead_code)]
	`#!\[allow\(`, // #![allow(missing_docs)]

	// Ruby
	`#\s*rubocop:\s*disable`, // # rubocop:disable

	// Shell
	`#\s*shellcheck\s+disable`, // # shellcheck disable=SC2086

	// Java
	`@SuppressWarnings`, // @SuppressWarnings("unchecked")

	// C#
	`#pragma\s+warning\s+disable`, // #pragma warning disable CS0618

	// PHP
	`//\s*phpcs:ignore`,    // // phpcs:ignore
	`//\s*@phpstan-ignore`, // // @phpstan-ignore

	// Swift
	`//\s*swiftlint:disable`, // // swiftlint:disable
}

// LinterIgnoreValidator validates that code does not contain linter ignore directives.
type LinterIgnoreValidator struct {
	validator.BaseValidator
	config      *config.LinterIgnoreValidatorConfig
	patterns    []*regexp.Regexp
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewLinterIgnoreValidator creates a new LinterIgnoreValidator.
func NewLinterIgnoreValidator(
	log logger.Logger,
	cfg *config.LinterIgnoreValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *LinterIgnoreValidator {
	v := &LinterIgnoreValidator{
		BaseValidator: *validator.NewBaseValidator("validate-linter-ignore", log),
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}

	// Compile patterns
	patterns := v.getPatterns()
	v.patterns = make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Error("failed to compile linter ignore pattern", "pattern", pattern, "error", err)
			continue
		}

		v.patterns = append(v.patterns, re)
	}

	// Log error when zero patterns compiled (will pass all content)
	if len(v.patterns) == 0 && len(patterns) > 0 {
		log.Error("all linter ignore patterns failed to compile", "total", len(patterns))
	}

	return v
}

// Validate checks for linter ignore directives in file content.
func (v *LinterIgnoreValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating for linter ignore directives")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Get content from context
	content := v.getContent(hookCtx)

	if content == "" {
		log.Debug("no content to validate")
		return validator.Pass()
	}

	// Check for ignore directives
	violations := v.findViolations(content)
	if len(violations) == 0 {
		log.Debug("no linter ignore directives found")
		return validator.Pass()
	}

	// Build error message
	message := v.formatViolations(violations)

	return validator.FailWithRef(validator.RefLinterIgnore, message)
}

// getContent extracts content from hook context.
func (*LinterIgnoreValidator) getContent(hookCtx *hook.Context) string {
	// For Write operations, get content directly
	if hookCtx.ToolInput.Content != "" {
		return hookCtx.ToolInput.Content
	}

	// For Edit operations, check new string
	if hookCtx.ToolName == hook.ToolTypeEdit {
		newStr := hookCtx.ToolInput.NewString

		// We need to check the new content being added
		if newStr != "" {
			return newStr
		}

		// If new string is empty, no content to validate
		return ""
	}

	return ""
}

// findViolations searches for linter ignore directive patterns in content.
func (v *LinterIgnoreValidator) findViolations(content string) []violation {
	var violations []violation

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		for _, pattern := range v.patterns {
			if match := pattern.FindString(line); match != "" {
				violations = append(violations, violation{
					line:      lineNum + 1,
					directive: strings.TrimSpace(match),
				})

				break // Only report first match per line
			}
		}
	}

	return violations
}

// formatViolations formats violation findings into error message.
func (*LinterIgnoreValidator) formatViolations(violations []violation) string {
	var sb strings.Builder

	sb.WriteString("Linter ignore directives are not allowed\n\n")

	for i, v := range violations {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("Line %d: %s", v.line, v.directive))
	}

	return sb.String()
}

// getPatterns returns the configured patterns or defaults.
func (v *LinterIgnoreValidator) getPatterns() []string {
	if v.config != nil && len(v.config.Patterns) > 0 {
		return v.config.Patterns
	}

	return defaultIgnorePatterns
}

// Category returns the validator category for parallel execution.
// LinterIgnoreValidator uses CategoryCPU because it only does pattern matching.
func (*LinterIgnoreValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

// violation represents a detected linter ignore directive.
type violation struct {
	line      int
	directive string
}
