package parser

import (
	"path/filepath"
	"slices"
	"strings"
)

// programWrite is what one command writes: the paths it names, and whether
// it also changes files it does not name.
type programWrite struct {
	op      WriteOp
	targets []string
	unknown bool
}

// optionSpec says which options of a program take a value: short ones
// from the next word or the rest of their cluster, attached ones only from
// the rest of their cluster (sed -i.bak), long ones from "=" or the next
// word. abbrev allows GNU-style unique prefixes of long options.
type optionSpec struct {
	short    string
	attached string
	long     string
	abbrev   bool
}

// scanArgs splits args into operands, reporting each option and its value.
func scanArgs(args []string, spec optionSpec, onOpt func(name, value string)) []string {
	var operands []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions:
			return append(operands, args[i+1:]...)
		case arg == "-" || !strings.HasPrefix(arg, "-"):
			operands = append(operands, arg)
		case strings.HasPrefix(arg, "--"):
			i = scanLong(args, i, spec, onOpt)
		default:
			i = scanShort(args, i, spec, onOpt)
		}
	}

	return operands
}

// scanLong reports one long option, returning the index of the last word
// it uses.
func scanLong(args []string, i int, spec optionSpec, onOpt func(name, value string)) int {
	name, value, hasValue := strings.Cut(args[i], "=")
	name = spec.longName(name)

	if !hasValue && slices.Contains(strings.Fields(spec.long), name) && i+1 < len(args) {
		value = args[i+1]
		i++
	}

	onOpt(name, value)

	return i
}

// longName resolves an abbreviated long option to the one it stands for.
func (spec optionSpec) longName(name string) string {
	if !spec.abbrev || slices.Contains(strings.Fields(spec.long), name) {
		return name
	}

	match := ""

	for long := range strings.FieldsSeq(spec.long) {
		if strings.HasPrefix(long, name) {
			if match != "" {
				return name
			}

			match = long
		}
	}

	if match == "" {
		return name
	}

	return match
}

// scanShort reports each option of a short cluster, returning the index of
// the last word it uses.
func scanShort(args []string, i int, spec optionSpec, onOpt func(name, value string)) int {
	cluster := args[i][1:]

	for j, c := range cluster {
		rest := cluster[j+len(string(c)):]

		switch {
		case strings.ContainsRune(spec.attached, c):
			onOpt("-"+string(c), rest)

			return i
		case strings.ContainsRune(spec.short, c):
			if rest == "" && i+1 < len(args) {
				rest = args[i+1]
				i++
			}

			onOpt("-"+string(c), rest)

			return i
		default:
			onOpt("-"+string(c), "")
		}
	}

	return i
}

var (
	copySpec = optionSpec{
		short:  "St",
		long:   "--suffix --target-directory",
		abbrev: true,
	}
	installSpec = optionSpec{
		short:  "gmoSt",
		long:   "--group --mode --owner --suffix --target-directory --strip-program",
		abbrev: true,
	}
	sedSpec = optionSpec{
		short:    "efl",
		attached: "iI",
		long:     "--expression --file --line-length",
		abbrev:   true,
	}
	curlSpec = optionSpec{
		short: "AbcCdDeEFHKmoPQrtTuUwxXyYz",
		long: `--output --dump-header --cookie-jar --trace --trace-ascii --libcurl
			--etag-save --stderr --output-dir --config --hsts --alt-svc --data --data-raw
			--data-binary --data-urlencode --header --user --request --user-agent --referer
			--cookie --form --upload-file --write-out --proxy --range --max-time
			--connect-timeout --url --cacert --cert --key --retry`,
	}
)

// Options that name what a write program does or where it writes.
var (
	targetDirOpts   = nameSet("-t --target-directory")
	makeDirOpts     = nameSet("-d --directory")
	sedInPlaceOpts  = nameSet("-i -I --in-place")
	sedScriptOpts   = nameSet("-e -f --expression --file")
	curlRemoteOpts  = nameSet("-O --remote-name --remote-name-all")
	curlUnknownOpts = nameSet("-J --remote-header-name -K --config")
	curlDirOpts     = nameSet("--output-dir")
	curlURLOpts     = nameSet("--url")
)

// minDestOperands is the fewest operands of a copy that names a destination.
const minDestOperands = 2

// curlOutputs are the curl options that write the file they name; "-"
// stands for stdout or stderr.
var curlOutputs = nameSet(`-o --output -D --dump-header -c --cookie-jar --trace
	--trace-ascii --libcurl --etag-save --stderr --hsts --alt-svc`)

// writesOf returns what cmd writes. Its arguments may still carry the mark
// of a command substitution, which makes the path they name unknown.
func writesOf(cmd Command) programWrite {
	switch cmd.Name {
	case "tee":
		return programWrite{op: WriteOpTee, targets: extractTeeTargets(cmd.Args)}
	case "cp", "copy":
		return programWrite{op: WriteOpCopy, targets: destination(cmd.Args, copySpec)}
	case "mv", "move":
		return programWrite{op: WriteOpMove, targets: destination(cmd.Args, copySpec)}
	case "ln":
		return programWrite{op: WriteOpLink, targets: linkDestination(cmd.Args)}
	case "install":
		return programWrite{op: WriteOpOutput, targets: installDestination(cmd.Args)}
	case "dd":
		return programWrite{op: WriteOpOutput, targets: ddOutputs(cmd.Args)}
	case "sed":
		return programWrite{op: WriteOpEdit, targets: sedInPlaceFiles(cmd.Args)}
	case "curl":
		return curlWrites(cmd.Args)
	case "unzip":
		return programWrite{op: WriteOpUnpack, unknown: unzipExtracts(cmd.Args)}
	case "patch":
		return programWrite{op: WriteOpUnpack, unknown: patchApplies(cmd.Args)}
	case gitProgram:
		return programWrite{op: WriteOpUnpack, unknown: gitRewritesTree(cmd.Args)}
	default:
		return programWrite{}
	}
}

// destination returns where cp or mv puts its sources: the -t directory,
// else the last of at least two operands.
func destination(args []string, spec optionSpec) []string {
	var dirs []string

	operands := scanArgs(args, spec, func(name, value string) {
		if targetDirOpts[name] {
			dirs = append(dirs, value)
		}
	})

	if len(dirs) > 0 {
		return dirs
	}

	if len(operands) < minDestOperands {
		return nil
	}

	return operands[len(operands)-1:]
}

// linkDestination returns where ln creates its links. A single operand
// links it into the current directory under its own name.
func linkDestination(args []string) []string {
	if dest := destination(args, copySpec); len(dest) > 0 {
		return dest
	}

	operands := scanArgs(args, copySpec, func(string, string) {})
	if len(operands) != 1 {
		return nil
	}

	base := filepath.Base(strings.TrimRight(operands[0], "/"))
	if base == "/" || base == "." {
		return nil
	}

	return []string{base}
}

// installDestination returns the file or directory install copies to.
// install -d only creates directories.
func installDestination(args []string) []string {
	creates := false

	operands := scanArgs(args, installSpec, func(name, _ string) {
		if makeDirOpts[name] {
			creates = true
		}
	})

	if creates || len(operands) == 0 {
		return nil
	}

	return destination(args, installSpec)
}

// ddOutputs returns the of= file of dd.
func ddOutputs(args []string) []string {
	var outputs []string

	for _, arg := range args {
		if path, ok := strings.CutPrefix(arg, "of="); ok {
			outputs = append(outputs, path)
		}
	}

	return outputs
}

// sedInPlaceFiles returns the files sed -i rewrites: every operand after
// the script. BSD sed takes the word after -i as a backup suffix, which is
// read here as an operand, so the file is still among the targets.
func sedInPlaceFiles(args []string) []string {
	inPlace, scripted := false, false

	operands := scanArgs(args, sedSpec, func(name, _ string) {
		inPlace = inPlace || sedInPlaceOpts[name]
		scripted = scripted || sedScriptOpts[name]
	})

	if !inPlace {
		return nil
	}

	operands = slices.DeleteFunc(operands, func(op string) bool { return op == "" || op == "-" })
	if !scripted && len(operands) > 0 {
		operands = operands[1:]
	}

	return operands
}

// curlWrites returns the files curl writes. A name taken from the server
// (-J) or options read from a file (-K) make the target unknown.
func curlWrites(args []string) programWrite {
	write := programWrite{op: WriteOpOutput}
	remote, dir := false, ""

	var urls []string

	operands := scanArgs(args, curlSpec, func(name, value string) {
		switch {
		case curlOutputs[name] && value != "-":
			write.targets = append(write.targets, value)
		case curlRemoteOpts[name]:
			remote = true
		case curlUnknownOpts[name]:
			write.unknown = true
		case curlDirOpts[name]:
			dir = value
		case curlURLOpts[name]:
			urls = append(urls, value)
		}
	})

	if remote {
		for _, url := range append(urls, operands...) {
			name, ok := remoteName(url)
			if !ok {
				write.unknown = true

				continue
			}

			write.targets = append(write.targets, name)
		}
	}

	if dir != "" {
		for i, target := range write.targets {
			if !filepath.IsAbs(target) {
				write.targets[i] = filepath.Join(dir, target)
			}
		}
	}

	return write
}

// remoteName returns the file name curl -O saves url under: the last
// segment of its path.
func remoteName(url string) (string, bool) {
	if _, rest, found := strings.Cut(url, "://"); found {
		url = rest
	}

	url, _, _ = strings.Cut(url, "#")
	url, _, _ = strings.Cut(url, "?")

	i := strings.LastIndex(url, "/")
	if i < 0 {
		return "", false
	}

	name := url[i+1:]
	if name == "" || name == "." || name == ".." || HasUnresolvedVars(name) || marked(name) {
		return "", false
	}

	return name, true
}

// unzipReadOnly are unzip options that list, test or print an archive
// instead of extracting it.
const unzipReadOnly = "lptvzZ"

// unzipExtracts reports unzip extracting an archive, which writes files the
// command line does not name.
func unzipExtracts(args []string) bool {
	operands := 0

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "-") && arg != "-" && !strings.HasPrefix(arg, "--"):
			if strings.ContainsAny(arg[1:], unzipReadOnly) {
				return false
			}
		case strings.HasPrefix(arg, "--"):
		default:
			operands++
		}
	}

	return operands > 0
}

// patchApplies reports patch changing files, which are named in the patch
// rather than on the command line.
func patchApplies(args []string) bool {
	return !slices.ContainsFunc(args, func(arg string) bool {
		return arg == "--dry-run" || arg == "--check" || arg == "--help" || arg == "--version"
	})
}

// stashWrites are the git stash subcommands that change the work tree;
// stash without one pushes.
var stashWrites = nameSet("push save pop apply branch")

// gitRewritesTree reports a git command that changes work tree files it
// does not name: reset --hard, --merge or --keep, and stash push, pop,
// apply or branch.
func gitRewritesTree(args []string) bool {
	idx := gitSubcommandIndex(args)
	if idx < 0 {
		return false
	}

	rest := args[idx+1:]

	switch args[idx] {
	case "reset":
		return slices.ContainsFunc(rest, func(arg string) bool {
			return arg == "--hard" || arg == "--merge" || arg == "--keep"
		})
	case "stash":
		return len(rest) == 0 || strings.HasPrefix(rest[0], "-") || stashWrites[rest[0]]
	default:
		return false
	}
}
