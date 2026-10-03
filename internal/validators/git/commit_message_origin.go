package git

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
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
	reasonReuseMoved     = "a command earlier on the line may move %q before " +
		"its message is reused"
	reasonReuseRepo = "the reused commit is read from a repository klaudiush cannot " +
		"resolve (cd or git -C it cannot follow, --git-dir, --work-tree or GIT_DIR)"
	reasonTemplateGap    = "the -t template path uses %s"
	reasonTemplateConfig = "--allow-empty-message lets git commit the unedited " +
		"commit.template, which klaudiush does not read"
)

const (
	repairGap = "Write the message literally in -m or in a quoted heredoc " +
		"(-m \"$(cat <<'EOF' ... EOF)\"), or set the variable to a literal earlier on " +
		"the same line"
	repairEditor = "Pass the message with -m or a quoted heredoc on -F -, add " +
		"--no-edit to keep a prepared message, or run git with GIT_EDITOR=:"
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
	cleanupFlag       = "--cleanup"
	fixupAmendPrefix  = "amend:"
	fixupRewordPrefx  = "reword:"
	reeditMessageFlag = "--reedit-message"
	reuseMessageFlag  = gitDirFlag
	reeditShortFlag   = configFlag
	trueEditor        = "true"
)

var (
	reuseFlags = []string{
		reuseMessageFlag,
		"--reuse-message",
		reeditShortFlag,
		reeditMessageFlag,
	}
	templateFlags = []string{"-t", "--template"}
)

// abbreviatedOptions are the long commit options that choose or edit the
// message, which git also accepts abbreviated.
var abbreviatedOptions = []string{
	"--file", "--message", "--template", "--reuse-message", reeditMessageFlag,
	"--fixup", "--squash", "--edit", "--no-edit", cleanupFlag, "--allow-empty-message",
}

// noOpEditors leave the prepared message as it is. A bare "true" is looked
// up in PATH, so it counts only when it resolves to one of the system ones.
var noOpEditors = []string{":", "/usr/bin/true", "/bin/true"}

// exactOptions are commit options that are also a prefix of a longer one,
// which git takes as themselves, not as an abbreviation.
var exactOptions = []string{"--all", "--allow-empty"}

// gitConfigVars set git config from the environment, which may name the
// editor.
var gitConfigVars = []string{"GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"}

// repoVars point git at another repository than the directory it runs in.
var repoVars = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR"}

// repoOptions point git at another repository than the directory it runs in.
var repoOptions = []string{"--git-dir", "--work-tree"}

// cleanupKeys change which lines git's cleanup drops.
var cleanupKeys = []string{"commit.cleanup", "core.commentchar", "core.commentstring"}

// stripModes are the --cleanup modes that drop # lines after an editor.
var stripModes = []string{"", "strip", "default"}

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
func editorRuns(gitCmd *parser.GitCommand, fixupReuses bool) bool {
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

	switch {
	case gitCmd.HasFlag(reeditShortFlag), gitCmd.HasFlag(reeditMessageFlag):
		return true
	case fixupReuses:
		return true
	case slices.ContainsFunc(commitMessageFlags, gitCmd.HasFlag),
		slices.ContainsFunc(commitFileFlags, gitCmd.HasFlag),
		gitCmd.HasFlag(reuseMessageFlag), gitCmd.HasFlag("--reuse-message"),
		gitCmd.HasFlag(fixupFlag):
		return false
	default:
		return true
	}
}

// checkEditor fails unless the editor git opens leaves the prepared message
// as it is: GIT_EDITOR, else core.editor from git -c, set to ":" or the
// system true. Anything else may write any message, including an editor fed
// by a pipe.
func (src messageSource) checkEditor(gitCmd *parser.GitCommand) error {
	editor, known := src.editor(gitCmd)

	switch {
	case known && src.isNoOpEditor(editor):
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

	settings, ok := src.configSettings(gitCmd)
	if !ok {
		return "", false
	}

	editor, found := "", false

	for _, setting := range settings {
		key, value, _ := strings.Cut(setting, "=")

		switch key = strings.ToLower(key); {
		case strings.HasPrefix(key, "include.") || strings.HasPrefix(key, "includeif."):
			return "", false
		case key == coreEditorKey:
			editor, found = value, true
		}
	}

	return editor, found
}

// configSettings returns the key=value settings git -c gives, in order. It
// reports false when one cannot be seen or comes from the environment
// through --config-env.
func (src messageSource) configSettings(gitCmd *parser.GitCommand) ([]string, bool) {
	args := src.cmd.Args

	end := slices.Index(args, gitCmd.Subcommand)
	if end < 0 {
		return nil, false
	}

	var settings []string

	for i := 0; i < end; i++ {
		switch arg := args[i]; {
		case arg == configEnvFlag, strings.HasPrefix(arg, configEnvFlag+"="):
			return nil, false
		case arg != configFlag || i+1 >= end:
			continue
		}

		i++

		setting, gap := src.flagText(gitCmd, parser.FlagValue{Value: args[i], Arg: args[i]})
		if gap != "" || src.substituted(i) {
			return nil, false
		}

		settings = append(settings, setting)
	}

	return settings, true
}

// substituted reports an argument built from a substitution.
func (src messageSource) substituted(i int) bool {
	return i < len(src.cmd.SubstitutedArgs) && src.cmd.SubstitutedArgs[i]
}

// isNoOpEditor reports an editor that leaves the message as it is. git runs
// a bare "true" from PATH, so it counts only when PATH is the one klaudiush
// runs with and finds the system true there.
func (src messageSource) isNoOpEditor(editor string) bool {
	editor = strings.TrimSpace(editor)
	if slices.Contains(noOpEditors, editor) {
		return true
	}

	path := src.cmd.Env("PATH")
	if editor != trueEditor || !path.Known || path.Value != os.Getenv("PATH") {
		return false
	}

	found, err := osexec.LookPath(trueEditor)

	return err == nil && slices.Contains(noOpEditors, found)
}

// cleanupStrips reports that git drops # lines from the message an editor
// leaves: the default or strip cleanup with the default comment character.
func (src messageSource) cleanupStrips(gitCmd *parser.GitCommand) bool {
	if mode, ok := lastValue(gitCmd, []string{cleanupFlag}); ok &&
		!slices.Contains(stripModes, mode.Value) || !ok && gitCmd.HasFlag(cleanupFlag) {
		return false
	}

	settings, ok := src.configSettings(gitCmd)
	if !ok {
		return false
	}

	return !slices.ContainsFunc(settings, func(setting string) bool {
		key, _, _ := strings.Cut(setting, "=")

		return slices.Contains(cleanupKeys, strings.ToLower(key))
	})
}

// fixupRev returns the commit --fixup=amend: or reword: takes its message
// from, which autosquash makes the final message when the editor leaves it.
// The mode is read from the value the shell builds, and a value klaudiush
// cannot see counts as reusing a message, with the gap that hides it.
func (src messageSource) fixupRev(gitCmd *parser.GitCommand) (rev, gap string, ok bool) {
	fv, found := lastValue(gitCmd, []string{fixupFlag})
	if !found {
		return "", "", false
	}

	value, gap := src.flagText(gitCmd, fv)
	if gap != "" {
		return "", gap, true
	}

	mode, rev, cut := strings.Cut(value, ":")
	if !cut || mode+":" != fixupAmendPrefix && mode+":" != fixupRewordPrefx {
		return "", "", false
	}

	return rev, "", true
}

// reusedMessage returns the message of the commit -C, -c or
// --fixup=amend:/reword: takes it from.
func (v *CommitValidator) reusedMessage(
	ctx context.Context,
	gitCmd *parser.GitCommand,
	src messageSource,
	rev, gap string,
	strip bool,
) (string, error) {
	if gap != "" {
		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseGapFormat, gap), repairReuse)
	}

	dir := src.gitDir(gitCmd)

	switch {
	case rev == "" || strings.HasPrefix(rev, "-"):
		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseUnread, rev), repairReuse)
	case !dir.known || src.otherRepo(gitCmd):
		return "", opaqueSourceWith(reasonReuseRepo, repairReuse)
	case src.refsMayMove(gitCmd):
		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseMoved, rev), repairReuse)
	}

	message, err := commitMessage(ctx, src.join(dir.path, "."), rev)
	if err != nil {
		v.Logger().Debug("Reused commit message is unreadable", "rev", rev, "error", err)

		return "", opaqueSourceWith(fmt.Sprintf(reasonReuseUnread, rev), repairReuse)
	}

	if strip {
		message = stripComments(message)
	}

	return strings.TrimSpace(message), nil
}

// otherRepo reports git pointed at a repository other than its directory's.
func (src messageSource) otherRepo(gitCmd *parser.GitCommand) bool {
	for _, option := range repoOptions {
		if _, ok := gitCmd.GlobalOptions[option]; ok {
			return true
		}
	}

	return slices.ContainsFunc(repoVars, func(name string) bool {
		env := src.cmd.Env(name)

		return !env.Known || env.Set
	})
}

// refsMayMove reports a command or file write earlier on the line that is
// not known to leave refs alone, so a ref -C names may point elsewhere when
// the commit runs.
func (src messageSource) refsMayMove(gitCmd *parser.GitCommand) bool {
	if src.parsed == nil {
		return false
	}

	if src.cmd.Dynamic || slices.Contains(src.cmd.SubstitutedArgs, true) {
		return true
	}

	if len(src.parsed.WritesBefore(gitCmd.Location)) > 0 {
		return true
	}

	return slices.ContainsFunc(
		src.parsed.CommandsBefore(gitCmd.Location),
		func(cmd parser.Command) bool { return !readOnlyCommand(cmd) },
	)
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

// templateMessage returns the template git commits as the message when
// nothing else gives one: with an editor that leaves it, or with
// --allow-empty-message.
func (v *CommitValidator) templateMessage(
	gitCmd *parser.GitCommand,
	src messageSource,
	strip bool,
) (string, error) {
	fv, ok := lastValue(gitCmd, templateFlags)
	if !ok {
		if gitCmd.HasFlag("--allow-empty-message") {
			return "", opaqueSourceWith(reasonTemplateConfig, repairTemplate)
		}

		return "", nil
	}

	path, gap := src.flagText(gitCmd, fv)

	switch {
	case gap != "":
		return "", opaqueSourceWith(fmt.Sprintf(reasonTemplateGap, gap), repairTemplate)
	case isStdinPath(path) || isFdPath(path):
		return "", opaqueSourceWith(
			fmt.Sprintf(reasonTemplateGap, parser.GapProcSubst),
			repairTemplate,
		)
	}

	message, err := v.readMessagePath(src, path, src.gitDir(gitCmd), gitCmd.Location)
	if err != nil {
		return "", err
	}

	if strip {
		message = stripComments(message)
	}

	return strings.TrimSpace(message), nil
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
