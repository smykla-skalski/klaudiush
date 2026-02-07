package doctor

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/prompt"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// Runner orchestrates health checks and fixes
type Runner struct {
	registry *Registry
	reporter Reporter
	prompter prompt.Prompter
	logger   logger.Logger
}

// RunOptions configures the doctor run behavior
type RunOptions struct {
	// Verbose enables detailed output
	Verbose bool

	// AutoFix automatically applies fixes without prompting (--fix flag)
	AutoFix bool

	// Interactive prompts user for fixes
	Interactive bool

	// Categories filters checks by category
	Categories []Category

	// Global checks global context
	Global bool

	// Project checks project context
	Project bool
}

// NewRunner creates a new Runner
func NewRunner(
	registry *Registry,
	reporter Reporter,
	prompter prompt.Prompter,
	logger logger.Logger,
) *Runner {
	return &Runner{
		registry: registry,
		reporter: reporter,
		prompter: prompter,
		logger:   logger,
	}
}

// Run executes health checks and applies fixes if needed
func (r *Runner) Run(ctx context.Context, opts RunOptions) error {
	r.logger.Info("starting doctor run", "verbose", opts.Verbose, "autoFix", opts.AutoFix)

	// Step 1: Execute checks
	var results []CheckResult

	if len(opts.Categories) > 0 {
		// Run specific categories
		for _, category := range opts.Categories {
			categoryResults := r.registry.RunCategory(ctx, category)
			results = append(results, categoryResults...)
		}
	} else {
		// Run all checks
		results = r.registry.RunAll(ctx)
	}

	r.logger.Info("checks completed", "total", len(results))

	// Step 2: Report results
	r.reporter.Report(results, opts.Verbose)

	// Step 3: Check if there are errors with fixes available
	fixableErrors := r.collectFixableResults(results)

	if len(fixableErrors) == 0 {
		// No fixable errors, we're done
		return r.determineExitError(results)
	}

	// Step 4: Apply fixes if needed
	switch {
	case opts.AutoFix:
		r.logger.Info("auto-fix mode enabled, applying fixes", "count", len(fixableErrors))

		if err := r.applyFixes(ctx, fixableErrors, false); err != nil {
			return errors.Wrap(err, "failed to apply fixes")
		}
	case opts.Interactive:
		r.logger.Info("interactive mode enabled, prompting for fixes", "count", len(fixableErrors))

		if err := r.promptAndApplyFixes(ctx, fixableErrors); err != nil {
			return errors.Wrap(err, "failed to apply fixes")
		}
	default:
		// Just suggest fixes
		r.logger.Info("suggesting fixes", "count", len(fixableErrors))
		r.suggestFixes(fixableErrors)
	}

	// Step 5: Re-run failed checks after fixes
	if opts.AutoFix || opts.Interactive {
		r.logger.Info("re-running failed checks after fixes")

		rerunResults := r.rerunChecks(ctx, fixableErrors)
		r.reporter.Report(rerunResults, opts.Verbose)

		// Combine original passed checks with rerun results
		combinedResults := r.combineResults(results, rerunResults)

		return r.determineExitError(combinedResults)
	}

	return r.determineExitError(results)
}

// collectFixableResults returns results that have errors and fixes available
func (*Runner) collectFixableResults(results []CheckResult) []CheckResult {
	var fixable []CheckResult

	for _, result := range results {
		if result.IsError() && result.HasFix() {
			fixable = append(fixable, result)
		}
	}

	return fixable
}

// applyFixes applies fixes for the given results
func (r *Runner) applyFixes(ctx context.Context, results []CheckResult, interactive bool) error {
	for _, result := range results {
		fixer, ok := r.registry.GetFixer(result.FixID)
		if !ok {
			r.logger.Error("fixer not found", "fixID", result.FixID)
			continue
		}

		r.logger.Info("applying fix", "check", result.Name, "fixer", fixer.ID())

		if err := fixer.Fix(ctx, interactive); err != nil {
			return errors.Wrapf(err, "failed to fix %q", result.Name)
		}

		r.logger.Info("fix applied successfully", "check", result.Name)
	}

	return nil
}

// promptAndApplyFixes prompts the user and applies fixes
func (r *Runner) promptAndApplyFixes(ctx context.Context, results []CheckResult) error {
	for _, result := range results {
		fixer, ok := r.registry.GetFixer(result.FixID)
		if !ok {
			r.logger.Error("fixer not found", "fixID", result.FixID)
			continue
		}

		// Prompt user
		prompt := fmt.Sprintf("Apply fix for %q?", result.Name)

		confirmed, err := r.prompter.Confirm(prompt, true)
		if err != nil {
			return errors.Wrap(err, "failed to get user confirmation")
		}

		if !confirmed {
			r.logger.Info("fix skipped by user", "check", result.Name)
			continue
		}

		// Apply fix
		r.logger.Info("applying fix", "check", result.Name, "fixer", fixer.ID())

		if err := fixer.Fix(ctx, true); err != nil {
			return errors.Wrapf(err, "failed to fix %q", result.Name)
		}

		r.logger.Info("fix applied successfully", "check", result.Name)
	}

	return nil
}

// suggestFixes prints suggested fixes to the user
func (r *Runner) suggestFixes(results []CheckResult) {
	fmt.Println("\nSuggested fixes:")

	for _, result := range results {
		fixer, ok := r.registry.GetFixer(result.FixID)
		if !ok {
			continue
		}

		fmt.Printf("  - %s: %s\n", result.Name, fixer.Description())
	}

	fmt.Println("\nRun 'klaudiush doctor --fix' to apply fixes automatically")
}

// rerunChecks re-runs checks for the given results
func (r *Runner) rerunChecks(ctx context.Context, results []CheckResult) []CheckResult {
	// Build a map of check names to rerun
	checkNames := make(map[string]bool)
	for _, result := range results {
		checkNames[result.Name] = true
	}

	// Get all checkers and filter by name
	allResults := r.registry.RunAll(ctx)

	var rerunResults []CheckResult

	for _, result := range allResults {
		if checkNames[result.Name] {
			rerunResults = append(rerunResults, result)
		}
	}

	return rerunResults
}

// combineResults combines original passed checks with rerun results
func (*Runner) combineResults(original, rerun []CheckResult) []CheckResult {
	// Build a map of rerun results by name
	rerunMap := make(map[string]CheckResult)
	for _, result := range rerun {
		rerunMap[result.Name] = result
	}

	// Combine results
	var combined []CheckResult

	for _, result := range original {
		if rerunResult, ok := rerunMap[result.Name]; ok {
			// Use rerun result
			combined = append(combined, rerunResult)
		} else if result.IsPassed() || result.IsSkipped() {
			// Keep original passed/skipped result
			combined = append(combined, result)
		}
	}

	return combined
}

// determineExitError determines if an error should be returned based on results
func (r *Runner) determineExitError(results []CheckResult) error {
	hasErrors := false
	errorCount := 0
	warningCount := 0

	for _, result := range results {
		if result.IsError() {
			hasErrors = true
			errorCount++
		} else if result.IsWarning() {
			warningCount++
		}
	}

	r.logger.Info("final status",
		"errors", errorCount,
		"warnings", warningCount,
		"total", len(results),
	)

	if hasErrors {
		return errors.New("health checks failed")
	}

	return nil
}
