package file

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// violation represents a line matching one of a validator's patterns.
type violation struct {
	line      int
	directive string
}

// compilePatterns compiles the given regex patterns, logging and skipping any that fail.
func compilePatterns(log logger.Logger, label string, patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			log.Error("failed to compile "+label+" pattern", "pattern", pattern, "error", err)
			continue
		}

		compiled = append(compiled, re)
	}

	if len(compiled) == 0 && len(patterns) > 0 {
		log.Error("all "+label+" patterns failed to compile", "total", len(patterns))
	}

	return compiled
}

// findPatternViolations returns each line matching any pattern, one match per line.
func findPatternViolations(content string, patterns []*regexp.Regexp) []violation {
	var violations []violation

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		for _, pattern := range patterns {
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

// formatPatternViolations renders violations under the given message header.
func formatPatternViolations(header string, violations []violation) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%s\n\n", header)

	for i, v := range violations {
		if i > 0 {
			fmt.Fprint(&sb, "\n")
		}

		fmt.Fprintf(&sb, "Line %d: %s", v.line, v.directive)
	}

	return sb.String()
}

// getWriteOrEditContent extracts the new content from a Write or Edit hook
// context. After the tool ran there is nothing new to check: pre-tool
// validation saw the same input, and the file on disk also holds what was
// there before.
func getWriteOrEditContent(hookCtx *hook.Context) string {
	if hookCtx.IsAfterTool() {
		return ""
	}

	if hookCtx.ToolInput.Content != "" {
		return hookCtx.ToolInput.Content
	}

	if hookCtx.ToolName == hook.ToolTypeEdit {
		return hookCtx.ToolInput.NewString
	}

	return ""
}

// validatePatterns runs the shared rule-check → extract → match → report flow
// used by pattern-based file validators.
func validatePatterns(
	ctx context.Context,
	hookCtx *hook.Context,
	checkRules func(context.Context, *hook.Context) *validator.Result,
	patterns []*regexp.Regexp,
	header string,
	ref validator.Reference,
) *validator.Result {
	if result := checkRules(ctx, hookCtx); result != nil {
		return result
	}

	content := getWriteOrEditContent(hookCtx)
	if content == "" {
		return validator.Pass()
	}

	cov := fileCoverage(hookCtx, false)

	violations := findPatternViolations(content, patterns)
	if len(violations) == 0 {
		return cov.mark(validator.Pass())
	}

	return cov.mark(validator.FailWithRef(ref, formatPatternViolations(header, violations)))
}
