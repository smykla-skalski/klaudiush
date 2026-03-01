package fixers

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
)

var (
	// ErrBinaryInstallNotSupported is returned when binary installation is not supported.
	ErrBinaryInstallNotSupported = errors.New("automatic binary installation not supported")

	// ErrMiseNotFound is returned when the mise command is not available.
	ErrMiseNotFound = errors.New("mise command not found")
)

// InstallBinaryFixer attempts to install the klaudiush binary.
type InstallBinaryFixer struct {
	prompter prompt.Prompter
}

// NewInstallBinaryFixer creates a new InstallBinaryFixer.
func NewInstallBinaryFixer(prompter prompt.Prompter) *InstallBinaryFixer {
	return &InstallBinaryFixer{
		prompter: prompter,
	}
}

// ID returns the fixer identifier.
func (*InstallBinaryFixer) ID() string {
	return "install_binary"
}

// Description returns a human-readable description.
func (*InstallBinaryFixer) Description() string {
	return "Install klaudiush binary to PATH"
}

// CanFix checks if this fixer can fix the given result.
func (f *InstallBinaryFixer) CanFix(result doctor.CheckResult) bool {
	return result.FixID == f.ID() && result.Status == doctor.StatusFail
}

// Fix attempts to install the binary.
func (f *InstallBinaryFixer) Fix(ctx context.Context, interactive bool) error {
	// Check if we're in the klaudiush repository
	if !f.isInKlaudiushRepo() {
		return errors.WithMessage(
			ErrBinaryInstallNotSupported,
			"not in klaudiush repository. Please install manually with: mise run install",
		)
	}

	if interactive {
		msg := "Install klaudiush binary using 'mise run install'?"

		confirmed, err := f.prompter.Confirm(msg, true)
		if err != nil {
			return errors.Wrap(err, "failed to get confirmation")
		}

		if !confirmed {
			return ErrUserCancelled
		}
	}

	// Check if mise is available
	if _, err := exec.LookPath("mise"); err != nil {
		return errors.Wrapf(ErrMiseNotFound, "install mise from https://mise.jdx.dev")
	}

	// Run mise run install
	cmd := exec.CommandContext(ctx, "mise", "run", "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return errors.Wrap(err, "failed to run 'mise run install'")
	}

	return nil
}

func (*InstallBinaryFixer) isInKlaudiushRepo() bool {
	// Check if we have a .mise.toml in current directory
	if _, err := os.Stat(".mise.toml"); err == nil {
		// Check if it's the klaudiush repo by looking for specific files
		checkFiles := []string{
			"cmd/klaudiush/main.go",
			"go.mod",
		}

		for _, file := range checkFiles {
			if _, err := os.Stat(file); err != nil {
				return false
			}
		}

		// Check go.mod for klaudiush module
		data, err := os.ReadFile("go.mod")
		if err == nil {
			content := string(data)
			if filepath.Base(filepath.Dir(content)) == "klaudiush" ||
				len(content) > 0 && (content[:50] == "module github.com/smykla-skalski/klaudiush" ||
					len(content) > 42 && content[:42] == "module github.com/smykla-skalski/klaudiush") {
				return true
			}
		}

		return true
	}

	return false
}
