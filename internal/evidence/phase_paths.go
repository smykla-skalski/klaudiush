package evidence

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
)

const parentDir = ".."

// maxLinkHops bounds the symbolic links destination follows, as the
// kernel's own limit does.
const maxLinkHops = 40

// maxAliasScan bounds the entries scanned for links to protected files. A
// larger repository fails closed: no path is writable.
const maxAliasScan = 200_000

var errScanLimit = errors.New("too many entries to scan for protected links")

// policyNames are path components that hold klaudiush or harness
// configuration, compared without case since macOS and Windows file systems
// ignore it.
var policyNames = []string{
	".klaudiush", "klaudiush.toml", ".git", ".gemini", ".claude", ".codex", ".mcp.json",
}

// Writable reports whether a file tool may change path while the phase is
// restricted. Both the path as written and the file it resolves to through
// symbolic links must lie inside the repository, match a writable pattern,
// and be neither configuration nor a file a prerequisite runs. A symbolic
// link that dangles anywhere along the path is refused, since a write
// through it lands wherever it points. A file that is a second name for a
// protected file is refused too: a symbolic link with a protected name that
// leads to the target, or a hard link.
func (p *Phase) Writable(repoRoot, path string) bool {
	if len(p.WritablePaths) == 0 || path == "" || repoRoot == "" || !filepath.IsAbs(path) {
		return false
	}

	path = filepath.Clean(path)
	if danglingComponent(path) {
		return false
	}

	root := resolveExisting(repoRoot)
	target := resolveExisting(path)

	spellings := []string{target}
	if lexical, ok := relInside(filepath.Clean(repoRoot), path); ok {
		spellings = append(spellings, filepath.Join(root, lexical))
	} else if lexical, ok = relInside(root, path); ok {
		spellings = append(spellings, filepath.Join(root, lexical))
	}

	for _, spelling := range spellings {
		rel, ok := relInside(root, spelling)
		if !ok || definesPolicy(rel) || !matchAny(p.WritablePaths, rel) {
			return false
		}
	}

	protected := p.prerequisiteFiles(root)
	if slices.ContainsFunc(append(spellings, path), protected.covers) {
		return false
	}

	return !aliasesProtected(root, target, protected)
}

// relInside returns path relative to root, slash separated, when it lies
// strictly inside root.
func relInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == parentDir ||
		strings.HasPrefix(rel, parentDir+string(filepath.Separator)) {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

func definesPolicy(rel string) bool {
	for part := range strings.SplitSeq(rel, "/") {
		for _, name := range policyNames {
			if strings.EqualFold(part, name) {
				return true
			}
		}
	}

	return false
}

// fileSet is a set of protected absolute paths, compared without case.
type fileSet []string

func (s fileSet) covers(path string) bool {
	for _, protected := range s {
		if strings.EqualFold(protected, path) {
			return true
		}
	}

	return false
}

// coversTree reports a path that is a protected path or lies under one.
func (s fileSet) coversTree(path string) bool {
	for _, protected := range s {
		if strings.EqualFold(protected, path) ||
			hasFoldPrefix(path, protected+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

func hasFoldPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// prerequisiteFiles lists every spelling of the files the prerequisite
// commands run, made absolute against the repository root, where checks run:
// as written, through existing links, and where a write through a dangling
// link would land. A bare program name is looked up on PATH, so only its
// name inside the repository root counts.
func (p *Phase) prerequisiteFiles(root string) fileSet {
	var files fileSet

	for _, check := range p.Requires {
		for _, argv := range check.Commands {
			for i, word := range programWords(argv) {
				files = append(files, spellingsOf(root, word, i == 0)...)
			}
		}
	}

	return files
}

func spellingsOf(root, word string, program bool) []string {
	switch {
	case word == "":
		return nil
	case filepath.IsAbs(word):
	case program && !strings.ContainsAny(word, `/\`):
		return []string{filepath.Join(root, word)}
	default:
		word = filepath.Join(root, word)
	}

	word = filepath.Clean(word)
	spellings := []string{word, resolveExisting(word)}

	if dest, ok := destination(word); ok {
		spellings = append(spellings, dest)
	}

	return spellings
}

// aliasesProtected reports a target that another name reaches: a hard link
// shares its content with a file that may be protected, and a symbolic link
// with a protected name may lead to it or to a directory above it. A link
// that cannot be followed may lead anywhere, so it covers the repository.
func aliasesProtected(root, target string, prerequisites fileSet) bool {
	if info, err := os.Stat(target); err == nil {
		if linkedElsewhere(info, func(other os.FileInfo) bool {
			return sameAsProtected(root, other, prerequisites)
		}) {
			return true
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return true
	}

	links, err := protectedLinks(root)
	if err != nil {
		return true
	}

	return links.coversTree(target)
}

// sameAsProtected reports whether info is the same file as a prerequisite
// or a file under a protected name in the repository.
func sameAsProtected(root string, info os.FileInfo, prerequisites fileSet) bool {
	for _, path := range prerequisites {
		if other, err := os.Stat(path); err == nil && os.SameFile(info, other) {
			return true
		}
	}

	found := false

	err := walkPolicy(root, func(path string, _ fs.DirEntry) {
		if other, statErr := os.Stat(path); statErr == nil && os.SameFile(info, other) {
			found = true
		}
	})

	return found || err != nil
}

// protectedLinks returns where every symbolic link under a protected name
// in the repository leads, so a write there changes configuration.
func protectedLinks(root string) (fileSet, error) {
	var dests fileSet

	err := walkPolicy(root, func(path string, entry fs.DirEntry) {
		if entry.Type()&fs.ModeSymlink == 0 {
			return
		}

		dest, ok := destination(path)
		if !ok {
			dest = root
		}

		dests = append(dests, dest)
	})

	return dests, err
}

// walkPolicy calls visit for every entry of the repository whose path has
// a protected name, without following symbolic links. Git's object store
// is skipped. A walk that fails or exceeds maxAliasScan returns an error.
func walkPolicy(root string, visit func(path string, entry fs.DirEntry)) error {
	objects := filepath.Join(root, ".git", "objects")
	seen := 0

	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if seen++; seen > maxAliasScan {
			return errScanLimit
		}

		if path == objects {
			return filepath.SkipDir
		}

		if rel, ok := relInside(root, path); ok && definesPolicy(rel) {
			visit(path, entry)
		}

		return nil
	})
}

// danglingComponent reports a path with a symbolic link anywhere along it
// whose target does not exist, or a component that cannot be checked.
func danglingComponent(path string) bool {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)

		switch {
		case err == nil:
			if info.Mode()&fs.ModeSymlink != 0 {
				if _, evalErr := filepath.EvalSymlinks(current); evalErr != nil {
					return true
				}
			}
		case !errors.Is(err, fs.ErrNotExist):
			return true
		}

		if current == filepath.Dir(current) {
			return false
		}
	}
}

// resolveExisting resolves the symbolic links of the longest existing prefix
// of an absolute path and keeps the rest as written.
func resolveExisting(path string) string {
	path = filepath.Clean(path)

	var rest []string

	for current := path; ; current = filepath.Dir(current) {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(append([]string{resolved}, rest...)...)
		}

		if current == filepath.Dir(current) {
			return path
		}

		rest = append([]string{filepath.Base(current)}, rest...)
	}
}

// destination follows an absolute path through every symbolic link,
// existing or dangling, to the file a write through it would change. It
// fails on a link loop or a component it cannot check.
func destination(path string) (string, bool) {
	volume := filepath.VolumeName(path)
	resolved := volume + string(filepath.Separator)
	rest := splitPath(strings.TrimPrefix(filepath.Clean(path), volume))
	hops := 0

	for len(rest) > 0 {
		name := rest[0]
		rest = rest[1:]

		if name == parentDir {
			resolved = filepath.Dir(resolved)

			continue
		}

		next := filepath.Join(resolved, name)

		info, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) {
			return filepath.Join(append([]string{next}, rest...)...), true
		}

		if err != nil {
			return "", false
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			resolved = next

			continue
		}

		if hops++; hops > maxLinkHops {
			return "", false
		}

		link, err := os.Readlink(next)
		if err != nil {
			return "", false
		}

		if filepath.IsAbs(link) {
			volume = filepath.VolumeName(link)
			resolved = volume + string(filepath.Separator)
			link = strings.TrimPrefix(link, volume)
		}

		rest = append(splitPath(link), rest...)
	}

	return resolved, true
}

func splitPath(path string) []string {
	var parts []string

	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}

	return parts
}
