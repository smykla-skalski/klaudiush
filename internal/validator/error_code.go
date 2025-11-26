package validator

// ErrorCode represents a unique identifier for validation errors.
// Error codes are organized by category (GIT, FILE, SEC) with numeric suffixes.
type ErrorCode string

// Git-related error codes (GIT001-GIT099).
const (
	// ErrGitNoSignoff indicates missing -s/--signoff flag.
	ErrGitNoSignoff ErrorCode = "GIT001"

	// ErrGitNoGPGSign indicates missing -S/--gpg-sign flag.
	ErrGitNoGPGSign ErrorCode = "GIT002"

	// ErrGitNoStaged indicates no files staged for commit.
	ErrGitNoStaged ErrorCode = "GIT003"

	// ErrGitBadTitle indicates commit message title issues.
	ErrGitBadTitle ErrorCode = "GIT004"

	// ErrGitBadBody indicates commit message body issues.
	ErrGitBadBody ErrorCode = "GIT005"

	// ErrGitFeatCI indicates incorrect use of feat(ci) or fix(ci).
	ErrGitFeatCI ErrorCode = "GIT006"

	// ErrGitNoRemote indicates missing remote for push.
	ErrGitNoRemote ErrorCode = "GIT007"

	// ErrGitNoBranch indicates missing branch for push.
	ErrGitNoBranch ErrorCode = "GIT008"

	// ErrGitFileNotExist indicates file does not exist for git add.
	ErrGitFileNotExist ErrorCode = "GIT009"

	// ErrGitMissingFlags indicates missing required flags on commit.
	ErrGitMissingFlags ErrorCode = "GIT010"

	// ErrGitPRRef indicates PR reference in commit message.
	ErrGitPRRef ErrorCode = "GIT011"

	// ErrGitClaudeAttr indicates Claude attribution in commit message.
	ErrGitClaudeAttr ErrorCode = "GIT012"

	// ErrGitConventionalCommit indicates invalid conventional commit format.
	ErrGitConventionalCommit ErrorCode = "GIT013"
)

// File-related error codes (FILE001-FILE099).
const (
	// ErrShellcheck indicates shellcheck validation failure.
	ErrShellcheck ErrorCode = "FILE001"

	// ErrTerraformFmt indicates terraform fmt validation failure.
	ErrTerraformFmt ErrorCode = "FILE002"

	// ErrTflint indicates tflint validation failure.
	ErrTflint ErrorCode = "FILE003"

	// ErrActionlint indicates actionlint validation failure.
	ErrActionlint ErrorCode = "FILE004"

	// ErrMarkdownLint indicates markdown linting failure.
	ErrMarkdownLint ErrorCode = "FILE005"
)

// Security-related error codes (SEC001-SEC099).
const (
	// ErrSecretsAPIKey indicates detected API key.
	ErrSecretsAPIKey ErrorCode = "SEC001"

	// ErrSecretsPassword indicates detected hardcoded password.
	ErrSecretsPassword ErrorCode = "SEC002"

	// ErrSecretsPrivKey indicates detected private key.
	ErrSecretsPrivKey ErrorCode = "SEC003"

	// ErrSecretsToken indicates detected token.
	ErrSecretsToken ErrorCode = "SEC004"

	// ErrSecretsConnString indicates detected connection string with credentials.
	ErrSecretsConnString ErrorCode = "SEC005"
)

// minCategoryLength is the minimum length for a valid error code category.
const minCategoryLength = 3

// String returns the string representation of the error code.
func (e ErrorCode) String() string {
	return string(e)
}

// Category returns the category prefix of the error code (e.g., "GIT", "FILE", "SEC").
func (e ErrorCode) Category() string {
	code := string(e)
	if len(code) < minCategoryLength {
		return ""
	}

	// Extract letters before digits
	for i, c := range code {
		if c >= '0' && c <= '9' {
			return code[:i]
		}
	}

	return code
}
