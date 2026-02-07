// Package tui provides terminal user interface components.
package tui

import (
	pkgConfig "github.com/smykla-labs/klaudiush/pkg/config"
)

// UI defines the interface for terminal user interface operations.
// This interface abstracts the TUI implementation to allow for both
// interactive (huh) and fallback (simple prompt) implementations.
type UI interface {
	// RunInitForm runs the initialization configuration form.
	// Returns the configured config and whether git exclude should be added.
	RunInitForm(opts InitFormOptions) (*pkgConfig.Config, bool, error)

	// IsInteractive returns true if running in an interactive terminal.
	IsInteractive() bool
}

// InitFormOptions contains options for the init form.
type InitFormOptions struct {
	// Global indicates whether this is a global or project config.
	Global bool

	// DefaultSignoff is the default signoff value from git config.
	DefaultSignoff string

	// ShowGitExclude indicates whether to show the git exclude option.
	ShowGitExclude bool
}

// InitFormResult contains the results from the init form.
type InitFormResult struct {
	// Signoff is the configured signoff value.
	Signoff string

	// BellEnabled indicates whether bell notifications are enabled.
	BellEnabled bool

	// AddToExclude indicates whether to add config to .git/info/exclude.
	AddToExclude bool
}

// FormField represents a form field with its metadata.
type FormField struct {
	// Name is the field name.
	Name string

	// Label is the display label.
	Label string

	// Description is the field description.
	Description string

	// DefaultValue is the default value.
	DefaultValue string

	// Required indicates if the field is required.
	Required bool
}
