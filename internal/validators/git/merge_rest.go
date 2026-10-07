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
	commitTitleField   = "commit_title"

	mergeMethodSquash = "squash"
	mergeMethodMerge  = "merge"
	mergeMethodRebase = "rebase"

	maxMergeBodyBytes = 1 << 20

	unreadableBody = "(unreadable request body)"
)

// fieldValue is one value a REST merge request sends for a field. file names
// where a value read at request time comes from; its content is not read.
type fieldValue struct {
	text string
	file string
}

// restMergeFields holds every value a REST merge request sends for each
// field, from the body and from the query string. With --input, gh api sends
// its -f/-F fields as query parameters next to the body, and a URL can carry
// its own query. GitHub documents the merge fields only as body parameters and
// does not say which value it reads when a key arrives more than once, so
// every value is kept and checked rather than guessing a winner.
type restMergeFields struct {
	values map[string][]fieldValue

	// unreadable is set when the request body cannot be read, so any field
	// may also carry a value that is not seen here.
	unreadable bool
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
			if target, ok := v.httpClientMerge(result, req, cmd); ok {
				targets = append(targets, target)
			}
		}

		return targets
	}

	return nil
}

// ghAPIMerge reads a gh api call that merges a pull request. With --input, gh
// sends that file or stdin as the whole body and moves the fields to the query
// string, so a key can then arrive twice and both values are checked. The host gh talks to - the one
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
		body = v.stdinFields(result, cmd)
	}

	body.addFields(expandFields(result, apiCmd.Fields), apiCmd.FieldFiles)
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
	cmd parser.Command,
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

	// httpie and xh query items sit in the body text but go to the URL.
	bodyText := req.Body
	if len(req.Query) > 0 {
		bodyText = parser.StripQueryItems(bodyText)
	}

	var body restMergeFields

	switch {
	case req.BodyFile != "":
		body = v.readFieldsFromFile(result, req.BodyFile, req.WorkingDirectory, req.Location)
	case bodyText != "":
		body = fieldsFromText(result.ExpandVars(bodyText))
	default:
		body = v.stdinFields(result, cmd)
	}

	body.addQuery(rawURL)

	for key, texts := range req.Query {
		for _, text := range texts {
			body.add(key, fieldValue{text: result.ExpandVars(text)})
		}
	}

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

// add records one value of a field, once.
func (b *restMergeFields) add(key string, value fieldValue) {
	if slices.Contains(b.values[key], value) {
		return
	}

	if b.values == nil {
		b.values = map[string][]fieldValue{}
	}

	b.values[key] = append(b.values[key], value)
}

// addFields records fields, and fields read from files.
func (b *restMergeFields) addFields(fields, fieldFiles map[string]string) {
	for key, text := range fields {
		b.add(key, fieldValue{text: text})
	}

	for key, path := range fieldFiles {
		b.add(key, fieldValue{file: path})
	}
}

// addQuery records the query parameters of the request target.
func (b *restMergeFields) addQuery(raw string) {
	for key, texts := range parser.QueryFields(raw) {
		for _, text := range texts {
			b.add(key, fieldValue{text: text})
		}
	}
}

// get returns every value sent for a field. An unreadable body adds one value
// that cannot be read.
func (b *restMergeFields) get(key string) []fieldValue {
	values := b.values[key]
	if b.unreadable {
		values = append(slices.Clone(values), fieldValue{file: unreadableBody})
	}

	return values
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
		return restMergeFields{unreadable: true}
	}

	return fieldsFromText(content)
}

// stdinFields reads a request body sent on stdin: a heredoc or pipe, or a file
// redirected with <.
func (v *MergeValidator) stdinFields(
	result *parser.ParseResult,
	cmd parser.Command,
) restMergeFields {
	if cmd.Stdin == "" && cmd.StdinFile != "" {
		return v.readFieldsFromFile(result, cmd.StdinFile, cmd.WorkingDirectory, cmd.Location)
	}

	return fieldsFromText(cmd.Stdin)
}

func fieldsFromText(text string) restMergeFields {
	fields, ok := parser.ParseRequestFields(text)
	if !ok {
		return restMergeFields{unreadable: true}
	}

	var body restMergeFields

	body.addFields(fields, nil)

	return body
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
// validated as a squash. A commit message or title that cannot be read is
// treated like --body-file, whose content the gh pr merge checks cannot see
// either. When a field carries several values, the merge is validated as a
// squash if any of them squashes, and every readable message and title is
// checked.
func restMergeCommand(
	prPath string,
	hostname string,
	body restMergeFields,
) *parser.GHMergeCommand {
	mergeCmd := &parser.GHMergeCommand{
		APIPath:  prPath,
		Hostname: hostname,
	}

	setMergeMethod(mergeCmd, body.get(mergeMethodField))

	messages, messageFile := splitFieldValues(body.get(commitMessageField))

	switch {
	case len(messages) > 0:
		// Body and BodyFile together would let an unread file excuse a
		// message that lacks the signoff, so a readable message stands alone.
		mergeCmd.Body = messages[0]
		mergeCmd.AltBodies = messages[1:]
	case messageFile != "":
		mergeCmd.BodyFile = messageFile
	}

	if titles, _ := splitFieldValues(body.get(commitTitleField)); len(titles) > 0 {
		mergeCmd.Subject = titles[0]
		mergeCmd.AltSubjects = titles[1:]
	}

	return mergeCmd
}

// setMergeMethod marks the merge method. No value is a merge commit.
func setMergeMethod(mergeCmd *parser.GHMergeCommand, methods []fieldValue) {
	if len(methods) == 0 {
		mergeCmd.Merge = true

		return
	}

	rebase := false

	for _, method := range methods {
		switch {
		case method.file != "":
			return
		case method.text == "", method.text == mergeMethodMerge:
		case method.text == mergeMethodRebase:
			rebase = true
		default:
			// squash, or a method that cannot be read
			mergeCmd.Squash = method.text == mergeMethodSquash

			return
		}
	}

	if rebase {
		mergeCmd.Rebase = true
	} else {
		mergeCmd.Merge = true
	}
}

// splitFieldValues returns the readable values of a field, in order, and the
// source of the first value that cannot be read.
func splitFieldValues(values []fieldValue) ([]string, string) {
	var (
		texts []string
		file  string
	)

	for _, value := range values {
		switch {
		case value.file == "":
			texts = append(texts, value.text)
		case file == "":
			file = value.file
		}
	}

	return texts, file
}
