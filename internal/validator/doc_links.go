package validator

// BaseDocURL is the base URL for documentation.
// Can be customized via configuration.
const BaseDocURL = "https://github.com/smykla-labs/klaudiush/blob/main/docs/errors"

// DefaultDocLinks maps error codes to documentation URLs.
// These links provide detailed explanations and examples.
var DefaultDocLinks = map[ErrorCode]string{
	// Git documentation
	ErrGitNoSignoff:          BaseDocURL + "/GIT001.md",
	ErrGitNoGPGSign:          BaseDocURL + "/GIT002.md",
	ErrGitNoStaged:           BaseDocURL + "/GIT003.md",
	ErrGitBadTitle:           BaseDocURL + "/GIT004.md",
	ErrGitBadBody:            BaseDocURL + "/GIT005.md",
	ErrGitFeatCI:             BaseDocURL + "/GIT006.md",
	ErrGitNoRemote:           BaseDocURL + "/GIT007.md",
	ErrGitNoBranch:           BaseDocURL + "/GIT008.md",
	ErrGitFileNotExist:       BaseDocURL + "/GIT009.md",
	ErrGitMissingFlags:       BaseDocURL + "/GIT010.md",
	ErrGitPRRef:              BaseDocURL + "/GIT011.md",
	ErrGitClaudeAttr:         BaseDocURL + "/GIT012.md",
	ErrGitConventionalCommit: BaseDocURL + "/GIT013.md",

	// File documentation
	ErrShellcheck:   BaseDocURL + "/FILE001.md",
	ErrTerraformFmt: BaseDocURL + "/FILE002.md",
	ErrTflint:       BaseDocURL + "/FILE003.md",
	ErrActionlint:   BaseDocURL + "/FILE004.md",
	ErrMarkdownLint: BaseDocURL + "/FILE005.md",

	// Security documentation
	ErrSecretsAPIKey:     BaseDocURL + "/SEC001.md",
	ErrSecretsPassword:   BaseDocURL + "/SEC002.md",
	ErrSecretsPrivKey:    BaseDocURL + "/SEC003.md",
	ErrSecretsToken:      BaseDocURL + "/SEC004.md",
	ErrSecretsConnString: BaseDocURL + "/SEC005.md",
}

// GetDocLink returns the documentation URL for an error code.
// Returns empty string if no doc link is available.
func GetDocLink(code ErrorCode) string {
	if link, ok := DefaultDocLinks[code]; ok {
		return link
	}

	return ""
}
