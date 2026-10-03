package git

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// Reasons a commit message source is opaque, completing opaqueSummary.
const (
	reasonGapFormat    = "the message uses %s"
	reasonEditorFormat = "git opens the editor %q for the message, and klaudiush " +
		"cannot see what it writes"
	reasonEditorUnknown = "git opens an editor for the message (from git config, " +
		"VISUAL, EDITOR or the default vi), and klaudiush cannot see what it writes"
	reasonReuseGapFormat = "the commit whose message is reused uses %s"
	reasonReuseUnread    = "the message of the reused commit %q cannot be read"
	reasonReuseMoved     = "a git command earlier on the line may move %q before " +
		"its message is reused"
	reasonTemplateGap    = "the -t template path uses %s"
	reasonTemplateConfig = "--allow-empty-message lets git commit the unedited " +
		"commit.template, which klaudiush does not read"
)

const (
	repairGap = "Write the message literally in -m or in a quoted heredoc " +
		"(-m \"$(cat <<'EOF' ... EOF)\"), or set the variable to a literal earlier on " +
		"the same line"
	repairEditor = "Pass the message with -m or a quoted heredoc on -F -, add " +
		"--no-edit to keep a prepared message, or run git with GIT_EDITOR=true"
	repairReuse = "Reuse a commit that exists by a literal ref or hash, run the " +
		"commands that move it separately first, or pass the message with -m"
	repairTemplate = "Pass the message with -m or a quoted heredoc on -F -, " +
		"or name a template file with -t at a literal path"
)

const (
	gitEditorVar      = "GIT_EDITOR"
	coreEditorKey     = "core.editor"
	configFlag        = "-c"
	configEnvFlag     = "--config-env"
	fixupFlag         = "--fixup"
	squashFlag        = "--squash"
	amendFlag         = "--amend"
	fixupAmendPrefix  = "amend:"
	fixupRewordPrefx  = "reword:"
	reeditMessageFlag = "--reedit-message"
	reuseMessageFlag  = gitDirFlag
	reeditShortFlag   = configFlag
)

var (
	reuseFlags = []string{
		reuseMessageFlag,
		"--reuse-message",
		reeditShortFlag,
		reeditMessageFlag,
	}
	reeditFlags   = []string{reeditShortFlag, reeditMessageFlag}
	templateFlags = []string{"-t", "--template"}
)

// abbreviatedOptions are the long commit options that choose or edit the
// message, which git also accepts abbreviated.
var abbreviatedOptions = []string{
	"--file", "--message", "--template", "--reuse-message", reeditMessageFlag,
	"--fixup", "--squash", "--edit", "--no-edit",
}

// noOpEditors leave the prepared message as it is.
var noOpEditors = []string{":", "true", "/usr/bin/true", "/bin/true"}

// gitConfigVars set git config from the environment, which may name the
// editor.
var gitConfigVars = []string{"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"}

// fullObjectID matches a SHA-1 or SHA-256 object name, which no ref move
// changes.
var fullObjectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// commentLine matches a line git's default cleanup removes after an editor.
var commentLine = regexp.MustCompile(`(?m)^#.*(?:\n|$)`)

// flagText returns the value a flag gets once the shell builds it, or the gap
// that keeps it unknown.
func (src messageSource) flagText(
	gitCmd *parser.GitCommand,
	fv parser.FlagValue,
) (value, gap string) {
	if text, ok := src.cmd.ArgText(fv.Arg); ok {
		if fv.Arg != fv.Value {
			prefix, glued := strings.CutSuffix(fv.Arg, fv.Value)
			if text, ok = text.TrimPrefix(prefix); !glued || !ok {
				return "", parser.GapAmbiguous
			}
		}

		return text.Value()
	}

	switch {
	case fv.Flag != "" && gitCmd.HasDynamicValue(fv.Flag):
		return "", parser.GapCommandOutput
	case parser.HasUnresolvedVars(fv.Value):
		return "", fmt.Sprintf(parser.GapReferenceFormat, fv.Value)
	default:
		return fv.Value, ""
	}
}

// lastValue returns the last value given to any of flags, which is the one
// git keeps.
func lastValue(gitCmd *parser.GitCommand, flags []string) (parser.FlagValue, bool) {
	values := gitCmd.ValuesOf(flags...)
	if len(values) == 0 {
		return parser.FlagValue{}, false
	}

	return values[len(values)-1], true
}

// inlineMessage joins every -m value as git does, one paragraph each, with
// the variables they use expanded.
func (src messageSource) inlineMessage(gitCmd *parser.GitCommand) (string, error) {
	var paragraphs []string

	for _, fv := range gitCmd.ValuesOf(commitMessageFlags...) {
		text, gap := src.flagText(gitCmd, fv)
		if gap != "" {
			return "", opaqueSourceWith(fmt.Sprintf(reasonGapFormat, gap), repairGap)
		}

		if text = strings.TrimSpace(text); text != "" {
			paragraphs = append(paragraphs, text)
		}
	}

	return strings.Join(paragraphs, "\n\n"), nil
}

// stdinMessage returns what feeds -F -, with the variables of a heredoc or
// here-string expanded.
func (src messageSource) stdinMessage(gitCmd *parser.GitCommand) (string, error) {
	text, ok := src.cmd.StdinText()
	if !ok {
		if parser.HasUnresolvedVars(gitCmd.Stdin) {
			gap := fmt.Sprintf(parser.GapReferenceFormat, strings.TrimSpace(gitCmd.Stdin))

			return "", opaqueSourceWith(fmt.Sprintf(reasonGapFormat, gap), repairGap)
		}

		return strings.TrimSpace(gitCmd.Stdin), nil
	}

	value, gap := text.Value()
	if gap != "" {
		return "", opaqueSourceWith(fmt.Sprintf(reasonGapFormat, gap), repairGap)
	}

	return strings.TrimSpace(value), nil
}

// editorRuns reports whether git commit opens an editor for the message.
func editorRuns(gitCmd *parser.GitCommand) bool {
	if gitCmd.HasFlag("--dry-run") {
		return false
	}

	for _, flag := range slices.Backward(gitCmd.Flags) {
		switch flag {
		case "-e", "--edit":
			return true
		case "--no-edit":
			return false
		}
	}

	fixup, hasFixup := lastValue(gitCmd, []string{fixupFlag})

	switch {
	case slices.ContainsFunc(reeditFlags, gitCmd.HasFlag):
		return true
	case hasFixup && (strings.HasPrefix(fixup.Value, fixupAmendPrefix) ||
		strings.HasPrefix(fixup.Value, fixupRewordPrefx)):
		return true
	case slices.ContainsFunc(commitMessageFlags, gitCmd.HasFlag),
		slices.ContainsFunc(commitFileFlags, gitCmd.HasFlag),
		gitCmd.HasFlag(reuseMessageFlag), gitCmd.HasFlag("--reuse-message"), gitCmd.HasFlag(fixupFlag):
		return false
	default:
		return true
	}
}

// checkEditor fails unless the editor git opens leaves the prepared message
// as it is: GIT_EDITOR, else core.editor from git -c, set to true or ":".
// Anything else may write any message, including an editor fed by a pipe.
func (src messageSource) checkEditor(gitCmd *parser.GitCommand) error {
	editor, known := src.editor(gitCmd)

	switch {
	case known && isNoOpEditor(editor):
		return nil
	case known:
		return opaqueSourceWith(fmt.Sprintf(reasonEditorFormat, editor), repairEditor)
	default:
		return opaqueSourceWith(reasonEditorUnknown, repairEditor)
	}
}

// editor returns the editor git runs when klaudiush can tell it.
func (src messageSource) editor(gitCmd *parser.GitCommand) (string, bool) {
	gitEditor := src.cmd.Env(gitEditorVar)

	switch {
	case !gitEditor.Known:
		return "", false
	case gitEditor.Set:
		return gitEditor.Value, true
	}

	for _, name := range gitConfigVars {
		if env := src.cmd.Env(name); !env.Known || env.Set {
			return "", false
		}
	}

	return src.configEditor(gitCmd)
}

// configEditor returns core.editor as git -c sets it, the last one winning.
// A setting klaudiush cannot see, or one --config-env takes from the
// environment, is unknown.
func (src messageSource) configEditor(gitCmd *parser.GitCommand) (string, bool) {
	args := src.cmd.Args

	end := slices.Index(args, gitCmd.Subcommand)
	if end < 0 {
		return "", false
	}

	editor, found := "", false

	for i := 0; i < end; i++ {
		switch arg := args[i]; {
		case arg == configEnvFlag && i+1 < end && isEditorKey(args[i+1]),
			strings.HasPrefix(arg, configEnvFlag+"=") && isEditorKey(arg[len(configEnvFlag)+1:]):
			return "", false
		case arg != configFlag || i+1 >= end:
			continue
		}

		i++

		setting, gap := src.flagText(gitCmd, parser.FlagValue{Value: args[i], Arg: args[i]})
		if gap != "" || src.substituted(i) {
			return "", false
		}

		if key, value, _ := strings.Cut(setting, "="); strings.EqualFold(key, coreEditorKey) {
			editor, found = value, true
		}
	}

	return editor, found
}

// substituted reports an argument built from a substitution.
func (src messageSource) substituted(i int) bool {
	return i < len(src.cmd.SubstitutedArgs) && src.cmd.SubstitutedArgs[i]
}

// isEditorKey reports a key=value setting, or its key, that names core.editor.
func isEditorKey(setting string) bool {
	key, _, _ := strings.Cut(setting, "=")

	return strings.EqualFold(key, coreEditorKey)
}

func isNoOpEditor(editor string) bool {
	return slices.Contains(noOpEditors, strings.TrimSpace(editor))
}

// reusedMessage returns the message of the commit -C or -c reuses.
func (v *CommitValidator) reusedMessage(
	ctx context.Context,
	gitCmd *parser.GitCommand,
	src messageSource,
	fv parser.FlagValue,
) (string, error) {
	rev, gap := src.flagText(gitCmd, fv)
	if gap != "" {
		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseGapFormat, gap), repairReuse)
	}

	dir := src.gitDir(gitCmd)

	switch {
	case rev == "" || strings.HasPrefix(rev, "-"):
		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseUnread, rev), repairReuse)
	case !dir.known:
		return "", opaqueSource(reasonDirectory)
	case !fullObjectID.MatchString(rev) && src.refsMayMove(gitCmd):
		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseMoved, rev), repairReuse)
	}

	message, err := commitMessage(ctx, src.join(dir.path, "."), rev)
	if err != nil {
		v.Logger().Debug("Reused commit message is unreadable", "rev", rev, "error", err)

		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseUnread, rev), repairReuse)
	}

	if slices.Contains(reeditFlags, fv.Flag) {
		message = stripComments(message)
	}

	return strings.TrimSpace(message), nil
}

// refsMayMove reports a git or gh command earlier on the line that is not
// known to leave refs alone, so a ref -C names may point elsewhere when the
// commit runs.
func (src messageSource) refsMayMove(gitCmd *parser.GitCommand) bool {
	if src.parsed == nil {
		return false
	}

	for _, cmd := range src.parsed.CommandsBefore(gitCmd.Location) {
		switch cmd.Name {
		case gitCommand:
			if !readOnlyCommand(cmd) {
				return true
			}
		case "gh", "hub":
			return true
		}
	}

	return false
}

// commitMessage reads the full message of rev in dir.
func commitMessage(ctx context.Context, dir, rev string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitCommandTimeout)
	defer cancel()

	result := exec.NewCommandRunner(gitCommandTimeout).Run(
		ctx, gitCommand, "-C", dir, "log", "-1", "--no-show-signature",
		"--format=%B", rev+"^{commit}", "--",
	)
	if result.Err != nil {
		return "", errors.Wrapf(result.Err, "read message of %s", rev)
	}

	return result.Stdout, nil
}

// templateMessage returns the template an editor that changes nothing
// commits as the message.
func (v *CommitValidator) templateMessage(
	gitCmd *parser.GitCommand,
	src messageSource,
) (string, error) {
	fv, ok := lastValue(gitCmd, templateFlags)
	if !ok {
		if gitCmd.HasFlag("--allow-empty-message") {
			return "", opaqueSourceWith(reasonTemplateConfig, repairTemplate)
		}

		return "", nil
	}

	_, gap := src.flagText(gitCmd, fv)

	switch {
	case gap != "":
		return "", opaqueSourceWith(fmt.Sprintf(reasonTemplateGap, gap), repairTemplate)
	case isStdinPath(fv.Value) || isFdPath(fv.Value):
		return "", opaqueSourceWith(
			fmt.Sprintf(reasonTemplateGap, parser.GapProcSubst),
			repairTemplate,
		)
	}

	message, err := v.readMessagePath(src, fv.Value, src.gitDir(gitCmd), gitCmd.Location)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stripComments(message)), nil
}

// usesTemplate reports a commit whose prepared message is the template: one
// git does not fill from another commit.
func usesTemplate(gitCmd *parser.GitCommand) bool {
	return !gitCmd.HasFlag(amendFlag) && !gitCmd.HasFlag(fixupFlag) &&
		!gitCmd.HasFlag(squashFlag)
}

// stripComments removes the lines git's default cleanup drops after an
// editor runs.
func stripComments(message string) string {
	return commentLine.ReplaceAllString(message, "")
}
