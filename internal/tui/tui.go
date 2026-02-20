package tui

import (
	"os"

	"golang.org/x/term"
)

// New creates a new UI instance based on terminal capabilities.
// If the terminal is interactive (TTY), it returns a HuhUI.
// Otherwise, it returns a FallbackUI for non-interactive environments.
//

func New() UI {
	if IsTerminal() {
		return NewHuhUI()
	}

	return NewFallbackUI()
}

// NewWithFallback creates a UI instance with explicit fallback preference.
// If noTUI is true, it returns a FallbackUI regardless of terminal capabilities.
//

func NewWithFallback(noTUI bool) UI {
	if noTUI {
		return NewFallbackUI()
	}

	return New()
}

// IsTerminal checks if stdin and stdout are connected to a terminal.
func IsTerminal() bool {
	//nolint:gosec // G115: file descriptors are always small positive integers; uintptr→int is safe
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
