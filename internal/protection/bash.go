package protection

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// Violation is one way a shell command would change policy. Program is
// the program or redirect that does it, Target the word that names the
// file, and Command a klaudiush subcommand that changes policy (Match is
// then empty).
type Violation struct {
	Match
	Program string
	Target  string
	Command string
}

// varRef matches the ${NAME} form the parser renders variables in, with
// any operator after the name.
var varRef = regexp.MustCompile(`\$\{[^}]*\}`)

// wordBreaks split command text into words that may be paths: code
// passed to an interpreter, or a whole command line.
var wordBreaks = " \t\n\r\x00\"'`(),;|&<>=:$"

// commandCheck holds what one shell command check needs.
type commandCheck struct {
	git       func(dir string, args ...string) string
	set       *Set
	result    *parser.ParseResult
	scope     *parser.VarScope
	raw       string
	mentions  *bool
	seen      map[string]bool
	violation []Violation
}

// CheckCommand returns the ways a parsed shell command would change a
// protected file or klaudiush policy. raw is the command as written; a
// write whose target is only known when the command runs counts when raw
// names a protected path anywhere.
func (s *Set) CheckCommand(
	ctx context.Context,
	result *parser.ParseResult,
	raw string,
) []Violation {
	c := &commandCheck{
		git:    func(dir string, args ...string) string { return gitOutput(ctx, dir, args...) },
		set:    s,
		result: result,
		raw:    raw,
		seen:   make(map[string]bool),
	}

	for _, fw := range result.FileWrites {
		c.checkWrite(fw)
	}

	c.scope = nil

	if result.DynamicWrites > 0 && c.mentionsProtected() {
		c.add(Violation{Program: "redirect", Target: "$(...)", Match: c.firstMention()})
	}

	for _, cmd := range result.Commands {
		c.checkCommand(cmd)
	}

	return c.violation
}

func (c *commandCheck) add(v Violation) {
	key := v.Path + "\x00" + v.Command
	if c.seen[key] {
		return
	}

	c.seen[key] = true
	c.violation = append(c.violation, v)
}

func (c *commandCheck) checkWrite(fw parser.FileWrite) {
	if fw.TargetUnknown {
		c.checkUnknownTarget(fw)

		return
	}

	c.scope = fw.Vars
	dir := c.dir(fw.WorkingDirectory, fw.DirUnknown)
	target := fw.Path

	if fw.Dynamic {
		target = unknownPart + target
	}

	if target == "!" || target == "|" {
		target = unknownPart
	}

	program := strings.ToLower(fw.Operation.String())
	if m, ok := c.checkWord(target, dir, false, program); ok {
		c.add(Violation{Match: m, Program: program, Target: fw.Path})
	}
}

// checkUnknownTarget blocks a program that changes unnamed files below a
// directory holding protected files, such as unzip -d .claude. The working
// directory, project root and home stay allowed, as for any program that
// names them: extracting there is common, and the per-command checks still
// see protected names on the line.
func (c *commandCheck) checkUnknownTarget(fw parser.FileWrite) {
	if fw.Scope == "" {
		return
	}

	program := strings.ToLower(fw.Operation.String())
	if m, ok := c.checkWord(fw.Scope, c.set.workDir, true, program); ok {
		c.add(Violation{Match: m, Program: fw.Source, Target: fw.Scope})
	}
}

func (c *commandCheck) checkCommand(cmd parser.Command) {
	c.scope = cmd.Vars
	dir := c.dir(cmd.WorkingDirectory, cmd.DirUnknown)

	if isKlaudiush(cmd) || c.set.runsKlaudiushBinary(cmd, dir) {
		c.checkKlaudiush(cmd, dir)

		return
	}

	program := programName(cmd)
	if program == programGit {
		dir = gitDir(cmd.Args, dir)
	}

	c.checkRewrites(cmd, program, dir)

	if eff := commandEffect(cmd); eff != effectNone {
		c.checkCandidates(cmd, eff, program, dir)
		c.checkBinaryCopies(cmd, program, dir)
	}

	if program == programFind && commandEffect(cmd) != effectNone {
		if m, ok := c.checkFind(cmd, dir); ok {
			c.add(Violation{Match: m, Program: program, Target: m.Path})
		}
	}

	if commandEffect(cmd) == effectNone {
		return
	}

	unknownInput := readsArgsFromUnknownInput(cmd, program)

	if (cmd.Dynamic || unknownInput) && c.mentionsProtected() {
		c.add(Violation{Match: c.firstMention(), Program: program, Target: "$(...)"})

		return
	}

	if unknownInput && !readOnlyPrograms[launchedProgram(cmd.Args)] {
		if m, ok := c.listsProtectedNames(); ok {
			c.add(Violation{Match: m, Program: program, Target: m.Path})
		}
	}
}

// listsProtectedNames reports a bare glob in the command, such as * or .*,
// that expands to a protected name in the working directory: its output
// fed to xargs names protected files although no word in the command does.
func (c *commandCheck) listsProtectedNames() (Match, bool) {
	for _, token := range splitTokens(c.raw) {
		expanded := c.expand(token)
		if !hasGlobMeta(expanded) || meaningful(expanded) ||
			strings.Contains(expanded, unknownPart) {
			continue
		}

		if m, ok := c.checkPattern(expanded, c.set.workDir); ok {
			return m, true
		}
	}

	return Match{}, false
}

// readsArgsFromUnknownInput reports xargs or parallel reading arguments
// from input klaudiush could not reconstruct, such as printf output with
// NUL separators.
func readsArgsFromUnknownInput(cmd parser.Command, program string) bool {
	return (program == "xargs" || program == "parallel") && cmd.Stdin == ""
}

// checkBinaryCopies blocks commands that copy or link the klaudiush binary:
// a copy under another name runs policy commands klaudiush does not
// recognize by name.
func (c *commandCheck) checkBinaryCopies(cmd parser.Command, program, dir string) {
	if len(c.set.executables) == 0 {
		return
	}

	for _, arg := range cmd.Args {
		expanded := c.expand(arg)
		if strings.HasPrefix(arg, "-") || expanded == "" || hasGlobMeta(expanded) {
			continue
		}

		if c.set.isKlaudiushFile(c.set.absolute(expanded, dir)) {
			c.add(Violation{Program: program, Command: program + " " + arg})

			return
		}
	}
}

// checkRewrites checks git commands and patch, which change files they do
// not name.
func (c *commandCheck) checkRewrites(cmd parser.Command, program, dir string) {
	var (
		m  Match
		ok bool
	)

	switch program {
	case programGit:
		m, ok = c.checkGitRewrite(cmd, dir)
	case programPatch:
		m, ok = c.checkPatchCommand(cmd, dir)
	default:
		return
	}

	if ok {
		c.add(Violation{Match: m, Program: program, Target: strings.Join(cmd.Args, " ")})
	}
}

func (c *commandCheck) checkCandidates(cmd parser.Command, eff effect, program, dir string) {
	for _, cand := range candidates(cmd, eff, c.isDir(dir)) {
		if program == programGit {
			cand.word = c.gitPathspec(cand.word, dir)
		}

		if m, ok := c.checkWord(cand.word, dir, cand.tree, program); ok {
			c.add(Violation{Match: m, Program: program, Target: cand.word})
		}

		if cmd.Dynamic && strings.HasPrefix(cand.word, "/") {
			if m, ok := c.checkWord(unknownPart+cand.word, dir, cand.tree, program); ok {
				c.add(Violation{Match: m, Program: program, Target: cand.word})
			}
		}
	}
}

// checkKlaudiush blocks klaudiush commands that change policy and files
// a read-only one writes by flag, such as suggest --output.
func (c *commandCheck) checkKlaudiush(cmd parser.Command, dir string) {
	if sub, ok := policySubcommand(cmd); ok {
		c.add(Violation{Program: programKlaudiush, Command: sub})

		return
	}

	for _, path := range outputPaths(cmd.Args) {
		if m, ok := c.checkWord(path, dir, false, programKlaudiush); ok {
			c.add(Violation{Match: m, Program: programKlaudiush, Target: path})
		}
	}
}

// gitPathspec turns a pathspec relative to the repository root (":/x")
// into a path.
func (c *commandCheck) gitPathspec(word, dir string) string {
	if !strings.HasPrefix(word, ":/") {
		return word
	}

	rest, ok := strings.CutPrefix(word, ":/")
	if !ok || strings.HasPrefix(dir, unknownPart) {
		return word
	}

	top := strings.TrimSpace(c.git(dir, "rev-parse", "--show-toplevel"))
	if top == "" {
		return unknownPart + "/" + rest
	}

	return filepath.Join(top, rest)
}

// candidate is a word that may name a file a command changes; tree says
// the command changes what is below it too.
type candidate struct {
	word string
	tree bool
}

// candidates returns the words of cmd that may name a file it changes.
func candidates(cmd parser.Command, eff effect, isDir func(string) bool) []candidate {
	if eff == effectDest {
		tree := programName(cmd) == programDitto || copiesTrees(cmd.Args)

		dests := destinations(cmd.Args)
		srcs := sources(cmd.Args)
		words := make([]candidate, 0, len(dests)*(len(srcs)+1))

		for _, dest := range dests {
			words = append(words, candidate{word: dest, tree: tree})

			intoDir := strings.HasSuffix(dest, "/") || len(srcs) > 1 || hasGlobMeta(dest) ||
				slices.ContainsFunc(cmd.Args, isTargetOption) || isDir(dest)
			if !intoDir {
				continue
			}

			for _, src := range srcs {
				joined := strings.TrimSuffix(dest, "/") + "/" + filepath.Base(localPart(src))
				words = append(words, candidate{word: joined, tree: tree})
			}
		}

		return words
	}

	if programName(cmd) == programFind {
		return nil
	}

	var words []candidate

	for _, arg := range cmd.Args {
		for _, word := range argWords(arg) {
			words = append(words, candidate{word: word, tree: true})
		}
	}

	words = append(words, linkTargets(cmd)...)

	for _, word := range codeWords(cmd.Stdin) {
		words = append(words, candidate{word: word, tree: true})
	}

	if cmd.StdinFile != "" {
		words = append(words, candidate{word: cmd.StdinFile, tree: true})
	}

	return words
}

// minLinkOperands is the fewest operands of an ln that names a target.
const minLinkOperands = 2

// linkTargets returns the targets of ln as the link will resolve them: a
// relative target is relative to the directory the link is created in,
// which is the last operand itself when that is a directory.
func linkTargets(cmd parser.Command) []candidate {
	if programName(cmd) != "ln" {
		return nil
	}

	var operands []string

	for _, arg := range cmd.Args {
		if !strings.HasPrefix(arg, "-") {
			operands = append(operands, arg)
		}
	}

	if len(operands) < minLinkOperands {
		return nil
	}

	last := operands[len(operands)-1]
	words := make([]candidate, 0, minLinkOperands*(len(operands)-1))

	for _, target := range operands[:len(operands)-1] {
		if filepath.IsAbs(target) || strings.HasPrefix(target, "~") {
			continue
		}

		words = append(words,
			candidate{word: filepath.Join(filepath.Dir(last), target), tree: true},
			candidate{word: filepath.Join(last, target), tree: true},
		)
	}

	return words
}

// copiesTrees reports whether a copy is recursive, copies a directory's
// contents (a source ending in / or /.), or deletes extra files in the
// destination, so it changes what is below the destination.
func copiesTrees(args []string) bool {
	for _, src := range sources(args) {
		if strings.HasSuffix(src, "/") || strings.HasSuffix(src, "/.") {
			return true
		}
	}

	for _, arg := range args {
		switch {
		case arg == "--recursive", arg == "--archive", strings.HasPrefix(arg, "--delete"):
			return true
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.ContainsAny(arg[1:], "rRa"):
			return true
		}
	}

	return false
}

// argWords returns the words of one argument that may name a file: the
// argument, the value of an option=value, and for code or text the words
// inside it.
func argWords(arg string) []string {
	words := []string{}

	if !strings.HasPrefix(arg, "-") {
		words = append(words, arg)
	}

	if i := strings.Index(arg, "="); i >= 0 && !strings.ContainsAny(arg[:i], " \t\n") {
		words = append(words, arg[i+1:])
	}

	if strings.ContainsAny(arg, " \t\n\"'`();,") {
		words = append(words, codeWords(arg)...)
	}

	return words
}

// maxCodeTokens bounds how many words of program text are paired up.
const maxCodeTokens = 64

// codeWords returns the words of program text, and each relative word also
// joined to the directory of every other word: code such as
// os.symlink("settings.json", ".claude/settings.local.json") resolves the
// first name next to the second.
func codeWords(text string) []string {
	tokens := splitTokens(text)
	if len(tokens) > maxCodeTokens {
		return tokens
	}

	words := append([]string{}, tokens...)

	for _, base := range tokens {
		dir := filepath.Dir(base)
		if !strings.Contains(base, "/") || dir == "." {
			continue
		}

		for _, name := range tokens {
			if name != base && !filepath.IsAbs(name) && !strings.HasPrefix(name, "~") {
				words = append(words, filepath.Join(dir, name))
			}
		}
	}

	return words
}

func splitTokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(wordBreaks, r)
	})
}

// dir resolves a command's working directory against the hook's.
func (c *commandCheck) dir(workingDirectory string, unknown bool) string {
	if unknown {
		return unknownPart
	}

	if workingDirectory == "" {
		return c.set.workDir
	}

	expanded := c.expand(workingDirectory)
	if strings.Contains(expanded, unknownPart) {
		return c.set.workDir
	}

	return c.set.absolute(expanded, c.set.workDir)
}

// expand substitutes known variables and ~ into word. Unknown variables
// become unknownPart.
func (c *commandCheck) expand(word string) string {
	dynamic := func(ref string) string {
		if c.isDynamic(varName(ref)) {
			return unknownPart
		}

		return ref
	}

	expanded := varRef.ReplaceAllStringFunc(word, dynamic)
	expanded = c.expandVars(expanded)
	expanded = varRef.ReplaceAllStringFunc(expanded, dynamic)
	expanded = varRef.ReplaceAllStringFunc(expanded, func(ref string) string {
		name := varName(ref)
		if value, ok := c.set.lookupEnv(name); ok && value != "" && !strings.Contains(value, "${") {
			return value
		}

		return unknownPart
	})

	if expanded == "~" || strings.HasPrefix(expanded, "~/") {
		expanded = c.set.home + expanded[1:]
	}

	return expanded
}

// isDynamic reports whether name holds command output for the command
// being checked, or at the end of the line when no command is.
func (c *commandCheck) isDynamic(name string) bool {
	if c.scope != nil {
		return c.scope.IsDynamic(name)
	}

	return c.result.DynamicVars[name]
}

// expandVars substitutes the assignments the command being checked saw,
// or the final ones when no command is.
func (c *commandCheck) expandVars(word string) string {
	if c.scope != nil {
		return c.scope.ExpandVars(word)
	}

	return c.result.ExpandVars(word)
}

// varName returns the variable a ${NAME...} reference names.
func varName(ref string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(ref, "${"), "}")
	if i := strings.IndexFunc(name, func(r rune) bool {
		return r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	}); i >= 0 {
		return name[:i]
	}

	return name
}

// checkWord reports the protected path a word names. tree says the
// operation changes what is below a directory too; program decides
// whether the working directory and its ancestors count.
func (c *commandCheck) checkWord(word, dir string, tree bool, program string) (Match, bool) {
	expanded := c.expand(word)
	if expanded == "" {
		return Match{}, false
	}

	if strings.HasPrefix(dir, unknownPart) && !filepath.IsAbs(expanded) &&
		!strings.HasPrefix(expanded, unknownPart) {
		expanded = dir + "/" + expanded
	}

	if hasGlobMeta(expanded) {
		if meaningful(expanded) {
			return c.checkPattern(expanded, dir)
		}

		if strings.Contains(expanded, unknownPart) && c.mentionsProtected() {
			return c.firstMention(), true
		}

		return c.checkBareGlob(expanded, dir, program)
	}

	path := c.set.absolute(expanded, dir)

	if !tree || (c.set.broad(path) && !broadPrograms[program]) {
		return c.set.Check(path)
	}

	return c.set.CheckTree(path)
}

// checkBareGlob handles a pattern with no literal name, such as * or
// */*: it names everything in the directory, which matters only to a
// program that changes whole trees.
func (c *commandCheck) checkBareGlob(word, dir, program string) (Match, bool) {
	if strings.Contains(word, unknownPart) || !broadPrograms[program] {
		return Match{}, false
	}

	return c.checkPattern(word, dir)
}

// meaningful reports whether a word has a literal name in it, not only
// unknown parts, wildcards and separators.
func meaningful(word string) bool {
	return strings.ContainsFunc(word, func(r rune) bool {
		return !strings.ContainsRune("/*?.[]{}!~"+unknownPart, r)
	})
}

// broad reports whether path is the working directory, project root, home
// or one of their ancestors: naming one of these does not by itself point
// at policy files.
func (s *Set) broad(path string) bool {
	key := s.key(filepath.Clean(path))
	for _, base := range []string{s.workDir, s.projectRoot, s.home} {
		if isUnder(s.key(base), key) {
			return true
		}
	}

	return false
}

func (c *commandCheck) checkPattern(word, dir string) (Match, bool) {
	if !strings.HasPrefix(word, unknownPart) && !filepath.IsAbs(word) {
		word = strings.TrimSuffix(dir, "/") + "/" + word
	}

	word = filepath.Clean(word)

	key := c.set.key(filepath.ToSlash(word))

	expr, err := globRegexp(key)
	if err != nil {
		return Match{}, false
	}

	re, err := regexp.Compile("^" + expr + "$")
	if err != nil {
		return Match{}, false
	}

	return c.set.CheckPattern(re, literalDir(word), literalTail(word))
}

// literalDir returns the directory written before the first pattern
// character, or "" when the word starts with one.
func literalDir(word string) string {
	i := strings.IndexAny(word, "*?[{"+unknownPart)
	if i <= 0 {
		return ""
	}

	j := strings.LastIndex(word[:i], "/")
	if j <= 0 {
		return ""
	}

	return word[:j]
}

// literalTail returns the whole names written after the last pattern
// character.
func literalTail(word string) []string {
	i := strings.LastIndexAny(word, "*?]}"+unknownPart)
	if i < 0 {
		return nil
	}

	rest := word[i+1:]

	j := strings.Index(rest, "/")
	if j < 0 {
		return nil
	}

	return components(rest[j:])
}

// mentionsProtected reports whether the command text names a protected
// path anywhere, as an argument, inside quotes or in a loop list.
func (c *commandCheck) mentionsProtected() bool {
	if c.mentions == nil {
		found := false
		c.mentions = &found

		if _, ok := c.findMention(); ok {
			found = true
		}
	}

	return *c.mentions
}

func (c *commandCheck) firstMention() Match {
	m, _ := c.findMention()

	return m
}

func (c *commandCheck) findMention() (Match, bool) {
	for _, token := range splitTokens(c.raw) {
		expanded := c.expand(token)
		if !meaningful(expanded) {
			continue
		}

		if hasGlobMeta(expanded) {
			if m, ok := c.checkPattern(expanded, c.set.workDir); ok {
				return m, true
			}

			continue
		}

		path := c.set.absolute(expanded, c.set.workDir)
		if c.set.broad(path) {
			continue
		}

		if m, ok := c.set.CheckTree(path); ok {
			return m, true
		}
	}

	return Match{}, false
}

// findNameTests are find tests that match a file's name; findPathTests
// match the whole path find prints.
var (
	findNameTests = map[string]bool{"-name": true, "-iname": true}
	findPathTests = map[string]bool{
		"-path": true, "-ipath": true, "-wholename": true, "-iwholename": true,
	}
)

// checkFind reports a protected file a find that deletes or runs commands
// can reach: one below a start path whose name or path passes the -name or
// -path tests. Without such tests every file below counts.
func (c *commandCheck) checkFind(cmd parser.Command, dir string) (Match, bool) {
	starts, names, paths, opaque := findParts(cmd.Args)
	if len(names) == 0 && len(paths) == 0 || opaque || strings.HasPrefix(dir, unknownPart) {
		for _, start := range starts {
			if m, ok := c.checkWord(start, dir, true, programFind); ok {
				return m, true
			}
		}

		return Match{}, false
	}

	if m, ok := c.findNamesProtected(names); ok {
		return m, true
	}

	for _, start := range starts {
		root := c.set.absolute(c.expand(start), dir)
		rootKey := c.set.key(root)

		for _, e := range c.set.entries {
			if !isUnder(e.key, rootKey) || c.set.allowedKey(e.key) {
				continue
			}

			if c.findTestsMatch(e, rootKey, start, names, paths) {
				return Match{Path: e.path, Reason: e.reason}, true
			}
		}
	}

	return Match{}, false
}

// findNamesProtected reports a -name test that matches a name protected
// wherever it appears, such as settings.json or .klaudiush: find reaches
// copies in subdirectories the protected set does not list.
func (c *commandCheck) findNamesProtected(names []string) (Match, bool) {
	for _, name := range names {
		re := c.findRegexp(name)
		if re == nil {
			continue
		}

		for _, r := range c.set.rules {
			if r.kind == ruleSuffix && slices.ContainsFunc(r.comps, re.MatchString) {
				return Match{Path: strings.Join(r.names, "/"), Reason: r.reason}, true
			}
		}
	}

	return Match{}, false
}

func (c *commandCheck) findTestsMatch(e entry, rootKey, start string, names, paths []string) bool {
	rel := strings.TrimPrefix(strings.TrimPrefix(e.key, rootKey), "/")

	for _, name := range names {
		re := c.findRegexp(name)
		for _, comp := range components(rel) {
			if re != nil && re.MatchString(comp) {
				return true
			}
		}
	}

	for _, pattern := range paths {
		re := c.findRegexp(pattern)
		printed := c.set.key(strings.TrimSuffix(start, "/") + "/" + rel)

		if re != nil && re.MatchString(printed) {
			return true
		}
	}

	return false
}

func (c *commandCheck) findRegexp(pattern string) *regexp.Regexp {
	expr, err := globRegexp(c.set.key(c.expand(pattern)))
	if err != nil {
		return nil
	}

	re, err := regexp.Compile("(?i)^" + strings.ReplaceAll(expr, "[^/]", ".") + "$")
	if err != nil {
		return nil
	}

	return re
}

// findParts splits find arguments into start paths and the patterns of its
// name and path tests. opaque reports a test klaudiush cannot evaluate,
// such as -regex.
func findParts(args []string) ([]string, []string, []string, bool) {
	var starts, names, paths []string

	opaque := false
	inExpr := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !inExpr && (strings.HasPrefix(arg, "-") || arg == "(" || arg == "!") {
			inExpr = true
		}

		switch {
		case !inExpr:
			starts = append(starts, arg)
		case findNameTests[arg] && i+1 < len(args):
			names = append(names, args[i+1])
			i++
		case findPathTests[arg] && i+1 < len(args):
			paths = append(paths, args[i+1])
			i++
		case arg == "-regex" || arg == "-iregex" || arg == "-o" || arg == "-or" ||
			arg == "!" || arg == "-not":
			opaque = true
		}
	}

	if len(starts) == 0 {
		starts = []string{"."}
	}

	return starts, names, paths, opaque
}

// isDir returns a check for whether a word names an existing directory,
// into which a copy puts its sources.
func (c *commandCheck) isDir(dir string) func(string) bool {
	return func(word string) bool {
		expanded := c.expand(word)
		if strings.Contains(expanded, unknownPart) {
			return true
		}

		info, err := os.Stat(c.set.absolute(expanded, dir))

		return err == nil && info.IsDir()
	}
}
