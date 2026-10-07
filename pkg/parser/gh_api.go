package parser

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrNotGHAPICommand is returned when the gh command is not an api command.
var ErrNotGHAPICommand = errors.New("not a gh api command")

const (
	apiSubCmd = "api"

	// minGHAPIArgsLen is the minimum arg count for "gh api".
	minGHAPIArgsLen = 1

	// GraphQLEndpoint is the normalized endpoint of a GraphQL request.
	GraphQLEndpoint = "graphql"

	// ghesAPIPrefix is the REST path prefix on GitHub Enterprise Server.
	ghesAPIPrefix = "api/v3/"

	// ghesGraphQLPath is the GraphQL path on GitHub Enterprise Server.
	ghesGraphQLPath = "api/graphql"

	methodGET  = "GET"
	methodPOST = "POST"
	methodPUT  = "PUT"
	methodHEAD = "HEAD"

	// queryFieldPrefix marks the field carrying a GraphQL document.
	queryFieldPrefix = "query="

	schemeSeparator = "://"
	shortFlagLen    = 2

	// argsPerFlag is what a flag consumes on its own, and
	// argsPerFlagWithValue what it consumes together with its value.
	argsPerFlag          = 1
	argsPerFlagWithValue = 2

	flagMethodShort = "-X"
	flagMethodLong  = "--method"
	flagInput       = "--input"
	flagHeaderLong  = "--header"
	flagFieldLong   = "--field"
	flagHostname    = "--hostname"

	// Short flags several clients spell the same way for different options:
	// -F is gh's --field and curl's --form, -b is gh's --body and curl's
	// --cookie, -m is gh's --merge and curl's --max-time.
	flagFieldShort = "-F"
	flagBodyShort  = "-b"
	flagMergeShort = "-m"

	// stdinPath is the --input value that means "read the body from stdin".
	stdinPath = "-"
)

// ghAPIValueFlags lists gh api flags that consume the following argument.
var ghAPIValueFlags = map[string]bool{
	flagMethodShort: true,
	flagMethodLong:  true,
	"-f":            true,
	"--raw-field":   true,
	flagFieldShort:  true,
	flagFieldLong:   true,
	"-H":            true,
	flagHeaderLong:  true,
	flagInput:       true,
	"-q":            true,
	"--jq":          true,
	"-t":            true,
	"--template":    true,
	flagHostname:    true,
	"--cache":       true,
	"-p":            true,
	"--preview":     true,
}

// ghAPIFieldFlags lists gh api flags that turn the default method into POST.
var ghAPIFieldFlags = map[string]bool{
	"-f":           true,
	"--raw-field":  true,
	flagFieldShort: true,
	flagFieldLong:  true,
	flagInput:      true,
}

// httpMethods lists the verbs accepted as a bare positional method.
var httpMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"PATCH":   true,
	"DELETE":  true,
	"HEAD":    true,
	"OPTIONS": true,
}

// writeMethods lists the verbs that change server state.
var writeMethods = map[string]bool{
	"POST":   true,
	"PUT":    true,
	"PATCH":  true,
	"DELETE": true,
}

// GHAPICommand represents a parsed gh api command.
type GHAPICommand struct {
	// Endpoint is the normalized request path: no scheme, no host, no leading
	// or trailing slash, no query string.
	Endpoint string

	// Method is the effective HTTP method, upper-case. gh defaults to GET, or
	// to POST as soon as any field flag is present.
	Method string

	// IsGraphQL reports whether the request targets the GraphQL endpoint.
	IsGraphQL bool

	// Query holds GraphQL document text found in the command: query fields,
	// heredocs and piped stdin.
	Query string

	// InputFile is the --input path, when the body comes from a file rather
	// than from a field or stdin. "-" means stdin and is not recorded here.
	InputFile string

	// Fields holds the key=value pairs sent with -f, --raw-field, -F and
	// --field. A later field overwrites an earlier one with the same key.
	Fields map[string]string

	// FieldFiles holds the fields whose value -F or --field reads from a file,
	// keyed by field name, with the file path as the value.
	FieldFiles map[string]string

	// Hostname is the --hostname value, empty when the default host is used.
	Hostname string

	// WorkingDirectory is the effective directory of the command, for
	// resolving a relative InputFile.
	WorkingDirectory string

	// Location is the position of the command in the source.
	Location Location

	// RawArgs contains all the raw arguments for debugging.
	RawArgs []string
}

// IsWriteMethod reports whether an HTTP method changes server state.
func IsWriteMethod(method string) bool {
	return writeMethods[method]
}

// IsWriteMethod reports whether the request changes server state.
func (c *GHAPICommand) IsWriteMethod() bool {
	return IsWriteMethod(c.Method)
}

// IsGHAPI checks if a command is a gh api command.
func IsGHAPI(cmd *Command) bool {
	if cmd.Name != ghCLI {
		return false
	}

	if len(cmd.Args) < minGHAPIArgsLen {
		return false
	}

	return cmd.Args[0] == apiSubCmd
}

// ParseGHAPICommand parses a Command into a GHAPICommand.
func ParseGHAPICommand(cmd Command) (*GHAPICommand, error) {
	if cmd.Name != ghCLI {
		return nil, ErrNotGHCommand
	}

	if !IsGHAPI(&cmd) {
		return nil, ErrNotGHAPICommand
	}

	api := &GHAPICommand{
		Query:            cmd.Stdin,
		WorkingDirectory: cmd.WorkingDirectory,
		Location:         cmd.Location,
		RawArgs:          cmd.Args,
	}

	state := ghAPIParseState{}

	args := cmd.Args[1:]
	for i := 0; i < len(args); {
		i += api.parseAPIArg(args, i, &state)
	}

	api.Endpoint = NormalizeAPIEndpoint(api.Endpoint)
	api.IsGraphQL = api.Endpoint == GraphQLEndpoint
	api.Method = state.resolveMethod()

	return api, nil
}

// ghAPIParseState accumulates the signals needed to resolve the method.
type ghAPIParseState struct {
	explicitMethod   string
	positionalMethod string
	hasFieldFlag     bool
}

// resolveMethod applies gh's own precedence: an explicit method wins, then a
// bare positional verb, then POST when fields are present, then GET.
func (s *ghAPIParseState) resolveMethod() string {
	switch {
	case s.explicitMethod != "":
		return s.explicitMethod
	case s.positionalMethod != "":
		return s.positionalMethod
	case s.hasFieldFlag:
		return methodPOST
	default:
		return methodGET
	}
}

// parseAPIArg consumes one argument and returns how many args it used.
func (c *GHAPICommand) parseAPIArg(args []string, idx int, state *ghAPIParseState) int {
	arg := args[idx]

	if !strings.HasPrefix(arg, "-") || arg == "-" {
		c.parseAPIPositional(arg, state)

		return argsPerFlag
	}

	name, value, attached := splitGHAPIFlag(arg)

	used := argsPerFlag

	if !attached && ghAPIValueFlags[name] && idx+1 < len(args) {
		value = args[idx+1]
		used = argsPerFlagWithValue
	}

	switch {
	case name == flagMethodShort || name == flagMethodLong:
		state.explicitMethod = strings.ToUpper(value)
	case name == flagHostname:
		c.Hostname = value
	case name == flagInput:
		state.hasFieldFlag = true

		if value != stdinPath {
			c.InputFile = value
		}
	case ghAPIFieldFlags[name]:
		state.hasFieldFlag = true

		// Only --field/-F expands a leading @ to a file; --raw-field takes the
		// value literally.
		expandsFile := name == flagFieldShort || name == flagFieldLong
		c.collectField(value, expandsFile)
		c.collectQueryField(value, expandsFile)
	}

	return used
}

// parseAPIPositional records a bare verb or the endpoint.
func (c *GHAPICommand) parseAPIPositional(arg string, state *ghAPIParseState) {
	if c.Endpoint == "" && state.positionalMethod == "" && httpMethods[strings.ToUpper(arg)] {
		state.positionalMethod = strings.ToUpper(arg)

		return
	}

	if c.Endpoint == "" {
		c.Endpoint = arg
	}
}

// collectField records a key=value field. A value read from a file is kept
// apart, since the text after the @ is a path rather than the value sent.
func (c *GHAPICommand) collectField(field string, expandsFile bool) {
	key, value, found := strings.Cut(field, "=")
	if !found || key == "" {
		return
	}

	if path, isFile := strings.CutPrefix(value, "@"); isFile && expandsFile {
		if c.FieldFiles == nil {
			c.FieldFiles = map[string]string{}
		}

		c.FieldFiles[key] = path
		delete(c.Fields, key)

		return
	}

	if c.Fields == nil {
		c.Fields = map[string]string{}
	}

	c.Fields[key] = value
	delete(c.FieldFiles, key)
}

// collectQueryField appends a query=... field value to the GraphQL document.
// When the flag expands a leading @, the value names a file holding the query
// rather than the query itself.
func (c *GHAPICommand) collectQueryField(value string, expandsFile bool) {
	query, isQuery := strings.CutPrefix(value, queryFieldPrefix)
	if !isQuery {
		return
	}

	if path, isFile := strings.CutPrefix(query, "@"); isFile && expandsFile {
		if path != stdinPath {
			c.InputFile = path
		}

		return
	}

	if c.Query != "" {
		c.Query += "\n"
	}

	c.Query += query
}

// splitGHAPIFlag splits --flag=value, -f=value and -fvalue into name and value.
// The bool reports whether a value was attached to the flag itself.
func splitGHAPIFlag(arg string) (string, string, bool) {
	// A single-dash flag can carry its value attached, as in -XPUT or
	// -fmessage=x. That split has to come first: cutting on "=" would read
	// -fmessage=x as a flag named "-fmessage" and lose the field entirely.
	if len(arg) > shortFlagLen && !strings.HasPrefix(arg, "--") {
		if short := arg[:shortFlagLen]; ghAPIValueFlags[short] {
			return short, strings.TrimPrefix(arg[shortFlagLen:], "="), true
		}
	}

	if name, value, found := strings.Cut(arg, "="); found {
		return name, value, true
	}

	return arg, "", false
}

// IsGHESAPIPath reports whether a URL path addresses a GitHub Enterprise
// Server API, which can live on any hostname.
func IsGHESAPIPath(path string) bool {
	return strings.HasPrefix(path, "/"+ghesAPIPrefix) ||
		strings.HasPrefix(path, "/"+ghesGraphQLPath)
}

// NormalizeAPIEndpoint strips the scheme, host, API prefix, query string and
// surrounding slashes so patterns can match a stable path. A GitHub Enterprise
// GraphQL path collapses to the same "graphql" as the github.com one.
func NormalizeAPIEndpoint(endpoint string) string {
	if idx := strings.Index(endpoint, "?"); idx != -1 {
		endpoint = endpoint[:idx]
	}

	if _, rest, found := strings.Cut(endpoint, schemeSeparator); found {
		_, path, hasPath := strings.Cut(rest, "/")
		if !hasPath {
			return ""
		}

		endpoint = path
	}

	// The Enterprise prefix is stripped after the slashes, so a bare path
	// carries it out too, not only a full URL.
	normalized := strings.TrimPrefix(strings.Trim(endpoint, "/"), ghesAPIPrefix)
	if normalized == ghesGraphQLPath {
		return GraphQLEndpoint
	}

	return normalized
}
