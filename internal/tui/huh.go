package tui

import (
	"github.com/charmbracelet/huh"

	pkgConfig "github.com/smykla-labs/klaudiush/pkg/config"
)

// HuhUI implements UI using charmbracelet/huh.
type HuhUI struct{}

// NewHuhUI creates a new HuhUI instance.
func NewHuhUI() *HuhUI {
	return &HuhUI{}
}

// IsInteractive returns true as HuhUI is for interactive terminals.
func (*HuhUI) IsInteractive() bool {
	return true
}

// RunInitForm runs the initialization configuration form using huh.
func (*HuhUI) RunInitForm(opts InitFormOptions) (*pkgConfig.Config, bool, error) {
	var result InitFormResult

	result.BellEnabled = true // Default to enabled

	// Build the form
	form := buildInitForm(opts, &result)

	// Run the form
	if err := form.Run(); err != nil {
		return nil, false, err
	}

	// Convert result to config
	cfg := buildConfigFromResult(&result)

	return cfg, result.AddToExclude, nil
}

// buildInitForm creates the huh form for initialization.
func buildInitForm(opts InitFormOptions, result *InitFormResult) *huh.Form {
	// Start with signoff field
	signoffInput := huh.NewInput().
		Title("Git Commit Signoff").
		Description("Validates that commits are signed off with this name/email.\nLeave empty to skip signoff validation.").
		Placeholder("Name <email@klaudiu.sh>").
		Value(&result.Signoff)

	if opts.DefaultSignoff != "" {
		result.Signoff = opts.DefaultSignoff
	}

	// Bell notification field
	bellConfirm := huh.NewConfirm().
		Title("Enable Notification Bell").
		Description("Sends a bell character to trigger terminal notifications.\nUseful for getting notified about permission prompts.").
		Affirmative("Yes").
		Negative("No").
		Value(&result.BellEnabled)

	// Build groups
	groups := []*huh.Group{
		huh.NewGroup(signoffInput, bellConfirm),
	}

	// Add git exclude option if applicable
	if opts.ShowGitExclude {
		result.AddToExclude = true // Default to yes

		excludeConfirm := huh.NewConfirm().
			Title("Add to .git/info/exclude").
			Description("Add config file to .git/info/exclude to prevent accidental commits.").
			Affirmative("Yes").
			Negative("No").
			Value(&result.AddToExclude)

		groups = append(groups, huh.NewGroup(excludeConfirm))
	}

	return huh.NewForm(groups...).
		WithTheme(huh.ThemeCharm()).
		WithShowHelp(true).
		WithKeyMap(huh.NewDefaultKeyMap())
}

// buildConfigFromResult converts the form result to a config struct.
func buildConfigFromResult(result *InitFormResult) *pkgConfig.Config {
	cfg := &pkgConfig.Config{
		Validators: &pkgConfig.ValidatorsConfig{},
	}

	// Set signoff if provided
	if result.Signoff != "" {
		cfg.Validators.Git = &pkgConfig.GitConfig{
			Commit: &pkgConfig.CommitValidatorConfig{
				Message: &pkgConfig.CommitMessageConfig{
					ExpectedSignoff: result.Signoff,
				},
			},
		}
	}

	// Set bell notification
	cfg.Validators.Notification = &pkgConfig.NotificationConfig{
		Bell: &pkgConfig.BellValidatorConfig{
			ValidatorConfig: pkgConfig.ValidatorConfig{
				Enabled: &result.BellEnabled,
			},
		},
	}

	return cfg
}
