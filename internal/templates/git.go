package templates

var (
	// GitAddTmpFilesTemplate formats error message for tmp/ files in git add
	GitAddTmpFilesTemplate = Parse(
		"git_add_tmp_files",
		`Files in tmp/ should be in .gitignore or .git/info/exclude

Files being added:
{{range .Files}}  - {{.}}
{{end}}
Add tmp/ to .git/info/exclude:
  echo 'tmp/' >> .git/info/exclude`,
	)

	// GitCommitFlagsTemplate formats error message for missing -sS flags
	GitCommitFlagsTemplate = Parse(
		"git_commit_flags",
		`Git commit must use -sS flags (signoff + GPG sign)

Current command: git commit {{.ArgsStr}}
Expected: git commit -sS -m "message"`,
	)

	// GitCommitNoStagedTemplate formats error message for no staged files
	GitCommitNoStagedTemplate = Parse(
		"git_commit_no_staged",
		`No files staged for commit and no -a/-A flag specified

Current status:
  Modified files (not staged): {{.ModifiedCount}}
  Untracked files: {{.UntrackedCount}}
  Staged files: 0

Did you forget to:
  • Stage files? Run 'git add <files>' or 'git add .'
  • Use -a flag? Run 'git commit -a' to commit all modified files`,
	)

	// BranchSpaceErrorTemplate formats error for branch names with spaces
	BranchSpaceErrorTemplate = Parse("branch_space_error", `Branch name appears to contain spaces

Branch names cannot contain spaces. Use hyphens instead.

Example: feat/my-feature not feat/my feature`)

	// BranchUppercaseTemplate formats error for uppercase in branch names
	BranchUppercaseTemplate = Parse("branch_uppercase", `Branch name must be lowercase

Branch name '{{.BranchName}}' contains uppercase characters

Use: {{.LowerBranch}}`)

	// BranchPatternTemplate formats error for invalid branch name pattern
	BranchPatternTemplate = Parse("branch_pattern", `Branch name must follow type/description format

Branch name '{{.BranchName}}' doesn't match pattern

Expected format: <type>/<description>
Valid types: feat, fix, docs, style, refactor, test, chore, ci, build, perf

Example: feat/add-user-auth or fix/login-bug-123`)

	// BranchMissingPartsTemplate formats error for missing type or description
	BranchMissingPartsTemplate = Parse(
		"branch_missing_parts",
		`Branch name must contain type and description

Branch name '{{.BranchName}}' is missing type or description

Expected format: <type>/<description>`,
	)

	// BranchInvalidTypeTemplate formats error for invalid branch type
	BranchInvalidTypeTemplate = Parse("branch_invalid_type", `Invalid branch type

Branch type '{{.BranchType}}' is not valid

Valid types: {{.ValidTypesStr}}`)

	// PushRemoteNotFoundTemplate formats error for missing remote
	PushRemoteNotFoundTemplate = Parse(
		"push_remote_not_found",
		`❌ Remote '{{.Remote}}' does not exist

Available remotes:
{{range .Remotes}}  {{.Name}}  {{.URL}}
{{end}}
Use 'git remote -v' to list all configured remotes.`,
	)

	// PushKongOrgTemplate formats error for Kong org push to origin
	PushKongOrgTemplate = Parse("push_kong_org", `🚫 Git push validation failed:

❌ Kong org projects should push to 'upstream' remote (main repo)
   Note: 'origin' is your fork, use 'upstream' for Kong repos

Expected: git push upstream branch-name`)

	// PushKumaWarningTemplate formats warning for Kuma push to upstream
	PushKumaWarningTemplate = Parse(
		"push_kuma_warning",
		`⚠️  Warning: Pushing to 'upstream' remote in kumahq/kuma
   This should only be done when explicitly intended
   Normal workflow: push to 'origin' (your fork)`,
	)

	// GitNoVerifyTemplate formats error for --no-verify flag usage
	GitNoVerifyTemplate = Parse("git_no_verify", `Git commit --no-verify is not allowed

The --no-verify flag bypasses pre-commit hooks and validation.
All commits must go through proper validation.

Use: git commit -sS -m "message"`)

	// PushBlockedRemoteTemplate formats error for push to blocked remote
	PushBlockedRemoteTemplate = Parse(
		"push_blocked_remote",
		`❌ Remote '{{.Remote}}' is blocked for push operations

Blocked remotes: [{{.BlockedRemotesStr}}]
{{- if .SuggestedRemotesStr}}
Suggested alternatives: [{{.SuggestedRemotesStr}}]
{{- else if .AvailableRemotesStr}}
Available remotes: [{{.AvailableRemotesStr}}]
{{- end}}`,
	)
)

// GitAddTmpFilesData holds data for GitAddTmpFilesTemplate
type GitAddTmpFilesData struct {
	Files []string
}

// GitCommitFlagsData holds data for GitCommitFlagsTemplate
type GitCommitFlagsData struct {
	ArgsStr string
}

// GitCommitNoStagedData holds data for GitCommitNoStagedTemplate
type GitCommitNoStagedData struct {
	ModifiedCount  int
	UntrackedCount int
}

// BranchUppercaseData holds data for BranchUppercaseTemplate
type BranchUppercaseData struct {
	BranchName  string
	LowerBranch string
}

// BranchPatternData holds data for BranchPatternTemplate
type BranchPatternData struct {
	BranchName string
}

// BranchMissingPartsData holds data for BranchMissingPartsTemplate
type BranchMissingPartsData struct {
	BranchName string
}

// BranchInvalidTypeData holds data for BranchInvalidTypeTemplate
type BranchInvalidTypeData struct {
	BranchType    string
	ValidTypesStr string
}

// PushRemoteNotFoundData holds data for PushRemoteNotFoundTemplate
type PushRemoteNotFoundData struct {
	Remote  string
	Remotes []RemoteInfo
}

// RemoteInfo represents a git remote
type RemoteInfo struct {
	Name string
	URL  string
}

// PushBlockedRemoteData holds data for PushBlockedRemoteTemplate
type PushBlockedRemoteData struct {
	Remote              string
	BlockedRemotesStr   string
	SuggestedRemotesStr string
	AvailableRemotesStr string
}
