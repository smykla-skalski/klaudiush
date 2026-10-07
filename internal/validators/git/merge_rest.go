package git

import (
	"path/filepath"
	"slices"

	"github.com/smykla-skalski/klaudiush/internal/validators"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// RESTPRMergePattern matches a command line that may merge a pull request
// through the REST API. The factory registers the merge validator on it, so a
// REST merge reaches the same checks as gh pr merge however it is sent.
const RESTPRMergePattern = `pulls/[^/\s"']+/merge\b`

const (
	defaultAPIHost = "api.github.com"

	mergeMethodField   = "merge_method"
	commitMessageField = "commit_message"

	mergeMethodSquash = "squash"
	mergeMethodMerge  = "merge"
	mergeMethodRebase = "rebase"

	maxMergeBodyBytes = 1 << 20

	unreadableBody = "(unreadable request body)"
)

// restMergeFields is what a REST merge request sends in its body.
type restMergeFields struct {
	fields     map[string]string
	fieldFiles map[string]string
	known      bool
}

// WithAPIHosts sets the hostnames treated as the GitHub API when a client
// other than gh sends a REST merge request. Empty keeps the default.
func (v *MergeValidator) WithAPIHosts(hosts []string) *MergeValidator {
	if len(hosts) > 0 {
		v.apiHosts = hosts
	}

	return v
}

func (v *MergeValidator) githubAPIHosts() []string {
	if len(v.apiHosts) > 0 {
		return v.apiHosts
	}

	return config.DefaultGitHubAPIHosts()
}

// findRESTMerge returns the first REST pull request merge in the command line,
// shaped as the gh pr merge it is equivalent to, so both go through the same
// checks.
func (v *MergeValidator) findRESTMerge(result *parser.ParseResult) (*parser.GHMergeCommand, bool) {
	for _, cmd := range result.Commands {
		switch {
		case parser.IsGHAPI(&cmd):
			if mergeCmd, ok := v.ghAPIMerge(result, cmd); ok {
				return mergeCmd, true
			}
		case parser.IsHTTPClient(&cmd):
			for _, req := range parser.ParseHTTPClientCommands(cmd) {
				if mergeCmd, ok := v.httpClientMerge(result, req, cmd.Stdin); ok {
					return mergeCmd, true
				}
			}
		}
	}

	return nil, false
}

// ghAPIMerge reads a gh api call that merges a pull request. With --input, gh
// sends that file or stdin as the whole body and moves the fields to the query
// string, so the fields are not the body then.
func (v *MergeValidator) ghAPIMerge(
	result *parser.ParseResult,
	cmd parser.Command,
) (*parser.GHMergeCommand, bool) {
	apiCmd, err := parser.ParseGHAPICommand(cmd)
	if err != nil {
		return nil, false
	}

	repo, number, ok := prMergeTarget(result, apiCmd.Method, apiCmd.Endpoint)
	if !ok {
		return nil, false
	}

	var body restMergeFields

	switch {
	case apiCmd.InputFile != "":
		body = v.readFieldsFromFile(
			result, apiCmd.InputFile, apiCmd.WorkingDirectory, apiCmd.Location,
		)
	case readsStdinBody(apiCmd.RawArgs):
		body = fieldsFromText(cmd.Stdin)
	default:
		body = restMergeFields{
			fields:     expandFields(result, apiCmd.Fields),
			fieldFiles: apiCmd.FieldFiles,
			known:      true,
		}
	}

	return restMergeCommand(repo, number, apiCmd.Hostname, body), true
}

func readsStdinBody(args []string) bool {
	for i, arg := range args {
		if arg == "--input=-" || (arg == "--input" && i+1 < len(args) && args[i+1] == "-") {
			return true
		}
	}

	return false
}

// httpClientMerge reads a curl, wget, httpie or xh request that merges a pull
// request on a GitHub API host.
func (v *MergeValidator) httpClientMerge(
	result *parser.ParseResult,
	req *parser.HTTPRequest,
	stdin string,
) (*parser.GHMergeCommand, bool) {
	host, path := parser.SplitRequestURL(result.ExpandVars(req.URL))
	if !slices.Contains(v.githubAPIHosts(), host) && !parser.IsGHESAPIPath(path) {
		return nil, false
	}

	repo, number, ok := prMergeTarget(result, req.Method, path)
	if !ok {
		return nil, false
	}

	var body restMergeFields

	switch {
	case req.BodyFile != "":
		body = v.readFieldsFromFile(result, req.BodyFile, req.WorkingDirectory, req.Location)
	case req.Body != "":
		body = fieldsFromText(result.ExpandVars(req.Body))
	default:
		body = fieldsFromText(stdin)
	}

	hostname := host
	if host == defaultAPIHost {
		hostname = ""
	}

	return restMergeCommand(repo, number, hostname, body), true
}

// prMergeTarget resolves the repository and number of a REST merge request.
func prMergeTarget(
	result *parser.ParseResult,
	method, endpoint string,
) (string, int, bool) {
	expanded := parser.NormalizeAPIEndpoint(result.ExpandVars(endpoint))
	if !parser.IsPRMergeRequest(method, expanded) {
		return "", 0, false
	}

	return parser.ParsePRMergeEndpoint(expanded)
}

// readFieldsFromFile reads a request body file, preferring content written
// earlier on the same command line, since a heredoc file does not exist yet
// when the hook runs.
func (v *MergeValidator) readFieldsFromFile(
	result *parser.ParseResult,
	path, workDir string,
	location parser.Location,
) restMergeFields {
	if content, ok := result.InlineFileContent(path, workDir, location); ok {
		return fieldsFromText(content)
	}

	if !filepath.IsAbs(path) && workDir != "" {
		path = filepath.Join(workDir, path)
	}

	content, ok := validators.ReadCapped(v.Logger(), filepath.Clean(path), maxMergeBodyBytes)
	if !ok {
		return restMergeFields{}
	}

	return fieldsFromText(content)
}

func fieldsFromText(text string) restMergeFields {
	fields, ok := parser.ParseRequestFields(text)

	return restMergeFields{fields: fields, known: ok}
}

func expandFields(result *parser.ParseResult, fields map[string]string) map[string]string {
	expanded := make(map[string]string, len(fields))

	for key, value := range fields {
		expanded[key] = result.ExpandVars(value)
	}

	return expanded
}

// restMergeCommand shapes a REST merge request as a gh pr merge. REST merges
// with a merge commit unless merge_method says otherwise. A merge method that
// cannot be read is treated like gh pr merge with neither --merge nor --rebase:
// validated as a squash. A commit message that cannot be read is treated like
// --body-file, whose content the gh pr merge checks cannot see either.
func restMergeCommand(
	repo string,
	number int,
	hostname string,
	body restMergeFields,
) *parser.GHMergeCommand {
	mergeCmd := &parser.GHMergeCommand{
		PRNumber: number,
		Repo:     repo,
		Hostname: hostname,
	}

	if !body.known {
		mergeCmd.BodyFile = unreadableBody

		return mergeCmd
	}

	if _, fromFile := body.fieldFiles[mergeMethodField]; !fromFile {
		switch body.fields[mergeMethodField] {
		case "", mergeMethodMerge:
			mergeCmd.Merge = true
		case mergeMethodRebase:
			mergeCmd.Rebase = true
		case mergeMethodSquash:
			mergeCmd.Squash = true
		}
	}

	if path, fromFile := body.fieldFiles[commitMessageField]; fromFile {
		mergeCmd.BodyFile = path
	} else {
		mergeCmd.Body = body.fields[commitMessageField]
	}

	return mergeCmd
}
