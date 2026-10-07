package git

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/validators"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// RESTPRMergePattern matches a command line that may merge a pull request
// through the REST API. The factory registers the merge validator on it, so a
// REST merge reaches the same checks as gh pr merge however it is sent.
const RESTPRMergePattern = `pulls/[^/\s"']+/merge\b`

const (
	defaultAPIHost    = "api.github.com"
	defaultGitHubHost = "github.com"

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

// restMerges returns the REST pull request merges one command sends, shaped
// as the gh pr merge each is equivalent to, so both go through the same checks.
func (v *MergeValidator) restMerges(result *parser.ParseResult, cmd parser.Command) []mergeTarget {
	switch {
	case parser.IsGHAPI(&cmd):
		if target, ok := v.ghAPIMerge(result, cmd); ok {
			return []mergeTarget{target}
		}
	case parser.IsHTTPClient(&cmd):
		var targets []mergeTarget

		for _, req := range parser.ParseHTTPClientCommands(cmd) {
			if target, ok := v.httpClientMerge(result, req, cmd.Stdin); ok {
				targets = append(targets, target)
			}
		}

		return targets
	}

	return nil
}

// ghAPIMerge reads a gh api call that merges a pull request. With --input, gh
// sends that file or stdin as the whole body and moves the fields to the query
// string, so the fields are not the body then. The host gh talks to - the one
// in a URL endpoint, or --hostname - is trusted: the command itself already
// sends gh's token there.
func (v *MergeValidator) ghAPIMerge(
	result *parser.ParseResult,
	cmd parser.Command,
) (mergeTarget, bool) {
	apiCmd, err := parser.ParseGHAPICommand(cmd)
	if err != nil {
		return mergeTarget{}, false
	}

	prPath, ok := prMergeTarget(result, apiCmd.Method, apiCmd.Endpoint)
	if !ok {
		return mergeTarget{}, false
	}

	rawEndpoint := result.ExpandVars(ghAPIEndpointArg(apiCmd))

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

	body.addQuery(rawEndpoint)

	hostname := strings.ToLower(apiCmd.Hostname)
	if hostname == "" {
		hostname = endpointHost(rawEndpoint)
	}

	target := mergeTarget{
		cmd:         restMergeCommand(prPath, gitHubHostname(hostname), body),
		signoffHint: restMergeSignoffHint,
	}

	if !safeHostname(hostname) {
		target.cmd.Hostname = ""
		target.unlistedHost = hostname
	}

	return target, true
}

// ghAPIEndpointArg returns the endpoint as written, before normalization drops
// its host.
func ghAPIEndpointArg(apiCmd *parser.GHAPICommand) string {
	for _, arg := range apiCmd.RawArgs[1:] {
		if parser.NormalizeAPIEndpoint(arg) == apiCmd.Endpoint && apiCmd.Endpoint != "" {
			return arg
		}
	}

	return ""
}

func endpointHost(endpoint string) string {
	host, _ := parser.SplitURL(endpoint)

	return host
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
// request on a GitHub API host. Only a configured host is fetched from: any
// other host was named by the command text alone, and gh would send its token
// to it.
func (v *MergeValidator) httpClientMerge(
	result *parser.ParseResult,
	req *parser.HTTPRequest,
	stdin string,
) (mergeTarget, bool) {
	rawURL := result.ExpandVars(req.URL)
	host, path := parser.SplitRequestURL(rawURL)
	listed := slices.Contains(v.githubAPIHosts(), host)

	if !listed && !parser.IsGHESAPIPath(path) {
		return mergeTarget{}, false
	}

	prPath, ok := prMergeTarget(result, req.Method, path)
	if !ok {
		return mergeTarget{}, false
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

	body.addQuery(rawURL)

	target := mergeTarget{
		cmd:         restMergeCommand(prPath, "", body),
		signoffHint: restMergeSignoffHint,
	}

	if listed && safeHostname(host) {
		target.cmd.Hostname = gitHubHostname(host)
	} else {
		target.unlistedHost = host
	}

	return target, true
}

// gitHubHostname maps the API host to the hostname gh expects, which is empty
// for github.com.
func gitHubHostname(host string) string {
	if host == defaultAPIHost || host == defaultGitHubHost {
		return ""
	}

	return host
}

// safeHostname reports whether a host can be handed to gh. A leading dash would
// read as a flag, and anything outside hostname characters is not a host.
func safeHostname(host string) bool {
	if strings.HasPrefix(host, "-") {
		return false
	}

	return strings.IndexFunc(host, isNotHostnameRune) == -1
}

func isNotHostnameRune(r rune) bool {
	switch {
	case r == '.', r == '-', r == ':', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return false
	default:
		return true
	}
}

// prMergeTarget resolves the pull request path of a REST merge request.
func prMergeTarget(
	result *parser.ParseResult,
	method, endpoint string,
) (string, bool) {
	expanded := parser.NormalizeAPIEndpoint(result.ExpandVars(endpoint))
	if !parser.IsPRMergeRequest(method, expanded) {
		return "", false
	}

	return parser.ParsePRMergeEndpoint(expanded)
}

// addQuery adds the query parameters of the request target to fields the
// body does not set itself.
func (b *restMergeFields) addQuery(raw string) {
	if !b.known {
		return
	}

	for key, value := range parser.QueryFields(raw) {
		if _, set := b.fields[key]; set {
			continue
		}

		if _, set := b.fieldFiles[key]; set {
			continue
		}

		if b.fields == nil {
			b.fields = map[string]string{}
		}

		b.fields[key] = value
	}
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
	prPath string,
	hostname string,
	body restMergeFields,
) *parser.GHMergeCommand {
	mergeCmd := &parser.GHMergeCommand{
		APIPath:  prPath,
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
