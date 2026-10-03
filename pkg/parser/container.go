package parser

import (
	"encoding/json"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// EntrypointOperation names a container --entrypoint value in an Opacity.
const EntrypointOperation = "--entrypoint"

// entrypointPrefix is the attached form of the entrypoint flag.
const entrypointPrefix = EntrypointOperation + "="

// containerRunners start a container whose program --entrypoint replaces.
var containerRunners = nameSet(`docker podman podman-remote nerdctl nerdctl.lima finch
	docker-compose podman-compose`)

// containerRunWords are the subcommands that take --entrypoint and an image
// (docker run, docker container create, docker compose run).
var containerRunWords = nameSet("run create")

// containerValueFlags are the run and create options of docker, podman,
// nerdctl and compose that take the next argument as their value. A boolean
// listed here would hide the image, so only flags known to take a value are
// listed; an unknown flag is read as boolean, which can only shift the image
// onto a value and fail closed.
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
	--health-timeout --hooks-dir --hostname --hostuser --image-volume --init-path
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

// containerShortValueFlags are the short run options that take a value
// (-e, -v, -w, ...). The rest (-d, -i, -t, -P, -q, -T) are boolean.
const containerShortValueFlags = "acehlmpuvw"

// containerRun is a container start that replaces the image's entrypoint:
// every --entrypoint value in order, and the arguments after the image.
type containerRun struct {
	entrypoints []string
	args        []string
}

// containerRuns returns the --entrypoint runs among a container runner's
// arguments. Every run or create word is tried as the subcommand, so global
// options before it (docker --context x run, docker compose -f y run) need
// no list of their own.
func containerRuns(args []string) []containerRun {
	var runs []containerRun

	for i, arg := range args {
		if !containerRunWords[arg] {
			continue
		}

		if run, ok := containerRunAt(args[i+1:]); ok {
			runs = append(runs, run)
		}
	}

	return runs
}

// containerRunAt reads the options of a run or create subcommand up to the
// image, keeping the --entrypoint values and the arguments after the image.
func containerRunAt(args []string) (containerRun, bool) {
	var run containerRun

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == endOfOptions:
			return run.withImage(args, i+1)
		case arg == EntrypointOperation:
			if i+1 < len(args) {
				run.entrypoints = append(run.entrypoints, args[i+1])
				i++
			}
		case strings.HasPrefix(arg, entrypointPrefix):
			run.entrypoints = append(run.entrypoints, strings.TrimPrefix(arg, entrypointPrefix))
		case strings.HasPrefix(arg, "--"):
			if containerValueFlags[arg] && takesNext(args, i) {
				i++
			}
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			if shortClusterTakesNext(arg) && takesNext(args, i) {
				i++
			}
		default:
			return run.withImage(args, i)
		}
	}

	return run, false
}

// withImage completes a run whose image is args[idx].
func (run containerRun) withImage(args []string, idx int) (containerRun, bool) {
	if idx >= len(args) || len(run.entrypoints) == 0 {
		return run, false
	}

	run.args = args[idx+1:]

	return run, true
}

// takesNext reports whether the value flag at i consumes the next argument.
// A next argument that is itself an option is read as an option: the parser
// drops an empty word (--name ""), and skipping such an option instead would
// hide an --entrypoint behind it.
func takesNext(args []string, i int) bool {
	return i+1 < len(args) && !strings.HasPrefix(args[i+1], "-")
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

	var array []string
	if json.Unmarshal([]byte(value), &array) == nil && len(array) > 0 && array[0] != "" {
		forms = append(forms, array)
	}

	if words := shellWords(value); len(words) > 1 {
		forms = append(forms, words)
	}

	return forms
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

// entrypointCommands returns the commands a container runner starts through
// --entrypoint, with the arguments after the image: docker run --entrypoint
// git alpine push runs git push whatever the image. Docker keeps the last
// --entrypoint and nerdctl joins them all, so both readings are followed.
func (w *astWalker) entrypointCommands(cmd Command) []Command {
	if !containerRunners[cmd.Name] {
		return nil
	}

	var cmds []Command

	for _, run := range containerRuns(cmd.Args) {
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
				cmds = append(cmds, childCommand(cmd, form[0], slices.Concat(form[1:], run.args)))
			}
		}
	}

	return cmds
}

// resolveEntrypoint substitutes known variables in an entrypoint value. One
// from an unknown variable or command output is recorded as opaque.
func (w *astWalker) resolveEntrypoint(cmd Command, value string) (string, bool) {
	detail := DetailWordOutput

	switch {
	case marked(value):
	case HasUnresolvedVars(value):
		expanded, ok := w.resolveWord(value)
		if ok && !marked(expanded) {
			return expanded, true
		}

		detail = DetailWordVariable
	default:
		return value, true
	}

	defer w.enter(cmd)()

	w.opaque(OpacityUnresolvedWord, EntrypointOperation, detail)

	return "", false
}
