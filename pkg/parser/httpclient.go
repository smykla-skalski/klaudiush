package parser

import (
	"strings"
)

// HTTPRequest is an HTTP request found in a shell command, from a client such
// as curl or wget rather than from gh.
type HTTPRequest struct {
	// Tool is the command that sends the request ("curl", "wget", ...).
	Tool string

	// Method is the effective HTTP method, upper-case.
	Method string

	// URL is the request target exactly as written.
	URL string

	// Body holds inline request body text, used to spot a GraphQL mutation.
	Body string

	// BodyFile is a body read from a file, from "-d @file" or "--body-file".
	BodyFile string

	// Query holds the "field==value" items httpie and xh add to the URL query
	// string, every value of each field in order.
	Query map[string][]string

	// WorkingDirectory is the effective directory of the command.
	WorkingDirectory string

	// Location is the position of the command in the source.
	Location Location

	// ExplicitAPICall marks a call whose syntax names an API client, such as
	// Octokit's request("PUT /repos/..."). Only such a call may be judged on a
	// path with no host, since any other framework's ".put('/repos/...')" is
	// just as likely to be a local route.
	ExplicitAPICall bool
}

// IsWriteMethod reports whether the request changes server state.
func (r *HTTPRequest) IsWriteMethod() bool {
	return IsWriteMethod(r.Method)
}

// flagRole is what a client flag's value contributes to the request.
type flagRole uint8

const (
	roleNone flagRole = iota
	roleMethod
	roleBody
	roleBodyFile
	roleURL
)

// clientFlag describes one flag of an HTTP client command.
type clientFlag struct {
	// name is the flag as written, including its dashes.
	name string

	// takesValue marks a flag that consumes the following argument when no
	// value is attached, so its value cannot be mistaken for the URL.
	takesValue bool

	// role is what the flag's value means to the request.
	role flagRole

	// implies is the method the flag's presence selects when none is explicit.
	implies string
}

// httpClientSpec describes how one client carries its method, URL and body.
type httpClientSpec struct {
	flags []clientFlag

	// positionalMethod allows a bare verb positional to set the method, the
	// form httpie and xh use.
	positionalMethod bool
}

// find returns the descriptor for a flag name.
func (s *httpClientSpec) find(name string) (clientFlag, bool) {
	for _, flag := range s.flags {
		if flag.name == name {
			return flag, true
		}
	}

	return clientFlag{}, false
}

// curlFlags lists the curl flags that matter, plus the value-taking flags that
// would otherwise have their value read as the URL.
var curlFlags = []clientFlag{
	{name: "-X", takesValue: true, role: roleMethod},
	{name: "--request", takesValue: true, role: roleMethod},
	{name: "--url", takesValue: true, role: roleURL},
	{name: "-d", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--data", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--data-raw", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--data-binary", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--data-urlencode", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--json", takesValue: true, role: roleBody, implies: methodPOST},
	{name: flagFieldShort, takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--form", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "-T", takesValue: true, role: roleBodyFile, implies: methodPUT},
	{name: "--upload-file", takesValue: true, role: roleBodyFile, implies: methodPUT},
	{name: "-I", implies: methodHEAD},
	{name: "--head", implies: methodHEAD},
	{name: "-H", takesValue: true},
	{name: flagHeaderLong, takesValue: true},
	{name: "-o", takesValue: true},
	{name: "--output", takesValue: true},
	{name: "-u", takesValue: true},
	{name: "--user", takesValue: true},
	{name: "-A", takesValue: true},
	{name: "--user-agent", takesValue: true},
	{name: "-e", takesValue: true},
	{name: "--referer", takesValue: true},
	{name: flagBodyShort, takesValue: true},
	{name: "--cookie", takesValue: true},
	{name: "-c", takesValue: true},
	{name: "--cookie-jar", takesValue: true},
	{name: flagMergeShort, takesValue: true},
	{name: "--max-time", takesValue: true},
	{name: "-w", takesValue: true},
	{name: "--write-out", takesValue: true},
	{name: "--connect-timeout", takesValue: true},
	{name: "--retry", takesValue: true},
	{name: "--resolve", takesValue: true},
	{name: "--cacert", takesValue: true},
	{name: "--cert", takesValue: true},
	{name: "--key", takesValue: true},
	{name: "--proxy", takesValue: true},
}

// wgetFlags lists the wget flags that carry the method or a body.
var wgetFlags = []clientFlag{
	{name: "--method", takesValue: true, role: roleMethod},
	{name: "--post-data", takesValue: true, role: roleBody, implies: methodPOST},
	{name: "--post-file", takesValue: true, role: roleBodyFile, implies: methodPOST},
	{name: "--body-data", takesValue: true, role: roleBody},
	{name: "--body-file", takesValue: true, role: roleBodyFile},
	{name: flagHeaderLong, takesValue: true},
	{name: "-O", takesValue: true},
	{name: "--output-document", takesValue: true},
}

// httpieFlags lists the httpie and xh flags whose value could be read as the
// URL. Both take the method as a bare positional verb instead of a flag.
var httpieFlags = []clientFlag{
	{name: "--raw", takesValue: true, role: roleBody},
	{name: "-a", takesValue: true},
	{name: "--auth", takesValue: true},
	{name: "-A", takesValue: true},
	{name: "--auth-type", takesValue: true},
	{name: "-o", takesValue: true},
	{name: "--output", takesValue: true},
	{name: "--proxy", takesValue: true},
}

var httpClientSpecs = map[string]httpClientSpec{
	"curl":  {flags: curlFlags},
	"wget":  {flags: wgetFlags},
	"http":  {flags: httpieFlags, positionalMethod: true},
	"https": {flags: httpieFlags, positionalMethod: true},
	"xh":    {flags: httpieFlags, positionalMethod: true},
	"xhs":   {flags: httpieFlags, positionalMethod: true},
}

// IsHTTPClientName reports whether a command name is a known HTTP client.
func IsHTTPClientName(name string) bool {
	_, known := httpClientSpecs[name]

	return known
}

// IsHTTPClient reports whether the command is a known HTTP client.
func IsHTTPClient(cmd *Command) bool {
	return IsHTTPClientName(cmd.Name)
}

// nextFlag starts a fresh set of curl options, and so a separate request.
const nextFlag = "--next"

// ParseHTTPClientCommands parses a curl, wget, httpie or xh invocation into
// every request it sends. One command can carry several: curl fetches each URL
// it is given, and --next starts a new request with its own options.
func ParseHTTPClientCommands(cmd Command) []*HTTPRequest {
	spec, known := httpClientSpecs[cmd.Name]
	if !known {
		return nil
	}

	var requests []*HTTPRequest

	for _, segment := range splitOnNext(cmd.Args) {
		requests = append(requests, parseClientSegment(cmd, &spec, segment)...)
	}

	return requests
}

// splitOnNext divides a client's arguments into one group per request.
func splitOnNext(args []string) [][]string {
	segments := [][]string{{}}

	for _, arg := range args {
		if arg == nextFlag {
			segments = append(segments, []string{})

			continue
		}

		last := len(segments) - 1
		segments[last] = append(segments[last], arg)
	}

	return segments
}

// parseClientSegment turns one request's arguments into a request per URL. The
// options in a segment apply to every URL it names, which is how curl treats
// them.
func parseClientSegment(cmd Command, spec *httpClientSpec, args []string) []*HTTPRequest {
	state := httpClientState{}

	for i := 0; i < len(args); {
		i += state.parseClientArg(args, i, spec)
	}

	method := state.resolveMethod()

	requests := make([]*HTTPRequest, 0, len(state.urls))

	for _, url := range state.urls {
		requests = append(requests, &HTTPRequest{
			Tool:             cmd.Name,
			Method:           method,
			URL:              url,
			Body:             state.body,
			BodyFile:         state.bodyFile,
			Query:            state.query,
			WorkingDirectory: cmd.WorkingDirectory,
			Location:         cmd.Location,
		})
	}

	return requests
}

// httpClientState accumulates one request's arguments.
type httpClientState struct {
	explicitMethod   string
	positionalMethod string
	impliedMethod    string
	hasDataItem      bool
	urls             []string
	body             string
	bodyFile         string
	query            map[string][]string
}

// resolveMethod applies the precedence every client shares: an explicit method
// flag, then a bare verb, then what a body flag implies, then GET.
func (s *httpClientState) resolveMethod() string {
	switch {
	case s.explicitMethod != "":
		return s.explicitMethod
	case s.positionalMethod != "":
		return s.positionalMethod
	case s.impliedMethod != "":
		return s.impliedMethod
	case s.hasDataItem:
		return methodPOST
	default:
		return methodGET
	}
}

// parseClientArg consumes one argument and returns how many args it used.
func (s *httpClientState) parseClientArg(args []string, idx int, spec *httpClientSpec) int {
	arg := args[idx]

	if !strings.HasPrefix(arg, "-") || arg == stdinPath {
		s.parseClientPositional(arg, spec)

		return argsPerFlag
	}

	name, value, attached := splitClientFlag(arg, spec)

	flag, known := spec.find(name)
	if !known {
		return argsPerFlag
	}

	used := argsPerFlag

	if !attached && flag.takesValue && idx+1 < len(args) {
		value = args[idx+1]
		used = argsPerFlagWithValue
	}

	s.applyClientFlag(flag, value)

	return used
}

// applyClientFlag records what a single flag says about the request.
func (s *httpClientState) applyClientFlag(flag clientFlag, value string) {
	if flag.implies != "" && s.impliedMethod == "" {
		s.impliedMethod = flag.implies
	}

	switch flag.role {
	case roleMethod:
		s.explicitMethod = strings.ToUpper(value)
	case roleURL:
		s.urls = append(s.urls, value)
	case roleBodyFile:
		s.setBodyFile(value)
	case roleBody:
		s.collectBody(value)
	case roleNone:
	}
}

// setBodyFile records a body path, ignoring the stdin marker, whose content is
// already carried on the command's stdin.
func (s *httpClientState) setBodyFile(path string) {
	if path == stdinPath {
		return
	}

	s.bodyFile = path
}

// collectBody records inline body text, following curl's "@path" form to a file.
func (s *httpClientState) collectBody(value string) {
	if path, isFile := strings.CutPrefix(value, "@"); isFile {
		s.setBodyFile(path)

		return
	}

	if s.body != "" {
		s.body += "\n"
	}

	s.body += value
}

// parseClientPositional records a bare verb, a URL, or an httpie data item.
func (s *httpClientState) parseClientPositional(arg string, spec *httpClientSpec) {
	if spec.positionalMethod && len(s.urls) == 0 && s.positionalMethod == "" &&
		httpMethods[strings.ToUpper(arg)] {
		s.positionalMethod = strings.ToUpper(arg)

		return
	}

	// httpie and xh send "field==value" items in the query string, not the body.
	if spec.positionalMethod && len(s.urls) > 0 {
		if key, value, ok := parseQueryItem(arg); ok {
			if s.query == nil {
				s.query = map[string][]string{}
			}

			s.query[key] = append(s.query[key], value)

			return
		}
	}

	// httpie request items ("field=value", "field:=json") make the call a POST.
	// Only the clients that take a positional method have them; curl fetches
	// every positional, so a URL of its own carrying a query string stays a URL.
	if spec.positionalMethod && len(s.urls) > 0 && strings.Contains(arg, "=") {
		s.hasDataItem = true

		s.collectBody(arg)

		return
	}

	s.urls = append(s.urls, arg)
}

// parseQueryItem reads an httpie "field==value" query item.
func parseQueryItem(arg string) (string, string, bool) {
	key, value, found := strings.Cut(arg, "==")
	if !found || key == "" || strings.ContainsAny(key, "=:@") {
		return "", "", false
	}

	return key, value, true
}

// splitClientFlag splits --flag=value and a short flag with an attached value.
func splitClientFlag(arg string, spec *httpClientSpec) (string, string, bool) {
	// The attached-value split comes first, or "-dbase=main" would read as a
	// flag named "-dbase" and its body would be lost.
	if len(arg) > shortFlagLen && !strings.HasPrefix(arg, "--") {
		short := arg[:shortFlagLen]
		if flag, known := spec.find(short); known && flag.takesValue {
			return short, strings.TrimPrefix(arg[shortFlagLen:], "="), true
		}
	}

	if name, value, found := strings.Cut(arg, "="); found {
		return name, value, true
	}

	return arg, "", false
}

// SplitURL separates a URL into its host and its path. The path keeps its
// leading slash so callers can recognise a GitHub Enterprise API prefix. A
// value with no scheme is treated as a path with no host.
func SplitURL(raw string) (string, string) {
	rest, found := cutScheme(raw)
	if !found {
		return "", raw
	}

	authority, path, hasPath := strings.Cut(rest, "/")
	if !hasPath {
		return normalizeHost(authority), ""
	}

	return normalizeHost(authority), "/" + path
}

// SplitRequestURL is SplitURL for a value a client is about to fetch, where a
// missing scheme means the client supplies one rather than that the value is a
// path. Without this, "curl api.github.com/repos/..." would read as hostless.
func SplitRequestURL(raw string) (string, string) {
	host, path := SplitURL(raw)
	if host != "" || !looksLikeHost(raw) {
		return host, path
	}

	return SplitURL("https://" + raw)
}

// looksLikeHost reports whether a scheme-less value starts with something that
// could be a hostname rather than a path segment.
func looksLikeHost(raw string) bool {
	if strings.HasPrefix(raw, "/") {
		return false
	}

	authority, _, _ := strings.Cut(raw, "/")

	return strings.Contains(authority, ".")
}

// cutScheme removes a leading http:// or https://, whatever its case, since a
// scheme is case-insensitive and curl accepts either spelling.
func cutScheme(raw string) (string, bool) {
	lower := strings.ToLower(raw)

	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(lower, scheme) {
			return raw[len(scheme):], true
		}
	}

	return "", false
}

// normalizeHost drops the userinfo and the port and lower-cases what is left,
// so neither a credential prefix nor a spelling defeats a host comparison.
// An IPv6 literal keeps its brackets and port, which no GitHub host uses.
func normalizeHost(authority string) string {
	if at := strings.LastIndex(authority, "@"); at != -1 {
		authority = authority[at+1:]
	}

	if !strings.HasPrefix(authority, "[") {
		if host, _, found := strings.Cut(authority, ":"); found {
			authority = host
		}
	}

	return strings.ToLower(authority)
}
