package file

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/smykla-labs/klaudiush/internal/linters"
	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
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
	checker     linters.GofumptChecker
	config      *config.GofumptValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewGofumptValidator creates a new GofumptValidator
func NewGofumptValidator(
	log logger.Logger,
	checker linters.GofumptChecker,
	cfg *config.GofumptValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *GofumptValidator {
	return &GofumptValidator{
		BaseValidator: *validator.NewBaseValidator("validate-gofumpt", log),
		checker:       checker,
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate validates Go code formatting using gofumpt
func (v *GofumptValidator) Validate(
	ctx context.Context,
	hookCtx *hook.Context,
) *validator.Result {
	log := v.Logger()
	log.Debug("validating Go code formatting")

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Get the file path
	filePath := hookCtx.GetFilePath()
	if filePath == "" {
		log.Debug("no file path provided")
		return validator.Pass()
	}

	// Get content based on operation type
	content, err := v.getContent(hookCtx, filePath)
	if err != nil {
		log.Debug("failed to get content", "error", err)
		return validator.Pass()
	}

	if content == "" {
		log.Debug("empty content, skipping validation")
		return validator.Pass()
	}

	// Run gofumpt using the linter
	lintCtx, cancel := context.WithTimeout(ctx, v.getTimeout())
	defer cancel()

	// Build options with auto-detection
	opts := v.buildGofumptOptions(filePath)
	result := v.checker.CheckWithOptions(lintCtx, content, opts)

	if result.Success {
		log.Debug("gofumpt passed")
		return validator.Pass()
	}

	log.Debug("gofumpt failed", "output", result.RawOut)

	return validator.FailWithRef(
		validator.RefGofumpt,
		v.formatGofumptOutput(result.RawOut),
	)
}

// getContent extracts Go code content from context
func (v *GofumptValidator) getContent(
	ctx *hook.Context,
	filePath string,
) (string, error) {
	log := v.Logger()

	// For Edit operations, skip validation initially (no fragment support)
	if ctx.EventType == hook.EventTypePreToolUse && ctx.ToolName == hook.ToolTypeEdit {
		log.Debug("skipping Edit operations (no fragment support)")
		return "", os.ErrNotExist
	}

	// Get content from context (Write operation)
	content := ctx.ToolInput.Content
	if content != "" {
		return content, nil
	}

	// Check if file exists
	if _, err := os.Stat(filePath); err != nil {
		log.Debug("file does not exist, skipping", "file", filePath)
		return "", err
	}

	// Read file content
	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is from Claude Code context
	if err != nil {
		log.Debug("failed to read file", "file", filePath, "error", err)
		return "", err
	}

	return string(data), nil
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
	) + "\n\nRun 'gofumpt -w <file>' to auto-fix."
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
