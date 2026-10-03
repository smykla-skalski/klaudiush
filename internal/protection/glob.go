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
	return strings.ContainsAny(s, "*?[{"+unknownPart)
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

	for _, r := range inner {
		switch r {
		case '\\', ']', '[', '^':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
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
					return -1, nil
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
