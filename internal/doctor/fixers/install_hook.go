package fixers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/cockroachdb/errors"

	"github.com/smykla-labs/klaudiush/internal/doctor"
	"github.com/smykla-labs/klaudiush/internal/doctor/settings"
	"github.com/smykla-labs/klaudiush/internal/prompt"
)

const (
	// DefaultHookTimeout is the default timeout for hook execution in seconds.
	DefaultHookTimeout = 30
)

// ErrUserCancelled is returned when the user cancels the operation.
var ErrUserCancelled = errors.New("user cancelled operation")

// InstallHookFixer registers the klaudiush dispatcher in Claude settings.
type InstallHookFixer struct {
	prompter prompt.Prompter
}

// NewInstallHookFixer creates a new InstallHookFixer.
func NewInstallHookFixer(prompter prompt.Prompter) *InstallHookFixer {
	return &InstallHookFixer{
		prompter: prompter,
	}
}

// ID returns the fixer identifier.
func (*InstallHookFixer) ID() string {
	return "install_hook"
}

// Description returns a human-readable description.
func (*InstallHookFixer) Description() string {
	return "Register klaudiush dispatcher in Claude settings"
}

// CanFix checks if this fixer can fix the given result.
func (f *InstallHookFixer) CanFix(result doctor.CheckResult) bool {
	return result.FixID == f.ID() && result.Status == doctor.StatusFail
}

// Fix registers the dispatcher in the settings file.
func (f *InstallHookFixer) Fix(_ context.Context, interactive bool) error {
	// Get binary path
	binaryPath, err := exec.LookPath("klaudiush")
	if err != nil {
		return errors.Wrap(err, "klaudiush binary not found in PATH")
	}

	// Determine which settings file to update
	settingsPath := settings.GetUserSettingsPath()

	if interactive {
		msg := fmt.Sprintf("Register dispatcher in %s?", settingsPath)

		confirmed, promptErr := f.prompter.Confirm(msg, true)
		if promptErr != nil {
			return errors.Wrap(promptErr, "failed to get confirmation")
		}

		if !confirmed {
			return ErrUserCancelled
		}
	}

	// Load existing settings or create new ones
	parser := settings.NewSettingsParser(settingsPath)

	claudeSettings, err := parser.Parse()
	if err != nil && !errors.Is(err, settings.ErrSettingsNotFound) {
		return errors.Wrap(err, "failed to parse existing settings")
	}

	if claudeSettings == nil {
		claudeSettings = &settings.ClaudeSettings{
			Hooks: make(map[string][]settings.HookConfig),
		}
	}

	// Check if already registered (shouldn't happen, but be defensive)
	registered, _ := parser.IsDispatcherRegistered(binaryPath)
	if registered {
		return nil
	}

	// Add PreToolUse hook configuration
	hookConfig := settings.HookConfig{
		Matcher: "Bash|Write|Edit",
		Hooks: []settings.HookCommandConfig{
			{
				Type:    "command",
				Command: binaryPath + " --hook-type PreToolUse",
				Timeout: DefaultHookTimeout,
			},
		},
	}

	// Add to PreToolUse hooks
	claudeSettings.Hooks["PreToolUse"] = append(
		claudeSettings.Hooks["PreToolUse"],
		hookConfig,
	)

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(claudeSettings, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal settings")
	}

	// Add newline at end of file
	data = append(data, '\n')

	// Write atomically with backup
	if err := AtomicWriteFile(settingsPath, data, true); err != nil {
		return errors.Wrap(err, "failed to write settings file")
	}

	return nil
}
