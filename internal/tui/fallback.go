package tui

import (
	"fmt"

	"github.com/smykla-labs/klaudiush/internal/prompt"
	pkgConfig "github.com/smykla-labs/klaudiush/pkg/config"
)

// FallbackUI implements UI using simple stdin/stdout prompts.
// This is used when the terminal is not interactive (CI, piped input, etc.).
type FallbackUI struct {
	prompter prompt.Prompter
}

// NewFallbackUI creates a new FallbackUI instance.
func NewFallbackUI() *FallbackUI {
	return &FallbackUI{
		prompter: prompt.NewStdPrompter(),
	}
}

// NewFallbackUIWithPrompter creates a FallbackUI with a custom prompter.
func NewFallbackUIWithPrompter(p prompt.Prompter) *FallbackUI {
	return &FallbackUI{
		prompter: p,
	}
}

// IsInteractive returns false as FallbackUI is for non-interactive terminals.
func (*FallbackUI) IsInteractive() bool {
	return false
}

// RunInitForm runs the initialization configuration form using simple prompts.
func (f *FallbackUI) RunInitForm(opts InitFormOptions) (*pkgConfig.Config, bool, error) {
	var result InitFormResult

	// Display header
	f.displayHeader(opts.Global)

	// Prompt for signoff
	if err := f.promptSignoff(opts.DefaultSignoff, &result); err != nil {
		return nil, false, err
	}

	fmt.Println()

	// Prompt for bell notification
	if err := f.promptBell(&result); err != nil {
		return nil, false, err
	}

	// Prompt for git exclude if applicable
	if opts.ShowGitExclude {
		fmt.Println()

		if err := f.promptGitExclude(&result); err != nil {
			return nil, false, err
		}
	}

	// Convert result to config
	cfg := buildConfigFromResult(&result)

	return cfg, result.AddToExclude, nil
}

// displayHeader displays the configuration header.
func (*FallbackUI) displayHeader(global bool) {
	fmt.Println("╔═══════════════════════════════════════════════╗")

	if global {
		fmt.Println("║   Klaudiush Global Configuration Setup       ║")
	} else {
		fmt.Println("║   Klaudiush Project Configuration Setup      ║")
	}

	fmt.Println("╚═══════════════════════════════════════════════╝")
	fmt.Println()
}

// promptSignoff prompts for signoff configuration.
//
//nolint:unparam // error return kept for consistent API
func (f *FallbackUI) promptSignoff(defaultSignoff string, result *InitFormResult) error {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Git Commit Signoff Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("This validates that commits are signed off with the specified name/email.")
	fmt.Println("Leave empty to skip signoff validation.")
	fmt.Println()

	signoff, err := f.prompter.Input("Signoff (Name <email>)", defaultSignoff)
	if err != nil {
		// Allow empty input
		signoff = ""
	}

	result.Signoff = signoff

	if signoff == "" {
		fmt.Println("✓ Signoff validation disabled")
	} else {
		fmt.Printf("✓ Signoff configured: %s\n", signoff)
	}

	return nil
}

// promptBell prompts for bell notification configuration.
func (f *FallbackUI) promptBell(result *InitFormResult) error {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Notification Bell Configuration")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("This sends a bell character to trigger terminal notifications.")
	fmt.Println("Useful for getting notified about permission prompts.")
	fmt.Println()

	enabled, err := f.prompter.Confirm("Enable notification bell", true)
	if err != nil {
		return err
	}

	result.BellEnabled = enabled

	if enabled {
		fmt.Println("✓ Notification bell enabled")
	} else {
		fmt.Println("✓ Notification bell disabled")
	}

	return nil
}

// promptGitExclude prompts for git exclude configuration.
func (f *FallbackUI) promptGitExclude(result *InitFormResult) error {
	addToExclude, err := f.prompter.Confirm("Add config file to .git/info/exclude?", true)
	if err != nil {
		return err
	}

	result.AddToExclude = addToExclude

	return nil
}
