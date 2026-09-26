package git

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/templates"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// BranchValidator validates git branch names.
type BranchValidator struct {
	validator.BaseValidator
	config *config.BranchValidatorConfig
}

// NewBranchValidator creates a new BranchValidator.
func NewBranchValidator(
	cfg *config.BranchValidatorConfig,
	log logger.Logger,
	ruleAdapter validator.RuleChecker,
) *BranchValidator {
	return &BranchValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			"validate-branch-name", log, ruleAdapter,
		),
		config: cfg,
	}
}

const (
	// minBranchParts is the minimum number of parts in a valid branch name.
	minBranchParts = 2
)

var (
	// Branch creation flags for git checkout.
	checkoutCreateFlags = []string{"-b", "--branch"}

	// Branch creation flags for git switch.
	switchCreateFlags = []string{"-c", "--create", "-C", "--force-create"}

	// Branch deletion flags for git branch.
	branchDeleteFlags = []string{"-d", "-D", "--delete"}

	// Branch query/list flags for git branch (non-creation operations).
	branchQueryFlags = []string{
		// List flags
		"-a", "--all",
		"-r", "--remotes",
		"-l", "--list",

		// Query/filter flags
		"--contains",
		"--no-contains",
		"--merged",
		"--no-merged",
		"--points-at",

		// Output formatting and verbosity
		"-v", "--verbose", "-vv",
		"--sort",
		"--format",
		"--show-current",
		"--column",
		"--no-column",

		// Modify flags (rename/copy)
		"-m", "-M", "--move",
		"-c", "-C", "--copy",
	}
)

// getProtectedBranches returns the list of protected branches
func (v *BranchValidator) getProtectedBranches() []string {
	if v.config != nil && len(v.config.ProtectedBranches) > 0 {
		return v.config.ProtectedBranches
	}

	return config.DefaultProtectedBranches
}

// getValidTypes returns the list of valid branch types
func (v *BranchValidator) getValidTypes() []string {
	if v.config != nil && len(v.config.ValidTypes) > 0 {
		return v.config.ValidTypes
	}

	return config.DefaultValidBranchTypes
}

// isRequireType returns whether type/description format is required
func (v *BranchValidator) isRequireType() bool {
	if v.config != nil && v.config.RequireType != nil {
		return *v.config.RequireType
	}

	return true // default: required
}

// isAllowUppercase returns whether uppercase letters are allowed
func (v *BranchValidator) isAllowUppercase() bool {
	if v.config != nil && v.config.AllowUppercase != nil {
		return *v.config.AllowUppercase
	}

	return false // default: not allowed
}

// Validate validates git branch names.
func (v *BranchValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("validating git branch command")

	// Check rules first
	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	parseResult, err := hookCtx.ParsedCommand()
	if err != nil {
		log.Error("failed to parse command", "error", err)
		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	for _, cmd := range parseResult.Commands {
		if cmd.Name != "git" {
			continue
		}

		gitCmd, err := parser.ParseGitCommand(cmd)
		if err != nil {
			v.Logger().Debug("failed to parse git command", "error", err)
			continue
		}

		result := v.validateGitCommand(gitCmd)
		if result != nil && !result.Passed {
			return result
		}
	}

	return validator.Pass()
}

// validateGitCommand validates a git command based on its subcommand.
func (v *BranchValidator) validateGitCommand(gitCmd *parser.GitCommand) *validator.Result {
	switch gitCmd.Subcommand {
	case "checkout":
		return v.validateCheckout(gitCmd)
	case "branch":
		return v.validateBranch(gitCmd)
	case "switch":
		return v.validateSwitch(gitCmd)
	default:
		return nil
	}
}

// validateCheckout validates git checkout -b/--branch commands that create new branches.
// Skips validation for commands without branch creation flags.
func (v *BranchValidator) validateCheckout(gitCmd *parser.GitCommand) *validator.Result {
	if !hasAnyFlag(gitCmd, checkoutCreateFlags) {
		return nil
	}

	return v.validateBranchCreation(gitCmd)
}

// validateBranch validates git branch commands that create new branches.
// Skips validation for delete operations and query/list operations.
func (v *BranchValidator) validateBranch(gitCmd *parser.GitCommand) *validator.Result {
	if hasAnyFlag(gitCmd, branchDeleteFlags) {
		return nil
	}

	if hasAnyFlag(gitCmd, branchQueryFlags) {
		return nil
	}

	return v.validateBranchCreation(gitCmd)
}

// validateSwitch validates git switch -c/--create/-C/--force-create commands that create new branches.
// Skips validation for commands without branch creation flags.
func (v *BranchValidator) validateSwitch(gitCmd *parser.GitCommand) *validator.Result {
	if !hasAnyFlag(gitCmd, switchCreateFlags) {
		return nil
	}

	return v.validateBranchCreation(gitCmd)
}

// validateBranchCreation performs the common validation logic for branch creation commands.
// Validates branch name format and checks for spaces.
func (v *BranchValidator) validateBranchCreation(gitCmd *parser.GitCommand) *validator.Result {
	branchName := v.extractBranchName(gitCmd)
	if branchName == "" {
		return nil
	}

	// A bare variable branch name (e.g. "git checkout -b $BRANCH") is an
	// unresolved expansion whose runtime value the hook cannot see, so its
	// format can't be checked. Skip rather than block.
	if isBareExpansion(branchName) {
		v.Logger().
			Debug("branch name is an unresolved variable; skipping validation", "branch", branchName)

		return nil
	}

	if strings.Contains(branchName, " ") {
		return v.createSpaceError()
	}

	return v.validateBranchName(branchName)
}

// createSpaceError creates an error for branch names with spaces.
func (*BranchValidator) createSpaceError() *validator.Result {
	message := templates.MustExecute(templates.BranchSpaceErrorTemplate, nil)
	return validator.FailWithRef(validator.RefGitBranchName, message)
}

// extractBranchName extracts the new branch name from a branch-creation command.
//
// For checkout and switch the parser captures the name as the creation flag's
// value (-b/--branch, -c/--create, -C/--force-create), so it is read from there
// - this both ignores trailing options like "--quiet" in
// "git checkout -b fix/foo main --quiet" and still catches a dash-prefixed name
// such as "git checkout -b --quiet". For git branch the new name is the first
// positional argument (any start-point follows it). The bash parser preserves
// quoted spaces within a single argument.
func (*BranchValidator) extractBranchName(gitCmd *parser.GitCommand) string {
	switch gitCmd.Subcommand {
	case "checkout", "switch":
		return gitCmd.ExtractBranchName()
	case "branch":
		if len(gitCmd.Args) > 0 {
			return gitCmd.Args[0]
		}
	}

	return ""
}

// hasAnyFlag checks if the git command has any of the flags in the provided list.
func hasAnyFlag(gitCmd *parser.GitCommand, flags []string) bool {
	return slices.ContainsFunc(flags, func(flag string) bool {
		return gitCmd.HasFlag(flag)
	})
}

// validateBranchName validates the branch name format (type/description).
// Skips validation for protected branches.
func (v *BranchValidator) validateBranchName(branchName string) *validator.Result {
	protectedBranches := v.getProtectedBranches()
	if slices.Contains(protectedBranches, branchName) {
		v.Logger().Debug("skipping protected branch", "branch", branchName)
		return validator.Pass()
	}

	allowUppercase := v.isAllowUppercase()
	if !allowUppercase && branchName != strings.ToLower(branchName) {
		message := templates.MustExecute(
			templates.BranchUppercaseTemplate,
			templates.BranchUppercaseData{
				BranchName:  branchName,
				LowerBranch: strings.ToLower(branchName),
			},
		)

		return validator.FailWithRef(validator.RefGitBranchName, message).
			WithFixHint("Use: " + strings.ToLower(branchName))
	}

	requireType := v.isRequireType()
	//nolint:nestif // Acceptable complexity for branch name validation
	if requireType {
		// Build pattern based on allow uppercase config
		var branchNamePattern *regexp.Regexp
		if allowUppercase {
			branchNamePattern = regexp.MustCompile(`^[a-zA-Z]+/[a-zA-Z0-9-]+$`)
		} else {
			branchNamePattern = regexp.MustCompile(`^[a-z]+/[a-z0-9-]+$`)
		}

		if !branchNamePattern.MatchString(branchName) {
			message := templates.MustExecute(
				templates.BranchPatternTemplate,
				templates.BranchPatternData{
					BranchName: branchName,
				},
			)

			return validator.FailWithRef(validator.RefGitBranchName, message)
		}

		parts := strings.SplitN(branchName, "/", minBranchParts)
		if len(parts) != minBranchParts {
			message := templates.MustExecute(
				templates.BranchMissingPartsTemplate,
				templates.BranchMissingPartsData{
					BranchName: branchName,
				},
			)

			return validator.FailWithRef(validator.RefGitBranchName, message)
		}

		branchType := parts[0]
		validTypes := v.getValidTypes()

		// Convert to lowercase for comparison if uppercase is allowed
		compareType := branchType
		if allowUppercase {
			compareType = strings.ToLower(branchType)
		}

		// Check if type is valid
		typeValid := false

		for _, t := range validTypes {
			if compareType == strings.ToLower(t) {
				typeValid = true
				break
			}
		}

		if !typeValid {
			message := templates.MustExecute(
				templates.BranchInvalidTypeTemplate,
				templates.BranchInvalidTypeData{
					BranchType:    branchType,
					ValidTypesStr: strings.Join(validTypes, ", "),
				},
			)

			return validator.FailWithRef(validator.RefGitBranchName, message)
		}
	}

	return validator.Pass()
}
