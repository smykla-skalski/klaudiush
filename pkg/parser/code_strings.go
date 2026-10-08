package parser

import (
	"regexp"
	"slices"
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
// shell=True, exec, spawn, getstatusoutput), code run from a string (eval,
// compile, Function), a name looked up at run time (getattr,
// globalThis[...], sys.modules), a file written and maybe run later (write,
// FileHandler, inplace) or an environment variable git or a shell may run
// (GIT_EDITOR, BASH_ENV). It is matched case-insensitively, after
// benignCalls are removed, and deliberately loose: a false match only keeps
// reading every string.
var stringRunners = regexp.MustCompile(
	`(?i)system|popen|\bshell\b|_shell|shell_|exec|eval|spawn|getattr|(?:^|[^\w.])compile\s*\(|runpy|` +
		`getstatusoutput|startfile|\bfunction\s*\(|\bcommand\s*\(|` +
		`write|chmod|symlink|appendfile|copyfile|\bdump\s*\(|o_wronly|o_rdwr|o_creat|o_append|` +
		`\bopen\s*\([^)]*,\s*(?:mode\s*=\s*)?["'][^"']*[wax+]|` +
		`environ|\benv\b|globalthis|\bglobal\s*\[|mainmodule|process\.binding|dlopen|` +
		`sys\.modules|attrgetter|methodcaller|__getattribute__|filehandler|inplace|filename\s*=`,
)

// benignCalls drops reads and writes stringRunners would otherwise match
// that cannot run anything: printing to the process's own streams, naming
// the running interpreter and reading an environment variable.
var benignCalls = strings.NewReplacer(
	"sys.stdout.write", " ", "sys.stderr.write", " ", "process.stdout.write", " ",
	"process.stderr.write", " ", "sys.executable", " ", "environ.get(", " (",
)

// jsRunners matches JavaScript that reaches a program or a module without
// naming it: Deno and Bun process APIs, Reflect, module.require, global
// objects and computed members of process.
var jsRunners = regexp.MustCompile(
	`\bDeno\b|\bBun\b|\bReflect\b|\bmodule\b|\bglobal\b|\bprocess\s*\[`,
)

// stdinPrograms run what arrives on their stdin or in their arguments as a
// command, beyond shells, interpreters and launchers: schedulers, remote
// and multiplexed shells, and tools with a shell escape.
var stdinPrograms = nameSet(`su flock env watch ssh tmux screen parallel xargs busybox
	script expect at batch crontab ed ex vi vim nvim sqlite3 psql mysql gdb lldb sed
	docker podman nerdctl kubectl make runuser sg newgrp pkexec chroot nsenter unshare
	systemd-run xterm tee dd cat cp install gmake bmake just task`)

// commandStringFlag matches an argv item that hands a wrapper, shell or
// interpreter its command line (-c, -lc, -e, --eval, --command), whatever
// program it follows.
var commandStringFlag = regexp.MustCompile(`^-(?:[A-Za-z]*c|e|-eval|-command)$`)

// spawnCall matches a subprocess call that starts a program: subprocess.run
// and its siblings, or the same names imported bare. Method calls on other
// objects (asyncio.run) do not match.
var spawnCall = regexp.MustCompile(
	`(?:subprocess\s*\.\s*|(?:^|[^\w.]))(?:run|call|check_call|check_output|Popen)\s*\(\s*`,
)

// subprocessUse matches each mention of the subprocess module, capturing
// the attribute read from it.
var subprocessUse = regexp.MustCompile(`\bsubprocess\b(?:\s*\.\s*(\w+))?`)

// subprocessAttrs are the attributes of subprocess that start a program
// from an argv, or are constants and exception types.
var subprocessAttrs = nameSet(`run call check_call check_output Popen PIPE DEVNULL STDOUT
	CalledProcessError TimeoutExpired CompletedProcess SubprocessError`)

// importAlias matches an import that renames what it imports.
var importAlias = regexp.MustCompile(`\bimport\b[^\n;]*\bas\s`)

// spawnFuncs are the subprocess functions that start a program. One used as
// a value (partial(subprocess.run, ...), r = subprocess.run) is called where
// spawnRunsStrings cannot see its argv.
var spawnFuncs = nameSet("run call check_call check_output Popen")

// spawnImport matches a spawn function imported by name, which may then be
// passed around under that bare name.
var spawnImport = regexp.MustCompile(
	`\bfrom\s+subprocess\s+import\b[^\n;]*\b(?:run|call|check_call|check_output|Popen)\b`,
)

// pythonImport matches a Python import statement, capturing the modules.
var pythonImport = regexp.MustCompile(
	`(?m)(?:^|[;:])[ \t]*(?:import[ \t]+([\w., \t]+(?:\\\r?\n[\w., \t]+)*)|` +
		`from[ \t]+(\.*[\w.]*)[ \t]+import\b)`,
)

// jsImport matches a JavaScript module load, capturing a literal module name.
// A load whose name is not a string literal captures nothing.
var jsImport = regexp.MustCompile(
	`\brequire\s*\(\s*(?:["'` + "`" + `]([^"'` + "`" + `]*))?|` +
		`\bimport\s*\(\s*(?:["'` + "`" + `]([^"'` + "`" + `]*))?|` +
		`\bfrom\s*["']([^"']*)|\bimport\s*["']([^"']*)`,
)

// pythonModules are standard library modules that cannot run a string as a
// command line other than through a name stringRunners matches or the
// subprocess calls stringsRun checks. Any other module, a local helper most
// of all, may run any string handed to it.
var pythonModules = nameSet(`__future__ abc argparse array ast asyncio base64 binascii
	bisect calendar collections colorsys concurrent configparser contextlib copy csv
	dataclasses datetime decimal difflib enum errno fileinput fnmatch fractions functools
	getpass gettext glob gzip hashlib heapq hmac html http io ipaddress itertools json
	keyword locale logging lzma math mimetypes numbers operator os pathlib platform pprint
	queue random re secrets shutil signal sqlite3 stat statistics string struct subprocess
	sys sysconfig tarfile tempfile textwrap threading time tomllib traceback types typing
	unicodedata unittest urllib uuid warnings weakref xml zipfile zlib zoneinfo
	requests pytest yaml toml tomli`)

// jsModules are the Node built-in modules with the same property.
// child_process is left out: a computed member (cp["ex" + "ec"]) reaches its
// exec without naming it.
var jsModules = nameSet(`assert buffer crypto events fs fs/promises os path
	process readline stream string_decoder timers url util zlib`)

// stringsRun reports whether Python or JavaScript source may run one of its
// plain strings as a command line: it calls something stringRunners
// matches, starts a program klaudiush cannot name or one that runs
// commands it is handed, or loads a module klaudiush does not know, whose
// functions may run what they are handed. Otherwise its strings are data
// (labels, messages, payloads) and only argv lists can start a command.
func stringsRun(code string, lang codeLang) bool {
	if strings.HasPrefix(code, "#!") {
		_, code, _ = strings.Cut(code, "\n")
	}

	if lang == langOther || codeUnsafe(code) ||
		stringRunners.MatchString(benignCalls.Replace(code)) {
		return true
	}

	if lang == langJavaScript && jsRunners.MatchString(code) {
		return true
	}

	if lang == langPython && (subprocessHidden(code) || spawnRunsStrings(code)) {
		return true
	}

	return unknownModule(code, lang)
}

// subprocessHidden reports whether Python source uses the subprocess module
// in a way spawnRunsStrings cannot follow: under another name, or through an
// attribute other than the argv calls and constants.
func subprocessHidden(code string) bool {
	if !strings.Contains(code, "subprocess") {
		return false
	}

	if importAlias.MatchString(code) || spawnImport.MatchString(code) {
		return true
	}

	for _, m := range subprocessUse.FindAllStringSubmatchIndex(code, -1) {
		if m[2] >= 0 {
			attr := code[m[2]:m[3]]
			if !subprocessAttrs[attr] ||
				spawnFuncs[attr] &&
					!strings.HasPrefix(strings.TrimLeft(code[m[3]:], regexSpace), "(") {
				return true
			}

			continue
		}

		if !inImport(code, m[0]) {
			return true
		}
	}

	return false
}

// inImport reports whether position at in code sits in an import statement.
func inImport(code string, at int) bool {
	start := strings.LastIndexAny(code[:at], "\n;") + 1

	return strings.Contains(code[start:at], "import")
}

// spawnRunsStrings reports whether a subprocess call in Python source may
// run a plain string: its argv is not a literal list whose program is
// spelled out, or an item of it names a program that runs commands handed
// to it (sh with input=, env bash, at now).
func spawnRunsStrings(code string) bool {
	for _, m := range spawnCall.FindAllStringIndex(code, -1) {
		items, ok := argvItems(code[m[1]:])
		if !ok || slices.ContainsFunc(items, commandRunner) {
			return true
		}
	}

	return false
}

// argvItems returns the quoted items of the list or tuple literal that opens
// rest, when its first item is a quoted program name. A comment in the list
// may hide an item or unbalance its quotes, so it reads as unknown.
func argvItems(rest string) ([]string, bool) {
	if rest == "" || rest[0] != '[' && rest[0] != '(' {
		return nil, false
	}

	body := strings.TrimLeft(rest[1:], regexSpace)
	if body == "" || !strings.ContainsRune(`"'`, rune(body[0])) {
		return nil, false
	}

	region := rest[1:listEnd(rest)]
	if strings.Contains(region, "#") {
		return nil, false
	}

	matches := listItem.FindAllStringSubmatch(region, -1)
	items := make([]string, 0, len(matches))

	for _, match := range matches {
		items = append(items, match[1]+match[2])
	}

	return items, true
}

// listEnd returns the index of the bracket closing the list that opens s,
// skipping quoted strings, or len(s) when it is not closed.
func listEnd(s string) int {
	depth := 0

	var quote byte

	for i := 0; i < len(s); i++ {
		c := s[i]

		switch {
		case quote != 0 && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[' || c == '(':
			depth++
		case c == ']' || c == ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return len(s)
}

// commandRunner reports whether an argv item names a program that runs
// commands handed to it: a shell, an interpreter, a launcher or one of
// stdinPrograms, a flag that hands one its command line, or a whole command
// line naming git, gh or a shell, which a wrapper klaudiush does not know
// (hyperfine, mosh) may hand to a shell. Case is folded, as macOS finds bash
// for Bash.
func commandRunner(item string) bool {
	item = strings.TrimSpace(item)
	if strings.ContainsAny(item, regexSpace) && mentionsCommand.MatchString(item) {
		return true
	}

	name := commandName(item)
	_, interp := interpreters[name]
	_, launch := launchers[name]

	return interp || launch || shells[name] || stdinPrograms[name] ||
		commandStringFlag.MatchString(item)
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
		modules := strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == '\\' })
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
