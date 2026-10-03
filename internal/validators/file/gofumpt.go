package file

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const (
	defaultGofumptTimeout = 10 * time.Second
)

var (
	goModModulePattern  = regexp.MustCompile(`^module\s+(\S+)`)
	goModVersionPattern = regexp.MustCompile(`^go\s+(\d+\.\d+(?:\.\d+)?)`)
)

// GofumptValidator validates Go code formatting using gofumpt
type GofumptValidator struct {
	validator.BaseValidator
	checker linters.GofumptChecker
	config  *config.GofumptValidatorConfig
}

// NewGofumptValidator creates a new GofumptValidator
func NewGofumptValidator(
	log logger.Logger,
	checker linters.GofumptChecker,
	cfg *config.GofumptValidatorConfig,
	ruleAdapter validator.RuleChecker,
) *GofumptValidator {
	return &GofumptValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules("validate-gofumpt", log, ruleAdapter),
		checker:       checker,
		config:        cfg,
	}
}

// Validate validates Go code formatting using gofumpt
func (v *GofumptValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating Go code formatting")

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	// Get the file path
	filePath := hookCtx.GetFilePath()
	if filePath == "" {
		log.Debug("no file path provided")
		return validator.Pass()
	}

	content, baseline, err := v.getContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	if content == "" {
		log.Debug("empty content, skipping validation")
		return validator.Pass()
	}

	opts := v.buildGofumptOptions(hook.CanonicalFilePath(hookCtx.WorkingDir, filePath))

	result, notRun := v.check(ctx, content, opts)
	if notRun != nil {
		return notRun
	}

	inspected := hookCtx.IsAfterTool() && (result.Success || isUnformatted(result))

	if result.Success {
		log.Debug("gofumpt passed")
		return inspectedIf(inspected, validator.Pass())
	}

	log.Debug("gofumpt failed", "output", result.RawOut)

	message := v.formatGofumptOutput(result.RawOut)

	if baseline != nil && v.baselineUnformatted(ctx, *baseline, opts) {
		log.Debug("file was not gofumpt-formatted before the edit")

		return inspectedIf(inspected, validator.WarnWithRef(
			validator.RefGofumpt,
			message+"\n\nThe file was not gofumpt-formatted before this edit either",
		))
	}

	return inspectedIf(inspected, validator.FailWithRef(validator.RefGofumpt, message))
}

// isUnformatted tells formatting differences apart from a failed run, such
// as a timeout, which reports no diff.
func isUnformatted(result *linters.LintResult) bool {
	return !result.Success && len(result.Findings) > 0
}

// check runs gofumpt on content. The second return value is set when
// gofumpt could not run, in which case the first must not be used.
func (v *GofumptValidator) check(
	ctx context.Context,
	content string,
	opts *linters.GofumptOptions,
) (*linters.LintResult, *validator.Result) {
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	result := v.checker.CheckWithOptions(lintCtx, content, opts)

	return result, lintUnavailable(lintCtx, "gofumpt", result)
}

// baselineUnformatted reports whether the file was already unformatted
// before the edit. A baseline gofumpt could not check counts as formatted,
// so the edit keeps its blocking finding.
func (v *GofumptValidator) baselineUnformatted(
	ctx context.Context,
	baseline string,
	opts *linters.GofumptOptions,
) bool {
	result, notRun := v.check(ctx, baseline, opts)

	return notRun == nil && isUnformatted(result)
}

// getContent returns the Go source to check. For an Edit or MultiEdit before
// it runs, that is the whole file with the edit applied, and baseline is the
// file as it is now, so formatting the edit did not break can be told apart.
// After the tool ran, it is the file on disk, with the edit reverted as the
// baseline when that is possible.
func (v *GofumptValidator) getContent(
	ctx *hook.Context,
	filePath string,
) (string, *string, error) {
	log := v.Logger()

	if content, ok, err := readToolResult(ctx, filePath); ok {
		if err != nil {
			return "", nil, err
		}

		if original, found := originalBeforeEdit(ctx, content); found && original != "" {
			return content, &original, nil
		}

		return content, nil, nil
	}

	if ctx.ToolName == hook.ToolTypeEdit || ctx.ToolName == hook.ToolTypeMultiEdit {
		return v.getEditContent(ctx, filePath)
	}

	if content := ctx.ToolInput.Content; content != "" {
		return content, nil, nil
	}

	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		log.Debug("failed to read file", "file", filePath, "error", err)
		return "", nil, errors.Wrap(err, "reading file")
	}

	return string(data), nil, nil
}

func (v *GofumptValidator) getEditContent(
	ctx *hook.Context,
	filePath string,
) (string, *string, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil && !os.IsNotExist(err) {
		return "", nil, errors.Wrap(err, "reading file for edit")
	}

	original := string(data)

	proposed, ok := proposedEditContent(ctx, original)
	if !ok {
		v.Logger().Debug("edit does not apply to the file, skipping", "file", filePath)
		return "", nil, nil
	}

	if original == "" {
		return proposed, nil, nil
	}

	return proposed, &original, nil
}

// buildGofumptOptions creates GofumptOptions with auto-detection from go.mod
func (v *GofumptValidator) buildGofumptOptions(filePath string) *linters.GofumptOptions {
	opts := &linters.GofumptOptions{}

	// Get extra rules flag from config
	if v.config != nil && v.config.ExtraRules != nil {
		opts.ExtraRules = *v.config.ExtraRules
	}

	// Get lang and modpath from config or auto-detect
	lang := ""
	modpath := ""

	if v.config != nil {
		lang = v.config.Lang
		modpath = v.config.ModPath
	}

	// Auto-detect from go.mod if not configured
	if lang == "" || modpath == "" {
		detectedLang, detectedModPath := v.autoDetectGoModSettings(filePath)

		if lang == "" && detectedLang != "" {
			lang = detectedLang
		}

		if modpath == "" && detectedModPath != "" {
			modpath = detectedModPath
		}
	}

	opts.Lang = lang
	opts.ModPath = modpath

	return opts
}

// autoDetectGoModSettings attempts to auto-detect Go version and module path from go.mod
func (v *GofumptValidator) autoDetectGoModSettings(filePath string) (lang, modpath string) {
	goModPath := v.findGoMod(filePath)
	if goModPath == "" {
		return "", ""
	}

	detectedLang, detectedModPath, err := v.parseGoMod(goModPath)
	if err != nil {
		return "", ""
	}

	if detectedLang != "" {
		v.Logger().Debug("auto-detected Go version", "lang", detectedLang, "go_mod", goModPath)
	}

	if detectedModPath != "" {
		v.Logger().
			Debug("auto-detected module path", "modpath", detectedModPath, "go_mod", goModPath)
	}

	return detectedLang, detectedModPath
}

// findGoMod walks up the directory tree to find go.mod
func (*GofumptValidator) findGoMod(startPath string) string {
	dir := filepath.Dir(startPath)

	// Walk up to 10 levels max
	for range 10 {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return goModPath
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}

		dir = parent
	}

	return ""
}

// parseGoMod extracts Go version and module path from go.mod
func (*GofumptValidator) parseGoMod(goModPath string) (lang, modpath string, err error) {
	data, err := os.ReadFile(goModPath) //nolint:gosec // goModPath is from findGoMod
	if err != nil {
		return "", "", err
	}

	content := string(data)

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)

		// Parse module directive
		if modpath == "" {
			if matches := goModModulePattern.FindStringSubmatch(line); len(matches) > 1 {
				modpath = matches[1]
			}
		}

		// Parse go directive
		if lang == "" {
			if matches := goModVersionPattern.FindStringSubmatch(line); len(matches) > 1 {
				// Convert "1.21" to "go1.21"
				lang = "go" + matches[1]
			}
		}

		// Stop if we found both
		if modpath != "" && lang != "" {
			break
		}
	}

	return lang, modpath, nil
}

// formatGofumptOutput formats gofumpt output for display
func (*GofumptValidator) formatGofumptOutput(output string) string {
	// Clean up the output - remove empty lines
	lines := strings.Split(output, "\n")

	var cleanLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	if len(cleanLines) == 0 {
		return "Go code formatting issues detected"
	}

	return "Go code formatting issues detected\n\n" + strings.Join(
		cleanLines,
		"\n",
	)
}

// getTimeout returns the configured timeout for gofumpt operations
func (v *GofumptValidator) getTimeout() time.Duration {
	if v.config != nil && v.config.Timeout.ToDuration() > 0 {
		return v.config.Timeout.ToDuration()
	}

	return defaultGofumptTimeout
}

// Category returns the validator category for parallel execution.
// GofumptValidator uses CategoryIO because it invokes gofumpt.
func (*GofumptValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
