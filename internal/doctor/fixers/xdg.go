package fixers

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// XDGFixer fixes XDG base directory issues.
type XDGFixer struct {
	prompter prompt.Prompter
	log      logger.Logger
}

// NewXDGFixer creates a new XDGFixer.
func NewXDGFixer(prompter prompt.Prompter, log logger.Logger) *XDGFixer {
	return &XDGFixer{
		prompter: prompter,
		log:      log,
	}
}

// ID returns the fixer identifier.
func (*XDGFixer) ID() string {
	return "xdg_fixer"
}

// Description returns a human-readable description.
func (*XDGFixer) Description() string {
	return "Migrate legacy paths and create XDG directories"
}

// CanFix checks if this fixer can fix the given result.
func (*XDGFixer) CanFix(result doctor.CheckResult) bool {
	return (result.FixID == "migrate_xdg" || result.FixID == "create_xdg_dirs") &&
		result.Status == doctor.StatusFail
}

// Fix attempts to fix XDG issues.
func (f *XDGFixer) Fix(_ context.Context, interactive bool) error {
	if xdg.NeedsMigration() {
		if err := f.runMigration(interactive); err != nil {
			return err
		}
	}

	return f.ensureDirs(interactive)
}

func (f *XDGFixer) runMigration(interactive bool) error {
	if interactive {
		confirmed, err := f.prompter.Confirm("Migrate legacy paths to XDG locations?", true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	result, err := xdg.Migrate(f.log)
	if err != nil {
		return errors.Wrap(err, "XDG migration failed")
	}

	f.log.Info("XDG migration complete",
		"moved", result.Moved,
		"symlinks", result.Symlinks,
		"skipped", result.Skipped,
	)

	return nil
}

func (f *XDGFixer) ensureDirs(interactive bool) error {
	dirs := []string{xdg.ConfigDir(), xdg.DataDir(), xdg.StateDir()}

	if interactive {
		msg := fmt.Sprintf("Create XDG directories (%s, %s, %s)?",
			xdg.ConfigDir(), xdg.DataDir(), xdg.StateDir())

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return nil
		}
	}

	for _, dir := range dirs {
		if err := xdg.EnsureDir(dir); err != nil {
			return errors.Wrapf(err, "creating %s", dir)
		}
	}

	return nil
}
