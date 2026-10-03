package git

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// maxMessageFileBytes bounds the -F file read; a real message is far smaller.
const maxMessageFileBytes = 1 << 20

const (
	stdinPath       = "/dev/stdin"
	fdDir           = "/dev/fd/"
	procDir         = "/proc/"
	procFdSegment   = "/fd/"
	stdinFd         = "0"
	messageLocation = "commit message"
	opaqueSummary   = "Commit message cannot be inspected: "
)

// Reasons a commit message source is opaque, completing opaqueSummary.
const (
	reasonFdSource = "-F reads a process substitution or file descriptor " +
		"whose content klaudiush cannot see"
	reasonOutputPath = "the -F path comes from command output klaudiush cannot see"
	reasonVarPath    = "the -F path depends on a variable klaudiush cannot resolve"
	reasonStdin      = "-F reads stdin, and klaudiush cannot see what feeds it"
	reasonDirectory  = "the -F path is relative to a directory klaudiush cannot resolve " +
		"(cd or git -C with a variable or command output)"
	reasonRewritten = "the -F file is written earlier in the command " +
		"with content klaudiush cannot see"
	reasonDynamicWrite = "the command writes to a file whose name klaudiush cannot see " +
		"before the commit, which may be the -F file"
	reasonUnreadable = "the -F file cannot be read (missing, unreadable, " +
		"over 1 MiB or not a regular file)"
)

const (
	repairInline = "Pass the message with -m, or write it in a quoted heredoc on " +
		"-F - (git commit ... -F - <<'EOF' ... EOF), keeping the other flags"
	repairUnreadable = "Create the message file at a literal path before this " +
		"command, or pass the message with -m or a quoted heredoc on -F -"
	requiredSource = "a message klaudiush can read: -m text, a quoted heredoc " +
		"on -F -, or a readable -F file at a literal path"
)

// envPathVars are the environment variables a -F path may use when the line
// does not set them; the hook inherits them from the same session.
var envPathVars = []string{"HOME", "TMPDIR"}

// varRef matches a variable reference as the parser renders it.
var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// opaqueMessageError reports a commit message source klaudiush cannot read,
// so the commit fails closed instead of skipping message validation.
type opaqueMessageError struct {
	reason string
	repair string
}

func (e *opaqueMessageError) Error() string {
	return opaqueSummary + e.reason
}

func opaqueSource(reason string) error {
	return &opaqueMessageError{reason: reason, repair: repairInline}
}

// opaqueMessageResult blocks a commit whose message klaudiush cannot read.
func opaqueMessageResult(err error) (*validator.Result, bool) {
	var opaqueErr *opaqueMessageError
	if !errors.As(err, &opaqueErr) {
		return nil, false
	}

	return validator.FailWithRef(
		validator.RefShellNesting,
		opaqueSummary+opaqueErr.reason,
	).AddFinding(validator.Finding{
		Reference: validator.RefShellNesting,
		Location:  messageLocation,
		Message:   opaqueErr.reason,
		Required:  requiredSource,
		Repair:    opaqueErr.repair,
	}), true
}

// messageSource is a git commit with what the parse knows about the command
// that runs it.
type messageSource struct {
	cmd     parser.Command
	parsed  *parser.ParseResult
	command string
}

// hasFileFlag reports a -F/--file flag even when its value rendered empty,
// as a substitution glued to the flag does.
func hasFileFlag(gitCmd *parser.GitCommand) bool {
	return slices.ContainsFunc(commitFileFlags, gitCmd.HasFlag)
}

// isStdinPath reports a path that reads the commit's stdin.
func isStdinPath(path string) bool {
	return path == "-" || path == stdinPath || path == fdDir+stdinFd ||
		path == procDir+"self"+procFdSegment+stdinFd
}

// isFdPath reports a file descriptor path, which is what a process
// substitution stands for.
func isFdPath(path string) bool {
	return strings.HasPrefix(path, fdDir) ||
		(strings.HasPrefix(path, procDir) && strings.Contains(path, procFdSegment))
}

// readMessageFile resolves the message of a -F/--file flag, or an
// opaqueMessageError when it cannot be seen.
func (v *CommitValidator) readMessageFile(
	gitCmd *parser.GitCommand,
	src messageSource,
	filePath string,
) (string, error) {
	switch {
	case isStdinPath(filePath):
		return v.readMessageStdin(gitCmd, src)
	case isFdPath(filePath):
		return "", opaqueSource(reasonFdSource)
	case filePath == "" || (src.cmd.Dynamic && !literalFileArg(src.command, filePath)):
		return "", opaqueSource(reasonOutputPath)
	}

	return v.readMessagePath(gitCmd, src, filePath)
}

// readMessageStdin returns what a heredoc, here-string, literal echo or a
// redirected file feeds to -F -.
func (v *CommitValidator) readMessageStdin(
	gitCmd *parser.GitCommand,
	src messageSource,
) (string, error) {
	if stdin := strings.TrimSpace(gitCmd.Stdin); stdin != "" {
		v.Logger().Debug("Reading commit message from stdin (-F -)")

		return stdin, nil
	}

	if file := src.cmd.StdinFile; file != "" && !isStdinPath(file) && !isFdPath(file) {
		return v.readMessagePath(gitCmd, src, file)
	}

	return "", opaqueSource(reasonStdin)
}

// readMessagePath returns the content of a message file: what the command
// writes to it first when that is known, else the file on disk.
func (v *CommitValidator) readMessagePath(
	gitCmd *parser.GitCommand,
	src messageSource,
	filePath string,
) (string, error) {
	workDir := gitCmd.GetWorkingDirectory()

	if src.parsed != nil {
		content, ok := src.parsed.InlineFileContent(filePath, workDir, gitCmd.Location)
		if ok {
			v.Logger().Debug("Reading commit message from inline file write", "path", filePath)

			return strings.TrimSpace(content), nil
		}

		if src.parsed.FileWrittenBefore(filePath, workDir, gitCmd.Location) {
			return "", opaqueSource(reasonRewritten)
		}
	}

	readPath, err := resolveMessagePath(gitCmd, src, filePath)
	if err != nil {
		return "", err
	}

	if src.parsed != nil && src.parsed.DynamicWrites > 0 {
		return "", opaqueSource(reasonDynamicWrite)
	}

	v.Logger().Debug("Reading commit message from file", "path", readPath)

	content, err := readRegularFile(readPath)
	if err != nil {
		v.Logger().Debug("Commit message file is unreadable", "error", err)

		return "", &opaqueMessageError{reason: reasonUnreadable, repair: repairUnreadable}
	}

	return strings.TrimSpace(content), nil
}

// resolveMessagePath resolves a -F path the way the shell and git would:
// variables from the line (and HOME/TMPDIR from the environment), a leading
// ~, and a relative path joined onto the commit's working directory.
func resolveMessagePath(
	gitCmd *parser.GitCommand,
	src messageSource,
	filePath string,
) (string, error) {
	readPath := expandPathVars(src, filePath)
	if parser.HasUnresolvedVars(readPath) || usesDynamicVar(src.cmd.Vars, filePath) {
		return "", opaqueSource(reasonVarPath)
	}

	readPath = expandTilde(readPath)
	if filepath.IsAbs(readPath) {
		return filepath.Clean(readPath), nil
	}

	cDir, hasC := gitCmd.GlobalOptions["-C"]
	if src.cmd.DirUnknown || (hasC && (cDir == "" || parser.HasUnresolvedVars(cDir))) {
		return "", opaqueSource(reasonDirectory)
	}

	if workDir := expandTilde(gitCmd.GetWorkingDirectory()); workDir != "" {
		readPath = filepath.Join(workDir, readPath)
	}

	return filepath.Clean(readPath), nil
}

// expandPathVars substitutes the line's variables, then the environment
// variables the line leaves untouched.
func expandPathVars(src messageSource, path string) string {
	path = src.cmd.Vars.ExpandVars(path)

	for _, name := range envPathVars {
		ref := "${" + name + "}"
		if !strings.Contains(path, ref) || mayAssignVar(src.command, name) {
			continue
		}

		if value, ok := os.LookupEnv(name); ok && value != "" {
			path = strings.ReplaceAll(path, ref, value)
		}
	}

	return path
}

// mayAssignVar reports command text that may set name, so the environment
// value is not what the shell uses.
func mayAssignVar(command, name string) bool {
	word := regexp.QuoteMeta(name)
	assignment := regexp.MustCompile(
		`(^|[^A-Za-z0-9_])` + word + `\+?=` +
			`|\b(read|for|local|declare|typeset|export|unset|readonly)\b[^;&|\n]*\b` + word + `\b`,
	)

	return assignment.MatchString(command)
}

// usesDynamicVar reports a path that names a variable holding command output.
func usesDynamicVar(vars *parser.VarScope, path string) bool {
	for _, ref := range varRef.FindAllStringSubmatch(path, -1) {
		if vars.IsDynamic(ref[1]) {
			return true
		}
	}

	return false
}

// literalFileArg reports whether value is written literally as the argument
// of -F or --file in the command text. A command with a substitution renders
// that word partially or drops it, letting -F take the next argument.
func literalFileArg(command, value string) bool {
	quoted := `['"]?` + literalPattern(value) + `['"]?(?:$|[\s;&|)<>])`
	pattern := `(?:^|[\s;&|(])(?:-[A-Za-z]*F\s*|--file(?:=|\s+))` + quoted

	return regexp.MustCompile(pattern).MatchString(command)
}

// literalPattern matches value as written, where a ${NAME} the parser
// rendered may be written $NAME.
func literalPattern(value string) string {
	var b strings.Builder

	rest := value
	for {
		loc := varRef.FindStringSubmatchIndex(rest)
		if loc == nil {
			b.WriteString(regexp.QuoteMeta(rest))

			return b.String()
		}

		b.WriteString(regexp.QuoteMeta(rest[:loc[0]]))
		b.WriteString(`\$\{?` + regexp.QuoteMeta(rest[loc[2]:loc[3]]) + `\}?`)
		rest = rest[loc[1]:]
	}
}

// readRegularFile reads a regular file of bounded size. It opens without
// blocking and checks the open file, so a FIFO or device at the path cannot
// hang the hook.
func readRegularFile(path string) (string, error) {
	file, err := os.OpenFile(filepath.Clean(path), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", errors.Wrapf(err, "open commit message file %s", path)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", errors.Wrapf(err, "stat commit message file %s", path)
	}

	if !info.Mode().IsRegular() {
		return "", errors.Newf("commit message file %s is not a regular file", path)
	}

	content, err := io.ReadAll(io.LimitReader(file, maxMessageFileBytes+1))
	if err != nil {
		return "", errors.Wrapf(err, "read commit message file %s", path)
	}

	if len(content) > maxMessageFileBytes {
		return "", errors.Newf("commit message file %s is larger than 1 MiB", path)
	}

	return string(content), nil
}
