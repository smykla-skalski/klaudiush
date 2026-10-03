package protection

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Match is a protected path an operation would change: Path as configured
// or found on disk, and Reason naming the kind of policy file it is.
type Match struct {
	Path   string
	Reason string
}

// matchRule returns the first rule that protects key.
func (s *Set) matchRule(key string) (rule, bool) {
	for _, r := range s.rules {
		if ruleMatches(r, key) {
			return r, true
		}
	}

	return rule{}, false
}

func ruleMatches(r rule, key string) bool {
	switch r.kind {
	case ruleAbs:
		if r.tree {
			return isUnder(key, r.path)
		}

		return key == r.path
	case ruleSuffix:
		return suffixMatches(r, components(key))
	case ruleGlob:
		return r.glob.MatchString(key)
	default:
		return false
	}
}

// suffixMatches reports whether comps end with the rule's names, or for a
// tree contain them anywhere.
func suffixMatches(r rule, comps []string) bool {
	n := len(r.comps)
	if n == 0 || len(comps) < n {
		return false
	}

	if !r.tree {
		return slices.Equal(comps[len(comps)-n:], r.comps)
	}

	for i := 0; i+n <= len(comps); i++ {
		if slices.Equal(comps[i:i+n], r.comps) {
			return true
		}
	}

	return false
}

func (s *Set) allowedKey(key string) bool {
	for _, r := range s.allow {
		if ruleMatches(r, key) {
			return true
		}
	}

	return false
}

// variants returns the spellings of an absolute path that comparisons
// try: as written (cleaned) and with symlinks resolved.
func variants(path string) []string {
	clean := filepath.Clean(path)
	if canon := canonical(clean); canon != clean {
		return []string{clean, canon}
	}

	return []string{clean}
}

// Check reports whether changing the file at path (absolute) changes a
// protected file: the path, or the file it resolves to through symlinks or
// shares through a hard link, is protected and not allowed. An allowed
// path that leads to a protected file still counts.
func (s *Set) Check(path string) (Match, bool) {
	for _, variant := range variants(path) {
		key := s.key(variant)

		r, ok := s.matchRule(key)
		if !ok || s.allowedKey(key) {
			continue
		}

		return Match{Path: variant, Reason: r.reason}, true
	}

	return s.checkHardLink(path)
}

// checkHardLink catches a second name for a protected file: writing through
// it changes the protected file's content.
func (s *Set) checkHardLink(path string) (Match, bool) {
	clean := filepath.Clean(path)

	info, err := fs.Stat(os.DirFS(filepath.Dir(clean)), filepath.Base(clean))
	if err != nil || !info.Mode().IsRegular() || !hasOtherNames(info) {
		return Match{}, false
	}

	for _, e := range s.entries {
		other, statErr := os.Stat(e.path)
		if statErr != nil || !os.SameFile(info, other) || s.allowedKey(e.key) {
			continue
		}

		return Match{Path: e.path, Reason: e.reason}, true
	}

	return Match{}, false
}

// CheckTree is Check for an operation that also changes everything below
// path, such as rm -r or chmod -R: a directory that holds a protected file
// matches too.
func (s *Set) CheckTree(path string) (Match, bool) {
	if m, ok := s.Check(path); ok {
		return m, true
	}

	for _, variant := range variants(path) {
		key := s.key(variant)

		for _, e := range s.entries {
			if key != e.key && isUnder(e.key, key) && !s.allowedKey(e.key) {
				return Match{Path: e.path, Reason: e.reason}, true
			}
		}

		if m, ok := s.suffixBelow(variant); ok {
			return m, true
		}
	}

	return Match{}, false
}

// suffixBelow reports a protected name, such as .klaudiush or
// .claude/settings.json, directly inside the existing directory dir.
func (s *Set) suffixBelow(dir string) (Match, bool) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return Match{}, false
	}

	for _, r := range s.rules {
		if r.kind != ruleSuffix {
			continue
		}

		candidate := filepath.Join(append([]string{dir}, r.names...)...)
		if _, statErr := os.Lstat(candidate); statErr != nil || s.allowedKey(s.key(candidate)) {
			continue
		}

		return Match{Path: candidate, Reason: r.reason}, true
	}

	return Match{}, false
}

// CheckPattern reports a protected path a path pattern can name. The pattern
// is a regular expression over keys; literalDir is the directory part
// written before the first unknown part, and tail the names written after
// the last one.
func (s *Set) CheckPattern(re *regexp.Regexp, literalDir string, tail []string) (Match, bool) {
	for _, e := range s.entries {
		if s.allowedKey(e.key) {
			continue
		}

		for key := e.key; ; key = parentKey(key) {
			if re.MatchString(key) {
				return Match{Path: e.path, Reason: e.reason}, true
			}

			if parentKey(key) == key {
				break
			}
		}
	}

	if literalDir != "" {
		key := s.key(filepath.Clean(literalDir))
		if r, ok := s.matchRule(key); ok && r.tree && !s.allowedKey(key) {
			return Match{Path: literalDir, Reason: r.reason}, true
		}
	}

	return s.checkTail(tail)
}

// checkTail matches the names written after an unknown part against the
// rules that protect a name wherever it appears.
func (s *Set) checkTail(tail []string) (Match, bool) {
	if len(tail) == 0 {
		return Match{}, false
	}

	comps := make([]string, 0, len(tail))
	for _, name := range tail {
		comps = append(comps, s.key(name))
	}

	for _, r := range s.rules {
		if r.kind != ruleSuffix || !suffixMatches(r, comps) {
			continue
		}

		return Match{Path: strings.Join(tail, "/"), Reason: r.reason}, true
	}

	return Match{}, false
}

func parentKey(key string) string {
	i := strings.LastIndex(key, "/")
	if i <= 0 {
		return "/"
	}

	return key[:i]
}

// Allowed reports whether protection.allow exempts path.
func (s *Set) Allowed(path string) bool {
	for _, variant := range variants(s.Resolve(path)) {
		if s.allowedKey(s.key(variant)) {
			return true
		}
	}

	return false
}

// Resolve makes path absolute against the hook's working directory,
// expanding a leading ~.
func (s *Set) Resolve(path string) string {
	return s.absolute(path, s.workDir)
}
