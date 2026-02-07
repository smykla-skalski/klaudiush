// Package reporters provides output formatting for doctor check results
package reporters

import (
	"fmt"
	"slices"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/doctor"
)

// categoryOrder defines the display order for categories
var categoryOrder = []doctor.Category{
	doctor.CategoryBinary,
	doctor.CategoryHook,
	doctor.CategoryConfig,
	doctor.CategoryTools,
}

// categoryNames maps categories to display names
var categoryNames = map[doctor.Category]string{
	doctor.CategoryBinary: "Binary",
	doctor.CategoryHook:   "Hook Registration",
	doctor.CategoryConfig: "Configuration",
	doctor.CategoryTools:  "Optional Tools",
}

// SimpleReporter provides simple checklist-style output
type SimpleReporter struct{}

// NewSimpleReporter creates a new SimpleReporter
func NewSimpleReporter() *SimpleReporter {
	return &SimpleReporter{}
}

// Report outputs the results in a simple checklist format
func (*SimpleReporter) Report(results []doctor.CheckResult, verbose bool) {
	// Group results by category
	categoryMap := groupByCategory(results)

	// Print header
	fmt.Println("Checking klaudiush health...")
	fmt.Println()

	// Print results by category in order
	printCategories(categoryMap, verbose)

	// Print summary
	printSummary(results)
}

// groupByCategory groups results by their category
func groupByCategory(results []doctor.CheckResult) map[doctor.Category][]doctor.CheckResult {
	categoryMap := make(map[doctor.Category][]doctor.CheckResult)

	for _, result := range results {
		categoryMap[result.Category] = append(categoryMap[result.Category], result)
	}

	return categoryMap
}

// getCategoryName returns the display name for a category
func getCategoryName(category doctor.Category) string {
	if name, ok := categoryNames[category]; ok {
		return name
	}

	// Fallback: capitalize first letter manually for unknown categories
	s := string(category)
	if len(s) == 0 {
		return "Other"
	}

	return strings.ToUpper(s[:1]) + s[1:]
}

// printCategories prints results grouped by category in defined order
func printCategories(categoryMap map[doctor.Category][]doctor.CheckResult, verbose bool) {
	// Print categories in defined order
	for _, category := range categoryOrder {
		categoryResults, ok := categoryMap[category]
		if !ok || len(categoryResults) == 0 {
			continue
		}

		fmt.Printf("%s:\n", getCategoryName(category))

		for _, result := range categoryResults {
			printResult(result, verbose)
		}

		fmt.Println()
	}

	// Print any unknown categories
	for category, categoryResults := range categoryMap {
		if slices.Contains(categoryOrder, category) {
			continue
		}

		fmt.Printf("%s:\n", getCategoryName(category))

		for _, result := range categoryResults {
			printResult(result, verbose)
		}

		fmt.Println()
	}
}

// printResult prints a single check result
func printResult(result doctor.CheckResult, verbose bool) {
	icon := getStatusIcon(result)

	// Print status line
	fmt.Printf("  %s %s", icon, result.Name)

	if result.Message != "" {
		fmt.Printf(" - %s", result.Message)
	}

	fmt.Println()

	// Print details in verbose mode
	if verbose && len(result.Details) > 0 {
		printDetails(result.Details)
	}

	// Print fix suggestion
	if result.HasFix() && result.Status == doctor.StatusFail {
		fmt.Printf("     → Run: klaudiush doctor --fix\n")
	}
}

// printDetails prints detail lines
func printDetails(details []string) {
	for _, detail := range details {
		fmt.Printf("     %s\n", detail)
	}
}

// printSummary prints the summary line
func printSummary(results []doctor.CheckResult) {
	errorCount, warningCount, passedCount := countResults(results)

	fmt.Printf("Summary: %d error(s), %d warning(s), %d passed\n",
		errorCount, warningCount, passedCount)
}

// getStatusIcon returns the appropriate icon for a check result
func getStatusIcon(result doctor.CheckResult) string {
	switch result.Status {
	case doctor.StatusPass:
		return "✅"
	case doctor.StatusFail:
		switch result.Severity {
		case doctor.SeverityError:
			return "❌"
		case doctor.SeverityWarning:
			return "⚠️"
		default:
			return "ℹ️"
		}
	case doctor.StatusSkipped:
		return "⊘"
	default:
		return "?"
	}
}

// countResults counts errors, warnings, and passed checks
func countResults(results []doctor.CheckResult) (errors, warnings, passed int) {
	for _, result := range results {
		switch {
		case result.IsPassed():
			passed++
		case result.IsError():
			errors++
		case result.IsWarning():
			warnings++
		}
	}

	return errors, warnings, passed
}
