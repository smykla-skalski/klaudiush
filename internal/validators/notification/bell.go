// Package notification provides validators for Claude Code notification events.
package notification

import (
	"context"
	"os"

	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// BellValidator sends a bell character to /dev/tty for all notification events.
type BellValidator struct {
	*validator.BaseValidator
}

// NewBellValidator creates a new BellValidator.
func NewBellValidator(log logger.Logger) *BellValidator {
	return &BellValidator{
		BaseValidator: validator.NewBaseValidator("bell", log),
	}
}

// Validate sends a bell character to /dev/tty for any notification event.
func (v *BellValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	v.Logger().Debug("handling notification", "notification_type", hookCtx.NotificationType)

	// Try to open /dev/tty
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		v.Logger().Debug("failed to open /dev/tty", "error", err)
		return validator.Pass() // Don't fail, just skip
	}

	defer func() {
		if closeErr := tty.Close(); closeErr != nil {
			v.Logger().Debug("failed to close /dev/tty", "error", closeErr)
		}
	}()

	// Write bell character (ASCII 7)
	_, err = tty.Write([]byte{7})
	if err != nil {
		v.Logger().Debug("failed to write bell to /dev/tty", "error", err)
		return validator.Pass() // Don't fail, just skip
	}

	v.Logger().Debug("sent bell to /dev/tty")

	return validator.Pass()
}
