package git

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-labs/klaudiush/internal/rules"
	"github.com/smykla-labs/klaudiush/internal/templates"
	"github.com/smykla-labs/klaudiush/internal/validator"
	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/logger"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

// BranchValidator validates git branch names.
type BranchValidator struct {
	validator.BaseValidator
	config      *config.BranchValidatorConfig
	ruleAdapter *rules.RuleValidatorAdapter
}

// NewBranchValidator creates a new BranchValidator.
func NewBranchValidator(
	cfg *config.BranchValidatorConfig,
	log logger.Logger,
	ruleAdapter *rules.RuleValidatorAdapter,
) *BranchValidator {
	return &BranchValidator{
		BaseValidator: *validator.NewBaseValidator("validate-branch-name", log),
		config:        cfg,
		ruleAdapter:   ruleAdapter,
	}
}

const (
	// minBranchParts is the minimum number of parts in a valid branch name.
	minBranchParts = 2
)

var (
	// Default protected branches that should skip validation.
	defaultProtectedBranches = []string{"main", "master"}

	// Default valid branch types.
	defaultValidBranchTypes = []string{
		"feat", "fix", "docs", "style", "refactor",
		"test", "chore", "ci", "build", "perf",
	}

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

	return defaultProtectedBranches
}

// getValidTypes returns the list of valid branch types
func (v *BranchValidator) getValidTypes() []string {
	if v.config != nil && len(v.config.ValidTypes) > 0 {
		return v.config.ValidTypes
	}

	return defaultValidBranchTypes
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

	// Check rules first if rule adapter is configured
	if v.ruleAdapter != nil {
		if result := v.ruleAdapter.CheckRules(ctx, hookCtx); result != nil {
			return result
		}
	}

	bashParser := parser.NewBashParser()

	parseResult, err := bashParser.Parse(hookCtx.ToolInput.Command)
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

// extractBranchName extracts the branch name from a git command.
func (v *BranchValidator) extractBranchName(gitCmd *parser.GitCommand) string {
	switch gitCmd.Subcommand {
	case "checkout":
		return v.extractCheckoutBranchName(gitCmd)
	case "branch":
		return v.extractBranchCommandName(gitCmd)
	case "switch":
		return v.extractSwitchBranchName(gitCmd)
	default:
		return ""
	}
}

// extractCheckoutBranchName extracts the branch name from git checkout -b <branch> [start-point].
// The bash parser handles quoted strings, preserving spaces in a single argument.
func (*BranchValidator) extractCheckoutBranchName(gitCmd *parser.GitCommand) string {
	for _, flag := range checkoutCreateFlags {
		for i, f := range gitCmd.Flags {
			if f == flag && i+1 < len(gitCmd.Flags) {
				return gitCmd.Flags[i+1]
			}
		}
	}

	if len(gitCmd.Args) > 0 {
		return gitCmd.Args[0]
	}

	return ""
}

// extractBranchCommandName extracts the branch name from git branch <branch> [start-point].
// The bash parser handles quoted strings, preserving spaces in a single argument.
func (*BranchValidator) extractBranchCommandName(gitCmd *parser.GitCommand) string {
	if len(gitCmd.Args) > 0 {
		return gitCmd.Args[0]
	}

	return ""
}

// extractSwitchBranchName extracts the branch name from git switch -c <branch> [start-point].
// The bash parser handles quoted strings, preserving spaces in a single argument.
func (*BranchValidator) extractSwitchBranchName(gitCmd *parser.GitCommand) string {
	for _, flag := range switchCreateFlags {
		for i, f := range gitCmd.Flags {
			if f == flag && i+1 < len(gitCmd.Flags) {
				return gitCmd.Flags[i+1]
			}
		}
	}

	if len(gitCmd.Args) > 0 {
		return gitCmd.Args[0]
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

		return validator.FailWithRef(validator.RefGitBranchName, message)
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
