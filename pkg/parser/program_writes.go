package parser

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// programWrite is what one command writes: the paths it names, and whether
// it also changes files it does not name. Those files are below root (relative to the
// command's directory, the work tree around it when tree is set), or
// anywhere.
type programWrite struct {
	op       WriteOp
	targets  []string
	unknown  bool
	root     string
	anywhere bool
	tree     bool
}

// optionSpec says which options of a program take a value: short ones
// from the next word or the rest of their cluster, attached ones only from
// the rest of their cluster (sed -i.bak), long ones from "=" or the next
// word. flags are long options without a value. abbrev allows GNU-style
// unique prefixes of the long options and flags.
type optionSpec struct {
	short    string
	attached string
	long     string
	flags    string
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
	known := strings.Fields(spec.long + " " + spec.flags)
	if !spec.abbrev || slices.Contains(known, name) {
		return name
	}

	match := ""

	for _, long := range known {
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

	for j := 0; j < len(cluster); {
		c, size := utf8.DecodeRuneInString(cluster[j:])
		j += size
		rest := cluster[j:]

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
		short: "St",
		long:  "--suffix --target-directory --sparse",
		flags: `--recursive --force --interactive --no-clobber --link --symbolic-link
			--parents --update --verbose --archive --no-target-directory
			--strip-trailing-slashes --dereference --no-dereference --relative --logical
			--physical`,
		abbrev: true,
	}
	installSpec = optionSpec{
		short: "gmoSt",
		long:  "--group --mode --owner --suffix --target-directory --strip-program",
		flags: `--strip --directory --compare --preserve-timestamps --verbose
			--no-target-directory --preserve-context`,
		abbrev: true,
	}
	sedSpec = optionSpec{
		short:    "efl",
		attached: "iI",
		long:     "--expression --file --line-length",
		flags: `--in-place --quiet --silent --regexp-extended --separate --null-data
			--posix --sandbox --debug --unbuffered --follow-symlinks`,
		abbrev: true,
	}
	perlSpec = optionSpec{
		short:    "eE",
		attached: "iIMmdDxlC0F",
	}
	curlSpec = optionSpec{
		short: "AbcCdDeEFHKmoPQrtTuUwxXyYz",
		long: `--output --dump-header --cookie-jar --trace --trace-ascii --libcurl
			--etag-save --stderr --output-dir --config --hsts --alt-svc --data --data-raw
			--data-binary --data-urlencode --header --user --request --user-agent --referer
			--cookie --form --upload-file --write-out --proxy --range --max-time
			--connect-timeout --url --cacert --cert --key --retry --json --resolve
			--connect-to --form-string --data-ascii --oauth2-bearer --variable`,
	}
	wgetSpec = optionSpec{
		short: "OoaPeUtTwQARDIXlBi",
		long: `--output-document --output-file --append-output --directory-prefix
			--user-agent --tries --timeout --wait --input-file --header --post-data
			--post-file --user --password --level --accept --reject --domains`,
		flags:  "--recursive --mirror --content-disposition --page-requisites",
		abbrev: true,
	}
	rsyncSpec = optionSpec{
		short: "eTBfM",
		long: `--rsh --exclude --include --exclude-from --include-from --files-from
			--filter --backup-dir --suffix --chmod --temp-dir --partial-dir --log-file
			--compare-dest --copy-dest --link-dest --rsync-path --block-size --max-size
			--min-size --max-delete --timeout --contimeout --modify-window --port
			--password-file --bwlimit --out-format --log-file-format --chown --usermap
			--groupmap --iconv --checksum-choice --compress-choice --compress-level
			--skip-compress --info --debug --protocol --address --sockopts --outbuf
			--remote-option --stop-after --stop-at --max-alloc --checksum-seed
			--write-batch --only-write-batch --read-batch --mkpath-mode`,
	}
)

// Options that name what a write program does or where it writes.
var (
	targetDirOpts   = nameSet("-t --target-directory")
	makeDirOpts     = nameSet("-d --directory")
	sedInPlaceOpts  = nameSet("-i -I --in-place")
	sedScriptOpts   = nameSet("-e -f --expression --file")
	perlScriptOpts  = nameSet("-e -E")
	curlRemoteOpts  = nameSet("-O --remote-name --remote-name-all")
	curlUnknownOpts = nameSet("-J --remote-header-name")
	curlConfigOpts  = nameSet("-K --config")
	curlDirOpts     = nameSet("--output-dir")
	curlURLOpts     = nameSet("--url")
	wgetOutputOpts  = nameSet("-O --output-document -o --output-file -a --append-output")
	wgetDirOpts     = nameSet("-P --directory-prefix")
	wgetUnknownOpts = nameSet(
		"-r --recursive -m --mirror -i --input-file --content-disposition -p --page-requisites",
	)
	curlDirOutputOps = nameSet("-o --output")
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
	case "rsync":
		return programWrite{op: WriteOpCopy, targets: destination(cmd.Args, rsyncSpec)}
	case "ln":
		return programWrite{op: WriteOpLink, targets: linkDestination(cmd.Args)}
	case "install":
		return programWrite{op: WriteOpOutput, targets: installDestination(cmd.Args)}
	case "dd":
		return programWrite{op: WriteOpOutput, targets: ddOutputs(cmd.Args)}
	case "sed", "gsed":
		return programWrite{
			op:      WriteOpEdit,
			targets: inPlaceFiles(cmd.Args, sedSpec, sedScriptOpts),
		}
	case "perl":
		return programWrite{
			op:      WriteOpEdit,
			targets: inPlaceFiles(cmd.Args, perlSpec, perlScriptOpts),
		}
	case "curl":
		return curlWrites(cmd.Args)
	case "wget":
		return wgetWrites(cmd.Args)
	default:
		return treeWritesOf(cmd)
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

// inPlaceFiles returns the files sed -i or perl -i rewrites: every operand
// after the script. BSD sed takes the word after -i as a backup suffix,
// which is read here as an operand, so the file is still among the targets.
func inPlaceFiles(args []string, spec optionSpec, scriptOpts map[string]bool) []string {
	inPlace, scripted := false, false

	operands := scanArgs(args, spec, func(name, _ string) {
		inPlace = inPlace || sedInPlaceOpts[name]
		scripted = scripted || scriptOpts[name]
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

	var urls, named []string

	operands := scanArgs(args, curlSpec, func(name, value string) {
		switch {
		case curlDirOutputOps[name] && value != "-":
			named = append(named, value)
		case curlOutputs[name] && value != "-":
			write.targets = append(write.targets, value)
		case curlRemoteOpts[name]:
			remote = true
		case curlUnknownOpts[name]:
			write.unknown = true
		case curlConfigOpts[name]:
			write.unknown, write.anywhere = true, true
		case curlDirOpts[name]:
			dir = value
		case curlURLOpts[name]:
			urls = append(urls, value)
		}
	})

	if remote {
		named = append(named, remoteNames(append(urls, operands...), &write)...)
	}

	write.targets = append(write.targets, underDir(dir, named)...)
	write.root = dir

	return write
}

// wgetWrites returns the files wget writes: -O, else each URL's own name.
// A recursive or listed download writes files the command does not name.
func wgetWrites(args []string) programWrite {
	write := programWrite{op: WriteOpOutput}
	dir, document := "", false

	operands := scanArgs(args, wgetSpec, func(name, value string) {
		switch {
		case wgetOutputOpts[name]:
			document = document || name == "-O" || name == "--output-document"

			if value != "-" {
				write.targets = append(write.targets, value)
			}
		case wgetDirOpts[name]:
			dir = value
		case wgetUnknownOpts[name]:
			write.unknown = true
		}
	})

	if !document {
		write.targets = append(write.targets, underDir(dir, remoteNames(operands, &write))...)
	}

	write.root = dir

	return write
}

// remoteNames returns the names downloads of urls are saved under, marking
// write unknown for a URL without one.
func remoteNames(urls []string, write *programWrite) []string {
	names := make([]string, 0, len(urls))

	for _, url := range urls {
		name, ok := remoteName(url)
		if !ok {
			write.unknown = true

			continue
		}

		names = append(names, name)
	}

	return names
}

// underDir joins relative paths onto dir.
func underDir(dir string, paths []string) []string {
	if dir == "" {
		return paths
	}

	joined := make([]string, 0, len(paths))

	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}

		joined = append(joined, path)
	}

	return joined
}

// remoteName returns the file name a download of url is saved under: the
// last segment of its path.
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
	if name == "" || name == "." || name == parentDir || HasUnresolvedVars(name) || marked(name) ||
		strings.ContainsAny(name, "{}[]") {
		return "", false
	}

	return name, true
}
