package validator

// DefaultSuggestions maps references to fix suggestions.
// These hints provide actionable guidance for resolving validation failures.
var DefaultSuggestions = map[Reference]string{
	// Git suggestions
	RefGitNoSignoff:          "Add -s flag: git commit -sS -m \"message\"",
	RefGitNoGPGSign:          "Add -S flag: git commit -sS -m \"message\"",
	RefGitMissingFlags:       "Add -sS flags: git commit -sS -m \"message\"",
	RefGitNoStaged:           "Stage files first: git add <files> && git commit -sS -m \"message\"",
	RefGitBadTitle:           "Use format: type(scope): description (max 50 chars)",
	RefGitBadBody:            "Wrap body lines at 72 characters",
	RefGitFeatCI:             "Use ci(...) instead of feat(ci) or fix(ci)",
	RefGitNoRemote:           "Specify remote: git push <remote> <branch>",
	RefGitNoBranch:           "Specify branch: git push <remote> <branch>",
	RefGitFileNotExist:       "Verify the file exists before adding",
	RefGitPRRef:              "Remove PR reference from commit message (use in PR body instead)",
	RefGitClaudeAttr:         "Remove Claude attribution from commit message",
	RefGitConventionalCommit: "Use conventional commit format: type(scope): description",
	RefGitForbiddenPattern:   "Remove forbidden pattern from commit message",
	RefGitSignoffMismatch:    "Use correct signoff identity: git config user.name and user.email",
	RefGitListFormat:         "Add empty line before list items in commit body",

	// File suggestions
	RefShellcheck:   "Run 'shellcheck <file>' to see detailed errors",
	RefTerraformFmt: "Run 'terraform fmt' or 'tofu fmt' to fix formatting",
	RefTflint:       "Run 'tflint' to see detailed linting issues",
	RefActionlint:   "Run 'actionlint' to see workflow issues",
	RefMarkdownLint: "Check markdown formatting and structure",

	// Security suggestions
	RefSecretsAPIKey:     "Remove API key and use environment variables or secret management",
	RefSecretsPassword:   "Remove hardcoded password and use secret management",
	RefSecretsPrivKey:    "Remove private key from code; use secure key storage",
	RefSecretsToken:      "Remove token and use environment variables or secret management",
	RefSecretsConnString: "Use environment variables for database connection strings",
}

// GetSuggestion returns the fix suggestion for a reference.
// Returns empty string if no suggestion is available.
func GetSuggestion(ref Reference) string {
	if suggestion, ok := DefaultSuggestions[ref]; ok {
		return suggestion
	}

	return ""
}
