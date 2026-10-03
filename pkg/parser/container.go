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

// minEntrypointAbbrev is the shortest prefix of --entrypoint (--e) that
// docker-compose v1 (docopt, whose run has no --env) accepts for it.
const minEntrypointAbbrev = len("--e")

// optionWithValue is how many arguments an option and its value take.
const optionWithValue = 2

// maxContainerReadings bounds how many readings of one runner's options are
// tried, so ambiguous options or many run words cannot slow the hook.
const maxContainerReadings = 32

// maxContainerRunWords bounds how many run or create words one runner's
// arguments are read from.
const maxContainerRunWords = 8

// maxEntrypoints bounds the --entrypoint options one reading keeps, so a
// long list of them cannot slow the hook.
const maxEntrypoints = 8

// maxSplitChoices bounds how many substituted arguments holding several
// words are each read both whole and split, in every combination.
const maxSplitChoices = 4

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
	--service-ports --sig-proxy --tls-verify --tty --use-aliases --use-api-socket
	--unsetenv-all --no-hostname --insecure-registry`)

// containerShortValueFlags are the short run options that take a value
// (-e, -v, -w, ...). The rest (-d, -i, -t, -P, -q, -T) are boolean.
const containerShortValueFlags = "acefhlmpuvwH"

// containerGroupWords are subcommands that come before run or create
// (docker compose run, docker container run). Read as an image, one shows
// that the run word before it was an option value (docker --context run
// compose run), so it does not end where run words are looked for.
var containerGroupWords = nameSet("compose container run create")

// containerRun is a container start that replaces the image's entrypoint:
// every --entrypoint value in order, the arguments after the image, and the
// image's index among the runner's arguments.
type containerRun struct {
	entrypoints []string
	args        []string
	image       int
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
// lastImage is the furthest image position any reading reached.
//
// With all set it reads every run, not only those with an --entrypoint, for
// the image each reading reaches (images). A word in the place of an option,
// an option's value or the image that shifts may stand for any options too.
type runReader struct {
	args      []string
	runs      []containerRun
	images    []containerImage
	shifts    func(string) bool
	readings  int
	offset    int
	lastImage int
	exhausted bool
	all       bool
	dynamic   string
}

// containerImage is where a reading of a run found the image, and whether
// an --entrypoint came before it.
type containerImage struct {
	index      int
	entrypoint bool
}

// containerRuns reads the --entrypoint runs among a container runner's
// arguments. A run or create word is tried as the subcommand until one is
// read up to its image: what follows the furthest image is the container's
// own command line, whose run words docker never reads. Words before it are
// all tried, so global options before the subcommand (docker --context x
// run, docker compose -f y run) need no list of their own.
func containerRuns(args []string) *runReader {
	r := &runReader{}

	if !slices.ContainsFunc(args, mentionsEntrypoint) {
		return r
	}

	starts, limit := 0, -1

	for i, arg := range args {
		if limit >= 0 && i > limit {
			break
		}

		if !containerRunWords[arg] {
			continue
		}

		if starts++; starts > maxContainerRunWords {
			r.exhausted = true

			break
		}

		r.args, r.offset, r.lastImage = args[i+1:], i+1, -1
		r.fork(0, nil)

		if r.lastImage >= 0 {
			limit = max(limit, i+1+r.lastImage)
		}
	}

	return r
}

// tracked reports whether the runs need following or fail closed.
func (r *runReader) tracked() bool {
	return len(r.runs) > 0 || r.exhausted || r.dynamic != ""
}

// merge adds what another reading of the same arguments found.
func (r *runReader) merge(other *runReader) {
	for _, run := range other.runs {
		if !slices.ContainsFunc(r.runs, run.equal) {
			r.runs = append(r.runs, run)
		}
	}

	r.exhausted = r.exhausted || other.exhausted

	if other.dynamic != "" {
		r.dynamic = other.dynamic
	}
}

// mayHideEntrypoint reports a run or create among args next to a variable
// that may hold an --entrypoint, which only the walker can resolve.
func mayHideEntrypoint(args []string) bool {
	return slices.ContainsFunc(args, HasUnresolvedVars) &&
		slices.ContainsFunc(args, func(arg string) bool { return containerRunWords[arg] })
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
	_, _, ok := entrypointOption(arg)

	return ok || globsOptions(arg) || (dynamicWord(arg) != "" &&
		strings.Contains(arg, EntrypointOperation[:minEntrypointAbbrev]))
}

// globsOptions reports a glob that may match files named like options
// (*rm, --entrypoin*): one starting with - or with the glob itself. Whether
// it was quoted is not known, so it is taken as unquoted.
func globsOptions(arg string) bool {
	at := strings.IndexAny(arg, globChars)

	return at == 0 || (at > 0 && strings.HasPrefix(arg, "-"))
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
		case r.all && strings.HasPrefix(arg, "-") && optionNameDetail(arg) != "":
			r.dynamic = optionNameDetail(arg)

			return
		case globsOptions(arg):
			r.dynamic = DetailWordOutput

			return
		case r.all && strings.HasPrefix(arg, "-") && r.shifts(arg):
			r.dynamic = DetailWordSplit

			return
		case strings.HasPrefix(arg, "--"):
			unknown := !containerBoolFlags[arg] && !strings.Contains(arg, "=")
			i = r.skipValue(i, entrypoints, containerValueFlags[arg], unknown)
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			i = r.skipValue(i, entrypoints, shortClusterTakesNext(arg), false)
		case leadingExpansion(arg) != "":
			r.dynamic = leadingExpansion(arg)

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
	if len(*entrypoints) >= maxEntrypoints {
		r.exhausted = true

		return len(r.args)
	}

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

	if r.all && r.shifts(r.args[i+1]) {
		r.dynamic = DetailWordSplit

		return len(r.args)
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
	if idx >= len(r.args) {
		return
	}

	if !containerGroupWords[r.args[idx]] {
		r.lastImage = max(r.lastImage, idx)
	}

	if r.all {
		r.payloadImage(idx, len(entrypoints) > 0)

		return
	}

	if len(entrypoints) == 0 {
		return
	}

	if detail := leadingExpansion(r.args[idx]); detail != "" {
		r.dynamic = detail

		return
	}

	run := containerRun{entrypoints: entrypoints, args: r.args[idx+1:], image: r.offset + idx}
	if !slices.ContainsFunc(r.runs, run.equal) {
		r.runs = append(r.runs, run)
	}
}

// leadingExpansion says why a word in the place of an option or the image
// may stand for options, or returns "". A word that starts with a literal
// (img:$TAG) is an image whatever the variable holds; command output is
// rendered after the literal text, so it is never trusted.
func leadingExpansion(word string) string {
	switch {
	case strings.HasPrefix(word, "${"):
		return DetailWordVariable
	case marked(word) || bracesExpand(word) || globsOptions(word):
		return DetailWordOutput
	default:
		return ""
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

	if words := shellWords(value); len(words) > 0 && words[0] != "" &&
		!slices.Equal(words, forms[0]) {
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
	return isContainerRunner(name) ||
		HasUnresolvedVars(name) || strings.Contains(name, unresolvedProgram)
}

// isContainerRunner reports a known container runner (docker.exe too). Only
// these are looked for among another command's arguments: a variable there
// is far more often data than a runner, and checking each would cost a pass
// over the rest of the arguments.
func isContainerRunner(name string) bool {
	return containerRunners[strings.TrimSuffix(name, ".exe")]
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

	r := &runReader{}

	readings, complete := w.expandedArgs(cmd.Args)
	if !complete && slices.ContainsFunc(readings[1].args, mentionsEntrypoint) {
		r.exhausted = true
	}

	for _, reading := range readings {
		read := containerRuns(reading.args)
		if !w.spendArgs(cmd, read.readings*len(reading.args)) {
			return nil
		}

		for i := range read.runs {
			read.runs[i].args = reading.payload(cmd.Args, read.runs[i].image)
		}

		r.merge(read)
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

// expandedArgs substitutes the variables it can resolve in args. Whether an
// argument was quoted is not known, so each substituted argument that holds
// several words is read both whole and split, in every combination. Past
// maxSplitChoices such arguments it returns only the all-whole and all-split
// readings and reports false.
func (w *astWalker) expandedArgs(args []string) ([]argReading, bool) {
	if !slices.ContainsFunc(args, HasUnresolvedVars) {
		origins := make([]int, len(args))
		for i := range origins {
			origins[i] = i
		}

		return []argReading{{args: args, origins: origins}}, true
	}

	choices := make([][][]string, 0, len(args))
	splits := 0

	for _, arg := range args {
		expanded, ok := w.resolveWord(arg)
		if !HasUnresolvedVars(arg) || !ok || marked(expanded) {
			choices = append(choices, [][]string{{arg}})

			continue
		}

		fields := strings.Fields(expanded)
		if len(fields) == 1 && fields[0] == expanded {
			choices = append(choices, [][]string{fields})

			continue
		}

		choices = append(choices, [][]string{{expanded}, fields})
		splits++
	}

	if splits > maxSplitChoices {
		return []argReading{pickChoices(choices, 0, false), pickChoices(choices, 0, true)}, false
	}

	readings := make([]argReading, 0, 1<<splits)
	for mask := range 1 << splits {
		readings = append(readings, pickChoices(choices, mask, false))
	}

	return readings, true
}

// argReading is one reading of a runner's arguments with variables
// substituted, and for each word the index of the argument it came from.
type argReading struct {
	args    []string
	origins []int
}

// payload returns the container's command line after the image at index
// image: the rest of the image's own expansion, then the original arguments.
// Those are not substituted here, since the outer shell leaves a quoted
// script ('git ${SUB}') for the container's shell to expand.
func (a argReading) payload(original []string, image int) []string {
	origin := a.origins[image]

	end := image + 1
	for end < len(a.args) && a.origins[end] == origin {
		end++
	}

	return slices.Concat(a.args[image+1:end], original[origin+1:])
}

// pickChoices builds one reading of args: bit n of mask, or allSplit, picks
// the split form of the nth argument that has one.
func pickChoices(choices [][][]string, mask int, allSplit bool) argReading {
	var (
		out argReading
		bit int
	)

	for origin, options := range choices {
		pick := options[0]

		if len(options) > 1 {
			if allSplit || mask&(1<<bit) != 0 {
				pick = options[1]
			}

			bit++
		}

		out.args = append(out.args, pick...)

		for range pick {
			out.origins = append(out.origins, origin)
		}
	}

	return out
}

// resolveEntrypoint checks an entrypoint value, recording as opaque one from
// an unknown variable, command output, a glob or a brace expansion. In a
// JSON array only expansions count: its brackets are not a glob.
func (w *astWalker) resolveEntrypoint(cmd Command, value string) (string, bool) {
	detail := commandWordDetail(value)

	if array, ok := jsonArray(value); ok {
		detail = ""

		for _, element := range array {
			if d := dynamicWord(element); d != "" {
				detail = d
			}
		}
	}

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
