// Package notification provides validators for Claude Code notification events.
package notification

import (
	"context"
	"os"
	"os/exec"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
)

// BellValidator sends a bell character to /dev/tty for all notification events.
type BellValidator struct {
	*validator.BaseValidator
	config      *config.BellValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewBellValidator creates a new BellValidator.
func NewBellValidator(
	log logger.Logger,
	cfg *config.BellValidatorConfig,
	ruleAdapter *rules.RuleValidatorAdapter,
) *BellValidator {
	return &BellValidator{
		BaseValidator: validator.NewBaseValidator("bell", log),
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

// Validate sends a bell character to /dev/tty for any notification event.
func (v *BellValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	v.Logger().Debug("handling notification", "notification_type", hookCtx.NotificationType)

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	// Check if custom command is configured
	customCmd := v.getCustomCommand()
	if customCmd != "" {
		return v.executeCustomCommand(ctx, customCmd)
	}

	// Default behavior: send bell character
	return v.sendBell()
}

// sendBell sends a bell character (ASCII 7) to /dev/tty
func (v *BellValidator) sendBell() *validator.Result {
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

// executeCustomCommand executes the configured custom command
func (v *BellValidator) executeCustomCommand(ctx context.Context, cmdStr string) *validator.Result {
	v.Logger().Debug("executing custom notification command", "command", cmdStr)

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)

	err := cmd.Run()
	if err != nil {
		v.Logger().Debug("failed to execute custom command", "error", err)
		return validator.Pass() // Don't fail, just skip
	}

	v.Logger().Debug("executed custom notification command successfully")

	return validator.Pass()
}

// getCustomCommand returns the configured custom command.
func (v *BellValidator) getCustomCommand() string {
	if v.config != nil && v.config.CustomCommand != "" {
		return v.config.CustomCommand
	}

	return ""
}

// Category returns the validator category for parallel execution.
// BellValidator uses CategoryIO because it writes to /dev/tty or executes commands.
func (*BellValidator) Category() validator.ValidatorCategory {
	return validator.CategoryIO
}
