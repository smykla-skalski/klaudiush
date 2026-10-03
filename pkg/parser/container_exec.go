package parser

import (
	"slices"
	"strings"
)

// ContainerExecOperation names a container runner's exec in an Opacity: the
// container or an exec option comes from a word klaudiush cannot read.
const ContainerExecOperation = "container exec"

// containerExecWord is the subcommand that runs a program in a running
// container (docker exec, podman container exec, docker compose exec).
const containerExecWord = "exec"

// containerExecGroups are the words that may come before exec (docker
// container exec, docker compose exec).
var containerExecGroups = nameSet("container compose")

// containerGlobalValueFlags are the global options of docker, podman,
// nerdctl and compose that take the next argument as their value.
var containerGlobalValueFlags = nameSet(`--address --ansi --bridge-ip --cdi-spec-dir
	--cgroup-manager --cni-netconfpath --cni-path --config --conmon --connection --context
	--data-root --env-file --events-backend --file --host --host-gateway-ip --hooks-dir
	--hosts-dir --identity --imagestore --log-level --module --namespace --network-cmd-path
	--network-config-dir --out --parallel --profile --progress --project-directory
	--project-name --root --runroot --runtime --runtime-flag --snapshotter --ssh
	--storage-driver --storage-opt --tlscacert --tlscert --tlskey --tmpdir --url
	--volumepath`)

// containerGlobalBoolFlags are the long global options known to stand alone.
var containerGlobalBoolFlags = nameSet(`--all-resources --compatibility --debug --debug-full
	--dry-run --experimental --insecure-registry --no-ansi --noout --remote
	--skip-hostname-check --syslog --tls --tlsverify --transient-store --verbose`)

// containerGlobalShortValueFlags are the short global options that take a
// value (-c, -H, -l, -f, -p, -n, -a); containerGlobalShortBoolFlags stand
// alone. Any other short option leaves where exec is unknown.
const (
	containerGlobalShortValueFlags = "cHlfpna"
	containerGlobalShortBoolFlags  = "Dhrv"
)

// containerExecValueFlags are the exec options of docker, podman, nerdctl
// and compose that take the next argument as their value.
var containerExecValueFlags = nameSet(`--detach-keys --env --env-file --index --preserve-fd
	--preserve-fds --user --workdir`)

// containerExecBoolFlags are the long exec options known to stand alone.
// Any other option without a value attached is read both ways.
var containerExecBoolFlags = nameSet(`--detach --dry-run --help --interactive --latest
	--no-TTY --privileged --tty`)

// containerExecShortValueFlags are the short exec options that take a value
// (-e, -u, -w); containerExecShortBoolFlags stand alone (-l is podman's
// --latest, which runs in the newest container and takes no container name).
const (
	containerExecShortValueFlags = "euw"
	containerExecShortBoolFlags  = "dilTt"
	containerExecLatestFlag      = "--latest"
)

// containerExecSubcommand returns the index of a container runner's exec
// subcommand, after global options and the container or compose group, or
// -1. A word it cannot place (an option of unknown arity, another
// subcommand, a word from a variable) leaves exec unknown, so the arguments
// are scanned as before.
func containerExecSubcommand(args []string) int {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == containerExecWord:
			return i
		case containerExecGroups[arg]:
		case strings.HasPrefix(arg, "--"):
			if strings.Contains(arg, "=") || containerGlobalBoolFlags[arg] {
				continue
			}

			if !containerGlobalValueFlags[arg] {
				return -1
			}

			i++
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			takes, known := globalShortCluster(arg)
			if !known {
				return -1
			}

			if takes {
				i++
			}
		default:
			return -1
		}
	}

	return -1
}

// globalShortCluster reads a short global option cluster (-D, -Hhost, -c):
// whether its value is the next argument, and whether every letter up to
// the value is known.
func globalShortCluster(arg string) (takesNext, known bool) {
	for j := 1; j < len(arg); j++ {
		switch {
		case strings.IndexByte(containerGlobalShortValueFlags, arg[j]) >= 0:
			return j == len(arg)-1, true
		case strings.IndexByte(containerGlobalShortBoolFlags, arg[j]) < 0:
			return false, false
		}
	}

	return false, true
}

// execReader reads exec options up to the container and the program after
// it. Where an option may or may not take the next argument (an option it
// does not know, a value the parser dropped as an empty word, a value
// starting with -), it follows both readings, so the one the runner uses is
// among them. A reading is started once per place, so readings that meet
// again cost nothing; past maxContainerReadings it stops and reports
// exhausted. A word in the place of an option or the container that comes
// from a variable, command output, a glob or a brace expansion may stand
// for any options or for the container and the program, so it is reported
// as dynamic. programs holds the index in args of each program found.
type execReader struct {
	args      []string
	programs  []int
	started   map[execStart]bool
	readings  int
	exhausted bool
	dynamic   string
}

// execStart is where a reading starts: an argument, and whether --latest
// came before it.
type execStart struct {
	at     int
	latest bool
}

// containerExecs reads the programs a container runner's exec runs, and
// reports false when its arguments have no exec subcommand it can place.
func containerExecs(args []string) (*execReader, bool) {
	at := containerExecSubcommand(args)
	if at < 0 {
		return nil, false
	}

	r := &execReader{args: args, started: make(map[execStart]bool)}
	r.fork(at+1, false)

	return r, true
}

// fork follows a reading from i, within maxContainerReadings.
func (r *execReader) fork(i int, latest bool) {
	start := execStart{at: i, latest: latest}
	if r.started[start] {
		return
	}

	r.started[start] = true

	if r.readings >= maxContainerReadings {
		r.exhausted = true

		return
	}

	r.readings++
	r.read(i, latest)
}

// read continues a reading at i. latest is set once --latest is seen.
func (r *execReader) read(i int, latest bool) {
	for ; i < len(r.args); i++ {
		arg := r.args[i]

		switch {
		case arg == endOfOptions:
			r.operands(i+1, latest)

			return
		case globsOptions(arg):
			r.dynamic = DetailWordOutput

			return
		case strings.HasPrefix(arg, "--"):
			name, _, attached := strings.Cut(arg, "=")
			if name == containerExecLatestFlag && attached && !latest {
				// --latest=false takes a container, any other value does not.
				r.fork(i+1, true)
			}

			latest = latest || (name == containerExecLatestFlag && !attached)

			if attached || containerExecBoolFlags[name] {
				continue
			}

			takes := containerExecValueFlags[name]
			i = r.skipValue(i, latest, takes, !takes)
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			takes, known, hasLatest := execShortCluster(arg)
			latest = latest || hasLatest
			i = r.skipValue(i, latest, takes, !known)
		default:
			r.operands(i, latest)

			return
		}
	}
}

// execShortCluster reads a short exec option cluster (-it, -uroot, -e) the
// way pflag does: the first value flag takes the rest as its value, or the
// next argument when it is last. known is false for a letter it does not
// know, whose arity is then unknown.
func execShortCluster(arg string) (takesNext, known, latest bool) {
	for j := 1; j < len(arg); j++ {
		switch {
		case strings.IndexByte(containerExecShortValueFlags, arg[j]) >= 0:
			return j == len(arg)-1, true, latest
		case strings.IndexByte(containerExecShortBoolFlags, arg[j]) < 0:
			return false, false, latest
		case arg[j] == 'l':
			latest = true
		}
	}

	return false, true, latest
}

// skipValue returns where reading goes on after the option at i: past its
// value when it surely takes one, and at the next argument otherwise. A
// value that may be absent or may be an option is also read the other way.
func (r *execReader) skipValue(i int, latest, takes, unknown bool) int {
	if i+1 >= len(r.args) || (!takes && !unknown) {
		return i
	}

	if takes && !strings.HasPrefix(r.args[i+1], "-") {
		return i + 1
	}

	r.fork(i+optionWithValue, latest)

	return i
}

// operands records the program after the container at i. With --latest
// podman takes no container, so the word at i is the program.
func (r *execReader) operands(i int, latest bool) {
	if i >= len(r.args) {
		return
	}

	if latest {
		r.program(i)

		return
	}

	if detail := leadingExpansion(r.args[i]); detail != "" {
		r.dynamic = detail

		return
	}

	r.program(i + 1)
}

// program records the program at p. A -- there is taken to end options,
// so the word after it is the program.
func (r *execReader) program(p int) {
	if p < len(r.args) && r.args[p] == endOfOptions {
		p++
	}

	if p < len(r.args) && !slices.Contains(r.programs, p) {
		r.programs = append(r.programs, p)
	}
}

// containerExecCommands returns the programs a container runner's exec
// runs, with their arguments: docker exec c git push runs git push, and
// docker exec c echo git push runs only echo. Known variables are
// substituted and split first, as for --entrypoint. replace reports that
// every reading placed exec, so these commands are all the runner runs;
// otherwise its arguments are still scanned as before.
func (w *astWalker) containerExecCommands(cmd Command) (cmds []Command, replace bool) {
	if !isContainerRunner(cmd.Name) {
		return nil, false
	}

	readings, complete := w.expandedArgs(cmd.Args)
	found, all := false, true
	exhausted, dynamic := !complete, ""

	for _, reading := range readings {
		r, ok := containerExecs(reading.args)
		if !ok {
			all = false

			continue
		}

		found = true

		if !w.spendArgs(cmd, r.readings*len(reading.args)) {
			return nil, true
		}

		exhausted = exhausted || r.exhausted
		if r.dynamic != "" {
			dynamic = r.dynamic
		}

		for _, p := range r.programs {
			child := childCommand(cmd, reading.args[p], reading.payload(cmd.Args, p))
			if !slices.ContainsFunc(cmds, child.sameCall) {
				cmds = append(cmds, child)
			}
		}
	}

	if !found {
		return nil, false
	}

	switch {
	case dynamic != "":
		w.opaqueExec(cmd, dynamic)
	case exhausted:
		w.opaqueExec(cmd, DetailEntrypointOptions)
	}

	return cmds, all
}

// opaqueExec records that the container or options of cmd's exec cannot
// be known.
func (w *astWalker) opaqueExec(cmd Command, detail string) {
	defer w.enter(cmd)()

	w.opaque(OpacityUnresolvedWord, ContainerExecOperation, detail)
}
