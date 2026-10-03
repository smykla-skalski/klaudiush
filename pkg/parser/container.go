package parser

import (
	"encoding/json"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// EntrypointOperation names a container --entrypoint value in an Opacity.
const EntrypointOperation = "--entrypoint"

// DetailEntrypointOptions is the fixed reason, set as Opacity.Detail, that a
// container's --entrypoint is opaque because too many options before it may
// or may not take a value.
const DetailEntrypointOptions = "it follows more options of unknown arity than klaudiush reads"

// minEntrypointAbbrev is the shortest prefix of --entrypoint (--ent) that
// docker-compose v1 (docopt) and podman-compose (argparse) accept for it.
const minEntrypointAbbrev = len("--ent")

// optionWithValue is how many arguments an option and its value take.
const optionWithValue = 2

// maxContainerReadings bounds how many readings of one runner's options are
// tried, so ambiguous options or many run words cannot slow the hook.
const maxContainerReadings = 32

// maxContainerRunWords bounds how many run or create words one runner's
// arguments are read from.
const maxContainerRunWords = 8

// argsPerWorkUnit is how many container arguments read or copied cost one
// unit of the parse's work budget.
const argsPerWorkUnit = 64

// containerRunners start a container whose program --entrypoint replaces.
var containerRunners = nameSet(`docker podman podman-remote nerdctl nerdctl.lima finch
	docker-compose podman-compose container udocker`)

// containerRunWords are the subcommands that take --entrypoint and an image
// (docker run, docker container create, docker compose run).
var containerRunWords = nameSet("run create")

// containerValueFlags are the run and create options of docker, podman,
// nerdctl and compose that take the next argument as their value.
var containerValueFlags = nameSet(`--add-host --annotation --arch --attach --authfile
	--blkio-weight --blkio-weight-device --cap-add --cap-drop --cert-dir --cgroup-conf
	--cgroup-parent --cgroupns --cgroups --chrootdirs --cidfile --conmon-pidfile --cpu-count
	--cpu-percent --cpu-period --cpu-quota --cpu-rt-period --cpu-rt-runtime --cpu-shares
	--cpus --cpuset-cpus --cpuset-mems --creds --decryption-key --detach-keys --device
	--device-cgroup-rule --device-read-bps --device-read-iops --device-write-bps
	--device-write-iops --dns --dns-opt --dns-option --dns-search --domainname --env
	--env-file --env-from-file --env-merge --expose --gidmap --gpus --group-add --group-entry
	--health-cmd --health-interval --health-log-destination --health-max-log-count
	--health-max-log-size --health-on-failure --health-retries --health-start-interval
	--health-start-period --health-startup-cmd --health-startup-interval
	--health-startup-retries --health-startup-success --health-startup-timeout
	--health-timeout --hooks-dir --hostname --hosts-file --hostuser --image-volume --init-path
	--io-maxbandwidth --io-maxiops --ip --ip6 --ipc --isolation --kernel-memory --label
	--label-file --link --link-local-ip --log-driver --log-opt --mac-address --memory
	--memory-reservation --memory-swap --memory-swappiness --mount --name --net --net-alias
	--network --network-alias --oom-score-adj --os --passwd-entry --personality --pid
	--pidfile --pids-limit --platform --pod --pod-id-file --preserve-fd --preserve-fds
	--publish --pull --rdt-class --requires --restart --retry --retry-delay --runtime
	--sdnotify --seccomp-policy --secret --security-opt --shm-size --shm-size-systemd
	--stop-signal --stop-timeout --storage-opt --subgidname --subuidname --sysctl --systemd
	--timeout --tmpfs --tz --uidmap --ulimit --umask --unsetenv --user --userns --uts
	--variant --volume --volume-driver --volumes-from --workdir`)

// containerBoolFlags are the long run and create options known to stand
// alone. Any other option without a value attached is read both ways.
var containerBoolFlags = nameSet(`--build --detach --disable-content-trust --env-host --help
	--http-proxy --init --interactive --no-TTY --no-deps --no-healthcheck --no-hosts
	--oom-kill-disable --passwd --privileged --publish-all --quiet --quiet-build --quiet-pull
	--read-only --read-only-tmpfs --remove-orphans --replace --rm --rmi --rootfs
	--service-ports --sig-proxy --tls-verify --tty --use-aliases --use-api-socket`)

// containerShortValueFlags are the short run options that take a value
// (-e, -v, -w, ...). The rest (-d, -i, -t, -P, -q, -T) are boolean.
const containerShortValueFlags = "acehlmpuvw"

// containerRun is a container start that replaces the image's entrypoint:
// every --entrypoint value in order, and the arguments after the image.
type containerRun struct {
	entrypoints []string
	args        []string
}

func (run containerRun) equal(other containerRun) bool {
	return slices.Equal(run.entrypoints, other.entrypoints) && slices.Equal(run.args, other.args)
}

// runReader reads run or create options up to the image. Where an option
// may or may not take the next argument (an option it does not know, a
// value the parser dropped as an empty word, a value starting with -), it
// follows both readings, so the one the runner uses is among them. Past
// maxContainerReadings it stops and reports exhausted. A word in the place
// of an option or the image that comes from a variable, command output or a
// brace expansion may stand for any options, so it is reported as dynamic.
type runReader struct {
	args      []string
	runs      []containerRun
	readings  int
	exhausted bool
	dynamic   string
}

// containerRuns reads the --entrypoint runs among a container runner's
// arguments. Every run or create word is tried as the subcommand, so global
// options before it (docker --context x run, docker compose -f y run) need
// no list of their own.
func containerRuns(args []string) *runReader {
	r := &runReader{}

	if !slices.ContainsFunc(args, mentionsEntrypoint) {
		return r
	}

	starts := 0

	for i, arg := range args {
		if !containerRunWords[arg] {
			continue
		}

		if starts++; starts > maxContainerRunWords {
			r.exhausted = true

			break
		}

		r.args = args[i+1:]
		r.fork(0, nil)
	}

	return r
}

// tracked reports whether the runs need following or fail closed.
func (r *runReader) tracked() bool {
	return len(r.runs) > 0 || r.exhausted || r.dynamic != ""
}

// entrypointOption splits an argument naming --entrypoint, or a prefix of
// it at least as long as --ent, into its attached value, if any.
func entrypointOption(arg string) (value string, attached, ok bool) {
	name, value, attached := strings.Cut(arg, "=")

	ok = name == EntrypointOperation ||
		(len(name) >= minEntrypointAbbrev && strings.HasPrefix(EntrypointOperation, name))

	return value, attached, ok
}

// mentionsEntrypoint reports an argument that is or may expand to an
// --entrypoint option, so runs without one are not read at all.
func mentionsEntrypoint(arg string) bool {
	return strings.Contains(arg, EntrypointOperation[:minEntrypointAbbrev])
}

// read continues a reading at i with the entrypoints found so far.
func (r *runReader) read(i int, entrypoints []string) {
	for ; i < len(r.args); i++ {
		arg := r.args[i]

		if value, attached, ok := entrypointOption(arg); ok {
			i = r.entrypoint(i, value, attached, &entrypoints)

			continue
		}

		switch {
		case arg == endOfOptions:
			r.image(i+1, entrypoints)

			return
		case strings.HasPrefix(arg, "--"):
			unknown := !containerBoolFlags[arg] && !strings.Contains(arg, "=")
			i = r.skipValue(i, entrypoints, containerValueFlags[arg], unknown)
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			i = r.skipValue(i, entrypoints, shortClusterTakesNext(arg), false)
		case dynamicWord(arg) != "":
			r.dynamic = dynamicWord(arg)

			return
		default:
			r.image(i, entrypoints)

			return
		}
	}
}

// entrypoint records the --entrypoint value at i and returns where reading
// goes on. A next argument starting with - may be the value or the next
// option after a value the parser dropped (--entrypoint ""), so both are
// followed.
func (r *runReader) entrypoint(i int, value string, attached bool, entrypoints *[]string) int {
	switch {
	case attached:
		*entrypoints = append(slices.Clone(*entrypoints), value)

		return i
	case i+1 >= len(r.args):
		return len(r.args)
	case strings.HasPrefix(r.args[i+1], "-"):
		r.fork(i+optionWithValue, append(slices.Clone(*entrypoints), r.args[i+1]))
		*entrypoints = append(slices.Clone(*entrypoints), "")

		return i
	default:
		*entrypoints = append(slices.Clone(*entrypoints), r.args[i+1])

		return i + 1
	}
}

// skipValue returns where reading goes on after the option at i: past its
// value when it surely takes one, and at the next argument otherwise. A
// value that may be absent or may be an option is also read the other way.
func (r *runReader) skipValue(i int, entrypoints []string, takes, unknown bool) int {
	if i+1 >= len(r.args) || (!takes && !unknown) {
		return i
	}

	if takes && !strings.HasPrefix(r.args[i+1], "-") {
		return i + 1
	}

	r.fork(i+optionWithValue, entrypoints)

	return i
}

// fork follows a reading from i, within maxContainerReadings.
func (r *runReader) fork(i int, entrypoints []string) {
	if r.readings >= maxContainerReadings {
		r.exhausted = true

		return
	}

	r.readings++
	r.read(i, entrypoints)
}

// image records a run whose image is args[idx].
func (r *runReader) image(idx int, entrypoints []string) {
	if idx >= len(r.args) || len(entrypoints) == 0 {
		return
	}

	if detail := dynamicWord(r.args[idx]); detail != "" {
		r.dynamic = detail

		return
	}

	run := containerRun{entrypoints: entrypoints, args: r.args[idx+1:]}
	if !slices.ContainsFunc(r.runs, run.equal) {
		r.runs = append(r.runs, run)
	}
}

// dynamicWord says why a word may split into several, or returns "".
func dynamicWord(word string) string {
	switch {
	case HasUnresolvedVars(word):
		return DetailWordVariable
	case marked(word) || bracesExpand(word):
		return DetailWordOutput
	default:
		return ""
	}
}

// shortClusterTakesNext reports whether a short option cluster (-it, -e,
// -dp) ends in a value flag with no value attached, the way pflag reads it:
// the first value flag in the cluster takes the rest as its value.
func shortClusterTakesNext(arg string) bool {
	for j := 1; j < len(arg); j++ {
		if strings.IndexByte(containerShortValueFlags, arg[j]) >= 0 {
			return j == len(arg)-1
		}
	}

	return false
}

// entrypointForms returns the programs and leading arguments an entrypoint
// value can stand for: the value as one program (docker), a JSON array
// (podman) and its shell words (compose). Every form is followed, so the
// reading a runner really uses is always among them.
func entrypointForms(value string) [][]string {
	if value == "" {
		return nil
	}

	forms := [][]string{{value}}

	if array, ok := jsonArray(value); ok {
		forms = append(forms, array)
	}

	if words := shellWords(value); len(words) > 1 {
		forms = append(forms, words)
	}

	return forms
}

// jsonArray parses a podman JSON array entrypoint.
func jsonArray(value string) ([]string, bool) {
	var array []string

	err := json.Unmarshal([]byte(value), &array)

	return array, err == nil && len(array) > 0 && array[0] != ""
}

// shellWords splits a value into words the way a shell would, falling back
// to whitespace when it is not a plain literal command line.
func shellWords(value string) []string {
	file, err := syntax.NewParser().Parse(strings.NewReader(value), "")
	if err == nil && len(file.Stmts) == 1 {
		if call := callExprOf(file.Stmts[0]); call != nil && len(call.Assigns) == 0 {
			if words, ok := literalArgs(call.Args); ok {
				return words
			}
		}
	}

	return strings.Fields(value)
}

// mayRunContainers reports a program that may be a container runner: a
// known one (docker.exe too), or one named by a variable or command output.
func mayRunContainers(name string) bool {
	return containerRunners[strings.TrimSuffix(name, ".exe")] ||
		HasUnresolvedVars(name) || strings.Contains(name, unresolvedProgram)
}

// entrypointCommands returns the commands a container runner starts through
// --entrypoint, with the arguments after the image: docker run --entrypoint
// git alpine push runs git push whatever the image. Docker keeps the last
// --entrypoint and nerdctl joins them all, so both readings are followed.
// Known variables are substituted and split first, so X="--entrypoint git";
// docker run $X img push is seen too.
func (w *astWalker) entrypointCommands(cmd Command) []Command {
	if !mayRunContainers(cmd.Name) {
		return nil
	}

	args := w.expandedArgs(cmd.Args)
	r := containerRuns(args)

	if !w.spendArgs(cmd, r.readings*len(args)) {
		return nil
	}

	switch {
	case r.dynamic != "":
		w.opaqueEntrypoint(cmd, r.dynamic)
	case r.exhausted:
		w.opaqueEntrypoint(cmd, DetailEntrypointOptions)
	}

	var cmds []Command

	for _, run := range r.runs {
		var ok bool
		if cmds, ok = w.runCommands(cmd, run, cmds); !ok {
			break
		}
	}

	return cmds
}

// runCommands appends to cmds the commands one run starts, one for each
// reading of its entrypoints, and reports false once the work budget is gone.
func (w *astWalker) runCommands(cmd Command, run containerRun, cmds []Command) ([]Command, bool) {
	values := make([]string, 0, len(run.entrypoints))

	for _, raw := range run.entrypoints {
		if value, ok := w.resolveEntrypoint(cmd, raw); ok {
			values = append(values, value)
		}
	}

	candidates := values
	if len(values) > 1 {
		candidates = []string{values[len(values)-1], strings.Join(values, " ")}
	}

	for _, value := range candidates {
		for _, form := range entrypointForms(value) {
			child := childCommand(cmd, form[0], slices.Concat(form[1:], run.args))
			if slices.ContainsFunc(cmds, child.sameCall) {
				continue
			}

			if !w.spendArgs(cmd, len(child.Args)) {
				return cmds, false
			}

			cmds = append(cmds, child)
		}
	}

	return cmds, true
}

// spendArgs charges the work budget for handling args arguments, one unit
// per argsPerWorkUnit, so long argument lists copied into many entrypoint
// readings cannot slow the hook. Once the budget is gone it fails closed.
func (w *astWalker) spendArgs(cmd Command, args int) bool {
	for range args / argsPerWorkUnit {
		if !w.state.spend() {
			w.opaque(OpacityWorkBudget, safeName(cmd.Name), "")

			return false
		}
	}

	return true
}

// sameCall reports whether two commands run the same program and arguments.
func (c Command) sameCall(other Command) bool {
	return c.Name == other.Name && slices.Equal(c.Args, other.Args)
}

// expandedArgs substitutes the variables it can resolve in args, splitting
// each substituted argument into words.
func (w *astWalker) expandedArgs(args []string) []string {
	if !slices.ContainsFunc(args, HasUnresolvedVars) {
		return args
	}

	out := make([]string, 0, len(args))

	for _, arg := range args {
		if !HasUnresolvedVars(arg) {
			out = append(out, arg)

			continue
		}

		if expanded, ok := w.resolveWord(arg); ok && !marked(expanded) {
			out = append(out, strings.Fields(expanded)...)
		} else {
			out = append(out, arg)
		}
	}

	return out
}

// resolveEntrypoint checks an entrypoint value, recording as opaque one from
// an unknown variable, command output, a glob or a brace expansion.
func (w *astWalker) resolveEntrypoint(cmd Command, value string) (string, bool) {
	if _, ok := jsonArray(value); ok {
		return value, true
	}

	detail := commandWordDetail(value)
	if detail == "" {
		return value, true
	}

	w.opaqueEntrypoint(cmd, detail)

	return "", false
}

// opaqueEntrypoint records that cmd's --entrypoint cannot be known.
func (w *astWalker) opaqueEntrypoint(cmd Command, detail string) {
	defer w.enter(cmd)()

	w.opaque(OpacityUnresolvedWord, EntrypointOperation, detail)
}
