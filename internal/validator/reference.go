package validator

import "strings"

// Reference is a URL that uniquely identifies a validation error.
// Format: https://klaudiu.sh/{CODE} where CODE is like GIT001, FILE001, SEC001.
type Reference string

// ReferenceBaseURL is the base URL for error references.
const ReferenceBaseURL = "https://klaudiu.sh"

// Git-related references (GIT001-GIT025).
const (
	// RefGitNoSignoff indicates missing -s/--signoff flag.
	RefGitNoSignoff Reference = ReferenceBaseURL + "/GIT001"

	// RefGitNoGPGSign indicates missing -S/--gpg-sign flag.
	RefGitNoGPGSign Reference = ReferenceBaseURL + "/GIT002"

	// RefGitNoStaged indicates no files staged for commit.
	RefGitNoStaged Reference = ReferenceBaseURL + "/GIT003"

	// RefGitBadTitle indicates commit message title issues.
	RefGitBadTitle Reference = ReferenceBaseURL + "/GIT004"

	// RefGitBadBody indicates commit message body issues.
	RefGitBadBody Reference = ReferenceBaseURL + "/GIT005"

	// RefGitFeatCI indicates incorrect use of feat(ci) or fix(ci).
	RefGitFeatCI Reference = ReferenceBaseURL + "/GIT006"

	// RefGitNoRemote indicates missing remote for push.
	RefGitNoRemote Reference = ReferenceBaseURL + "/GIT007"

	// RefGitNoBranch indicates missing branch for push.
	RefGitNoBranch Reference = ReferenceBaseURL + "/GIT008"

	// RefGitFileNotExist indicates file does not exist for git add.
	RefGitFileNotExist Reference = ReferenceBaseURL + "/GIT009"

	// RefGitMissingFlags indicates missing required flags on commit.
	RefGitMissingFlags Reference = ReferenceBaseURL + "/GIT010"

	// RefGitPRRef indicates PR reference in commit message.
	RefGitPRRef Reference = ReferenceBaseURL + "/GIT011"

	// RefGitClaudeAttr indicates Claude attribution in commit message.
	RefGitClaudeAttr Reference = ReferenceBaseURL + "/GIT012"

	// RefGitConventionalCommit indicates invalid conventional commit format.
	RefGitConventionalCommit Reference = ReferenceBaseURL + "/GIT013"

	// RefGitForbiddenPattern indicates forbidden pattern in commit message.
	RefGitForbiddenPattern Reference = ReferenceBaseURL + "/GIT014"

	// RefGitSignoffMismatch indicates signoff identity mismatch.
	RefGitSignoffMismatch Reference = ReferenceBaseURL + "/GIT015"

	// RefGitListFormat indicates list formatting issues in commit body.
	RefGitListFormat Reference = ReferenceBaseURL + "/GIT016"

	// RefGitMergeMessage indicates merge commit message validation failure.
	RefGitMergeMessage Reference = ReferenceBaseURL + "/GIT017"

	// RefGitMergeSignoff indicates missing signoff in merge commit body.
	RefGitMergeSignoff Reference = ReferenceBaseURL + "/GIT018"

	// RefGitBlockedFiles indicates attempting to add blocked files (e.g., tmp/*).
	RefGitBlockedFiles Reference = ReferenceBaseURL + "/GIT019"

	// RefGitBranchName indicates branch naming violations (spaces, uppercase, patterns).
	RefGitBranchName Reference = ReferenceBaseURL + "/GIT020"

	// RefGitNoVerify indicates --no-verify flag is not allowed.
	RefGitNoVerify Reference = ReferenceBaseURL + "/GIT021"

	// RefGitKongOrgPush indicates Kong org push to origin remote is blocked.
	RefGitKongOrgPush Reference = ReferenceBaseURL + "/GIT022"

	// RefGitPRValidation indicates PR validation failure (title, body, markdown, or labels).
	RefGitPRValidation Reference = ReferenceBaseURL + "/GIT023"

	// RefGitFetchNoRemote indicates remote does not exist for git fetch.
	RefGitFetchNoRemote Reference = ReferenceBaseURL + "/GIT024"

	// RefGitBlockedRemote indicates push to a blocked remote.
	RefGitBlockedRemote Reference = ReferenceBaseURL + "/GIT025"
)

// File-related references (FILE001-FILE009).
const (
	// RefShellcheck indicates shellcheck validation failure.
	RefShellcheck Reference = ReferenceBaseURL + "/FILE001"

	// RefTerraformFmt indicates terraform fmt validation failure.
	RefTerraformFmt Reference = ReferenceBaseURL + "/FILE002"

	// RefTflint indicates tflint validation failure.
	RefTflint Reference = ReferenceBaseURL + "/FILE003"

	// RefActionlint indicates actionlint validation failure.
	RefActionlint Reference = ReferenceBaseURL + "/FILE004"

	// RefMarkdownLint indicates markdown linting failure.
	RefMarkdownLint Reference = ReferenceBaseURL + "/FILE005"

	// RefGofumpt indicates gofumpt formatting failure.
	RefGofumpt Reference = ReferenceBaseURL + "/FILE006"

	// RefRuffCheck indicates ruff Python validation failure.
	RefRuffCheck Reference = ReferenceBaseURL + "/FILE007"

	// RefOxlintCheck indicates oxlint JavaScript/TypeScript validation failure.
	RefOxlintCheck Reference = ReferenceBaseURL + "/FILE008"

	// RefRustfmtCheck indicates rustfmt Rust code formatting failure.
	RefRustfmtCheck Reference = ReferenceBaseURL + "/FILE009"
)

// Security-related references (SEC001-SEC005).
const (
	// RefSecretsAPIKey indicates detected API key.
	RefSecretsAPIKey Reference = ReferenceBaseURL + "/SEC001"

	// RefSecretsPassword indicates detected hardcoded password.
	RefSecretsPassword Reference = ReferenceBaseURL + "/SEC002"

	// RefSecretsPrivKey indicates detected private key.
	RefSecretsPrivKey Reference = ReferenceBaseURL + "/SEC003"

	// RefSecretsToken indicates detected token.
	RefSecretsToken Reference = ReferenceBaseURL + "/SEC004"

	// RefSecretsConnString indicates detected connection string with credentials.
	RefSecretsConnString Reference = ReferenceBaseURL + "/SEC005"
)

// Shell-related references (SHELL001-SHELL005).
const (
	// RefShellBackticks indicates unescaped backticks in double-quoted strings.
	RefShellBackticks Reference = ReferenceBaseURL + "/SHELL001"
)

// GitHub CLI-related references (GH001-GH005).
const (
	// RefGHIssueValidation indicates gh issue create validation failure (body markdown).
	RefGHIssueValidation Reference = ReferenceBaseURL + "/GH001"
)

// Plugin-related references (PLUG001-PLUG005).
const (
	// RefPluginPathTraversal indicates path traversal detected in plugin path.
	RefPluginPathTraversal Reference = ReferenceBaseURL + "/PLUG001"

	// RefPluginPathNotAllowed indicates plugin path not in allowed directory.
	RefPluginPathNotAllowed Reference = ReferenceBaseURL + "/PLUG002"

	// RefPluginInvalidExtension indicates invalid plugin file extension.
	RefPluginInvalidExtension Reference = ReferenceBaseURL + "/PLUG003"

	// RefPluginInsecureRemote indicates insecure connection to remote gRPC plugin.
	RefPluginInsecureRemote Reference = ReferenceBaseURL + "/PLUG004"

	// RefPluginDangerousChars indicates dangerous characters in plugin path.
	RefPluginDangerousChars Reference = ReferenceBaseURL + "/PLUG005"
)

// Session-related references (SESS001-SESS005).
const (
	// RefSessionPoisoned indicates the session has been poisoned by a previous blocking error.
	RefSessionPoisoned Reference = ReferenceBaseURL + "/SESS001"
)

// minCodeLength is the minimum length for a valid reference code.
const minCodeLength = 3

// String returns the URL string.
func (r Reference) String() string {
	return string(r)
}

// Code extracts the error code from the URL.
// Example: "GIT001" from "https://klaudiu.sh/GIT001".
func (r Reference) Code() string {
	s := string(r)
	if idx := strings.LastIndex(s, "/"); idx != -1 {
		return s[idx+1:]
	}

	return s
}

// Category returns the category prefix of the reference (e.g., "GIT", "FILE", "SEC").
func (r Reference) Category() string {
	code := r.Code()
	if len(code) < minCodeLength {
		return ""
	}

	for i, c := range code {
		if c >= '0' && c <= '9' {
			return code[:i]
		}
	}

	return code
}
