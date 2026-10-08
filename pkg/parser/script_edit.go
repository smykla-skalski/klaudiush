package parser

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// DetailScriptEdited is the Opacity.Detail of a script whose content
// klaudiush captured from a write earlier on the line, when a command
// between that write and the run may have edited the file.
const DetailScriptEdited = "a command after it is written on the line may edit it, " +
	"so what runs may differ from the content klaudiush saw"

// readOnlyPrograms leave the content of the files they name as it is:
// readers, filters, checksums, system status and metadata changes. A
// redirect on one of them is a write klaudiush tracks on its own.
var readOnlyPrograms = nameSet(`cat head tail less more wc ls stat file grep egrep fgrep
	rg diff cmp test [ md5sum md5 shasum sha1sum sha256sum chmod chown chgrp touch echo
	printf realpath readlink basename dirname du true false pwd cd mkdir jq tr cut column
	nl od hexdump sleep date which uname whoami id hostname printenv tput clear seq base64
	df ps free nproc uptime curl wget`)

// editPrograms change files they do not name: builds, archives, patches and
// syncs.
var editPrograms = nameSet(`make gmake bmake just task ninja unzip tar bsdtar cpio 7z rsync
	patch install`)

// formatterPrograms rewrite the files under a directory they are given.
var formatterPrograms = nameSet(`black isort autoflake autopep8 yapf prettier gofmt goimports
	gofumpt rustfmt clang-format shfmt`)

// writeFlags make a linter or formatter rewrite the files it checks.
var writeFlags = nameSet("-w --write --fix -i --inplace fmt format fix")

// filterWrites maps the filters that only read their files to the flag
// that makes them write one: a short option letter (sed -i, sed -Ei, sort -o)
// or text in an option (awk -i inplace).
var filterWrites = func() map[string]string {
	flags := make(map[string]string)

	for pair := range strings.FieldsSeq("sed=i sort=o awk=inplace") {
		program, flag, _ := strings.Cut(pair, "=")
		flags[program] = flag
	}

	return flags
}()

// longWrites are the long options that make sed or sort write a file.
var longWrites = []string{"--in-place", "--output"}

// filterWrite reports whether arg turns on the write flag of a filter: a
// short option cluster holding its letter (sed -Ei, sort -ro), a long
// in-place or output option, or text in an option (awk -i inplace).
func filterWrite(arg, flag string) bool {
	if len([]rune(flag)) > 1 {
		return strings.Contains(arg, flag)
	}

	if slices.ContainsFunc(
		longWrites,
		func(long string) bool { return strings.HasPrefix(arg, long) },
	) {
		return true
	}

	return strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
		strings.Contains(arg[1:], flag)
}

// checkFlags keep a formatter or linter to reporting (black --check,
// gofmt -l, tofu fmt -check).
var checkFlags = nameSet("--check -check -l -d --diff --dry-run --list-different")

// checkerPrograms read the files they are given and report on them: linters,
// type checkers, test runners and compilers.
var checkerPrograms = nameSet(`ruff flake8 pylint mypy pyright pyflakes bandit pytest
	py_compile compileall shellcheck yamllint ansible-lint actionlint tflint tofu terraform
	golangci-lint eslint tsc go cargo node-check markdownlint black prettier`)

// readOnlyGit are the git subcommands that leave work tree files as they
// are. Any other may rewrite them (checkout, restore, apply, stash, reset).
var readOnlyGit = nameSet(`add diff status log show ls-files blame commit fetch push
	branch tag remote config rev-parse describe shortlog grep`)

// codeFlags hand an interpreter its program as an argument (python -c,
// node -e, ruby -e, perl -e) or name a module it runs (python -m).
var codeFlags = nameSet("-c -e -E --eval -p --print -m -r")

// fileEdits matches program source that may change files: writes (write
// calls, write-mode opens, print to a file, Perl and awk output opens),
// dumps, renames, removals, permission changes, or starting another
// program, which may do any of these. Comparisons and arrows (a > b, ->)
// are not writes.
var fileEdits = regexp.MustCompile(
	`(?i)\.write\s*\(|write_(?:text|bytes)|writefile|appendfile|createwritestream|` +
		`open\s*\([^)]*,\s*(?:mode\s*=\s*)?["'][rbt]*[wax+][rwxabt+]*["']|\.open\s*\(\s*["'][wax+]|["']>>?[^=>]|\bfile\s*=|` +
		`print[f]?\s*>|\bdump\s*\(|os\.(?:rename|replace|remove|unlink|truncate|chmod|system)|` +
		`shutil\.|rmtree|popen|subprocess|child_process|\bexec\w*\s*\(|spawn|\bsystem\s*\(`,
)

// lastLineWriteSeq returns the execution position of the last write
// lastLineWrite reads target from, or 0 when it is unknown.
func (w *astWalker) lastLineWriteSeq(target string) int {
	for p := w; p != nil; p = p.parent {
		seq, found := 0, false

		for _, fw := range p.fileWrites {
			if path, known := w.writtenPath(fw); known && filepath.Clean(path) == target {
				seq, found = fw.Location.Seq, true
			}
		}

		if found {
			return seq
		}
	}

	return 0
}

// editedAfter reports a command run after the write at seq and before cmd
// that may change target without klaudiush capturing the result (see
// mayEdit). Writes klaudiush tracks (redirects, tee, cp, mv, sed -i) are
// handled by lastLineWrite, which then stops trusting the capture.
func (w *astWalker) editedAfter(cmd Command, target string, seq int) bool {
	if seq == 0 || cmd.Location.Seq == 0 {
		return false
	}

	for earlier := range w.earlierCommands() {
		if earlier.Location.Seq <= seq || earlier.Location.Seq >= cmd.Location.Seq {
			continue
		}

		if w.mayEdit(earlier, target) {
			return true
		}
	}

	return false
}

// mayEdit reports whether cmd may change target without klaudiush seeing
// the write. Shells, launchers, builtins and functions and aliases defined
// on the line are left out: the commands they run are recorded and judged
// on their own. An interpreter counts when its program may change files;
// git when its subcommand may rewrite the work tree; editPrograms always;
// any other program outside readOnlyPrograms as operandsEdit says.
func (w *astWalker) mayEdit(cmd Command, target string) bool {
	name := commandName(cmd.Name)

	if spec, ok := interpreters[name]; ok {
		return w.interpreterEdits(cmd, spec, target)
	}

	_, launcher := launchers[name]
	_, function := w.funcs[cmd.Name]
	_, alias := w.aliases[cmd.Name]

	switch {
	case readOnlyPrograms[name] || shells[name] || shellBuiltins[name] || launcher ||
		function || alias:
		return false
	case editPrograms[name]:
		return true
	case name == gitProgram:
		return gitEdits(cmd)
	case name == "find":
		return slices.ContainsFunc(cmd.Args, func(arg string) bool {
			return arg == "-delete" || strings.HasPrefix(arg, "-fprint")
		})
	}

	return w.operandsEdit(name, cmd.Args, target)
}

// operandsEdit reports whether a program named name, given args, may change
// target. A linter, checker or filter (checkerPrograms, sed, awk, sort)
// reads what it names unless it writes: a formatter outside check mode, a
// write flag (-w, --fix, --in-place, fmt, sed -i, sort -o, awk inplace).
// Any other program counts when an operand names target, matches it as a
// glob or cannot be resolved, or when it writes and an operand is a
// directory above target.
func (w *astWalker) operandsEdit(name string, args []string, target string) bool {
	filterFlag, filter := filterWrites[name]
	checking := slices.ContainsFunc(args, func(arg string) bool { return checkFlags[arg] })
	writes := !checking &&
		(formatterPrograms[name] || slices.ContainsFunc(args, func(arg string) bool {
			return writeFlags[arg] || strings.HasPrefix(arg, "--in-place") ||
				filter && filterWrite(arg, filterFlag) || strings.HasPrefix(arg, "--fix")
		}))
	reader := checkerPrograms[name] || formatterPrograms[name] || filter

	if reader && !writes {
		return false
	}

	return slices.ContainsFunc(args, func(arg string) bool {
		names, dir := w.mayName(arg, target)

		return names || dir && writes
	})
}

// gitEdits reports whether a git command may rewrite work tree files. A
// new branch (checkout -b, switch -c) leaves them as they are.
func gitEdits(cmd Command) bool {
	gitCmd, err := ParseGitCommand(cmd)
	if err != nil {
		return true
	}

	switch gitCmd.Subcommand {
	case "checkout":
		return !gitCmd.HasFlag("-b") && !gitCmd.HasFlag("-B")
	case "switch":
		return !gitCmd.HasFlag("-c") && !gitCmd.HasFlag("-C")
	default:
		return !readOnlyGit[gitCmd.Subcommand]
	}
}

// mayName reports whether an argument names target (the file alone or as a
// flag value, a glob matching it, or a word klaudiush cannot resolve: a
// variable left after expansion) and, apart from that, whether it names a
// directory that may hold it. A URL names no local file.
func (w *astWalker) mayName(arg, target string) (names, dir bool) {
	if strings.Contains(arg, "://") {
		return false, false
	}

	if strings.Contains(arg, "$") {
		arg = w.expandName(arg)
	}

	if marked(arg) || strings.Contains(arg, "$") || arg == "{}" {
		return true, false
	}

	if _, value, ok := strings.Cut(arg, "="); ok && strings.HasPrefix(arg, "-") {
		arg = value
	}

	base := filepath.Base(target)
	clean := filepath.Clean(arg)

	if strings.ContainsAny(arg, "*?[") {
		matched, err := filepath.Match(filepath.Base(clean), base)

		return err != nil || matched, true
	}

	if clean == base || strings.HasSuffix(clean, string(filepath.Separator)+base) {
		return true, false
	}

	return false, clean == "." || clean == parentDir || strings.HasSuffix(arg, "/") ||
		strings.HasSuffix(clean, "...")
}

// interpreterEdits reports whether an interpreter run may change target:
// its inline program, its program on stdin or the script it runs may change
// files or names target. Running target itself, whose content klaudiush
// has, does not count, nor does a module (python -m) or inline code that
// only reads. A script is read from its capture on the line or from disk,
// never through scriptSource, which would judge earlier edits again for
// every script and grow with each one.
func (w *astWalker) interpreterEdits(cmd Command, spec interpreter, target string) bool {
	operands := make([]string, 0, len(cmd.Args))
	inline := spec.codeFirst

	for _, arg := range cmd.Args {
		if codeFlags[arg] {
			inline = true
		}

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			operands = append(operands, arg)
		}
	}

	if module, ok := moduleArgs(cmd.Args); ok {
		return w.operandsEdit(module[0], module[1:], target)
	}

	if spec.codeFirst && len(operands) > 0 {
		return codeEdits(operands[0], target) ||
			w.operandsEdit(commandName(cmd.Name), cmd.Args, target)
	}

	if inline {
		inPlace := slices.ContainsFunc(cmd.Args, func(arg string) bool {
			return filterWrite(arg, "i")
		})

		return codeEdits(strings.Join(cmd.Args, " "), target) ||
			unknownModule(strings.Join(operands, "\n"), langPython) ||
			inPlace && slices.ContainsFunc(operands, func(arg string) bool {
				names, dir := w.mayName(arg, target)

				return names || dir
			})
	}

	if len(operands) == 0 || operands[0] == "-" || operands[0] == devStdin {
		return cmd.Stdin == "" || codeEdits(cmd.Stdin, target)
	}

	script := filepath.Clean(w.trackedPath(cmd.WorkingDirectory, w.expandName(operands[0])))
	if script == target {
		return false
	}

	text, found, captured := w.lastLineWrite(script)
	if found {
		return !captured || codeEdits(text, target)
	}

	text, status := w.resolver.ReadScript(script)

	return status != ScriptText || codeEdits(text, target)
}

// moduleArgs returns the module an interpreter runs with -m and the
// arguments after it.
func moduleArgs(args []string) ([]string, bool) {
	i := slices.Index(args, "-m")
	if i < 0 || i+1 >= len(args) {
		return nil, false
	}

	return args[i+1:], true
}

// codeEdits reports whether program source may change files or names
// target.
func codeEdits(code, target string) bool {
	return fileEdits.MatchString(code) || strings.Contains(code, filepath.Base(target))
}
