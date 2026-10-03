package protection

import (
	"regexp"
	"strings"
)

// unknownPart stands in for a part of a shell word klaudiush cannot know
// before the command runs: an unset variable or a command substitution. It
// may stand for any text, slashes included.
const unknownPart = "\x00"

// hasGlobMeta reports whether s has a glob character, brace list or
// unknown part, so it names paths by pattern rather than one path.
func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[{^"+unknownPart)
}

// globRegexp converts a shell glob into a regular expression body matching
// whole paths. It handles *, **, ?, bracket expressions, brace lists and
// unknown parts. A bracket or brace that does not close is literal, as in
// the shell. Dotfiles match * too: the result only widens what counts as
// protected.
func globRegexp(pattern string) (string, error) {
	var b strings.Builder

	writeGlob(&b, pattern)

	expr := b.String()
	if _, err := regexp.Compile(expr); err != nil {
		return "", err
	}

	return expr, nil
}

func writeGlob(b *strings.Builder, pattern string) {
	for i := 0; i < len(pattern); i++ {
		i = writeGlobAt(b, pattern, i)
	}
}

// writeGlobAt writes the regular expression for the glob element at i and
// returns the index of its last byte.
func writeGlobAt(b *strings.Builder, pattern string, i int) int {
	switch pattern[i] {
	case unknownPart[0]:
		b.WriteString(".*")
	case '*':
		if i+1 < len(pattern) && pattern[i+1] == '*' {
			b.WriteString(".*")

			return i + 1
		}

		b.WriteString("[^/]*")
	case '?':
		b.WriteString("[^/]")
	case '^':
		if i == 0 || pattern[i-1] == '/' {
			b.WriteString("[^/]*")

			return componentEnd(pattern, i) - 1
		}

		b.WriteString(regexp.QuoteMeta("^"))
	case '[':
		end := bracketEnd(pattern, i)
		if end < 0 {
			b.WriteString(`\[`)

			return i
		}

		b.WriteString(bracketClass(pattern[i+1 : end]))

		return end
	case '{':
		return writeBraces(b, pattern, i)
	case '\\':
		if i+1 < len(pattern) {
			b.WriteString(regexp.QuoteMeta(pattern[i+1 : i+2]))

			return i + 1
		}
	default:
		b.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
	}

	return i
}

// writeBraces writes a brace list as an alternation, or the brace itself
// when the list does not close.
func writeBraces(b *strings.Builder, pattern string, start int) int {
	end, alternatives := braceAlternatives(pattern, start)
	if end < 0 {
		b.WriteString(`\{`)

		return start
	}

	b.WriteString("(?:")

	for j, alt := range alternatives {
		if j > 0 {
			b.WriteByte('|')
		}

		writeGlob(b, alt)
	}

	b.WriteByte(')')

	return end
}

// bracketEnd returns the index of the ] closing the bracket expression at
// start, or -1.
func bracketEnd(pattern string, start int) int {
	i := start + 1
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		i++
	}

	if i < len(pattern) && pattern[i] == ']' {
		i++
	}

	for ; i < len(pattern); i++ {
		if strings.HasPrefix(pattern[i:], "[:") {
			if end := strings.Index(pattern[i+2:], ":]"); end >= 0 {
				i += end + len("[::")

				continue
			}
		}

		if pattern[i] == ']' {
			return i
		}
	}

	return -1
}

// bracketClass converts the inside of a bracket expression to a regular
// expression class that never matches a slash.
func bracketClass(inner string) string {
	negate := strings.HasPrefix(inner, "!") || strings.HasPrefix(inner, "^")
	if negate {
		inner = inner[1:]
	}

	var b strings.Builder

	b.WriteByte('[')

	if negate {
		b.WriteString("^/")
	}

	for i := 0; i < len(inner); i++ {
		if strings.HasPrefix(inner[i:], "[:") {
			if end := strings.Index(inner[i+2:], ":]"); end >= 0 {
				b.WriteString(inner[i : i+end+len("[::]")])

				i += end + len("[::")

				continue
			}
		}

		switch c := inner[i]; c {
		case '\\', ']', '[', '^':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}

	b.WriteByte(']')

	return b.String()
}

// braceAlternatives splits the brace list at start into its comma-separated
// alternatives, honoring nested braces. It returns -1 when the brace does
// not close or holds no comma, which the shell leaves as written.
func braceAlternatives(pattern string, start int) (int, []string) {
	depth := 0
	last := start + 1

	var alternatives []string

	for i := start; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				if alternatives == nil {
					return braceRange(pattern[start+1:i], i)
				}

				return i, append(alternatives, pattern[last:i])
			}
		case ',':
			if depth == 1 {
				alternatives = append(alternatives, pattern[last:i])
				last = i + 1
			}
		}
	}

	return -1, nil
}

// braceRange expands a {a..e} or {1..3} sequence, which the shell treats as
// a list of every letter or number between the ends.
func braceRange(inner string, end int) (int, []string) {
	from, to, ok := strings.Cut(inner, "..")
	if !ok || len(from) != 1 || len(to) != 1 {
		return -1, nil
	}

	lo, hi := from[0], to[0]
	if lo > hi {
		lo, hi = hi, lo
	}

	var alternatives []string

	for c := lo; ; c++ {
		alternatives = append(alternatives, string(rune(c)))

		if c == hi {
			break
		}
	}

	return end, alternatives
}

// componentEnd returns the index of the slash ending the path component at
// i, or the pattern length.
func componentEnd(pattern string, i int) int {
	if j := strings.IndexByte(pattern[i:], '/'); j >= 0 {
		return i + j
	}

	return len(pattern)
}
