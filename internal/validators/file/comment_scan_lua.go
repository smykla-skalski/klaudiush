package file

import "strings"

// Long brackets carry their exact closing delimiter across lines; quotes and
// comment markers inside them are literal, including mismatched bracket levels.
func luaLongBracket(line string) (closing string, size int) {
	if len(line) == 0 || line[0] != '[' {
		return "", 0
	}

	i := 1
	for i < len(line) && line[i] == '=' {
		i++
	}

	if i == len(line) || line[i] != '[' {
		return "", 0
	}

	return "]" + line[1:i] + "]", i + 1
}

func scanLua(line string, state stringState) (int, stringState) {
	comments, end := scanLuaComments(line, state)
	if len(comments) == 0 {
		return -1, end
	}

	return comments[0].start, end
}

type commentSpan struct {
	start int
	end   int
}

func scanLuaComments(line string, state stringState) ([]commentSpan, stringState) {
	var comments []commentSpan

	start := 0

	if strings.HasPrefix(string(state), "z") {
		start = len(line) - len(strings.TrimLeft(line, " \t\r\n\v\f"))
		if start == len(line) {
			return comments, state
		}

		state = state[1:]
	}

	for i := start; i < len(line); i++ {
		switch {
		case state == "\"" || state == "'":
			i, state = scanLuaQuoted(line, i, state[0])
		case state != stateCode:
			i, state = scanLuaLong(line, i, state)
			if state != stateCode {
				return comments, state
			}
		case line[i] == '\'' || line[i] == '"':
			i, state = scanLuaQuoted(line, i+1, line[i])
		case strings.HasPrefix(line[i:], "--"):
			comments = append(comments, commentSpan{start: i, end: len(line)})

			closing, size := luaLongBracket(line[i+2:])
			if size == 0 {
				return comments, stateCode
			}

			i, state = scanLuaLong(line, i+2+size, stringState(closing))
			comments[len(comments)-1].end = i + 1
		case line[i] == '[':
			if closing, size := luaLongBracket(line[i:]); size > 0 {
				state, i = stringState(closing), i+size-1
			}
		}
	}

	return comments, state
}

func scanLuaLong(line string, i int, state stringState) (int, stringState) {
	end := strings.Index(line[i:], string(state))
	if end < 0 {
		return len(line) - 1, state
	}

	return i + end + len(state) - 1, stateCode
}

func scanLuaQuoted(line string, i int, quote byte) (int, stringState) {
	for ; i < len(line); i++ {
		switch line[i] {
		case quote:
			return i, stateCode
		case '\\':
			i++
			if strings.TrimSuffix(line[i:], "\r") == "" {
				return len(line) - 1, stringState(quote)
			}

			if line[i] == 'z' {
				i++

				i += len(line[i:]) - len(strings.TrimLeft(line[i:], " \t\r\n\v\f"))
				if i == len(line) {
					return i - 1, stringState("z" + string(quote))
				}

				i--
			}
		}
	}

	return len(line) - 1, stateCode
}
