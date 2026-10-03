package validator

import (
	"strconv"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// DefaultSuggestions maps references to fix suggestions.
// These hints provide actionable guidance for resolving validation failures.
//
//nolint:gosec // G101: strings contain command examples with flag names, not hardcoded credentials
var DefaultSuggestions = map[Reference]string{
	// Git suggestions
	RefGitNoSignoff:          "Add -s flag: git commit -sS -m \"message\"",
	RefGitNoGPGSign:          "Add -S flag: git commit -sS -m \"message\"",
	RefGitMissingFlags:       "Add -sS flags to your command, keeping ALL existing arguments. Example: git commit -sS -m \"your message\"",
	RefGitNoStaged:           "Stage specific files with git add <files> (check git status first), then retry the commit",
	RefGitBadTitle:           titleLengthSuggestion(config.DefaultTitleMaxLength),
	RefGitBadBody:            bodyLineSuggestion(config.DefaultBodyMaxLineLength),
	RefGitFeatCI:             "Use ci(...) instead of feat(ci) or fix(ci)",
	RefGitNoRemote:           "Specify remote: git push <remote> <branch>",
	RefGitNoBranch:           "Specify branch: git push <remote> <branch>",
	RefGitFileNotExist:       "Verify the file exists before adding",
	RefGitPRRef:              "Remove PR reference from commit message (use in PR body instead)",
	RefGitClaudeAttr:         "Remove AI attribution from the commit message or PR description",
	RefGitConventionalCommit: conventionalSuggestion(config.DefaultTitleMaxLength),
	RefGitForbiddenPattern:   "Remove forbidden pattern from commit message",
	RefGitSignoffMismatch:    "Use correct signoff identity: git config user.name and user.email",
	RefGitListFormat:         "Add empty line before list items in commit body",
	RefGitMergeMessage:       "Fix PR title/body to follow commit message conventions before merge",
	RefGitMergeSignoff:       "Add --body flag with Signed-off-by trailer to gh pr merge command",
	RefGitBlockedFiles:       "Remove blocked files from your git add command. Do not stage these files.",
	RefGitBranchName:         "Use lowercase kebab-case for branch names (e.g., feat/my-feature)",
	RefGitNoVerify:           "Remove --no-verify flag and fix any pre-commit hook issues",
	RefGitKongOrgPush:        "Push to 'upstream' remote instead: git push upstream <branch>",
	RefGitPRValidation:       "Fix the issue and retry gh pr create",
	RefGitFetchNoRemote:      "Specify valid remote: git fetch <remote> (use 'git remote -v' to list remotes)",
	RefGitBlockedRemote:      "Use an allowed remote for push",
	RefGitBlockedBranch:      "Push to a different branch or create a PR",

	// File suggestions
	RefShellcheck:   "Run 'shellcheck <file>' to see detailed errors",
	RefTerraformFmt: "Run 'terraform fmt' or 'tofu fmt' to fix formatting",
	RefTflint:       "Run 'tflint' to see detailed linting issues",
	RefActionlint:   "Run 'actionlint' to see workflow issues",
	RefMarkdownLint: "Fix the formatting issue and retry",
	RefGofumpt:      "Run 'gofumpt -w <file>' to auto-fix formatting",
	RefRuffCheck:    "Run 'ruff check <file>' to see Python code quality issues",
	RefOxlintCheck:  "Run 'oxlint <file>' to see JavaScript/TypeScript code quality issues",
	RefRustfmtCheck: "Run 'rustfmt <file>' to auto-fix formatting",
	RefLinterIgnore: "Fix linter errors properly instead of suppressing them with ignore directives",
	RefAIComments:   "Remove the comment or replace it with one that explains why, not what",

	// Security suggestions
	RefSecretsAPIKey:     "Remove API key and use environment variables or secret management",
	RefSecretsPassword:   "Remove hardcoded password and use secret management",
	RefSecretsPrivKey:    "Remove private key from code; use secure key storage",
	RefSecretsToken:      "Remove token and use environment variables or secret management",
	RefSecretsConnString: "Use environment variables for database connection strings",

	// Shell suggestions
	RefShellBackticks: "Use HEREDOC syntax or file-based input (git commit -F file.txt)",
	RefShellNesting:   "Fix the shell syntax, or run the inner command directly instead of through nested launchers, scripts or aliases",

	// GitHub CLI suggestions
	RefGHIssueValidation: "Fix markdown formatting in issue body (empty lines around headings, proper list spacing)",
	RefGHAPICommit:       "Clone the repository, stage the files, and run git commit -sS instead of creating the commit through the GitHub API",
	RefGHAPIUnverifiable: "Spell the endpoint literally instead of building it from a variable, or pass the GraphQL query inline with -f query=...",

	// Hook operation suggestions
	RefValidationUnavailable: "Ask the user to run 'klaudiush doctor' and fix the cause this message names",
	RefEvidenceMissing:       "Run the required check against the current files and let it finish before stopping",
	RefToolPhaseLocked:       "Satisfy the prerequisite checks with klaudiush evidence run before changing files",

	// MCP Elicitation suggestions
	RefMCPServerBlocked:    "Remove MCP server from deny list or use a different server",
	RefMCPServerNotAllowed: "Add MCP server to allow list in config",
	RefMCPURLModeBlocked:   "Use form mode instead of URL mode for MCP elicitation",
	RefMCPUntrustedSource: "Use a tool from a trusted MCP server, or ask the user to trust this " +
		"server in mcp_trust",
	RefMCPUnknownProvenance: "Ask the user to update the harness so it reports MCP provenance, " +
		"or to set mcp_trust.unknown_provenance",

	// Policy protection suggestions
	RefProtectedFile: "Leave policy files alone; if the change is needed, ask the user to make it " +
		"or to add the path to protection.allow",
	RefConfigChangeBlocked: "Restart the session to apply the settings change, or remove the source " +
		"from protection.config_change_sources",
	RefPolicyCommand: "Ask the user to run this klaudiush command themselves",
}

// GetSuggestion returns the fix suggestion for a reference.
// Returns empty string if no suggestion is available.
func GetSuggestion(ref Reference) string {
	if suggestion, ok := DefaultSuggestions[ref]; ok {
		return suggestion
	}

	return ""
}

// MessageLimits carries the effective commit message limits that fix hints
// quote. A zero field falls back to the built-in default.
type MessageLimits struct {
	TitleMaxLength    int
	BodyMaxLineLength int
}

// GetSuggestionWithLimits returns the fix suggestion for a reference, quoting
// the given limits instead of the defaults where the hint names one.
func GetSuggestionWithLimits(ref Reference, limits MessageLimits) string {
	title := limits.TitleMaxLength
	if title <= 0 {
		title = config.DefaultTitleMaxLength
	}

	body := limits.BodyMaxLineLength
	if body <= 0 {
		body = config.DefaultBodyMaxLineLength
	}

	switch ref {
	case RefGitBadTitle:
		return titleLengthSuggestion(title)
	case RefGitBadBody:
		return bodyLineSuggestion(body)
	case RefGitConventionalCommit:
		return conventionalSuggestion(title)
	default:
		return GetSuggestion(ref)
	}
}

func titleLengthSuggestion(maxLength int) string {
	return "Shorten title to max " + strconv.Itoa(maxLength) +
		" chars total including type(scope): prefix"
}

func bodyLineSuggestion(maxLength int) string {
	return "Wrap body lines at " + strconv.Itoa(maxLength) + " characters"
}

func conventionalSuggestion(maxLength int) string {
	return "Use format: type(scope): description (total title at most " +
		strconv.Itoa(maxLength) + " chars)"
}
