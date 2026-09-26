package github

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/rules"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/internal/validators"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

const (
	apiValidatorName = "validate-gh-api"

	// methodWildcard matches any HTTP method in a blocked endpoint rule.
	methodWildcard = "*"

	// reposPrefix is where every commit-creating REST endpoint lives.
	reposPrefix = "repos/"

	// maxRequestBodyBytes caps how much of a request body file is read.
	maxRequestBodyBytes = 1 << 20

	// maxScriptDepth bounds how far nested shell -c invocations are unwrapped.
	maxScriptDepth = 3

	// bypassExplanation names what a commit made this way skips.
	bypassExplanation = "bypassing commit validation (GPG signing, sign-off, conventional commit format)"

	// apiHelp names the intended path, so the refusal is not just a "no".
	apiHelp = "Clone the repository, stage the change, and commit with git commit -sS. " +
		"API calls that create no commit are unaffected: reads, " +
		"POST /repos/{owner}/{repo}/git/refs, gh pr create."

	// unverifiableHelp tells the caller how to make the call checkable.
	unverifiableHelp = "Spell the endpoint literally in the command, or pass the GraphQL query " +
		"with -f query=... or on stdin, so klaudiush can tell whether it creates a commit."
)

// scriptInterpreters run a program given in their arguments or on their stdin.
// Only their text is scanned for an API call: any other command's arguments are
// data, and a commit message or a PR comment that quotes a call is not one.
// Shells are not among them - their script is shell source, so it is parsed
// rather than scanned. See shellInterpreters.
var scriptInterpreters = map[string]bool{
	"node":      true,
	"nodejs":    true,
	"deno":      true,
	"bun":       true,
	"tsx":       true,
	"ts-node":   true,
	"python":    true,
	"python2":   true,
	"python3":   true,
	"ruby":      true,
	"perl":      true,
	"php":       true,
	"osascript": true,
}

// commandLaunchers run another command named in their arguments. Rather than
// model each one's own flags, the first argument naming a command this
// validator checks is taken as the command being launched, which resolves
// "sudo -u u gh api", "env FOO=1 gh api", "timeout 30 curl" and "uv run python"
// alike. A launcher running anything else is left alone.
var commandLaunchers = map[string]bool{
	"sudo":    true,
	"doas":    true,
	"env":     true,
	"command": true,
	"nice":    true,
	"ionice":  true,
	"nohup":   true,
	"stdbuf":  true,
	"timeout": true,
	"xargs":   true,
	"time":    true,
	"watch":   true,
	"npx":     true,
	"bunx":    true,
	"pnpm":    true,
	"yarn":    true,
	"uv":      true,
	"poetry":  true,
	"pipx":    true,
}

// shellInterpreters run a command line handed to them after -c or on stdin, so
// that script is parsed and checked like any other command line.
var shellInterpreters = map[string]bool{
	"sh":   true,
	"bash": true,
	"zsh":  true,
	"ksh":  true,
	"dash": true,
}

// blockedEndpoint pairs an HTTP method with a compiled endpoint pattern.
type blockedEndpoint struct {
	method  string
	pattern rules.Pattern
}

// APIValidator rejects gh api calls that create a commit through the GitHub
// API. Such a commit never runs git, so no commit validator ever sees it.
type APIValidator struct {
	validator.BaseValidator
	config      *config.APIValidatorConfig
	endpoints   []blockedEndpoint
	mutations   []string
	hosts       []string
	clientCalls []string
}

// NewAPIValidator creates a new APIValidator instance.
func NewAPIValidator(
	cfg *config.APIValidatorConfig,
	log logger.Logger,
	ruleAdapter validator.RuleChecker,
) *APIValidator {
	apiValidator := &APIValidator{
		BaseValidator: *validator.NewBaseValidatorWithRules(
			apiValidatorName, log, ruleAdapter,
		),
		config: cfg,
	}

	apiValidator.endpoints = compileBlockedEndpoints(
		apiValidator.blockedEndpointRules(),
		apiValidator.Logger(),
	)
	apiValidator.mutations = apiValidator.blockedMutations()
	apiValidator.hosts = apiValidator.configuredHosts()
	apiValidator.clientCalls = apiValidator.blockedClientCalls()

	return apiValidator
}

// blockedEndpointRules returns the configured endpoint rules, or the defaults.
func (v *APIValidator) blockedEndpointRules() []string {
	if v.config != nil && len(v.config.BlockedEndpoints) > 0 {
		return v.config.BlockedEndpoints
	}

	return config.DefaultBlockedGHAPIEndpoints()
}

// blockedMutations returns the configured GraphQL mutations, or the defaults.
func (v *APIValidator) blockedMutations() []string {
	if v.config != nil && len(v.config.BlockedGraphQLMutations) > 0 {
		return v.config.BlockedGraphQLMutations
	}

	return config.DefaultBlockedGHAPIMutations()
}

// blocksUnverifiable reports whether a write whose endpoint or GraphQL body
// cannot be read is rejected. Default: true, since an opaque write to the
// GitHub API cannot be shown to be safe.
func (v *APIValidator) blocksUnverifiable() bool {
	if v.config != nil && v.config.BlockUnverifiableCalls != nil {
		return *v.config.BlockUnverifiableCalls
	}

	return true
}

// checksHTTPClients reports whether clients other than gh are inspected.
func (v *APIValidator) checksHTTPClients() bool {
	if v.config != nil && v.config.CheckHTTPClients != nil {
		return *v.config.CheckHTTPClients
	}

	return true
}

// configuredHosts returns the hostnames treated as the GitHub API.
func (v *APIValidator) configuredHosts() []string {
	if v.config != nil && len(v.config.Hosts) > 0 {
		return v.config.Hosts
	}

	return config.DefaultGitHubAPIHosts()
}

// blockedClientCalls returns the library method names that create a commit.
func (v *APIValidator) blockedClientCalls() []string {
	if v.config != nil && len(v.config.BlockedClientCalls) > 0 {
		return v.config.BlockedClientCalls
	}

	return config.DefaultBlockedGHAPIClientCalls()
}

// compileBlockedEndpoints turns "METHOD pattern" rules into matchers. A rule
// that does not compile is skipped rather than failing the whole hook.
func compileBlockedEndpoints(specs []string, log logger.Logger) []blockedEndpoint {
	blocked := make([]blockedEndpoint, 0, len(specs))

	for _, spec := range specs {
		method, patternStr, found := strings.Cut(strings.TrimSpace(spec), " ")
		if !found {
			log.Error("Ignoring gh api rule without a method", "rule", spec)

			continue
		}

		pattern, err := rules.CompilePattern(strings.TrimSpace(patternStr))
		if err != nil {
			log.Error("Ignoring gh api rule with an invalid pattern", "rule", spec, "error", err)

			continue
		}

		blocked = append(blocked, blockedEndpoint{
			method:  strings.ToUpper(method),
			pattern: pattern,
		})
	}

	return blocked
}

// Validate rejects gh api invocations that would create a commit.
func (v *APIValidator) Validate(ctx context.Context, hookCtx *hook.Context) *validator.Result {
	log := v.Logger()
	log.Debug("Running gh api validation")

	if result := v.CheckRules(ctx, hookCtx); result != nil {
		return result
	}

	parsed, err := hookCtx.ParsedCommand()
	if err != nil {
		log.Error("Failed to parse command", "error", err)

		return validator.Warn(fmt.Sprintf("Failed to parse command: %v", err))
	}

	if result := v.checkParsed(parsed, 0); result != nil {
		return result
	}

	log.Debug("No commit-creating API calls found")

	return validator.Pass()
}

// checkParsed checks every command of one parsed command line.
func (v *APIValidator) checkParsed(parsed *parser.ParseResult, depth int) *validator.Result {
	for _, cmd := range parsed.Commands {
		if result := v.checkCommand(parsed, cmd, depth); result != nil {
			return result
		}
	}

	return nil
}

// checkCommand dispatches one parsed command to the matching request source.
func (v *APIValidator) checkCommand(
	parsed *parser.ParseResult,
	cmd parser.Command,
	depth int,
) *validator.Result {
	if inner, launched := v.unwrapLauncher(cmd); launched && depth < maxScriptDepth {
		return v.checkCommand(parsed, inner, depth+1)
	}

	switch {
	case parser.IsGHAPI(&cmd):
		apiCmd, err := parser.ParseGHAPICommand(cmd)
		if err != nil {
			v.Logger().Debug("Skipping unparseable gh api command", "error", err)

			return nil
		}

		return v.checkAPICommand(parsed, apiCmd)

	case v.checksHTTPClients() && parser.IsHTTPClient(&cmd):
		for _, req := range parser.ParseHTTPClientCommands(cmd) {
			if result := v.checkHTTPRequest(parsed, req); result != nil {
				return result
			}
		}

		return nil

	// A shell is handed shell source, which is parsed and checked command by
	// command. Scanning that same text for a call name could only add false
	// positives: an echo or a commit message inside the script is data.
	case v.checksHTTPClients() && shellInterpreters[cmd.Name]:
		return v.checkShellScript(cmd, depth)

	case v.checksHTTPClients() && scriptInterpreters[cmd.Name]:
		return v.checkScriptText(scriptText(cmd))

	default:
		return nil
	}
}

// unwrapLauncher returns the command a launcher runs, when one of the commands
// this validator checks appears among its arguments.
func (v *APIValidator) unwrapLauncher(cmd parser.Command) (parser.Command, bool) {
	if !commandLaunchers[cmd.Name] {
		return parser.Command{}, false
	}

	for i, arg := range cmd.Args {
		if !v.isCheckedCommand(arg) {
			continue
		}

		return parser.Command{
			Name:             arg,
			Args:             cmd.Args[i+1:],
			Type:             cmd.Type,
			Stdin:            cmd.Stdin,
			WorkingDirectory: cmd.WorkingDirectory,
			Location:         cmd.Location,
		}, true
	}

	return parser.Command{}, false
}

// isCheckedCommand reports whether a name is one this validator inspects.
func (v *APIValidator) isCheckedCommand(name string) bool {
	if name == ghCommand {
		return true
	}

	if !v.checksHTTPClients() {
		return false
	}

	return parser.IsHTTPClientName(name) || scriptInterpreters[name] || shellInterpreters[name]
}

// scriptText is the program an interpreter was handed, whether it arrived in
// the arguments or on stdin.
func scriptText(cmd parser.Command) string {
	return strings.Join(cmd.Args, " ") + "\n" + cmd.Stdin
}

// checkShellScript parses the script a shell was given, after -c or on stdin,
// and checks it, so wrapping a call in sh -c does not hide it.
func (v *APIValidator) checkShellScript(cmd parser.Command, depth int) *validator.Result {
	if depth >= maxScriptDepth {
		return nil
	}

	for _, script := range []string{shellScriptArg(cmd), cmd.Stdin} {
		if script == "" {
			continue
		}

		inner, err := parser.NewBashParser().Parse(script)
		if err != nil {
			v.Logger().Debug("Cannot parse shell script", "error", err)

			continue
		}

		if result := v.checkParsed(inner, depth+1); result != nil {
			return result
		}
	}

	return nil
}

// shellScriptArg returns the command line a shell was given after -c.
func shellScriptArg(cmd parser.Command) string {
	for i, arg := range cmd.Args {
		if arg == "-c" && i+1 < len(cmd.Args) {
			return cmd.Args[i+1]
		}
	}

	return ""
}

// checkAPICommand returns a failure when the call creates a commit, nil otherwise.
func (v *APIValidator) checkAPICommand(
	parsed *parser.ParseResult,
	apiCmd *parser.GHAPICommand,
) *validator.Result {
	endpoint := parsed.ExpandVars(apiCmd.Endpoint)

	if apiCmd.IsGraphQL || endpoint == parser.GraphQLEndpoint {
		return v.checkGraphQL(parsed, apiCmd)
	}

	return v.checkREST(apiCmd, endpoint)
}

// checkREST matches the request against the blocked endpoint rules.
func (v *APIValidator) checkREST(
	apiCmd *parser.GHAPICommand,
	endpoint string,
) *validator.Result {
	if v.blocksEndpoint(apiCmd.Method, endpoint) {
		return v.fail(fmt.Sprintf(
			"gh api %s %s creates a commit through the GitHub API, %s",
			apiCmd.Method, endpoint, bypassExplanation,
		))
	}

	if v.isOpaqueWrite(apiCmd.Method, endpoint) {
		return v.failUnverifiable(fmt.Sprintf(
			"gh api %s %s cannot be checked: the endpoint is not spelled literally, "+
				"so there is no way to tell whether it creates a commit",
			apiCmd.Method, describeEndpoint(endpoint),
		))
	}

	return nil
}

// isOpaqueWrite reports whether the call changes server state through an
// endpoint that could not be resolved to a literal path.
func (v *APIValidator) isOpaqueWrite(method, endpoint string) bool {
	if !v.blocksUnverifiable() || !parser.IsWriteMethod(method) {
		return false
	}

	return endpoint == "" || parser.HasUnresolvedVars(endpoint)
}

// describeEndpoint renders an endpoint for a message, naming the empty case.
func describeEndpoint(endpoint string) string {
	if endpoint == "" {
		return "(no endpoint in the command)"
	}

	return endpoint
}

// checkGraphQL matches the mutation name inside the query body, since the path
// is always /graphql and carries no signal.
func (v *APIValidator) checkGraphQL(
	parsed *parser.ParseResult,
	apiCmd *parser.GHAPICommand,
) *validator.Result {
	query := apiCmd.Query

	if apiCmd.InputFile != "" {
		body, ok := v.readFile(
			parsed, apiCmd.InputFile, apiCmd.WorkingDirectory, apiCmd.Location,
		)
		if !ok {
			if !v.blocksUnverifiable() {
				return nil
			}

			return v.failUnverifiable(fmt.Sprintf(
				"gh api graphql reads its query from %s, which cannot be read, "+
					"so there is no way to tell whether it creates a commit",
				apiCmd.InputFile,
			))
		}

		query += "\n" + body
	}

	if mutation := v.matchMutation(query); mutation != "" {
		return v.fail(fmt.Sprintf(
			"gh api graphql calls the %s mutation, which creates a commit through the GitHub API, %s",
			mutation,
			bypassExplanation,
		))
	}

	// A write whose query never appears in the command - built by command
	// substitution, say - is as unprovable as one whose file cannot be read.
	if strings.TrimSpace(query) == "" && apiCmd.IsWriteMethod() && v.blocksUnverifiable() {
		return v.failUnverifiable(
			"gh api graphql sends a query that is not spelled out in the command, " +
				"so there is no way to tell whether it creates a commit",
		)
	}

	return nil
}

// readFile returns a request body from a file, preferring content written
// earlier in the same command line over what is on disk, since a file created
// by a heredoc does not exist yet when the PreToolUse hook runs.
func (v *APIValidator) readFile(
	parsed *parser.ParseResult,
	filePath, workDir string,
	location parser.Location,
) (string, bool) {
	if content, ok := parsed.InlineFileContent(filePath, workDir, location); ok {
		return content, true
	}

	path := filePath
	if !filepath.IsAbs(path) && workDir != "" {
		path = filepath.Join(workDir, path)
	}

	return v.readBodyFile(filepath.Clean(path))
}

// readBodyFile reads at most maxRequestBodyBytes of a regular file.
func (v *APIValidator) readBodyFile(path string) (string, bool) {
	return validators.ReadCapped(v.Logger(), path, maxRequestBodyBytes)
}

// checkHTTPRequest applies the same endpoint rules to a curl, wget, httpie or
// xh call, once its URL is shown to address the GitHub API.
func (v *APIValidator) checkHTTPRequest(
	parsed *parser.ParseResult,
	req *parser.HTTPRequest,
) *validator.Result {
	host, path := parser.SplitRequestURL(parsed.ExpandVars(req.URL))
	if !v.isGitHubAPI(host, path) {
		return nil
	}

	endpoint := parser.NormalizeAPIEndpoint(path)

	if endpoint == parser.GraphQLEndpoint {
		return v.checkRequestBody(parsed, req)
	}

	if v.blocksEndpoint(req.Method, endpoint) {
		return v.fail(fmt.Sprintf(
			"%s %s %s creates a commit through the GitHub API, %s",
			req.Tool, req.Method, endpoint, bypassExplanation,
		))
	}

	// The host is known to be GitHub here, so an unreadable path is as
	// unprovable as it is for gh api.
	if v.isOpaqueWrite(req.Method, endpoint) {
		return v.failUnverifiable(fmt.Sprintf(
			"%s %s %s cannot be checked: the endpoint is not spelled literally, "+
				"so there is no way to tell whether it creates a commit",
			req.Tool, req.Method, describeEndpoint(endpoint),
		))
	}

	return nil
}

// checkRequestBody looks for a commit-creating mutation in a GraphQL body sent
// by a client other than gh.
func (v *APIValidator) checkRequestBody(
	parsed *parser.ParseResult,
	req *parser.HTTPRequest,
) *validator.Result {
	body := req.Body

	if req.BodyFile != "" {
		content, ok := v.readFile(parsed, req.BodyFile, req.WorkingDirectory, req.Location)
		if !ok {
			if !v.blocksUnverifiable() {
				return nil
			}

			return v.failUnverifiable(fmt.Sprintf(
				"%s sends a GraphQL body from %s, which cannot be read, "+
					"so there is no way to tell whether it creates a commit",
				req.Tool, req.BodyFile,
			))
		}

		body += "\n" + content
	}

	if mutation := v.matchMutation(body); mutation != "" {
		return v.fail(fmt.Sprintf(
			"%s sends the %s mutation to the GitHub GraphQL API, "+
				"which creates a commit, %s",
			req.Tool, mutation, bypassExplanation,
		))
	}

	if strings.TrimSpace(body) == "" && req.IsWriteMethod() && v.blocksUnverifiable() {
		return v.failUnverifiable(fmt.Sprintf(
			"%s sends a GraphQL query that is not spelled out in the command, "+
				"so there is no way to tell whether it creates a commit",
			req.Tool,
		))
	}

	return nil
}

// checkScriptText looks for API calls written inside an inline script body, so
// a request made from node -e or python -c is seen even though the command
// itself is only an interpreter invocation.
func (v *APIValidator) checkScriptText(command string) *validator.Result {
	if !v.checksHTTPClients() {
		return nil
	}

	if call := parser.FindCallsInText(command, v.clientCalls); call != "" {
		return v.fail(fmt.Sprintf(
			"the script calls %s, which creates a commit through the GitHub API, %s",
			call, bypassExplanation,
		))
	}

	for _, req := range parser.FindAPICallsInText(command) {
		host, path := parser.SplitURL(req.URL)
		endpoint := parser.NormalizeAPIEndpoint(path)

		if host != "" {
			if !v.isGitHubAPI(host, path) {
				continue
			}
		} else if !req.ExplicitAPICall || !strings.HasPrefix(endpoint, reposPrefix) {
			// A path with no host is only a GitHub call when the syntax names
			// an API client and the path sits under repos/, where every
			// commit-creating REST endpoint lives. Anything else is as likely
			// to be a local route in the same script.
			continue
		}

		if v.blocksEndpoint(req.Method, endpoint) {
			return v.fail(fmt.Sprintf(
				"the script sends %s %s, which creates a commit through the GitHub API, %s",
				req.Method, endpoint, bypassExplanation,
			))
		}
	}

	return nil
}

// isGitHubAPI reports whether a host and path address the GitHub API. A path
// under the Enterprise Server prefixes counts on any host.
func (v *APIValidator) isGitHubAPI(host, path string) bool {
	return slices.Contains(v.hosts, host) || parser.IsGHESAPIPath(path)
}

// blocksEndpoint reports whether a rule rejects this method and endpoint.
func (v *APIValidator) blocksEndpoint(method, endpoint string) bool {
	for _, blocked := range v.endpoints {
		if blocked.method != methodWildcard && blocked.method != method {
			continue
		}

		if blocked.pattern.Match(endpoint) {
			return true
		}
	}

	return false
}

// matchMutation returns the blocked mutation found in a GraphQL body.
func (v *APIValidator) matchMutation(body string) string {
	for _, mutation := range v.mutations {
		if strings.Contains(body, mutation) {
			return mutation
		}
	}

	return ""
}

// fail builds the blocking result. FixHint comes from the suggestions registry.
func (*APIValidator) fail(message string) *validator.Result {
	return validator.FailWithRef(validator.RefGHAPICommit, message).
		AddDetail("help", apiHelp)
}

// failUnverifiable blocks a write that cannot be shown to be safe.
func (*APIValidator) failUnverifiable(message string) *validator.Result {
	return validator.FailWithRef(validator.RefGHAPIUnverifiable, message).
		AddDetail("help", unverifiableHelp)
}
