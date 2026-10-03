package evidence

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// interpreterOptions describes the options of a program that runs the
// script its first operand names: flags take no argument, valued options
// take one attached or as the next word, attached options take an optional
// argument only as the rest of their word, long options take none and
// longValued ones take one after = or as the next word. plus marks shells,
// where +o and +O work as -o and -O. Any other option, including those that
// replace the script with inline code, a module or standard input, leaves
// the script unknown.
type interpreterOptions struct {
	flags      string
	valued     string
	attached   string
	long       []string
	longValued []string
	plus       bool
}

const (
	helpOption    = "--help"
	versionOption = "--version"
)

var (
	pythonOptions = interpreterOptions{
		flags:  "bBdEhiIOPqsSuvVx",
		valued: "WX",
		long: []string{
			helpOption,
			versionOption,
			"--help-env",
			"--help-xoptions",
			"--help-all",
		},
		longValued: []string{"--check-hash-based-pycs"},
	}
	nodeOptions = interpreterOptions{
		valued: "rC",
		long: []string{
			"--no-warnings", "--no-deprecation", "--trace-warnings", "--enable-source-maps",
			"--experimental-vm-modules", "--preserve-symlinks", "--throw-deprecation",
		},
		longValued: []string{
			"--require", "--import", "--loader", "--experimental-loader", "--conditions",
			"--input-type", "--env-file", "--title", "--stack-size",
		},
	}
	rubyOptions = interpreterOptions{
		flags:      "acdlnpsSvwyU",
		valued:     "IrEF",
		attached:   "WKT0",
		long:       []string{"--verbose", versionOption, helpOption, "--yydebug", "--jit"},
		longValued: []string{"--encoding", "--external-encoding", "--internal-encoding"},
	}
	perlOptions = interpreterOptions{
		flags:    "acnpsStTuUwWvhX",
		valued:   "I",
		attached: "MmdDlF0iC",
	}
	shellOptions = interpreterOptions{
		flags:  "abefhkmnptuvxBCHPilrTE",
		valued: "oO",
		long: []string{
			"--norc", "--noprofile", "--posix", "--login", "--restricted", "--verbose",
			"--noediting", "--debugger", "--dump-strings", helpOption, versionOption, "--protected",
		},
		longValued: []string{"--rcfile", "--init-file"},
		plus:       true,
	}
)

var (
	pythonName = regexp.MustCompile(`^python[0-9.]*$`)
	shellNames = []string{"sh", "bash", "dash", "zsh", "ksh"}
)

func optionsFor(program string) (interpreterOptions, bool) {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(program)), ".exe")

	switch {
	case pythonName.MatchString(name):
		return pythonOptions, true
	case name == "node" || name == "nodejs":
		return nodeOptions, true
	case name == "ruby":
		return rubyOptions, true
	case name == "perl":
		return perlOptions, true
	case slices.Contains(shellNames, name):
		return shellOptions, true
	default:
		return interpreterOptions{}, false
	}
}

// programWords lists the words of a prerequisite command that name a file
// it runs: the program, and for an interpreter the values of its options
// and the script. When the script cannot be identified every operand
// counts, so no file the command may run stays writable.
func programWords(argv []string) []string {
	opts, ok := optionsFor(argv[0])
	if !ok {
		return argv[:1]
	}

	words, ok := opts.scriptWords(argv[1:])
	if !ok {
		return argv
	}

	return append([]string{argv[0]}, words...)
}

// scriptWords returns the option values and the script among args, or
// false when the script cannot be identified.
func (o interpreterOptions) scriptWords(args []string) ([]string, bool) {
	var words []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		var (
			value    string
			consumes bool
			known    bool
		)

		switch {
		case arg == "--":
			if i+1 < len(args) {
				words = append(words, args[i+1])
			}

			return words, true
		case strings.HasPrefix(arg, "--"):
			value, consumes, known = o.longOption(arg)
		case len(arg) > 1 && (arg[0] == '-' || (o.plus && arg[0] == '+')):
			value, consumes, known = o.shortCluster(arg[1:])
		default:
			return append(words, arg), arg != "-"
		}

		if !known {
			return nil, false
		}

		if value != "" {
			words = append(words, value)
		}

		if consumes {
			if i++; i >= len(args) {
				return nil, false
			}

			words = append(words, args[i])
		}
	}

	return words, true
}

func (o interpreterOptions) longOption(arg string) (string, bool, bool) {
	name, value, hasValue := strings.Cut(arg, "=")

	switch {
	case slices.Contains(o.longValued, name):
		return value, !hasValue, true
	case slices.Contains(o.long, name) && !hasValue:
		return "", false, true
	default:
		return "", false, false
	}
}

// shortCluster reads a cluster of short options, such as -uW or -Wignore,
// returning an attached value, whether the next argument is consumed, and
// whether every option was known.
func (o interpreterOptions) shortCluster(cluster string) (string, bool, bool) {
	for i, opt := range cluster {
		rest := cluster[i+1:]

		switch {
		case strings.ContainsRune(o.flags, opt):
		case strings.ContainsRune(o.valued, opt):
			return rest, rest == "", true
		case strings.ContainsRune(o.attached, opt):
			return rest, false, true
		default:
			return "", false, false
		}
	}

	return "", false, true
}
