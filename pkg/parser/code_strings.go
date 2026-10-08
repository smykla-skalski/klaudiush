package parser

import (
	"regexp"
	"strings"
)

// codeLang is the language of program source, as far as stringsRun needs it.
// Only Python and JavaScript have their plain strings judged; any other
// language keeps every string as a possible command line.
type codeLang int

const (
	langOther codeLang = iota
	langPython
	langJavaScript
)

// stringRunners matches code that may run a string as a command line, or
// hand one to something that does: a shell call (os.system, popen,
// shell=True, exec, spawn, getoutput), code run from a string (eval,
// compile, Function), a string split into an argv (split, shlex) or a name
// looked up at run time (getattr). It is matched case-insensitively and
// deliberately loose: a false match only keeps reading every string.
var stringRunners = regexp.MustCompile(
	`(?i)system|popen|shell|exec|eval|spawn|split|getattr|compile|runpy|` +
		`\bfunction\s*\(|\bcommand\s*\(`,
)

// commandStringPrograms are launchers that take a command line as one string
// or read one from stdin. With shells and interpreters, a plain string naming
// one may start it from an argv whose other items klaudiush cannot see.
var commandStringPrograms = nameSet(
	"su flock env watch ssh tmux screen parallel xargs busybox script expect",
)

// commandStringFlag matches a flag that hands a shell or an interpreter its
// command line (-c, -lc, -e, --eval, --command).
var commandStringFlag = regexp.MustCompile(`^-(?:[A-Za-z]*c|e|-eval|-command)$`)

// pythonImport matches a Python import statement, capturing the modules.
var pythonImport = regexp.MustCompile(
	`(?m)(?:^|[;:])[ \t]*(?:import[ \t]+([\w., \t]+)|from[ \t]+(\.*[\w.]*)[ \t]+import\b)`,
)

// jsImport matches a JavaScript module load, capturing a literal module name.
// A load whose name is not a string literal captures nothing.
var jsImport = regexp.MustCompile(
	`\brequire\s*\(\s*(?:["'` + "`" + `]([^"'` + "`" + `]*))?|` +
		`\bimport\s*\(\s*(?:["'` + "`" + `]([^"'` + "`" + `]*))?|` +
		`\bfrom\s*["']([^"']*)|\bimport\s*["']([^"']*)`,
)

// pythonModules are standard library modules that cannot run a string as a
// command line other than through a name stringRunners matches. Any other
// module, a local helper most of all, may run any string handed to it.
var pythonModules = nameSet(`__future__ abc argparse array ast asyncio base64 binascii
	bisect calendar collections colorsys concurrent configparser contextlib copy csv
	dataclasses datetime decimal difflib enum errno fileinput fnmatch fractions functools
	getpass gettext glob gzip hashlib heapq hmac html http io ipaddress itertools json
	keyword locale logging lzma math mimetypes numbers operator os pathlib platform pprint
	queue random re secrets shutil signal sqlite3 stat statistics string struct subprocess
	sys sysconfig tarfile tempfile textwrap threading time tomllib traceback types typing
	unicodedata unittest urllib uuid warnings weakref xml zipfile zlib zoneinfo`)

// jsModules are the Node built-in modules with the same property.
var jsModules = nameSet(`assert buffer child_process crypto events fs fs/promises os path
	process readline stream string_decoder timers url util zlib`)

// stringsRun reports whether Python or JavaScript source may run one of its
// plain strings as a command line: it calls something stringRunners
// matches, names a program that takes a command string, or loads a module
// klaudiush does not know, whose functions may run what they are handed.
// Otherwise its strings are data (labels, messages, payloads) and only
// argv lists can start a command.
func stringsRun(code string, lang codeLang) bool {
	if lang == langOther || codeUnsafe(code) || stringRunners.MatchString(code) {
		return true
	}

	for _, m := range quotedLiteral.FindAllStringSubmatchIndex(code, -1) {
		text := strings.TrimSpace(literalEscapes.Replace(submatchText(code, m)))
		if commandStringFlag.MatchString(text) {
			return true
		}

		first, _, _ := strings.Cut(text, " ")
		if commandStringProgram(first) {
			return true
		}
	}

	return unknownModule(code, lang)
}

// commandStringProgram reports whether word names a shell, an interpreter or
// one of commandStringPrograms. A mixed-case bare word ("Bash", the hook
// tool name) is a name rather than a program spelling.
func commandStringProgram(word string) bool {
	if !strings.Contains(word, "/") && word != strings.ToLower(word) &&
		word != strings.ToUpper(word) {
		return false
	}

	name := commandName(word)
	_, ok := interpreters[name]

	return ok || shells[name] || commandStringPrograms[name]
}

// unknownModule reports whether source loads a module outside the known set
// for its language, or one whose name is computed.
func unknownModule(code string, lang codeLang) bool {
	if lang == langJavaScript {
		for _, m := range jsImport.FindAllStringSubmatch(code, -1) {
			name := strings.TrimPrefix(m[1]+m[2]+m[3]+m[4], "node:")
			if !jsModules[name] {
				return true
			}
		}

		return false
	}

	for _, m := range pythonImport.FindAllStringSubmatch(code, -1) {
		modules := strings.Split(m[1], ",")
		if m[1] == "" {
			modules = []string{m[2]}
		}

		for _, module := range modules {
			name, _, _ := strings.Cut(strings.TrimSpace(module), " ")
			top, _, _ := strings.Cut(name, ".")

			if !pythonModules[top] {
				return true
			}
		}
	}

	return false
}
