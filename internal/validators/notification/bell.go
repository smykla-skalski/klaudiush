// Package notification provides validators for Claude Code notification events.
package notification

import (
	"context"
	"os"

	"github.com/smykla-labs/claude-hooks/internal/validator"
	"github.com/smykla-labs/claude-hooks/pkg/hook"
	"github.com/smykla-labs/claude-hooks/pkg/logger"
)

// BellValidator sends a bell character to /dev/tty when Claude Code sends a notification.
type BellValidator struct {
	*validator.BaseValidator
}

// NewBellValidator creates a new BellValidator.
func NewBellValidator(log logger.Logger) *BellValidator {
	return &BellValidator{
		BaseValidator: validator.NewBaseValidator("bell", log),
	}
}

// Validate sends a bell character to /dev/tty if the notification type is "bell".
func (v *BellValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	v.Logger().Debug("validating notification", "notification_type", hookCtx.NotificationType)

	// Only handle bell notifications
	if hookCtx.NotificationType != "bell" {
		return validator.Pass()
	}

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
