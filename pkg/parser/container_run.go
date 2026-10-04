package parser

import (
	"slices"
	"strings"
)

// ContainerRunOperation names, in an Opacity, a container runner's
// subcommand, a run option or the image that comes from a word klaudiush
// cannot read, so the program the container runs is unknown.
const ContainerRunOperation = "container run"

// payloadImage records the image a reading of a run reached at idx. An image
// from a variable or command output may be an option, and one that may
// split may carry the container's command line, so either is dynamic.
func (r *runReader) payloadImage(idx int, entrypoint bool) {
	word := r.args[idx]

	if detail := leadingExpansion(word); detail != "" && !r.tagged(word) {
		r.dynamic = detail

		return
	}

	if r.shifts(word) {
		r.dynamic = DetailWordSplit

		return
	}

	image := containerImage{index: r.offset + idx, entrypoint: entrypoint}
	if !slices.Contains(r.images, image) {
		r.images = append(r.images, image)
	}
}

// runSubcommand returns the index of a container runner's run or create
// subcommand after its global options and the container or compose group,
// or -1 when another subcommand comes first. A word in the place of the
// subcommand or a global option that comes from a variable or command
// output, or a value that may split, could be run with any options, so it
// is reported as dynamic. A word after an option it does not know is read
// as that option's value.
func runSubcommand(args []string, shifts func(string) bool) (int, string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case containerRunWords[arg]:
			return i, ""
		case containerExecGroups[arg]:
			continue
		case arg == endOfOptions:
			return -1, ""
		case leadingExpansion(arg) != "":
			return -1, leadingExpansion(arg)
		case !strings.HasPrefix(arg, "-") || arg == "-":
			return -1, ""
		case optionNameDetail(arg) != "":
			return -1, optionNameDetail(arg)
		case shifts(arg):
			return -1, DetailWordSplit
		}

		takes, known := globalOption(arg)
		if !takes || i+1 >= len(args) {
			continue
		}

		if shifts(args[i+1]) {
			return -1, DetailWordSplit
		}

		if detail := leadingExpansion(args[i+1]); detail != "" && !known {
			return -1, detail
		}

		if !containerRunWords[args[i+1]] {
			i++
		}
	}

	return -1, ""
}

// optionNameDetail says why an option's name, the part before any =, is
// not literal, or returns "". Even quoted ("--$X") it may name any option,
// --entrypoint included; only a value after = may be an expansion.
func optionNameDetail(arg string) string {
	name, _, _ := strings.Cut(arg, "=")

	return dynamicWord(name)
}

// globalOption reports whether a global option takes, or may take, the next
// argument as its value, and whether klaudiush knows that it does.
func globalOption(arg string) (takes, known bool) {
	if strings.HasPrefix(arg, "--") {
		if strings.Contains(arg, "=") || containerGlobalBoolFlags[arg] {
			return false, true
		}

		return true, containerGlobalValueFlags[arg]
	}

	takes, known = globalShortCluster(arg)

	return takes || !known, known
}

// containerRunCommands returns the programs a container runner's run or
// create starts without an --entrypoint when the program word, or a word
// before it, is not literal: docker run img $X runs whatever X holds, and
// X="img git push"; docker run $X runs git push. A literal program after a
// literal image is found by scanning the arguments, as before. A run whose
// subcommand, options or image come from words klaudiush cannot read, or
// whose options it cannot read up to the image, is opaque.
func (w *astWalker) containerRunCommands(cmd Command) []Command {
	if !isContainerRunner(cmd.Name) || !slices.ContainsFunc(cmd.Args, mayBeDynamic) {
		return nil
	}

	readings, complete := w.expandedArgs(cmd.Args)
	exhausted, dynamic := !complete, ""

	var cmds []Command

	for _, reading := range readings {
		r := readRuns(reading.args, cmd)

		if !w.spendArgs(cmd, r.readings*len(reading.args)) {
			return nil
		}

		exhausted = exhausted || r.exhausted
		if r.dynamic != "" {
			dynamic = r.dynamic
		}

		for _, image := range r.images {
			if child, ok := payloadCommand(cmd, reading, image); ok &&
				!slices.ContainsFunc(cmds, child.sameCall) {
				cmds = append(cmds, child)
			}
		}
	}

	switch {
	case dynamic != "":
		w.opaqueRun(cmd, dynamic)
	case exhausted:
		w.opaqueRun(cmd, DetailEntrypointOptions)
	}

	return cmds
}

// readFrom reads the run that starts at or after from, and returns where
// the next one may start: an image that is itself a run, create, compose
// or container word may have been an option's value (docker --context run
// -D run img), so the subcommand is looked for again from there. It returns
// -1 when no run is left to read.
func (r *runReader) readFrom(args []string, from int) int {
	at, detail := runSubcommand(args[from:], r.shifts)
	if detail != "" {
		r.dynamic = detail

		return -1
	}

	if at < 0 {
		return -1
	}

	start := from + at + 1
	known := len(r.images)
	r.args, r.offset = args[start:], start
	r.fork(0, nil)

	for _, image := range r.images[known:] {
		if containerGroupWords[args[image.index]] && image.index > from {
			return image.index
		}
	}

	return -1
}

// payloadCommand returns the program a run without an --entrypoint starts
// after the image, when it or the image's own expansion is not literal.
func payloadCommand(cmd Command, reading argReading, image containerImage) (Command, bool) {
	if image.entrypoint {
		return Command{}, false
	}

	payload := reading.payload(cmd.Args, image.index)
	if len(payload) == 0 {
		return Command{}, false
	}

	next := image.index + 1
	fromImage := next < len(reading.args) &&
		reading.origins[next] == reading.origins[image.index]

	if !fromImage && !mayBeDynamic(payload[0]) {
		return Command{}, false
	}

	return childCommand(cmd, payload[0], payload[1:]), true
}

// readRuns reads every run among a reading of a runner's arguments, up to
// maxContainerRunWords of them.
func readRuns(args []string, cmd Command) *runReader {
	r := &runReader{all: true, shifts: cmd.mayShift, tagged: cmd.tagged}

	for from, scans := 0, 0; from >= 0 && r.dynamic == ""; scans++ {
		if scans >= maxContainerRunWords {
			r.exhausted = true

			break
		}

		from = r.readFrom(args, from)
	}

	return r
}

// opaqueRun records that the program cmd's run starts cannot be known.
func (w *astWalker) opaqueRun(cmd Command, detail string) {
	defer w.enter(cmd)()

	w.opaque(OpacityUnresolvedWord, ContainerRunOperation, detail)
}
