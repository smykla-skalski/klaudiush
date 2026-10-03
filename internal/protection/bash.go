package protection

import (
	"path/filepath"
	"regexp"
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
var wordBreaks = " \t\n\r\"'`()[]{},;|&<>=:$"

// commandCheck holds what one shell command check needs.
type commandCheck struct {
	set       *Set
	result    *parser.ParseResult
	raw       string
	mentions  *bool
	seen      map[string]bool
	violation []Violation
}

// CheckCommand returns the ways a parsed shell command would change a
// protected file or klaudiush policy. raw is the command as written; a
// write whose target is only known when the command runs counts when raw
// names a protected path anywhere.
func (s *Set) CheckCommand(result *parser.ParseResult, raw string) []Violation {
	c := &commandCheck{set: s, result: result, raw: raw, seen: make(map[string]bool)}

	for _, fw := range result.FileWrites {
		c.checkWrite(fw)
	}

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
	dir := c.dir(fw.WorkingDirectory)
	target := fw.Path

	if fw.Dynamic {
		target = unknownPart + target
	}

	program := strings.ToLower(fw.Operation.String())
	if m, ok := c.checkWord(target, dir, false, program); ok {
		c.add(Violation{Match: m, Program: program, Target: fw.Path})
	}
}

func (c *commandCheck) checkCommand(cmd parser.Command) {
	if sub, ok := PolicyCommand(cmd); ok {
		c.add(Violation{Program: "klaudiush", Command: sub})

		return
	}

	eff := commandEffect(cmd)
	if eff == effectNone {
		return
	}

	program := programName(cmd)
	dir := c.dir(cmd.WorkingDirectory)

	if program == programGit {
		dir = gitDir(cmd.Args, dir)
	}

	for _, cand := range candidates(cmd, eff) {
		if m, ok := c.checkWord(cand.word, dir, cand.tree, program); ok {
			c.add(Violation{Match: m, Program: program, Target: cand.word})
		}

		if cmd.Dynamic && strings.HasPrefix(cand.word, "/") {
			if m, ok := c.checkWord(unknownPart+cand.word, dir, cand.tree, program); ok {
				c.add(Violation{Match: m, Program: program, Target: cand.word})
			}
		}
	}

	if program == programFind {
		if m, ok := c.checkFind(cmd, dir); ok {
			c.add(Violation{Match: m, Program: program, Target: m.Path})
		}
	}

	if cmd.Dynamic && c.mentionsProtected() {
		c.add(Violation{Match: c.firstMention(), Program: program, Target: "$(...)"})
	}
}

// candidate is a word that may name a file a command changes; tree says
// the command changes what is below it too.
type candidate struct {
	word string
	tree bool
}

// candidates returns the words of cmd that may name a file it changes.
func candidates(cmd parser.Command, eff effect) []candidate {
	if eff == effectDest {
		tree := copiesTrees(cmd.Args)

		dests := destinations(cmd.Args)
		srcs := sources(cmd.Args)
		words := make([]candidate, 0, len(dests)*(len(srcs)+1))

		for _, dest := range dests {
			words = append(words, candidate{word: dest, tree: tree})

			for _, src := range srcs {
				joined := strings.TrimSuffix(dest, "/") + "/" + filepath.Base(src)
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

	for word := range strings.FieldsSeq(cmd.Stdin) {
		words = append(words, candidate{word: word, tree: true})
	}

	if cmd.StdinFile != "" {
		words = append(words, candidate{word: cmd.StdinFile, tree: true})
	}

	return words
}

// copiesTrees reports whether a copy is recursive or deletes extra files
// in the destination, so it changes what is below the destination.
func copiesTrees(args []string) bool {
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
		words = append(words, splitTokens(arg)...)
	}

	return words
}

func splitTokens(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return strings.ContainsRune(wordBreaks, r)
	})
}

// dir resolves a command's working directory against the hook's.
func (c *commandCheck) dir(workingDirectory string) string {
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
	expanded := c.result.ExpandVars(word)
	expanded = varRef.ReplaceAllStringFunc(expanded, func(ref string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(ref, "${"), "}")
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

// checkWord reports the protected path a word names. tree says the
// operation changes what is below a directory too; program decides
// whether the working directory and its ancestors count.
func (c *commandCheck) checkWord(word, dir string, tree bool, program string) (Match, bool) {
	expanded := c.expand(word)
	if expanded == "" {
		return Match{}, false
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
		if !meaningful(expanded) || hasGlobMeta(expanded) {
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
	if len(names) == 0 && len(paths) == 0 || opaque {
		for _, start := range starts {
			if m, ok := c.checkWord(start, dir, true, programFind); ok {
				return m, true
			}
		}

		return Match{}, false
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
