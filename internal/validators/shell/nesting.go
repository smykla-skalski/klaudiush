package shell

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// NestingValidator blocks a command whose final program the parser cannot
// see: one that does not parse, or one with a part the parser cannot follow
// (deep nesting, an unreadable script, an unresolved git subcommand, an
// exhausted inspection budget). Either could hide a git command from every
// other validator. Each finding names the opaque operation, the programs
// that led to it and a way to make it inspectable.
type NestingValidator struct {
	validator.BaseValidator
}

// NewNestingValidator creates a new NestingValidator instance.
func NewNestingValidator(log logger.Logger) *NestingValidator {
	return &NestingValidator{
		BaseValidator: *validator.NewBaseValidator("validate-nesting", log),
	}
}

const (
	locationCommand = "command"
	originSeparator = " > "
	parseFailedText = "Command does not parse as bash, so what it runs cannot be inspected " +
		"(klaudiush parses commands as bash, not zsh)"
	truncatedText = "Command cannot be fully inspected, so what it runs is unknown"
	kibibyte      = 1 << 10
)

// parsePosition matches the line:column prefix of a shell syntax error.
var parsePosition = regexp.MustCompile(`^(\d+):(\d+):`)

// Validate fails when the command could not be parsed or was cut off.
func (*NestingValidator) Validate(_ context.Context, hookCtx *hook.Context) *validator.Result {
	parsed, err := hookCtx.ParsedCommand()

	var zshErr *parser.ZshSyntaxError

	switch {
	case errors.As(err, &zshErr):
		return validator.FailWithRef(validator.RefShellNesting, zshSummary(zshErr)).
			AddFinding(zshSyntaxFinding(err, zshErr))
	case errors.Is(err, parser.ErrParseFailed):
		return validator.FailWithRef(validator.RefShellNesting, parseFailedText).
			AddFinding(parseFailedFinding(err))
	case err != nil || !parsed.Truncated:
		return validator.Pass()
	}

	findings := make([]validator.Finding, 0, len(parsed.Opacities))
	for _, o := range parsed.Opacities {
		findings = append(findings, opacityFinding(o))
	}

	return validator.FailWithRef(
		validator.RefShellNesting,
		truncatedSummary(parsed.Opacities, parsed.MoreOpacities),
	).AddFinding(findings...)
}

// Category returns the validator category for parallel execution.
func (*NestingValidator) Category() validator.ValidatorCategory {
	return validator.CategoryCPU
}

// parseFailedFinding reports where the syntax error is, never the text
// around it.
func parseFailedFinding(err error) validator.Finding {
	location := locationCommand
	if m := parsePosition.FindStringSubmatch(err.Error()); m != nil {
		location = "line " + m[1] + ", column " + m[2]
	}

	return validator.Finding{
		Reference: validator.RefShellNesting,
		Location:  location,
		Message:   "command does not parse as bash",
		Required:  "valid bash syntax",
		Repair: "If the syntax at that position is broken (unclosed quote, bracket or " +
			"heredoc), fix it. Commands are parsed as bash, not zsh, so rewrite zsh syntax in bash",
	}
}

// zshSummary names the zsh construct, so zsh syntax is not reported as
// broken syntax.
func zshSummary(zshErr *parser.ZshSyntaxError) string {
	switch {
	case zshErr.Possible:
		return "Command does not parse as bash and uses zsh syntax (" + zshErr.Construct +
			") klaudiush cannot inspect"
	case zshErr.Construct == "":
		return "Command does not parse as bash and may use zsh syntax, " +
			"so klaudiush cannot inspect it"
	default:
		return "Command uses zsh syntax (" + zshErr.Construct + ") that bash does not parse, " +
			"so klaudiush cannot inspect it"
	}
}

func zshSyntaxFinding(err error, zshErr *parser.ZshSyntaxError) validator.Finding {
	f := parseFailedFinding(err)

	if zshErr.Possible {
		f.Message = "command does not parse as bash; " + zshErr.Construct +
			" are zsh syntax klaudiush cannot inspect"
		f.Repair = "Rewrite " + zshErr.Construct + " in bash, and fix the syntax at " +
			"that position if it is broken; klaudiush parses every command as bash"

		return f
	}

	if zshErr.Construct == "" {
		f.Message = "command does not parse as bash, though the zsh grammar accepts it"
	} else {
		f.Message = "zsh syntax bash does not parse (" + zshErr.Construct + "), and " +
			"klaudiush inspects commands as bash"
	}

	f.Repair = "Rewrite the command in bash syntax, fixing any syntax error at that " +
		"position; klaudiush parses every command as bash, whatever the login shell"

	return f
}

// truncatedSummary names the single cause, or counts several.
func truncatedSummary(opacities []parser.Opacity, more bool) string {
	switch {
	case more:
		return fmt.Sprintf(
			"Command cannot be fully inspected: more than %d parts are opaque, the first are listed",
			len(opacities),
		)
	case len(opacities) == 0:
		return truncatedText
	case len(opacities) == 1:
		return "Command cannot be inspected: " + causeSummary(opacities[0].Cause)
	default:
		return fmt.Sprintf(
			"Command cannot be fully inspected: %d parts are opaque",
			len(opacities),
		)
	}
}

func causeSummary(cause parser.OpacityCause) string {
	switch cause {
	case parser.OpacityDepthLimit:
		return "it nests launchers, scripts or aliases too deeply"
	case parser.OpacityWorkBudget:
		return "it runs more commands and scripts than klaudiush inspects"
	case parser.OpacityUnreadableScript:
		return "it runs a script klaudiush cannot read"
	case parser.OpacityScriptSyntax:
		return "it runs a script that does not parse as bash"
	case parser.OpacityUnresolvedProgram:
		return "it runs a git subcommand klaudiush cannot resolve"
	case parser.OpacityUnresolvedArgs:
		return "it calls a function whose arguments klaudiush cannot follow"
	case parser.OpacityUnresolvedWord:
		return "it runs eval, git, gh or a container entrypoint with a word klaudiush cannot resolve"
	default:
		return "part of it is opaque"
	}
}

// opacityFinding explains one opaque operation with its origin and repair.
func opacityFinding(o parser.Opacity) validator.Finding {
	f := validator.Finding{
		Reference: validator.RefShellNesting,
		Location:  originLocation(o.Origin),
	}

	switch o.Cause {
	case parser.OpacityDepthLimit:
		f.Message = fmt.Sprintf(
			"%s launches commands nested more than %d launchers, scripts or aliases deep",
			o.Operation, parser.MaxLaunchDepth,
		)
		f.Required = fmt.Sprintf(
			"at most %d nested launchers, scripts, aliases or functions",
			parser.MaxLaunchDepth,
		)
		f.Repair = "Run the inner command directly, dropping wrapper layers " +
			"(env, sudo, bash -c, eval, aliases)"
	case parser.OpacityWorkBudget:
		f.Message = fmt.Sprintf(
			"inspection stopped at %s after %d commands and scripts",
			o.Operation, parser.MaxParseWork,
		)
		f.Required = fmt.Sprintf(
			"at most %d commands and scripts per invocation",
			parser.MaxParseWork,
		)
		f.Repair = "Split the work into smaller commands, or call the programs " +
			"directly instead of through repeated aliases, functions or scripts"
	case parser.OpacityUnreadableScript:
		f.Message = "script " + o.Operation + " cannot be inspected: " + o.Detail
		f.Required = fmt.Sprintf(
			"every script the command runs is readable, under %d KiB",
			parser.MaxScriptBytes/kibibyte,
		)
		f.Repair = unreadableScriptRepair(o.Detail)
	case parser.OpacityScriptSyntax:
		f.Message = o.Operation + " does not parse as bash, so what follows the error is unknown"
		f.Required = "valid bash syntax in every nested script"
		f.Repair = "Fix the syntax of the nested script if it is broken, rewrite zsh syntax " +
			"in it in bash (scripts are parsed as bash, not zsh), or run its commands directly"
	case parser.OpacityUnresolvedProgram:
		f.Message = o.Operation + " is not a git builtin, an installed git command " +
			"or an alias klaudiush can see"
		f.Required = "a git subcommand klaudiush can resolve"
		f.Repair = "Use the builtin subcommand it stands for, or define the alias " +
			"in your global or repository git config in a separate command first"
	case parser.OpacityUnresolvedArgs:
		f.Message = "function " + o.Operation + " forwards arguments with positional " +
			"forms klaudiush does not substitute (slices, defaults, shift or ${10})"
		f.Required = `arguments forwarded as "$@", "$*" or $1 to $9`
		f.Repair = "Run the command inside the function directly, or forward " +
			`arguments with plain "$@"`
	case parser.OpacityUnresolvedWord:
		f.Message, f.Required, f.Repair = unresolvedWordFinding(o)
	default:
		f.Message = o.Operation + " cannot be inspected"
		f.Repair = validator.GetSuggestion(validator.RefShellNesting)
	}

	return f
}

// unresolvedWordFinding explains an eval line or a git or gh command word
// that comes from a variable or command output.
func unresolvedWordFinding(o parser.Opacity) (message, required, repair string) {
	if o.Operation == "eval" {
		message = "eval runs a command line that " + strings.TrimPrefix(o.Detail, "it ")
		required = "eval of literal text or variables assigned literally on the same line"
		repair = "Run the commands directly instead of through eval"

		if setup, ok := evalSetupRepairs[o.Tool]; ok {
			message = "eval runs the shell setup " + o.Tool + " prints, which klaudiush cannot see"
			repair = setup + "; or, if your exception policy allows it, add " +
				"# EXC:SHELL002:<reason> to the command"
		}

		return message, required, repair
	}

	if o.Operation == parser.EntrypointOperation {
		message = "the container --entrypoint or a word before it " + strings.TrimPrefix(
			o.Detail,
			"it ",
		)
		required = "a literal --entrypoint, options and image, or ones from variables " +
			"assigned literally on the same line"
		repair = "Write the entrypoint, options and image literally"

		if o.Detail == parser.DetailEntrypointOptions {
			required = "container options klaudiush can read up to the image"
			repair = "Attach option values with = (--opt=value), or drop options " +
				"before the image"
		}

		return message, required, repair
	}

	message = "the " + o.Operation + " command word " + strings.TrimPrefix(o.Detail, "it ")
	required = "a literal " + o.Operation + " subcommand, or one from a variable " +
		"assigned literally on the same line"

	if o.Detail == parser.DetailWordVariable {
		repair = "Write the subcommand literally, or assign the variable a " +
			"literal value earlier on the same line"
	} else {
		repair = "Write the subcommand literally instead of computing it"
	}

	return message, required, repair
}

// evalSetupRepairs replace eval of a tool's printed shell setup with a form
// klaudiush can inspect, keyed by parser.EvalSetupTools.
var evalSetupRepairs = map[string]string{
	"ssh-agent": "Run the command as the agent's child instead: ssh-agent <command>, " +
		"or ssh-agent bash -c 'ssh-add && <command>' when it needs a key " +
		"(ssh-add has no terminal there, so use a key without a passphrase or SSH_ASKPASS)",
	"mise":   "Run the command through mise instead: mise exec -- <command>",
	"direnv": "Run the command with the directory's environment instead: direnv exec . <command>",
	"rbenv":  "Run the command with the selected Ruby instead: rbenv exec <command>",
	"pyenv":  "Run the command with the selected Python instead: pyenv exec <command>",
	"nodenv": "Run the command with the selected Node instead: nodenv exec <command>",
	"conda":  "Run the command in the environment instead: conda run -n <env> <command>",
	"brew": "Call the program by its path instead: <prefix>/bin/<program>, " +
		"where brew --prefix prints <prefix>",
	"starship": "Drop the eval: starship init only sets up the interactive prompt, " +
		"which a single command does not need",
	"zoxide": "Drop the eval: run zoxide query <keywords> to print the directory, " +
		"then cd to that path literally",
	"fnm": "Run the command with fnm's Node instead: fnm exec --using=<version> <command>",
}

func unreadableScriptRepair(detail string) string {
	switch detail {
	case parser.DetailScriptVariable:
		return "Use a literal script path, or run the script's commands directly"
	case parser.DetailScriptDirectory:
		return "Use an absolute script path, or cd to a literal directory first"
	case parser.DetailScriptWritten:
		return "Write the script with literal content, or write it in a separate " +
			"command before running it"
	default:
		return "Run the script's commands directly, or keep the script a readable " +
			"regular file within the size limit"
	}
}

// originLocation shows the programs that led to an operation.
func originLocation(origin []string) string {
	if len(origin) == 0 {
		return locationCommand
	}

	return "via " + strings.Join(origin, originSeparator)
}
