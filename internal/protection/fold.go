package protection

import (
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Platforms with their own managed paths and case rules.
const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"
)

// foldsCase reports whether the default file systems of goos ignore case:
// APFS and HFS+ on macOS, NTFS on Windows. Paths are then compared case
// insensitively, so .CLAUDE/Settings.json is the protected file too.
func foldsCase(goos string) bool {
	return goos == goosDarwin || goos == goosWindows
}

// foldRune maps r to the smallest rune that case-folds to the same letter,
// so every spelling of a name compares equal: S, s and the long s (ſ) all
// become S, and the Kelvin sign becomes K.
func foldRune(r rune) rune {
	smallest := r

	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < smallest {
			smallest = f
		}
	}

	return smallest
}

// foldString folds every rune of s with foldRune.
func foldString(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder

	b.Grow(len(s))

	for _, r := range s {
		if r == utf8.RuneError {
			b.WriteRune(r)

			continue
		}

		b.WriteRune(foldRune(r))
	}

	return b.String()
}

// key returns the form of an absolute, clean path that comparisons use.
func (s *Set) key(path string) string {
	path = filepath.ToSlash(path)
	if s.foldCase {
		return foldString(path)
	}

	return path
}

// isUnder reports whether path is dir or below it. Both are keys.
func isUnder(path, dir string) bool {
	if path == dir {
		return true
	}

	if dir == "/" {
		return strings.HasPrefix(path, "/")
	}

	return strings.HasPrefix(path, dir+"/")
}

// components splits a key into its names, without empty ones.
func components(key string) []string {
	parts := strings.Split(key, "/")
	out := parts[:0]

	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}
