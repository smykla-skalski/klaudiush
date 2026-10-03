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

	install := configuredInstallTargets(f.cfg)

	targets := install.paths()
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

	if install.claudeEnabled {
		if _, err := settings.InstallClaudeDispatcher(
			settings.GetUserSettingsPath(),
			binaryPath,
		); err != nil {
			return errors.Wrap(err, "failed to install Claude hooks")
		}
	}

	if install.codexHooksPath != "" {
		if _, err := settings.InstallCodexDispatcher(
			install.codexHooksPath,
			binaryPath,
		); err != nil {
			return errors.Wrap(err, "failed to install Codex hooks")
		}
	}

	if install.geminiSettingsPath != "" {
		if err := installGemini(install, binaryPath); err != nil {
			return err
		}
	}

	if install.openCodePluginPath != "" {
		if _, err := settings.InstallOpenCodeDispatcher(
			install.openCodePluginPath,
			binaryPath,
		); err != nil {
			return errors.Wrap(err, "failed to install opencode bridge plugin")
		}
	}

	return nil
}

// installGemini registers the Gemini hooks, and BeforeToolSelection when the
// evidence tool phase needs it.
func installGemini(install installTargets, binaryPath string) error {
	if _, err := settings.InstallGeminiDispatcher(
		install.geminiSettingsPath,
		binaryPath,
	); err != nil {
		return errors.Wrap(err, "failed to install Gemini hooks")
	}

	if !install.geminiToolSelection {
		return nil
	}

	if _, err := settings.InstallGeminiToolSelection(
		install.geminiSettingsPath,
		binaryPath,
	); err != nil {
		return errors.Wrap(err, "failed to install Gemini tool selection hook")
	}

	return nil
}

// installTargets holds the provider hook files this fixer may write.
type installTargets struct {
	claudeEnabled      bool
	codexHooksPath     string
	geminiSettingsPath string
	openCodePluginPath string

	// geminiToolSelection registers Gemini BeforeToolSelection, which only
	// the evidence tool phase answers.
	geminiToolSelection bool
}

// paths lists the files that will be written, for the confirmation prompt.
func (t installTargets) paths() []string {
	var paths []string

	if t.claudeEnabled {
		paths = append(paths, settings.GetUserSettingsPath())
	}

	for _, path := range []string{
		t.codexHooksPath,
		t.geminiSettingsPath,
		t.openCodePluginPath,
	} {
		if path != "" {
			paths = append(paths, path)
		}
	}

	return paths
}

func configuredInstallTargets(cfg *pkgConfig.Config) installTargets {
	targets := installTargets{claudeEnabled: true}

	if cfg == nil {
		return targets
	}

	providers := cfg.GetProviders()
	targets.claudeEnabled = providers.GetClaude().IsEnabled()

	codexCfg := providers.GetCodex()
	if codexCfg.IsEnabled() && codexCfg.IsExperimentalEnabled() && codexCfg.HasHooksConfigPath() {
		targets.codexHooksPath = codexCfg.HooksConfigPath
	}

	geminiCfg := providers.GetGemini()
	if geminiCfg.IsEnabled() && geminiCfg.HasSettingsPath() {
		targets.geminiSettingsPath = geminiCfg.SettingsPath
	}

	targets.geminiToolSelection = cfg.Evidence.GetToolPhase().IsEnabled()

	// opencode falls back to the default plugin location: unlike the JSON-hook
	// providers there is no pre-existing file an operator must point at, so the
	// integration can be installed from an enable flag alone.
	openCodeCfg := providers.GetOpenCode()
	if openCodeCfg.IsEnabled() {
		targets.openCodePluginPath = settings.ResolveOpenCodePluginPath(openCodeCfg.PluginPath)
	}

	return targets
}
