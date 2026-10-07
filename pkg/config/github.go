// Package config provides configuration schema types for klaudiush validators.
package config

// GitHubConfig groups all GitHub CLI-related validator configurations.
type GitHubConfig struct {
	// Issue validator configuration
	Issue *IssueValidatorConfig `json:"issue,omitempty" koanf:"issue" toml:"issue,omitempty"`

	// API validator configuration
	API *APIValidatorConfig `json:"api,omitempty" koanf:"api" toml:"api,omitempty"`
}

// APIValidatorConfig configures the gh api validator, which rejects calls to
// GitHub endpoints that create commits outside git and therefore outside the
// commit validators.
type APIValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// BlockedEndpoints lists "METHOD path-pattern" entries. The method may be
	// "*" to match any. The pattern is matched against the normalized endpoint
	// path (no scheme, host, leading slash or query string) and is a glob
	// unless it contains regex syntax.
	// Default: the REST endpoints that create commits.
	BlockedEndpoints []string `json:"blocked_endpoints,omitempty" koanf:"blocked_endpoints" toml:"blocked_endpoints,omitempty"`

	// BlockedGraphQLMutations lists mutation names rejected when found in the
	// query body of a /graphql request.
	// Default: ["createCommitOnBranch"]
	BlockedGraphQLMutations []string `json:"blocked_graphql_mutations,omitempty" koanf:"blocked_graphql_mutations" toml:"blocked_graphql_mutations,omitempty"`

	// BlockUnverifiableCalls rejects a write whose endpoint was built from a
	// variable that cannot be resolved, or whose GraphQL body lives in a file
	// that cannot be read. Such a call cannot be shown to create no commit.
	// Default: true
	BlockUnverifiableCalls *bool `json:"block_unverifiable_calls,omitempty" koanf:"block_unverifiable_calls" toml:"block_unverifiable_calls,omitempty"`

	// CheckHTTPClients extends the same endpoint rules to curl, wget, httpie
	// and xh, and to REST calls written inside inline script bodies.
	// Default: true
	CheckHTTPClients *bool `json:"check_http_clients,omitempty" koanf:"check_http_clients" toml:"check_http_clients,omitempty"`

	// Hosts are the hostnames treated as the GitHub API when a non-gh client
	// sends the request. A path under /api/v3/ or /api/graphql is recognised on
	// any host, covering GitHub Enterprise Server.
	// Default: ["api.github.com"]
	Hosts []string `json:"hosts,omitempty" koanf:"hosts" toml:"hosts,omitempty"`

	// BlockedClientCalls lists API client library methods that create a commit.
	// A name matches only in call form, so mentioning it in prose is fine.
	// Default: the Octokit methods behind the blocked REST endpoints.
	BlockedClientCalls []string `json:"blocked_client_calls,omitempty" koanf:"blocked_client_calls" toml:"blocked_client_calls,omitempty"`
}

// IssueValidatorConfig configures the gh issue create validator.
type IssueValidatorConfig struct {
	ValidatorConfig `koanf:",squash"`

	// RequireBody requires issue body to be present.
	// Default: false (body is optional for issues)
	RequireBody *bool `json:"require_body,omitempty" koanf:"require_body" toml:"require_body,omitempty"`

	// MarkdownDisabledRules is a list of markdownlint rules to disable for issue body validation.
	// Default: ["MD013", "MD034", "MD041", "MD047"]
	// - MD013: Line length (issues often have long lines)
	// - MD034: Bare URLs (commonly used in issues)
	// - MD041: First line heading (issues often start with ### headings)
	// - MD047: Files should end with newline (gh CLI handles this)
	MarkdownDisabledRules []string `json:"markdown_disabled_rules,omitempty" koanf:"markdown_disabled_rules" toml:"markdown_disabled_rules,omitempty"`

	// Timeout for markdown linting operations.
	// Default: 10s
	Timeout Duration `json:"timeout,omitempty" koanf:"timeout" toml:"timeout,omitempty"`
}

// DefaultBlockedGHAPIEndpoints returns the REST endpoints that create a commit
// without running git: writing or deleting repository contents, creating a git
// commit object, and merging a branch. Merging a pull request is left to the
// merge validator, which checks PUT .../pulls/{number}/merge the way it checks
// gh pr merge.
func DefaultBlockedGHAPIEndpoints() []string {
	return []string{
		"PUT **/contents/**",
		"DELETE **/contents/**",
		"POST **/git/commits",
		"POST **/merges",
	}
}

// DefaultBlockedGHAPIMutations returns the GraphQL mutations that create a commit.
func DefaultBlockedGHAPIMutations() []string {
	return []string{"createCommitOnBranch"}
}

// DefaultGitHubAPIHosts returns the hostnames treated as the GitHub API when a
// client other than gh sends the request.
func DefaultGitHubAPIHosts() []string {
	return []string{"api.github.com"}
}

// DefaultBlockedGHAPIClientCalls returns the API client library methods that
// create a commit, named the way Octokit exposes them. pulls.merge stays here:
// a merge made from script code carries no fields the merge validator can read.
func DefaultBlockedGHAPIClientCalls() []string {
	return []string{
		"repos.createOrUpdateFileContents",
		"repos.deleteFile",
		"git.createCommit",
		"repos.merge",
		"pulls.merge",
		"createCommitOnBranch",
	}
}
