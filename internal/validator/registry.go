// Package validator provides the validator registry and predicate system.
package validator

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-labs/klaudiush/pkg/hook"
	"github.com/smykla-labs/klaudiush/pkg/parser"
)

// Predicate determines if a validator should be applied to a context.
type Predicate func(*hook.Context) bool

// Registration represents a validator registration with its predicate.
type Registration struct {
	Validator Validator
	Predicate Predicate
}

// Registry manages validator registrations and selection.
type Registry struct {
	registrations []Registration
}

// NewRegistry creates a new empty validator registry.
func NewRegistry() *Registry {
	return &Registry{
		registrations: make([]Registration, 0),
	}
}

// Register adds a validator with a predicate to the registry.
func (r *Registry) Register(validator Validator, predicate Predicate) {
	r.registrations = append(r.registrations, Registration{
		Validator: validator,
		Predicate: predicate,
	})
}

// FindValidators returns all validators whose predicates match the context.
func (r *Registry) FindValidators(ctx *hook.Context) []Validator {
	validators := make([]Validator, 0)

	for _, reg := range r.registrations {
		if reg.Predicate(ctx) {
			validators = append(validators, reg.Validator)
		}
	}

	return validators
}

// Count returns the number of registered validators.
func (r *Registry) Count() int {
	return len(r.registrations)
}

// Common Predicates

// EventTypeIs returns a predicate that matches the given event type.
func EventTypeIs(eventType hook.EventType) Predicate {
	return func(ctx *hook.Context) bool {
		return ctx.EventType == eventType
	}
}

// ToolTypeIs returns a predicate that matches the given tool type.
func ToolTypeIs(toolType hook.ToolType) Predicate {
	return func(ctx *hook.Context) bool {
		return ctx.ToolName == toolType
	}
}

// ToolTypeIn returns a predicate that matches any of the given tool types.
func ToolTypeIn(toolTypes ...hook.ToolType) Predicate {
	return func(ctx *hook.Context) bool {
		return slices.Contains(toolTypes, ctx.ToolName)
	}
}

// CommandMatches returns a predicate that matches if the command matches the pattern.
func CommandMatches(pattern string) Predicate {
	re := regexp.MustCompile(pattern)

	return func(ctx *hook.Context) bool {
		return re.MatchString(ctx.GetCommand())
	}
}

// CommandContains returns a predicate that matches if the command contains the substring.
func CommandContains(substring string) Predicate {
	return func(ctx *hook.Context) bool {
		return strings.Contains(ctx.GetCommand(), substring)
	}
}

// CommandStartsWith returns a predicate that matches if the command starts with the prefix.
func CommandStartsWith(prefix string) Predicate {
	return func(ctx *hook.Context) bool {
		cmd := strings.TrimSpace(ctx.GetCommand())

		return strings.HasPrefix(cmd, prefix)
	}
}

// FilePathMatches returns a predicate that matches if the file path matches the pattern.
func FilePathMatches(pattern string) Predicate {
	return func(ctx *hook.Context) bool {
		matched, err := filepath.Match(pattern, ctx.GetFilePath())

		return err == nil && matched
	}
}

// FilePathContains returns a predicate that matches if the file path contains the substring.
func FilePathContains(substring string) Predicate {
	return func(ctx *hook.Context) bool {
		return strings.Contains(ctx.GetFilePath(), substring)
	}
}

// FileExtensionIs returns a predicate that matches if the file has the given extension.
func FileExtensionIs(ext string) Predicate {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	return func(ctx *hook.Context) bool {
		return filepath.Ext(ctx.GetFilePath()) == ext
	}
}

// FileExtensionIn returns a predicate that matches if the file has any of the given extensions.
func FileExtensionIn(exts ...string) Predicate {
	// Normalize extensions
	normalized := make([]string, len(exts))

	for i, ext := range exts {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}

		normalized[i] = ext
	}

	return func(ctx *hook.Context) bool {
		fileExt := filepath.Ext(ctx.GetFilePath())
		return slices.Contains(normalized, fileExt)
	}
}

// BashWritesFileWithExtension returns a predicate that matches if a Bash command writes
// to a file with any of the given extensions.
func BashWritesFileWithExtension(exts ...string) Predicate {
	// Normalize extensions
	normalized := make([]string, len(exts))

	for i, ext := range exts {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}

		normalized[i] = ext
	}

	return func(ctx *hook.Context) bool {
		// Only apply to Bash commands
		if ctx.ToolName != hook.ToolTypeBash {
			return false
		}

		// Parse the bash command
		bashParser := parser.NewBashParser()

		result, err := bashParser.Parse(ctx.GetCommand())
		if err != nil {
			return false
		}

		// Check if any file write has a matching extension
		for _, fw := range result.FileWrites {
			fileExt := filepath.Ext(fw.Path)
			if slices.Contains(normalized, fileExt) {
				return true
			}
		}

		return false
	}
}

// Predicate Combinators

// And returns a predicate that matches if all predicates match.
func And(predicates ...Predicate) Predicate {
	return func(ctx *hook.Context) bool {
		for _, p := range predicates {
			if !p(ctx) {
				return false
			}
		}

		return true
	}
}

// Or returns a predicate that matches if any predicate matches.
func Or(predicates ...Predicate) Predicate {
	return func(ctx *hook.Context) bool {
		for _, p := range predicates {
			if p(ctx) {
				return true
			}
		}

		return false
	}
}

// Not returns a predicate that inverts the given predicate.
func Not(predicate Predicate) Predicate {
	return func(ctx *hook.Context) bool {
		return !predicate(ctx)
	}
}

// Always returns a predicate that always matches.
func Always() Predicate {
	return func(*hook.Context) bool {
		return true
	}
}

// Never returns a predicate that never matches.
func Never() Predicate {
	return func(*hook.Context) bool {
		return false
	}
}

// Git Command Predicates

// GitSubcommandIs returns a predicate that matches if any git command in the chain
// has the given subcommand. This properly handles command chains like "git add && git commit".
func GitSubcommandIs(subcommand string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if gitCmd.Subcommand == subcommand {
				return true
			}
		}

		return false
	}
}

// GitSubcommandIn returns a predicate that matches if any git command in the chain
// has any of the given subcommands.
func GitSubcommandIn(subcommands ...string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if slices.Contains(subcommands, gitCmd.Subcommand) {
				return true
			}
		}

		return false
	}
}

// GitHasFlag returns a predicate that matches if any git command in the chain has the given flag.
func GitHasFlag(flag string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if gitCmd.HasFlag(flag) {
				return true
			}
		}

		return false
	}
}

// GitHasAnyFlag returns a predicate that matches if any git command in the chain
// has any of the given flags.
func GitHasAnyFlag(flags ...string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if slices.ContainsFunc(flags, gitCmd.HasFlag) {
				return true
			}
		}

		return false
	}
}

// GitSubcommandWithFlag returns a predicate that matches if any git command in the chain
// has the given subcommand AND has the given flag.
func GitSubcommandWithFlag(subcommand, flag string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if gitCmd.Subcommand == subcommand && gitCmd.HasFlag(flag) {
				return true
			}
		}

		return false
	}
}

// GitSubcommandWithAnyFlag returns a predicate that matches if any git command in the chain
// has the given subcommand AND has any of the given flags.
func GitSubcommandWithAnyFlag(subcommand string, flags ...string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if gitCmd.Subcommand == subcommand && slices.ContainsFunc(flags, gitCmd.HasFlag) {
				return true
			}
		}

		return false
	}
}

// GitSubcommandWithoutFlag returns a predicate that matches if any git command in the chain
// has the given subcommand AND does NOT have the given flag.
func GitSubcommandWithoutFlag(subcommand, flag string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if gitCmd.Subcommand == subcommand && !gitCmd.HasFlag(flag) {
				return true
			}
		}

		return false
	}
}

// GitSubcommandWithoutAnyFlag returns a predicate that matches if any git command in the chain
// has the given subcommand AND does NOT have any of the given flags.
func GitSubcommandWithoutAnyFlag(subcommand string, flags ...string) Predicate {
	return func(ctx *hook.Context) bool {
		gitCmds := parseAllGitFromContext(ctx)

		for _, gitCmd := range gitCmds {
			if gitCmd.Subcommand == subcommand && !slices.ContainsFunc(flags, gitCmd.HasFlag) {
				return true
			}
		}

		return false
	}
}

// parseAllGitFromContext parses all git commands from a hook context.
// Returns all git commands found in command chains like "git add && git commit".
// Returns empty slice if no git commands are found or parsing fails.
func parseAllGitFromContext(ctx *hook.Context) []*parser.GitCommand {
	if ctx.ToolName != hook.ToolTypeBash {
		return nil
	}

	bashParser := parser.NewBashParser()

	result, err := bashParser.Parse(ctx.GetCommand())
	if err != nil {
		return nil
	}

	var gitCmds []*parser.GitCommand

	for _, cmd := range result.Commands {
		if cmd.Name == "git" {
			gitCmd, err := parser.ParseGitCommand(cmd)
			if err != nil {
				continue // Skip invalid git commands but continue processing
			}

			gitCmds = append(gitCmds, gitCmd)
		}
	}

	return gitCmds
}
