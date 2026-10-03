package parser

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ParallelOperation names GNU parallel in an Opacity: the command line it
// runs, or the input that fills it in, comes from a word klaudiush cannot
// read.
const ParallelOperation = "parallel"

// DetailParallelInput is the fixed reason, set as Opacity.Detail, that
// parallel runs command lines read from input klaudiush cannot see.
const DetailParallelInput = "it is read from stdin or a file klaudiush cannot see"

// parallelPrograms are GNU parallel and the programs that run it (sem is
// parallel --semaphore, env_parallel and parset wrap it), each joining its
// command words and an input into a command line run by the shell.
var parallelPrograms = nameSet("parallel sem env_parallel parset")

// parsetProgram takes the names of the variables it sets before
// parallel's options.
const parsetProgram = "parset"

// unknownInput stands in for an input klaudiush cannot see. It is the path
// find -exec fills in, which a program or git word may not be.
const unknownInput = findPath

// maxParallelJobs bounds the command lines built from literal inputs. Past
// it every input is taken as unknown.
const maxParallelJobs = 16

// maxParallelStarts bounds the places parallel's command may start at.
const maxParallelStarts = 8

// parallelValueFlags are the long options that take the next argument as
// their value.
var parallelValueFlags = nameSet(`--arg-file --arg-sep --arg-file-sep --basefile --bf
	--block --block-size --colsep --delimiter --delay --env --halt --halt-on-error
	--header --joblog --jobs --load --max-args --max-chars --max-replace-args --memfree
	--memsuspend --nice --results --res --retries --return --rpl --sshlogin
	--sshloginfile --slf --tagstring --tag-string --ctagstring --tmpdir --tempdir
	--timeout --transferfile --tf --workdir --wd --ssh --filter --extensionreplace --er
	--basenamereplace --bnr --dirnamereplace --dnr --basenameextensionreplace --bner
	--seqreplace --slotreplace --profile --termseq --limit --recstart --recend
	--sqlmaster --sqlworker --sqlandworker --sql --template --compress-program
	--decompress-program --ssh-delay --sshdelay --group-by --minversion
	--process-slot-var --trc`)

// parallelBoolFlags are the long options known to stand alone; --replace,
// --eof and --max-lines take a value only when it is attached.
var parallelBoolFlags = nameSet(`--keep-order --quote --ungroup --verbose --tag --null
	--no-run-if-empty --xargs --dry-run --dryrun --pipe --pipe-part --pipepart --plus
	--progress --bar --eta --line-buffer --lb --linebuffer --group --shuf --tty --fg
	--bg --semaphore --nonall --onall --cat --fifo --tee --shebang --no-notice
	--will-cite --round-robin --files --files0 --compress --resume --resume-failed
	--retry-failed --cleanup --transfer --interactive --exit --plain --csv --tmux
	--tmuxpane --replace --eof --max-lines --shebang-wrap`)

// parallelStopFlags make parallel print something and run nothing.
var parallelStopFlags = nameSet(`--version --help --citation --bibtex --number-of-cpus
	--number-of-cores --number-of-sockets --number-of-threads --record-env --embed
	--shellquote --minversion`)

// parallelPipeFlags feed the input to the command's stdin, not its line.
var parallelPipeFlags = nameSet("--pipe --pipe-part --pipepart --cat --fifo")

// parallelCodeFlags take Perl code that parallel runs.
var parallelCodeFlags = nameSet("--rpl --filter")

// parallelInputFlags read the inputs from a file or a database.
var parallelInputFlags = nameSet("-a --arg-file --sqlworker --sqlandworker")

// parallelColumnFlags split each input into columns ({1}, {2}), which
// klaudiush does not model, so the inputs are taken as unknown.
var parallelColumnFlags = nameSet("-C --colsep --csv")

// parallelTagFlags take a string that may hold {= perl =}.
var parallelTagFlags = nameSet("--tagstring --tag-string --ctagstring")

// parallelReplaceFlags set the string that stands for an input or a part of
// it, keyed by the default string they replace.
var parallelReplaceFlags = map[string]string{
	"-I": findPath, "--replace": findPath, "-i": findPath,
	"--extensionreplace": roleNoExt, "--er": roleNoExt,
	"--basenamereplace": roleBase, "--bnr": roleBase,
	"--dirnamereplace": roleDir, "--dnr": roleDir,
	"--basenameextensionreplace": roleBaseNoExt, "--bner": roleBaseNoExt,
	"--seqreplace": "{#}", "--slotreplace": "{%}",
}

// parallelShortValues are the short options that take a value, attached or
// as the next argument.
const parallelShortValues = "aCdEIjJLnNPsS"

// parallelShortOptional are the short options that take a value only when
// it is attached (-i{}, -e, -l1).
const parallelShortOptional = "eil"

// parallelShortBools are the short options known to stand alone.
const parallelShortBools = "0ghkmpqrtuvVxX"

// roleNoExt, roleBase, roleDir and roleBaseNoExt are the replacement
// strings for an input without its extension, its last element, its
// directory, and its last element without the extension.
const (
	roleNoExt     = "{.}"
	roleBase      = "{/}"
	roleDir       = "{//}"
	roleBaseNoExt = "{/.}"
)

// pathRoles are the replacement strings for parts of an input, longest
// first, then the whole input.
var pathRoles = []string{roleBaseNoExt, roleDir, roleBase, roleNoExt, findPath}

// perlExpr matches a {= perl expression =} replacement string.
var perlExpr = regexp.MustCompile(`\{=(.*?)=\}`)

// positionalReplace matches {N}, {N.}, {N/}, {N//} and {N/.}.
var positionalReplace = regexp.MustCompile(`\{([1-9][0-9]?)(\.|/|//|/\.)?\}`)

// plusReplace matches the replacement strings --plus adds ({+/}, {..},
// {##}, ...).
var plusReplace = regexp.MustCompile(`\{[+./#%:][^{}\s]*\}`)

// parallelOptions are the options read before parallel's command. replace
// maps a default replacement string to the one -I and its kind set.
type parallelOptions struct {
	replace   map[string]string
	argSep    string
	fileSep   string
	scripts   []string
	code      []string
	starts    []int
	dynamic   string
	quote     bool
	pipe      bool
	plus      bool
	argFile   bool
	columns   bool
	stop      bool
	exhausted bool
}

// parallelScripts returns the command lines GNU parallel runs: its command
// words joined, with each input substituted for its replacement strings or
// appended, and run by the shell. Without command words the inputs are the
// command lines. A command word that is not literal could inject anything
// into the line, so it is opaque, as is a command line read from input
// klaudiush cannot see; an unknown input filled into the line stands as {},
// which is opaque where a program or git word is expected. Perl code in
// {= =}, --rpl and --filter is returned to be scanned like an interpreter's.
func (w *astWalker) parallelScripts(cmd Command) (scripts, code []string) {
	if cmd.Name == parsetProgram && len(cmd.Args) > 0 {
		cmd.Args = cmd.Args[1:]
	}

	opts := readParallelOptions(cmd.Args, cmd.mayShift)
	if opts.stop {
		return nil, nil
	}

	scripts, code = opts.scripts, opts.code

	detail := opts.dynamic
	if opts.exhausted && detail == "" {
		detail = DetailEntrypointOptions
	}

	for _, start := range opts.starts {
		if detail != "" {
			break
		}

		lines, lineCode, why := w.parallelLines(cmd, &opts, start)
		scripts, code = append(scripts, lines...), append(code, lineCode...)
		detail = why
	}

	if detail != "" {
		w.opaqueParallel(cmd, detail)

		return nil, nil
	}

	return scripts, code
}

// readParallelOptions reads parallel's options up to where its command may
// start. An option it does not know may take the next argument or not, so
// both places are kept.
func readParallelOptions(args []string, shifts func(string) bool) parallelOptions {
	opts := parallelOptions{replace: map[string]string{}, argSep: ":::", fileSep: "::::"}
	seen := make(map[int]bool)
	queue := []int{0}

	for len(queue) > 0 && opts.dynamic == "" {
		at := queue[0]
		queue = queue[1:]

		if seen[at] {
			continue
		}

		seen[at] = true

		if start, ok := opts.commandAt(args, at, shifts); ok {
			if !slices.Contains(opts.starts, start) {
				opts.starts = append(opts.starts, start)
			}

			continue
		}

		queue = append(queue, opts.next(args, at, shifts)...)
	}

	if len(opts.starts) > maxParallelStarts {
		opts.exhausted = true
	}

	return opts
}

// commandAt reports that parallel's command starts at or after at: at a
// word that is not an option, after --, or at an input separator (no
// command).
func (o *parallelOptions) commandAt(args []string, at int, shifts func(string) bool) (int, bool) {
	if at >= len(args) {
		return len(args), true
	}

	arg := args[at]

	switch {
	case arg == endOfOptions:
		return at + 1, true
	case o.separator(arg) || !strings.HasPrefix(arg, "-") || arg == "-":
		return at, true
	case optionNameDetail(arg) != "":
		o.dynamic = optionNameDetail(arg)

		return 0, true
	case shifts(arg):
		o.dynamic = DetailWordSplit

		return 0, true
	default:
		return 0, false
	}
}

// next reads the option at index at and returns where reading goes on.
func (o *parallelOptions) next(args []string, at int, shifts func(string) bool) []int {
	arg := args[at]
	name, value, attached := strings.Cut(arg, "=")

	takes, known := true, true

	switch {
	case parallelStopFlags[name]:
		o.stop = true
	case strings.HasPrefix(arg, "--"):
		takes = !attached && parallelValueFlags[name]
		known = attached || parallelValueFlags[name] || parallelBoolFlags[name]
	default:
		name, value, attached, takes, known = shortParallelOption(arg)
	}

	if takes && at+1 < len(args) {
		value, attached = args[at+1], true

		if shifts(value) {
			o.dynamic = DetailWordSplit

			return nil
		}
	}

	if attached || !takes {
		o.apply(name, value, attached)
	}

	switch {
	case !known:
		return []int{at + 1, at + optionWithValue}
	case takes:
		return []int{at + optionWithValue}
	default:
		return []int{at + 1}
	}
}

// shortParallelOption reads a short option cluster (-j4, -kq, -I {}): the
// first letter taking a value takes the rest, or the next argument when it
// is last. A letter it does not know leaves the arity unknown.
func shortParallelOption(arg string) (name, value string, attached, takes, known bool) {
	for j := 1; j < len(arg); j++ {
		letter := arg[j]
		name = "-" + string(letter)

		switch {
		case strings.IndexByte(parallelShortValues, letter) >= 0:
			if j == len(arg)-1 {
				return name, "", false, true, true
			}

			return name, arg[j+1:], true, false, true
		case strings.IndexByte(parallelShortOptional, letter) >= 0:
			return name, arg[j+1:], j < len(arg)-1, false, true
		case strings.IndexByte(parallelShortBools, letter) < 0:
			return name, "", false, false, false
		case letter == 'q':
			name = "--quote"
		}
	}

	return name, "", false, false, true
}

// apply records what an option changes about the command lines.
func (o *parallelOptions) apply(name, value string, attached bool) {
	if role, ok := parallelReplaceFlags[name]; ok {
		if !attached || value == "" {
			value = findPath
		}

		o.replace[role] = value
	}

	switch {
	case name == "--quote":
		o.quote = true
	case name == "--plus":
		o.plus = true
	case parallelPipeFlags[name]:
		o.pipe = true
	case parallelInputFlags[name]:
		o.argFile = true
	case parallelColumnFlags[name]:
		o.columns = true
	case name == "--arg-sep" && attached:
		o.argSep = value
	case name == "--arg-file-sep" && attached:
		o.fileSep = value
	case parallelCodeFlags[name]:
		o.code = append(o.code, value)
	case parallelTagFlags[name]:
		o.code = append(o.code, perlCode(value)...)
	case name == "--ssh" && attached:
		o.scripts = append(o.scripts, value)
	}
}

// separator reports a word that starts a list of inputs.
func (o *parallelOptions) separator(arg string) bool {
	return o.inputSeparator(arg) || o.fileSeparator(arg)
}

func (o *parallelOptions) inputSeparator(arg string) bool {
	return arg == o.argSep || arg == o.argSep+"+"
}

func (o *parallelOptions) fileSeparator(arg string) bool {
	return arg == o.fileSep || arg == o.fileSep+"+"
}

// parallelInput is one input: a literal value, or one klaudiush cannot see.
type parallelInput struct {
	value string
	known bool
}

// parallelLines returns the command lines parallel runs when its command
// starts at start, or why they cannot be known.
func (w *astWalker) parallelLines(
	cmd Command,
	opts *parallelOptions,
	start int,
) (lines, code []string, detail string) {
	end := start
	for end < len(cmd.Args) && !opts.separator(cmd.Args[end]) {
		end++
	}

	words, detail := w.parallelWords(cmd, cmd.Args[start:end])
	if detail != "" {
		return nil, nil, detail
	}

	sources := w.parallelSources(cmd, opts, end)

	if len(words) == 0 {
		return commandInputs(sources)
	}

	line := strings.Join(words, " ")
	if opts.quote {
		line = quoteArgs(words)
	}

	code = perlCode(line)

	for _, job := range parallelJobs(sources, opts.pipe) {
		lines = append(lines, opts.fill(line, job))
	}

	return lines, code, ""
}

// parallelWords returns parallel's command words with known variables
// substituted, or why one cannot be known. The words are joined and run by
// a shell, so any part of one that is not literal could add commands.
func (w *astWalker) parallelWords(cmd Command, args []string) ([]string, string) {
	words := make([]string, 0, len(args))

	for _, arg := range args {
		switch {
		case marked(arg) || bracesExpand(arg):
			return nil, DetailWordOutput
		case globWord(arg) && cmd.mayShift(arg):
			return nil, DetailWordSplit
		case HasUnresolvedVars(arg):
			expanded, ok := w.resolveWord(arg)
			if !ok {
				return nil, DetailWordVariable
			}

			arg = expanded
		}

		words = append(words, arg)
	}

	return words, ""
}

// parallelSources returns the input lists after the command: each ::: list,
// a :::: list of files, an --arg-file, or stdin when there is none.
func (w *astWalker) parallelSources(cmd Command, opts *parallelOptions, at int) [][]parallelInput {
	var sources [][]parallelInput

	if opts.argFile {
		sources = append(sources, []parallelInput{{}})
	}

	for at < len(cmd.Args) {
		files := opts.fileSeparator(cmd.Args[at])

		end := at + 1
		for end < len(cmd.Args) && !opts.separator(cmd.Args[end]) {
			end++
		}

		source := make([]parallelInput, 0, end-at-1)
		for _, arg := range cmd.Args[at+1 : end] {
			source = append(source, w.parallelInput(arg, files || opts.columns))
		}

		sources = append(sources, source)
		at = end
	}

	if len(sources) > 0 {
		return sources
	}

	if cmd.Stdin == "" || opts.columns {
		return [][]parallelInput{{{}}}
	}

	var source []parallelInput

	for line := range strings.SplitSeq(strings.TrimSpace(cmd.Stdin), "\n") {
		source = append(source, parallelInput{value: line, known: true})
	}

	return [][]parallelInput{source}
}

// parallelInput reads one input word.
func (w *astWalker) parallelInput(arg string, file bool) parallelInput {
	switch {
	case file || marked(arg) || bracesExpand(arg) || globWord(arg):
		return parallelInput{}
	case HasUnresolvedVars(arg):
		expanded, ok := w.resolveWord(arg)

		return parallelInput{value: expanded, known: ok}
	default:
		return parallelInput{value: arg, known: true}
	}
}

// commandInputs returns the command lines parallel runs when it has no
// command words: the inputs themselves.
func commandInputs(sources [][]parallelInput) (lines, code []string, detail string) {
	for _, job := range parallelJobs(sources, false) {
		values := make([]string, 0, len(job))

		for _, input := range job {
			if !input.known {
				return nil, nil, DetailParallelInput
			}

			values = append(values, input.value)
		}

		lines = append(lines, strings.Join(values, " "))
	}

	return lines, nil, ""
}

// parallelJobs returns the inputs of each job: one from each source, in
// every combination. Past maxParallelJobs, or when inputs go to stdin, one
// job of unknown inputs stands for all.
func parallelJobs(sources [][]parallelInput, pipe bool) [][]parallelInput {
	count := 1

	for _, source := range sources {
		count *= max(len(source), 1)
		if count > maxParallelJobs {
			break
		}
	}

	if pipe || count > maxParallelJobs {
		return [][]parallelInput{make([]parallelInput, len(sources))}
	}

	jobs := [][]parallelInput{{}}

	for _, source := range sources {
		if len(source) == 0 {
			continue
		}

		next := make([][]parallelInput, 0, len(jobs)*len(source))

		for _, job := range jobs {
			for _, input := range source {
				next = append(next, append(slices.Clone(job), input))
			}
		}

		jobs = next
	}

	return jobs
}

// fill puts a job's inputs into the command line: in place of each
// replacement string, or appended when there is none. Inputs are quoted,
// as parallel quotes them; an unknown one stands as {}.
func (o *parallelOptions) fill(line string, job []parallelInput) string {
	if o.pipe {
		return line
	}

	pattern := o.replacePattern()
	if !pattern.MatchString(line) {
		all := make([]string, 0, len(job))
		for _, input := range job {
			all = append(all, input.quoted(findPath))
		}

		return strings.TrimSpace(line + " " + strings.Join(all, " "))
	}

	whole := joinInputs(job)

	return pattern.ReplaceAllStringFunc(line, func(token string) string {
		return o.replaceToken(token, whole, job)
	})
}

// replacePattern matches the replacement strings in use, longest first.
func (o *parallelOptions) replacePattern() *regexp.Regexp {
	alternatives := []string{perlExpr.String(), positionalReplace.String()}

	for _, role := range append([]string{"{#}", "{%}"}, pathRoles...) {
		alternatives = append(alternatives, regexp.QuoteMeta(o.token(role)))
	}

	if o.plus {
		alternatives = append(alternatives, plusReplace.String())
	}

	slices.SortStableFunc(alternatives, func(a, b string) int { return len(b) - len(a) })

	return regexp.MustCompile(strings.Join(alternatives, "|"))
}

// token returns the string standing for role.
func (o *parallelOptions) token(role string) string {
	if custom, ok := o.replace[role]; ok {
		return custom
	}

	return role
}

// replaceToken returns what one replacement string becomes for a job.
func (o *parallelOptions) replaceToken(
	token string,
	whole parallelInput,
	job []parallelInput,
) string {
	if perlExpr.MatchString(token) {
		return unknownInput
	}

	if m := positionalReplace.FindStringSubmatch(token); m != nil && m[0] == token {
		n, err := strconv.Atoi(m[1])
		if err != nil || n > len(job) {
			return unknownInput
		}

		return job[n-1].quoted("{" + m[2] + "}")
	}

	if token == o.token(findPath) {
		return whole.value
	}

	for _, role := range pathRoles {
		if token == o.token(role) && len(job) == 1 {
			return job[0].quoted(role)
		}
	}

	if token == o.token("{#}") || token == o.token("{%}") {
		return "1"
	}

	return unknownInput
}

// quoted returns the input as parallel puts it in a command line, with the
// part of a path role names ({.}, {/}, {//}, {/.}).
func (in parallelInput) quoted(role string) string {
	if !in.known {
		return unknownInput
	}

	value := in.value

	switch role {
	case roleNoExt:
		value = strings.TrimSuffix(value, path.Ext(value))
	case roleBase:
		value = path.Base(value)
	case roleDir:
		value = path.Dir(value)
	case roleBaseNoExt:
		base := path.Base(value)
		value = strings.TrimSuffix(base, path.Ext(base))
	}

	return shellQuote(value)
}

// joinInputs is what {} stands for in a job: every input, each quoted on
// its own, joined by spaces.
func joinInputs(job []parallelInput) parallelInput {
	values := make([]string, 0, len(job))

	for _, input := range job {
		values = append(values, input.quoted(findPath))
	}

	return parallelInput{value: strings.Join(values, " "), known: true}
}

// perlCode returns the Perl expressions in {= =} replacement strings.
func perlCode(text string) []string {
	matches := perlExpr.FindAllStringSubmatch(text, -1)
	code := make([]string, 0, len(matches))

	for _, m := range matches {
		code = append(code, m[1])
	}

	return code
}

// opaqueParallel records that a command line parallel runs cannot be known.
func (w *astWalker) opaqueParallel(cmd Command, detail string) {
	defer w.enter(cmd)()

	w.opaque(OpacityUnresolvedWord, ParallelOperation, detail)
}
