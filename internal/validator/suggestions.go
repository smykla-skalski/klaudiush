package validator

// DefaultSuggestions maps error codes to fix suggestions.
// These hints provide actionable guidance for resolving validation failures.
var DefaultSuggestions = map[ErrorCode]string{
	// Git suggestions
	ErrGitNoSignoff:          "Add -s flag: git commit -sS -m \"message\"",
	ErrGitNoGPGSign:          "Add -S flag: git commit -sS -m \"message\"",
	ErrGitMissingFlags:       "Add -sS flags: git commit -sS -m \"message\"",
	ErrGitNoStaged:           "Stage files first: git add <files> && git commit -sS -m \"message\"",
	ErrGitBadTitle:           "Use format: type(scope): description (max 50 chars)",
	ErrGitBadBody:            "Wrap body lines at 72 characters",
	ErrGitFeatCI:             "Use ci(...) instead of feat(ci) or fix(ci)",
	ErrGitNoRemote:           "Specify remote: git push <remote> <branch>",
	ErrGitNoBranch:           "Specify branch: git push <remote> <branch>",
	ErrGitFileNotExist:       "Verify the file exists before adding",
	ErrGitPRRef:              "Remove PR reference from commit message (use in PR body instead)",
	ErrGitClaudeAttr:         "Remove Claude attribution from commit message",
	ErrGitConventionalCommit: "Use conventional commit format: type(scope): description",

	// File suggestions
	ErrShellcheck:   "Run 'shellcheck <file>' to see detailed errors",
	ErrTerraformFmt: "Run 'terraform fmt' or 'tofu fmt' to fix formatting",
	ErrTflint:       "Run 'tflint' to see detailed linting issues",
	ErrActionlint:   "Run 'actionlint' to see workflow issues",
	ErrMarkdownLint: "Check markdown formatting and structure",

	// Security suggestions
	ErrSecretsAPIKey:     "Remove API key and use environment variables or secret management",
	ErrSecretsPassword:   "Remove hardcoded password and use secret management",
	ErrSecretsPrivKey:    "Remove private key from code; use secure key storage",
	ErrSecretsToken:      "Remove token and use environment variables or secret management",
	ErrSecretsConnString: "Use environment variables for database connection strings",
}

// GetSuggestion returns the fix suggestion for an error code.
// Returns empty string if no suggestion is available.
func GetSuggestion(code ErrorCode) string {
	if suggestion, ok := DefaultSuggestions[code]; ok {
		return suggestion
	}

	return ""
}
