package git

import (
	"context"

	"github.com/smykla-labs/klaudiush/internal/templates"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

// NoVerifyValidator validates that git commit commands don't use --no-verify flag
type NoVerifyValidator struct {
	validator.BaseValidator
	config *config.NoVerifyValidatorConfig
}

// NewNoVerifyValidator creates a new NoVerifyValidator instance
func NewNoVerifyValidator(
	log logger.Logger,
	cfg *config.NoVerifyValidatorConfig,
) *NoVerifyValidator {
	return &NoVerifyValidator{
		BaseValidator: *validator.NewBaseValidator("validate-no-verify", log),
		config:        cfg,
	}
}

// Validate checks if git commit command contains --no-verify flag
func (v *NoVerifyValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running no-verify validation")

	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(hookCtx.GetCommand())
	if err != nil {
		log.Error("Failed to parse command", "error", err)
		return validator.Warn("Failed to parse command")
	}

	for _, cmd := range result.Commands {
		if cmd.Name != gitCommand || len(cmd.Args) == 0 || cmd.Args[0] != commitSubcommand {
			continue
		}

		gitCmd, err := parser.ParseGitCommand(cmd)
		if err != nil {
			log.Error("Failed to parse git command", "error", err)
			continue
		}

		if gitCmd.HasFlag("--no-verify") || gitCmd.HasFlag("-n") {
			message := templates.MustExecute(templates.GitNoVerifyTemplate, nil)

			return validator.FailWithRef(
				validator.RefGitNoVerify,
				"Git commit --no-verify is not allowed",
			).AddDetail("help", message)
		}
	}

	log.Debug("No --no-verify flag found")

	return validator.Pass()
}

// Ensure NoVerifyValidator implements validator.Validator
var _ validator.Validator = (*NoVerifyValidator)(nil)
