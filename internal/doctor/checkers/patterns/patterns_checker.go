// Package patternschecker provides health checkers for the pattern learning system.
package patternschecker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// PathProvider resolves pattern file paths.
type PathProvider interface {
	// ProjectDataFile returns the path to the project-local patterns file.
	ProjectDataFile() string

	// GlobalDataDir returns the global pattern data directory.
	GlobalDataDir() string

	// IsEnabled returns true if pattern tracking is enabled.
	IsEnabled() bool
}

// DefaultPathProvider implements PathProvider using default config paths.
type DefaultPathProvider struct {
	cfg        *config.PatternsConfig
	projectDir string
}

// NewDefaultPathProvider creates a DefaultPathProvider.
func NewDefaultPathProvider() *DefaultPathProvider {
	cwd, _ := os.Getwd()

	return &DefaultPathProvider{
		cfg:        &config.PatternsConfig{},
		projectDir: cwd,
	}
}

// NewDefaultPathProviderWithConfig creates a DefaultPathProvider with a specific config and project dir.
func NewDefaultPathProviderWithConfig(
	cfg *config.PatternsConfig,
	projectDir string,
) *DefaultPathProvider {
	return &DefaultPathProvider{
		cfg:        cfg,
		projectDir: projectDir,
	}
}

// ProjectDataFile returns the path to the project-local patterns file.
func (p *DefaultPathProvider) ProjectDataFile() string {
	return filepath.Join(p.projectDir, p.cfg.GetProjectDataFile())
}

// GlobalDataDir returns the global pattern data directory.
func (p *DefaultPathProvider) GlobalDataDir() string {
	dir := p.cfg.GetGlobalDataDir()
	if len(dir) > 1 && dir[0] == '~' && dir[1] == '/' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, dir[2:])
		}
	}

	return dir
}

// IsEnabled returns true if pattern tracking is enabled.
func (p *DefaultPathProvider) IsEnabled() bool {
	return p.cfg.IsEnabled()
}

// SeedDataChecker checks if project-local seed data exists.
type SeedDataChecker struct {
	provider PathProvider
}

// NewSeedDataChecker creates a new SeedDataChecker.
func NewSeedDataChecker() *SeedDataChecker {
	return &SeedDataChecker{
		provider: NewDefaultPathProvider(),
	}
}

// NewSeedDataCheckerWithProvider creates a SeedDataChecker with custom provider.
func NewSeedDataCheckerWithProvider(provider PathProvider) *SeedDataChecker {
	return &SeedDataChecker{
		provider: provider,
	}
}

// Name returns the name of the check.
func (*SeedDataChecker) Name() string {
	return "Pattern seed data"
}

// Category returns the category of the check.
func (*SeedDataChecker) Category() doctor.Category {
	return doctor.CategoryPatterns
}

// Check performs the seed data check.
func (c *SeedDataChecker) Check(_ context.Context) doctor.CheckResult {
	if !c.provider.IsEnabled() {
		return doctor.Skip("Pattern seed data", "Pattern tracking disabled")
	}

	path := c.provider.ProjectDataFile()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doctor.FailWarning("Pattern seed data", "No seed data file").
				WithDetails(
					"Expected at: "+path,
					"Seed data provides initial pattern hints",
				).
				WithFixID("seed_patterns")
		}

		return doctor.FailError("Pattern seed data", fmt.Sprintf("Failed to stat: %v", err))
	}

	if info.Size() == 0 {
		return doctor.FailWarning("Pattern seed data", "Seed data file is empty").
			WithDetails("File exists but has no content").
			WithFixID("seed_patterns")
	}

	// Try to parse to verify it's valid
	//nolint:gosec // G304: path is from trusted config
	data, err := os.ReadFile(path)
	if err != nil {
		return doctor.FailError("Pattern seed data", fmt.Sprintf("Failed to read: %v", err))
	}

	var pd patterns.PatternData
	if err := json.Unmarshal(data, &pd); err != nil {
		return doctor.FailError("Pattern seed data", "Invalid JSON in seed file").
			WithDetails(
				"Error: "+err.Error(),
				"File may be corrupted",
			).
			WithFixID("seed_patterns")
	}

	return doctor.Pass("Pattern seed data",
		fmt.Sprintf("Valid with %d patterns", len(pd.Patterns)))
}

// DataFileChecker checks if global pattern data files are valid.
type DataFileChecker struct {
	provider PathProvider
}

// NewDataFileChecker creates a new DataFileChecker.
func NewDataFileChecker() *DataFileChecker {
	return &DataFileChecker{
		provider: NewDefaultPathProvider(),
	}
}

// NewDataFileCheckerWithProvider creates a DataFileChecker with custom provider.
func NewDataFileCheckerWithProvider(provider PathProvider) *DataFileChecker {
	return &DataFileChecker{
		provider: provider,
	}
}

// Name returns the name of the check.
func (*DataFileChecker) Name() string {
	return "Pattern data files"
}

// Category returns the category of the check.
func (*DataFileChecker) Category() doctor.Category {
	return doctor.CategoryPatterns
}

// Check performs the data file check.
func (c *DataFileChecker) Check(_ context.Context) doctor.CheckResult {
	if !c.provider.IsEnabled() {
		return doctor.Skip("Pattern data files", "Pattern tracking disabled")
	}

	globalDir := c.provider.GlobalDataDir()

	_, err := os.Stat(globalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return doctor.FailWarning("Pattern data files", "No learned pattern data yet").
				WithDetails(
					"Expected at: "+globalDir,
					"Patterns are learned automatically during validation",
				)
		}

		return doctor.FailError("Pattern data files", fmt.Sprintf("Failed to stat: %v", err))
	}

	entries, err := os.ReadDir(globalDir)
	if err != nil {
		return doctor.FailError(
			"Pattern data files",
			fmt.Sprintf("Failed to read directory: %v", err),
		)
	}

	jsonFiles := 0
	corruptFiles := 0

	var corruptDetails []string

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		jsonFiles++

		filePath := filepath.Join(globalDir, entry.Name())

		//nolint:gosec // G304: path is from trusted config
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			corruptFiles++

			corruptDetails = append(corruptDetails, entry.Name()+": read error")

			continue
		}

		var pd patterns.PatternData
		if jsonErr := json.Unmarshal(data, &pd); jsonErr != nil {
			corruptFiles++

			corruptDetails = append(corruptDetails, entry.Name()+": invalid JSON")
		}
	}

	if corruptFiles > 0 {
		return doctor.FailError("Pattern data files", fmt.Sprintf("%d corrupt file(s)", corruptFiles)).
			WithDetails(corruptDetails...)
	}

	if jsonFiles == 0 {
		return doctor.FailWarning("Pattern data files", "No learned pattern data yet").
			WithDetails("Patterns are learned automatically during validation")
	}

	return doctor.Pass("Pattern data files",
		fmt.Sprintf("%d file(s) valid", jsonFiles))
}

// DescriptionChecker checks that all observed error codes have descriptions.
type DescriptionChecker struct {
	provider PathProvider
}

// NewDescriptionChecker creates a new DescriptionChecker.
func NewDescriptionChecker() *DescriptionChecker {
	return &DescriptionChecker{
		provider: NewDefaultPathProvider(),
	}
}

// NewDescriptionCheckerWithProvider creates a DescriptionChecker with custom provider.
func NewDescriptionCheckerWithProvider(provider PathProvider) *DescriptionChecker {
	return &DescriptionChecker{
		provider: provider,
	}
}

// Name returns the name of the check.
func (*DescriptionChecker) Name() string {
	return "Pattern code coverage"
}

// Category returns the category of the check.
func (*DescriptionChecker) Category() doctor.Category {
	return doctor.CategoryPatterns
}

// Check performs the description coverage check.
func (c *DescriptionChecker) Check(_ context.Context) doctor.CheckResult {
	if !c.provider.IsEnabled() {
		return doctor.Skip("Pattern code coverage", "Pattern tracking disabled")
	}

	codes := collectObservedCodes(c.provider)
	if len(codes) == 0 {
		return doctor.Skip("Pattern code coverage", "No pattern data to check")
	}

	descriptions := patterns.CodeDescriptions()

	var unknown []string

	for code := range codes {
		if _, ok := descriptions[code]; !ok {
			unknown = append(unknown, code)
		}
	}

	if len(unknown) > 0 {
		details := make([]string, 0, len(unknown)+1)
		details = append(details, "Unknown codes found in pattern data:")

		for _, code := range unknown {
			details = append(details, "  - "+code)
		}

		return doctor.FailWarning("Pattern code coverage",
			fmt.Sprintf("%d code(s) missing descriptions", len(unknown))).
			WithDetails(details...)
	}

	return doctor.Pass("Pattern code coverage",
		fmt.Sprintf("All %d observed codes have descriptions", len(codes)))
}

// collectObservedCodes gathers all unique error codes from pattern data files.
func collectObservedCodes(provider PathProvider) map[string]bool {
	codes := make(map[string]bool)

	// Check project file
	collectCodesFromFile(provider.ProjectDataFile(), codes)

	// Check global files
	globalDir := provider.GlobalDataDir()

	entries, err := os.ReadDir(globalDir)
	if err != nil {
		return codes
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		collectCodesFromFile(filepath.Join(globalDir, entry.Name()), codes)
	}

	return codes
}

// collectCodesFromFile reads a pattern file and adds all codes to the set.
func collectCodesFromFile(path string, codes map[string]bool) {
	//nolint:gosec // G304: path is from trusted config
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var pd patterns.PatternData
	if err := json.Unmarshal(data, &pd); err != nil {
		return
	}

	for _, p := range pd.Patterns {
		codes[p.SourceCode] = true
		codes[p.TargetCode] = true
	}
}
