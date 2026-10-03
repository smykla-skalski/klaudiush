package git

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// maxMessageFileBytes bounds the -F file read; a real message is far smaller.
const maxMessageFileBytes = 1 << 20

const (
	gitDirFlag      = "-C"
	stdinPath       = "/dev/stdin"
	fdDir           = "/dev/fd/"
	procDir         = "/proc/"
	procFdSegment   = "/fd/"
	stdinFd         = "0"
	homeVar         = "HOME"
	noFileFlag      = "--no-file"
	minAbbrevLen    = len("--m")
	messageLocation = "commit message"
	opaqueSummary   = "Commit message cannot be inspected: "
)

// Reasons a commit message source is opaque, completing opaqueSummary.
const (
	reasonFdSource = "-F reads a process substitution or file descriptor " +
		"whose content klaudiush cannot see"
	reasonSubstituted = "the -F value comes from a command or process substitution " +
		"klaudiush cannot see"
	reasonVarPath   = "the -F path depends on a variable klaudiush cannot resolve"
	reasonStdin     = "-F reads stdin, and klaudiush cannot see what feeds it"
	reasonTwoStdins = "-F reads stdin fed by both a redirect and a pipe, heredoc or " +
		"here-string, so which one git reads is unclear"
	reasonDirectory = "the -F path is relative to a directory klaudiush cannot resolve " +
		"(cd or git -C with a variable or command output, or repeated git -C)"
	reasonRewritten = "the message file is written earlier in the command " +
		"with content klaudiush cannot see"
	reasonChanged      = "a command earlier on the line may change the message file"
	reasonUnknownWrite = "the command writes to a file whose name klaudiush cannot see " +
		"before the commit, which may be the message file"
	reasonMissing    = "the message file does not exist"
	reasonNotRegular = "the message file is not a regular file"
	reasonTooLarge   = "the message file is larger than 1 MiB"
	reasonUnreadable = "the message file cannot be read (permission or I/O error)"
	reasonMaybeRun   = "the message file is written earlier only on a branch or in a " +
		"background job that may not run before the commit"
	reasonRepeated = "the commit has more than one -F/--file, and git reads only the last"
	reasonAbbrev   = "the commit abbreviates a message option (--file, --message, " +
		"--edit, ...), which klaudiush does not expand"
)

const (
	repairInline = "Pass the message with -m, or write it in a quoted heredoc on " +
		"-F - (git commit ... -F - <<'EOF' ... EOF), keeping the other flags"
	repairMissing = "Create the message file at a literal path before this " +
		"command, or pass the message with -m or a quoted heredoc on -F -"
	repairOneSource = "Give the message once: a single -m, -F or --file, " +
		"spelled out in full"
	repairSeparate = "Run the command that writes the message file separately " +
		"first, or pass the message with -m or a quoted heredoc on -F -"
	requiredSource = "a message klaudiush can read: -m text, a quoted heredoc " +
		"on -F -, or a readable -F file at a literal path"
)

// varRef matches a variable reference as the parser renders it.
var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// maxLinkHops bounds symlink following, as the kernel does.
const maxLinkHops = 40

var (
	errNotRegular = errors.New("not a regular file")
	errTooLarge   = errors.New("larger than 1 MiB")
)

// readFailureReason names why a message file on disk could not be read.
func readFailureReason(err error) (reason, repair string) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return reasonMissing, repairMissing
	case errors.Is(err, errNotRegular):
		return reasonNotRegular, repairInline
	case errors.Is(err, errTooLarge):
		return reasonTooLarge, repairInline
	default:
		return reasonUnreadable, repairMissing
	}
}

// homeAssignment matches command text that may set HOME, so the hook's own
// HOME is not what the shell expands.
var homeAssignment = regexp.MustCompile(
	`(^|[^A-Za-z0-9_])HOME\+?=` +
		`|\b(read|for|local|declare|typeset|export|unset|readonly)\b[^;&|\n]*\bHOME\b`,
)

// readOnlyPrograms read the files they name without changing them.
var readOnlyPrograms = []string{
	"cat", "head", "tail", "grep", "egrep", "fgrep", "rg", "wc", "test", "[",
	"ls", "stat", "file", "less", "more", "diff", "cmp", "echo", "printf",
	"realpath", "readlink", "basename", "dirname", "du", "md5", "md5sum",
	"cd", "pushd", "popd", "mkdir", "touch", "alias",
	"shasum", "sha1sum", "sha256sum",
}

// filePlacers put, replace or remove files at paths they are given, so a
// destination klaudiush cannot resolve may be the message file.
var filePlacers = []string{
	"cp", "mv", "rsync", "install", "ln", "rm", "rmdir", "tee", "dd", "truncate",
}

// readOnlyGitSubcommands leave the work tree files they name unchanged.
var readOnlyGitSubcommands = []string{"add", "diff", "status", "log", "show", "ls-files"}

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

func opaqueSourceWith(reason, repair string) error {
	return &opaqueMessageError{reason: reason, repair: repair}
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
// that runs it and the directory the shell starts in.
type messageSource struct {
	cmd    parser.Command
	parsed *parser.ParseResult
	cwd    string
	text   string
}

// hasFileFlag reports a -F/--file flag even when its value rendered empty,
// as a substitution glued to the flag does.
func hasFileFlag(gitCmd *parser.GitCommand) bool {
	return slices.ContainsFunc(commitFileFlags, gitCmd.HasFlag)
}

// shellDir is the directory the command starts in: the one the provider
// reports, else the hook's own.
func shellDir(hookCtx *hook.Context) string {
	if dir := hookCtx.GetWorkingDir(); dir != "" {
		return dir
	}

	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	return dir
}

// checkMessageFlags rejects message flags git resolves differently from the
// parser: a repeated -F, where git takes the last, and abbreviated long
// options, which git expands.
func checkMessageFlags(gitCmd *parser.GitCommand) error {
	fileFlags := 0

	for _, flag := range gitCmd.Flags {
		switch {
		case slices.Contains(commitFileFlags, flag), flag == noFileFlag:
			fileFlags++
		case slices.ContainsFunc(abbreviatedOptions, func(option string) bool {
			return abbreviates(flag, option)
		}):
			return opaqueSourceWith(reasonAbbrev, repairOneSource)
		}
	}

	if fileFlags > 1 {
		return opaqueSourceWith(reasonRepeated, repairOneSource)
	}

	return nil
}

// abbreviates reports a long flag git would expand to option.
func abbreviates(flag, option string) bool {
	return len(flag) >= minAbbrevLen && flag != option && strings.HasPrefix(option, flag)
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
	dynamic := slices.ContainsFunc(commitFileFlags, gitCmd.HasDynamicValue)

	switch {
	case !dynamic && isStdinPath(filePath):
		return v.readMessageStdin(gitCmd, src)
	case isFdPath(filePath):
		return "", opaqueSource(reasonFdSource)
	case filePath == "" || dynamic:
		return "", opaqueSource(reasonSubstituted)
	}

	return v.readMessagePath(src, filePath, src.gitDir(gitCmd), gitCmd.Location)
}

// readMessageStdin returns what a heredoc, here-string, literal echo or a
// redirected file feeds to -F -.
func (v *CommitValidator) readMessageStdin(
	gitCmd *parser.GitCommand,
	src messageSource,
) (string, error) {
	stdin, err := src.stdinMessage(gitCmd)
	if err != nil {
		return "", err
	}

	file := src.cmd.StdinFile

	switch {
	case stdin != "" && file != "":
		return "", opaqueSource(reasonTwoStdins)
	case stdin != "":
		v.Logger().Debug("Reading commit message from stdin (-F -)")

		return stdin, nil
	case file != "" && !isStdinPath(file) && !isFdPath(file):
		return v.readMessagePath(src, file, src.shellDir(), gitCmd.Location)
	default:
		return "", opaqueSource(reasonStdin)
	}
}

// join places a relative path under dir, itself relative to the shell's
// starting directory when not absolute.
func (src messageSource) join(dir, path string) string {
	dir = expandTilde(dir)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(src.cwd, dir)
	}

	return filepath.Clean(filepath.Join(dir, path))
}

// sameFile reports whether two absolute paths name one file, through
// symlinks or hard links when both exist.
func sameFile(a, b string) bool {
	if a == b {
		return true
	}

	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)

	if errA == nil && errB == nil && os.SameFile(infoA, infoB) {
		return true
	}

	return canonicalPath(a) == canonicalPath(b)
}

// canonicalPath follows symlinks in path, including a final link whose
// target does not exist yet: writing through it creates that target.
func canonicalPath(path string) string {
	for range maxLinkHops {
		dir, base := filepath.Split(path)
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			dir = resolved
		}

		path = filepath.Join(dir, base)

		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			return path
		}

		link, err := os.Readlink(path)
		if err != nil {
			return path
		}

		if !filepath.IsAbs(link) {
			link = filepath.Join(dir, link)
		}

		path = filepath.Clean(link)
	}

	return path
}

// containsPath reports whether dir is a directory above path, so copying
// into it, moving it or removing it can replace the file without naming it.
func containsPath(dir, path string) bool {
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		if sameFile(dir, parent) {
			return true
		}

		if parent == filepath.Dir(parent) {
			return false
		}
	}
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
		return "", errors.Wrapf(errNotRegular, "commit message file %s", path)
	}

	content, err := io.ReadAll(io.LimitReader(file, maxMessageFileBytes+1))
	if err != nil {
		return "", errors.Wrapf(err, "read commit message file %s", path)
	}

	if len(content) > maxMessageFileBytes {
		return "", errors.Wrapf(errTooLarge, "commit message file %s", path)
	}

	return string(content), nil
}
