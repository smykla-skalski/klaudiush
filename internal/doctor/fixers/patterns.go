package fixers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// PatternsFixer fixes issues with the pattern learning system.
type PatternsFixer struct {
	prompter   prompt.Prompter
	projectDir string
}

// NewPatternsFixer creates a new PatternsFixer.
func NewPatternsFixer(prompter prompt.Prompter) *PatternsFixer {
	cwd, _ := os.Getwd()

	return &PatternsFixer{
		prompter:   prompter,
		projectDir: cwd,
	}
}

// ID returns the fixer identifier.
func (*PatternsFixer) ID() string {
	return "seed_patterns"
}

// Description returns a human-readable description.
func (*PatternsFixer) Description() string {
	return "Write seed pattern data to project"
}

// CanFix checks if this fixer can fix the given result.
func (*PatternsFixer) CanFix(result doctor.CheckResult) bool {
	return result.FixID == "seed_patterns" && result.Status == doctor.StatusFail
}

// Fix writes seed patterns to the project data file.
func (f *PatternsFixer) Fix(_ context.Context, interactive bool) error {
	cfg := &config.PatternsConfig{}
	store := patterns.NewFilePatternStore(cfg, f.projectDir)

	if store.HasProjectData() {
		return nil
	}

	destPath := filepath.Join(f.projectDir, cfg.GetProjectDataFile())

	if interactive {
		msg := fmt.Sprintf("Write seed pattern data to %s?", destPath)

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	if err := patterns.EnsureSeedData(store); err != nil {
		return errors.Wrap(err, "failed to write seed data")
	}

	return nil
}
