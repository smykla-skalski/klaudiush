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

const deletedMarker = "deleted"

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
type Snapshot struct {
	root   string
	files  []string
	hashes map[string]string
}

// TakeSnapshot lists the tracked and untracked, not ignored, files of the
// work tree rooted at root.
func TakeSnapshot(ctx context.Context, root string) (*Snapshot, error) {
	out, err := runGit(ctx, root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, errors.Wrap(err, "failed to list repository files")
	}

	files := splitPaths(out, func(string) bool { return true })

	return &Snapshot{root: root, files: files, hashes: make(map[string]string)}, nil
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

	untrackedOut, err := runGit(ctx, s.root, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return Diff{}, errors.Wrap(err, "failed to list untracked files")
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

	s.hashes[path] = sum

	return sum, nil
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
		_, _ = io.WriteString(hash, "dir")
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

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	env := append(gitEnv(os.Environ()), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")

	result := gitRunner.RunWithOptions(ctx, kexec.RunOptions{
		Dir: dir,
		Env: env,
	}, "git", args...)
	if result.Err != nil {
		return nil, errors.Wrapf(
			result.Err,
			"git %s: %s",
			args[0],
			strings.TrimSpace(result.Stderr),
		)
	}

	return []byte(result.Stdout), nil
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
