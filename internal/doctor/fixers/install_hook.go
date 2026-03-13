package fixers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/prompt"
	pkgConfig "github.com/smykla-skalski/klaudiush/pkg/config"
)

// ErrUserCancelled is returned when the user cancels the operation.
var ErrUserCancelled = errors.New("user cancelled operation")

// InstallHookFixer registers the klaudiush dispatcher in configured provider hook files.
type InstallHookFixer struct {
	prompter prompt.Prompter
	cfg      *pkgConfig.Config
}

// NewInstallHookFixer creates a new InstallHookFixer.
func NewInstallHookFixer(prompter prompt.Prompter, cfg *pkgConfig.Config) *InstallHookFixer {
	return &InstallHookFixer{
		prompter: prompter,
		cfg:      cfg,
	}
}

// ID returns the fixer identifier.
func (*InstallHookFixer) ID() string {
	return "install_hook"
}

// Description returns a human-readable description.
func (*InstallHookFixer) Description() string {
	return "Register klaudiush dispatcher in configured hook settings"
}

// CanFix checks if this fixer can fix the given result.
func (f *InstallHookFixer) CanFix(result doctor.CheckResult) bool {
	return result.FixID == f.ID() && result.Status == doctor.StatusFail
}

// Fix registers the dispatcher in the settings file.
func (f *InstallHookFixer) Fix(_ context.Context, interactive bool) error {
	binaryPath, err := exec.LookPath("klaudiush")
	if err != nil {
		return errors.Wrap(err, "klaudiush binary not found in PATH")
	}

	claudeEnabled, codexHooksPath, geminiSettingsPath := configuredInstallTargets(f.cfg)

	var targets []string
	if claudeEnabled {
		targets = append(targets, settings.GetUserSettingsPath())
	}

	if codexHooksPath != "" {
		targets = append(targets, codexHooksPath)
	}

	if geminiSettingsPath != "" {
		targets = append(targets, geminiSettingsPath)
	}

	if len(targets) == 0 {
		return errors.New("no configured hook targets available for installation")
	}

	if interactive {
		msg := fmt.Sprintf("Register dispatcher in %s?", strings.Join(targets, ", "))

		confirmed, promptErr := f.prompter.Confirm(msg, true)
		if promptErr != nil {
			return errors.Wrap(promptErr, "failed to get confirmation")
		}

		if !confirmed {
			return ErrUserCancelled
		}
	}

	if claudeEnabled {
		if _, err := settings.InstallClaudeDispatcher(
			settings.GetUserSettingsPath(),
			binaryPath,
		); err != nil {
			return errors.Wrap(err, "failed to install Claude hooks")
		}
	}

	if codexHooksPath != "" {
		if _, err := settings.InstallCodexDispatcher(codexHooksPath, binaryPath); err != nil {
			return errors.Wrap(err, "failed to install Codex hooks")
		}
	}

	if geminiSettingsPath != "" {
		if _, err := settings.InstallGeminiDispatcher(geminiSettingsPath, binaryPath); err != nil {
			return errors.Wrap(err, "failed to install Gemini hooks")
		}
	}

	return nil
}

func configuredInstallTargets(cfg *pkgConfig.Config) (bool, string, string) {
	claudeEnabled := true
	codexHooksPath := ""
	geminiSettingsPath := ""

	if cfg == nil {
		return claudeEnabled, codexHooksPath, geminiSettingsPath
	}

	providers := cfg.GetProviders()
	claudeEnabled = providers.GetClaude().IsEnabled()

	codexCfg := providers.GetCodex()
	if codexCfg.IsEnabled() && codexCfg.IsExperimentalEnabled() && codexCfg.HasHooksConfigPath() {
		codexHooksPath = codexCfg.HooksConfigPath
	}

	geminiCfg := providers.GetGemini()
	if geminiCfg.IsEnabled() && geminiCfg.HasSettingsPath() {
		geminiSettingsPath = geminiCfg.SettingsPath
	}

	return claudeEnabled, codexHooksPath, geminiSettingsPath
}
