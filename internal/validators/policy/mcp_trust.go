package policy

import (
	"context"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// MCPTrustValidatorName is the runtime name of the MCP trust validator.
const MCPTrustValidatorName = "mcp-trust"

// maxShownName bounds a server name shown in a message. Names other than
// sdk servers are chosen by whoever configured the server.
const maxShownName = 64

// MCPTrustValidator decides whether an MCP tool call goes to a trusted
// server, by the provenance the harness reports rather than by the server
// name in the tool name.
type MCPTrustValidator struct {
	*validator.BaseValidator
	cfg *config.MCPTrustConfig
}

// NewMCPTrustValidator creates an MCPTrustValidator.
func NewMCPTrustValidator(log logger.Logger, cfg *config.MCPTrustConfig) *MCPTrustValidator {
	return &MCPTrustValidator{
		BaseValidator: validator.NewBaseValidator(MCPTrustValidatorName, log),
		cfg:           cfg,
	}
}

// Category returns the validator category.
func (*MCPTrustValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

// Validate checks the MCP server behind the tool call.
func (v *MCPTrustValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	if !hookCtx.IsMCPTool() {
		return validator.Pass()
	}

	server := hookCtx.MCPServer
	if !hasProvenance(server) {
		return v.unknown(hookCtx)
	}

	if v.trusted(hookCtx, server) {
		return validator.Pass()
	}

	message := "MCP tool " + shown(hookCtx.RawToolName) + " is served by " + describe(server) +
		", which mcp_trust does not trust"
	if claimed := claimedServer(hookCtx.RawToolName); spoofed(claimed, server.Name) {
		message += "; the tool name claims server " + shown(claimed)
	}

	finding := validator.Finding{
		Reference: validator.RefMCPUntrustedSource,
		Location:  shown(hookCtx.RawToolName),
		Message:   "Untrusted MCP server",
		Actual:    describe(server),
		Required:  v.requirement(),
		Repair:    "Use a trusted server's tool, or ask the user to trust this one",
	}

	if v.cfg.GetUntrusted() == config.MCPTrustActionWarn {
		return validator.WarnWithRef(validator.RefMCPUntrustedSource, message).AddFinding(finding)
	}

	return validator.FailWithRef(validator.RefMCPUntrustedSource, message).AddFinding(finding)
}

func hasProvenance(server *hook.MCPProvenance) bool {
	return server != nil &&
		(server.Source != "" || server.Command != "" || server.URL != "" || server.TCP != "")
}

// unknown applies unknown_provenance to a call whose payload does not say
// which server serves it.
func (v *MCPTrustValidator) unknown(hookCtx *hook.Context) *validator.Result {
	action := v.cfg.GetUnknownProvenance()
	if action == config.MCPTrustActionAllow {
		return validator.Pass()
	}

	message := "MCP tool " + shown(hookCtx.RawToolName) + " came without server provenance from " +
		hookCtx.ProviderName() + ", so its server cannot be verified"
	finding := validator.Finding{
		Reference: validator.RefMCPUnknownProvenance,
		Location:  shown(hookCtx.RawToolName),
		Message:   "No MCP server provenance in the hook payload",
		Required:  "mcp_trust.unknown_provenance = " + action,
		Repair:    "Use a tool that is not served over MCP, or ask the user to allow it",
	}

	if action == config.MCPTrustActionWarn {
		return validator.WarnWithRef(validator.RefMCPUnknownProvenance, message).AddFinding(finding)
	}

	return validator.FailWithRef(validator.RefMCPUnknownProvenance, message).AddFinding(finding)
}

func (v *MCPTrustValidator) trusted(hookCtx *hook.Context, server *hook.MCPProvenance) bool {
	if server.Source != "" && slices.Contains(v.cfg.TrustedSources, server.Source) {
		return true
	}

	tool := server.Tool
	if tool == "" {
		tool = ownToolName(hookCtx.RawToolName)
	}

	for _, entry := range v.cfg.Servers {
		if entryMatches(entry, server, tool) {
			return true
		}
	}

	return false
}

// entryMatches reports whether every field the entry sets matches. An
// entry without a provenance field never matches: a name alone is chosen
// by the server's configuration and proves nothing.
func entryMatches(entry *config.MCPTrustedServer, server *hook.MCPProvenance, tool string) bool {
	if !entry.HasProvenance() {
		return false
	}

	fields := []struct{ pattern, value string }{
		{entry.Name, server.Name},
		{entry.Source, server.Source},
		{entry.Command, server.Command},
	}

	for _, field := range fields {
		if field.pattern != "" && !globMatch(field.pattern, field.value) {
			return false
		}
	}

	if entry.URL != "" && !urlMatch(entry.URL, server.URL) {
		return false
	}

	if entry.Args != nil && !argsMatch(entry.Args, server.Args) {
		return false
	}

	if len(entry.Tools) == 0 {
		return true
	}

	for _, pattern := range entry.Tools {
		if globMatch(pattern, tool) {
			return true
		}
	}

	return false
}

// globMatch matches value against a glob. An empty value never matches, so
// a field the harness did not report cannot satisfy an entry.
func globMatch(pattern, value string) bool {
	if value == "" {
		return false
	}

	if pattern == value {
		return true
	}

	matched, err := path.Match(pattern, value)

	return err == nil && matched
}

// urlMatch compares an endpoint with a URL pattern part by part, so a glob
// in the host cannot reach into the path or query: https://*.example.com/*
// does not match https://evil.com?.example.com/x.
func urlMatch(pattern, value string) bool {
	want, err := url.Parse(pattern)
	if err != nil || want.Host == "" {
		return false
	}

	got, err := url.Parse(value)
	if err != nil || got.Host == "" || got.User != nil {
		return false
	}

	if !strings.EqualFold(want.Scheme, got.Scheme) ||
		!globMatch(strings.ToLower(want.Host), strings.ToLower(got.Host)) {
		return false
	}

	wantPath, gotPath := want.Path, got.Path
	if wantPath == "" {
		wantPath = "/"
	}

	if gotPath == "" {
		gotPath = "/"
	}

	return globMatch(wantPath, gotPath)
}

// argsMatch compares stdio arguments one glob per argument.
func argsMatch(patterns, args []string) bool {
	if len(patterns) != len(args) {
		return false
	}

	for i, pattern := range patterns {
		if pattern != args[i] && !globMatch(pattern, args[i]) {
			return false
		}
	}

	return true
}

func (v *MCPTrustValidator) requirement() string {
	var parts []string

	if len(v.cfg.TrustedSources) > 0 {
		parts = append(parts, "trusted_sources = ["+strings.Join(v.cfg.TrustedSources, ", ")+"]")
	}

	if len(v.cfg.Servers) > 0 {
		parts = append(parts, "a matching [[mcp_trust.servers]] entry")
	}

	if len(parts) == 0 {
		return "no MCP server is trusted"
	}

	return strings.Join(parts, " or ")
}

// describe names a server by its provenance, escaping the untrusted name.
func describe(server *hook.MCPProvenance) string {
	var parts []string

	if server.Name != "" {
		parts = append(parts, "server "+shown(server.Name))
	}

	switch {
	case server.Source != "":
		parts = append(parts, "source "+shown(server.Source))
	case server.URL != "":
		parts = append(parts, "url "+shown(server.URL))
	case server.Command != "":
		parts = append(parts, "command "+shown(server.Command))
	case server.TCP != "":
		parts = append(parts, "tcp "+shown(server.TCP))
	}

	return strings.Join(parts, ", ")
}

// shown quotes untrusted text for a message: printable runes only, bounded.
func shown(text string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) && r != '"' && r != '`' {
			return r
		}

		return '?'
	}, text)

	if len([]rune(cleaned)) > maxShownName {
		cleaned = string([]rune(cleaned)[:maxShownName]) + "..."
	}

	return `"` + cleaned + `"`
}

// claimedServer returns the server part of an mcp__<server>__<tool> name.
func claimedServer(toolName string) string {
	rest, ok := strings.CutPrefix(toolName, "mcp__")
	if !ok {
		return ""
	}

	if i := strings.LastIndex(rest, "__"); i > 0 {
		return rest[:i]
	}

	return ""
}

// spoofed reports a tool name whose server part differs from the server
// the harness reports.
func spoofed(claimed, reported string) bool {
	if claimed == "" || reported == "" {
		return false
	}

	return !strings.EqualFold(claimed, reported) && !strings.EqualFold(claimed, mangled(reported))
}

// mangled converts a plugin server's scoped name, plugin:<plugin>:<server>,
// into the plugin_<plugin>_<server> form its tool names use.
func mangled(name string) string {
	return strings.ReplaceAll(name, ":", "_")
}

// ownToolName returns the tool part of an mcp__<server>__<tool> name.
func ownToolName(toolName string) string {
	if _, after, found := strings.CutLast(toolName, "__"); found {
		return after
	}

	return toolName
}
