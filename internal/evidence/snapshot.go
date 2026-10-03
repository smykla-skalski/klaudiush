package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"

	kexec "github.com/smykla-skalski/klaudiush/internal/exec"
)

// ErrNotRepository marks a directory outside any git work tree.
var ErrNotRepository = errors.New("not a git repository")

// ErrUnreadable marks a work tree klaudiush cannot fully read.
var ErrUnreadable = errors.New("work tree not fully readable")

const (
	deletedMarker   = "deleted"
	submoduleMarker = "submodule"
)

// RepoRoot returns the top-level directory of the git work tree holding dir.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		return "", ErrNotRepository
	}

	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.WithSecondaryError(errors.Wrap(ErrNotRepository, dir), err)
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", ErrNotRepository
	}

	return canonicalDir(root), nil
}

// Snapshot is the state of a work tree at one moment: its files and, on
// demand, their content hashes.
// excludes digests the repository's own ignore list, .git/info/exclude:
// unlike .gitignore it is not a tracked file, so a change to it would
// otherwise hide new files without changing any digest.
type Snapshot struct {
	root     string
	files    []string
	hashes   map[string]string
	excludes string
}

// TakeSnapshot lists the tracked and untracked, not ignored, files of the
// work tree rooted at root.
func TakeSnapshot(ctx context.Context, root string) (*Snapshot, error) {
	out, err := listFiles(ctx, root, "--cached", "--others")
	if err != nil {
		return nil, err
	}

	files := splitPaths(out, func(string) bool { return true })

	excludes, err := excludesDigest(ctx, root)
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		root:     root,
		files:    files,
		hashes:   make(map[string]string),
		excludes: excludes,
	}, nil
}

func excludesDigest(ctx context.Context, root string) (string, error) {
	out, err := runGit(
		ctx,
		root,
		"rev-parse",
		"--path-format=absolute",
		"--git-path",
		"info/exclude",
	)
	if err != nil {
		return "", errors.Wrap(err, "failed to locate the repository exclude file")
	}

	data, err := os.ReadFile(strings.TrimSpace(string(out)))
	if errors.Is(err, fs.ErrNotExist) {
		return deletedMarker, nil
	}

	if err != nil {
		return "", errors.Wrap(err, "failed to read the repository exclude file")
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]), nil
}

// splitPaths returns the sorted, distinct NUL-separated paths keep accepts.
func splitPaths(out []byte, keep func(string) bool) []string {
	seen := make(map[string]bool)

	var paths []string

	for name := range bytes.SplitSeq(out, []byte{0}) {
		path := string(name)
		if path == "" || seen[path] || !keep(path) {
			continue
		}

		seen[path] = true

		paths = append(paths, path)
	}

	sort.Strings(paths)

	return paths
}

// Root returns the work tree root.
func (s *Snapshot) Root() string {
	return s.root
}

// ContentDigest identifies the content of every file the check covers, and
// returns how many files that is. It changes whenever one of those files is
// added, removed or modified, and only then.
func (s *Snapshot) ContentDigest(ctx context.Context, check *Check) (string, int, error) {
	hash := sha256.New()
	count := 0

	for _, path := range s.files {
		if !check.Covers(path) {
			continue
		}

		sum, err := s.hash(ctx, path)
		if err != nil {
			return "", 0, err
		}

		if sum == deletedMarker {
			continue
		}

		count++

		writeEntry(hash, path, sum)
	}

	writeEntry(hash, "\x00info/exclude", s.excludes)

	return digestOf(hash), count, nil
}

// Diff is the exact change a review covers: the base commit and the current
// content of every covered file that differs from it.
type Diff struct {
	Digest  string
	Base    string
	Changed []string
}

// ReviewDiff identifies the diff of the covered files, including untracked
// ones, against the merge base of the check's base branch and HEAD.
// Committing the reviewed changes leaves it unchanged.
func (s *Snapshot) ReviewDiff(ctx context.Context, check *Check) (Diff, error) {
	out, err := runGit(ctx, s.root, "merge-base", check.Base, "HEAD")
	if err != nil {
		return Diff{}, errors.Wrapf(err, "failed to resolve review base %q", check.Base)
	}

	base := strings.TrimSpace(string(out))

	changedOut, err := runGit(ctx, s.root, "diff", "--name-only", "-z", "--no-renames", base, "--")
	if err != nil {
		return Diff{}, errors.Wrap(err, "failed to list changed files")
	}

	untrackedOut, err := listFiles(ctx, s.root, "--others")
	if err != nil {
		return Diff{}, err
	}

	changed := splitPaths(append(append(changedOut, 0), untrackedOut...), check.Covers)

	hash := sha256.New()
	writeEntry(hash, "base", base)

	for _, path := range changed {
		sum, err := s.hash(ctx, path)
		if err != nil {
			return Diff{}, err
		}

		writeEntry(hash, path, sum)
	}

	return Diff{Digest: digestOf(hash), Base: base, Changed: changed}, nil
}

func digestOf(hash interface{ Sum([]byte) []byte }) string {
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func writeEntry(hash io.Writer, path, sum string) {
	_, _ = io.WriteString(hash, path)
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, sum)
	_, _ = hash.Write([]byte{'\n'})
}

// hash returns the content hash of a repository-relative path, or
// deletedMarker when it does not exist.
func (s *Snapshot) hash(ctx context.Context, path string) (string, error) {
	if sum, ok := s.hashes[path]; ok {
		return sum, nil
	}

	if err := ctx.Err(); err != nil {
		return "", errors.Wrap(err, "fingerprint interrupted")
	}

	sum, err := hashFile(s.root, filepath.FromSlash(path))
	if err != nil {
		return "", err
	}

	if sum == submoduleMarker {
		sum, err = s.submoduleState(ctx, path)
		if err != nil {
			return "", err
		}
	}

	s.hashes[path] = sum

	return sum, nil
}

// submoduleState identifies a submodule by its checked-out commit and by
// the content of every file in it, so each further edit inside it changes
// the digest. An uninitialized submodule is an empty directory, where git
// would answer for the parent repository instead. A nested repository
// without commits has no HEAD; its files still identify it.
func (s *Snapshot) submoduleState(ctx context.Context, path string) (string, error) {
	dir := filepath.Join(s.root, filepath.FromSlash(path))

	if _, err := os.Lstat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		return submoduleMarker + ":uninitialized", nil
	}

	head, err := runGit(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		head = []byte("unborn")
	}

	nested, err := TakeSnapshot(ctx, dir)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read submodule %s", path)
	}

	hash := sha256.New()
	writeEntry(hash, "HEAD", strings.TrimSpace(string(head)))

	for _, file := range nested.files {
		fileSum, err := nested.hash(ctx, file)
		if err != nil {
			return "", err
		}

		writeEntry(hash, file, fileSum)
	}

	return submoduleMarker + ":" + hex.EncodeToString(hash.Sum(nil)), nil
}

// hashFile hashes a file's content and permissions. A symlink hashes its
// target path; a directory is a submodule, which git lists as one path.
// Files are opened through the work tree root, so a path cannot reach
// outside it.
func hashFile(root, path string) (string, error) {
	tree, err := os.OpenRoot(root)
	if err != nil {
		return "", errors.Wrapf(err, "failed to open %s", root)
	}

	defer func() { _ = tree.Close() }()

	info, err := tree.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return deletedMarker, nil
	}

	if err != nil {
		return "", errors.Wrapf(err, "failed to stat %s", path)
	}

	hash := sha256.New()

	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := tree.Readlink(path)
		if err != nil {
			return "", errors.Wrapf(err, "failed to read link %s", path)
		}

		_, _ = io.WriteString(hash, "link:"+target)
	case info.IsDir():
		return submoduleMarker, nil
	default:
		if err := copyFile(hash, tree, path); err != nil {
			return "", err
		}

		_, _ = io.WriteString(hash, "mode:"+info.Mode().Perm().String())
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(dst io.Writer, tree *os.Root, path string) error {
	file, err := tree.Open(path)
	if err != nil {
		return errors.Wrapf(err, "failed to open %s", path)
	}

	_, copyErr := io.Copy(dst, file)
	closeErr := file.Close()

	if copyErr != nil {
		return errors.Wrapf(copyErr, "failed to read %s", path)
	}

	if closeErr != nil {
		return errors.Wrapf(closeErr, "failed to close %s", path)
	}

	return nil
}

// gitRunner runs git for fingerprints. Variables that point git at another
// repository, index or object store are dropped, so the caller's
// environment cannot make klaudiush fingerprint something other than the
// work tree on disk.
var gitRunner = kexec.NewCommandRunner(0)

// unreadableDirectory is how git reports a directory it skipped.
const unreadableDirectory = "could not open directory"

// listFiles lists work tree files with git ls-files. A directory git could
// not read would hide the files in it, so it fails the listing instead of
// leaving those files out of the fingerprint. Other complaints, such as a
// missing fsmonitor or an unreadable global ignore file, leave nothing out.
func listFiles(ctx context.Context, root string, which ...string) ([]byte, error) {
	args := append([]string{"ls-files", "-z", "--exclude-standard"}, which...)

	out, stderr, err := runGitStderr(ctx, root, args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list repository files")
	}

	for line := range strings.SplitSeq(stderr, "\n") {
		if strings.Contains(line, unreadableDirectory) {
			return nil, errors.Wrapf(ErrUnreadable, "git ls-files: %s", strings.TrimSpace(line))
		}
	}

	return out, nil
}

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, _, err := runGitStderr(ctx, dir, args...)

	return out, err
}

func runGitStderr(ctx context.Context, dir string, args ...string) ([]byte, string, error) {
	env := append(gitEnv(os.Environ()), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")

	result := gitRunner.RunWithOptions(ctx, kexec.RunOptions{
		Dir: dir,
		Env: env,
	}, "git", args...)
	if result.Err != nil {
		return nil, result.Stderr, errors.Wrapf(
			result.Err,
			"git %s: %s",
			args[0],
			strings.TrimSpace(result.Stderr),
		)
	}

	return []byte(result.Stdout), result.Stderr, nil
}

func gitEnv(environ []string) []string {
	kept := make([]string, 0, len(environ))

	for _, entry := range environ {
		if !strings.HasPrefix(entry, "GIT_") {
			kept = append(kept, entry)
		}
	}

	return kept
}
