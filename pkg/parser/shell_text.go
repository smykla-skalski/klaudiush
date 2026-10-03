package parser

import (
	"fmt"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Gaps name what the shell builds into a word that the parser cannot see.
// Each completes "it uses ...".
const (
	GapCommandOutput   = "command output ($(...) or backticks) other than a cat heredoc"
	GapProcSubst       = "a process substitution"
	GapArithmetic      = "an arithmetic expansion"
	GapGlob            = "an unquoted glob, brace or ~ pattern"
	GapCatDefined      = "a cat heredoc while cat is an alias or function on the line"
	GapAmbiguous       = "an argument written two ways that render alike"
	GapUnknownPart     = "shell syntax klaudiush does not render"
	GapReferenceFormat = "the reference %s, which klaudiush cannot resolve"
	gapOperatorFormat  = "the parameter expansion %s, whose value klaudiush does not compute"
	gapUnsetFormat     = "$%s, which is not set on the line or in the environment"
	gapUntrustedFmt    = "$%s, whose value klaudiush cannot know " +
		"(command output, a loop, read, or a change it cannot follow)"
	gapNestedFormat = "$%s, whose value refers to other variables"
	gapSplitFormat  = "$%s unquoted, which the shell splits or globs"
)

// heredocEscapable lists what a backslash escapes in an unquoted heredoc.
const heredocEscapable = "$`\\\n"

// unquotedSplit lists what makes an unquoted expansion split or glob.
const unquotedSplit = " \t\n" + globChars

// TextPart is one piece of a word or heredoc body as the shell builds it:
// literal Text, a reference to the variable Var (whose value fills Text once
// resolved), or a Gap saying why the parser cannot know the piece. unquoted
// marks a variable the shell splits and globs, and cat a cat heredoc
// substitution, which a same-line alias or function named cat would change.
type TextPart struct {
	Text string
	Var  string
	Gap  string

	resolved bool
	unquoted bool
	cat      bool
}

// ShellText is a word or heredoc body split into the pieces the shell builds
// it from, so a consumer can tell literal text from an expansion even when
// both render as "${NAME}".
type ShellText struct {
	Parts []TextPart
}

// Value returns the text the shell builds, or the first gap that keeps it
// from being known.
func (t ShellText) Value() (value, gap string) {
	var b strings.Builder

	for _, part := range t.Parts {
		switch {
		case part.Gap != "":
			return "", part.Gap
		case part.Var != "" && !part.resolved:
			return "", fmt.Sprintf(gapUntrustedFmt, part.Var)
		}

		b.WriteString(part.Text)
	}

	return b.String(), ""
}

// TrimPrefix removes literal text from the start, as a flag glued to its
// value ("-mtext", "--message=text") needs. It reports false when the
// prefix is not literal text at the start.
func (t ShellText) TrimPrefix(prefix string) (ShellText, bool) {
	parts := slices.Clone(t.Parts)

	for prefix != "" {
		if len(parts) == 0 || !parts[0].literal() {
			return ShellText{}, false
		}

		text := parts[0].Text

		switch {
		case strings.HasPrefix(text, prefix):
			parts[0].Text = text[len(prefix):]
			prefix = ""
		case strings.HasPrefix(prefix, text):
			prefix = prefix[len(text):]
			parts = parts[1:]
		default:
			return ShellText{}, false
		}
	}

	return ShellText{Parts: parts}, true
}

func (p TextPart) literal() bool {
	return p.Gap == "" && p.Var == "" && !p.cat
}

func (t *ShellText) text(s string) {
	t.Parts = append(t.Parts, TextPart{Text: s})
}

func (t *ShellText) gap(gap string) {
	t.Parts = append(t.Parts, TextPart{Gap: gap})
}

// textMode is the quoting context of a word part.
type textMode int

const (
	modeUnquoted textMode = iota
	modeDouble
	modeHeredoc
	modeHereString
)

// wordText splits an argument word into its pieces.
func wordText(word *syntax.Word) ShellText {
	var t ShellText

	if word != nil {
		t.addParts(word.Parts, modeUnquoted)
	}

	return t
}

func (t *ShellText) addParts(parts []syntax.WordPart, mode textMode) {
	for i, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			t.addLit(p.Value, mode, i == 0)
		case *syntax.SglQuoted:
			if p.Dollar {
				t.text(decodeANSIC(p.Value))
			} else {
				t.text(p.Value)
			}
		case *syntax.DblQuoted:
			t.addParts(p.Parts, modeDouble)
		case *syntax.ParamExp:
			t.addParam(p, mode == modeUnquoted)
		case *syntax.CmdSubst:
			t.addCmdSubst(p)
		case *syntax.ArithmExp:
			t.gap(GapArithmetic)
		case *syntax.ProcSubst:
			t.gap(GapProcSubst)
		case *syntax.ExtGlob:
			t.gap(GapGlob)
		default:
			t.gap(GapUnknownPart)
		}
	}
}

func (t *ShellText) addLit(value string, mode textMode, first bool) {
	switch mode {
	case modeUnquoted, modeHereString:
		if mode == modeUnquoted && (unescapedGlob(value) || bracesExpand(value)) ||
			first && strings.HasPrefix(value, "~") {
			t.gap(GapGlob)

			return
		}

		t.text(removeEscapes(value, allEscapable))
	case modeDouble:
		t.text(removeEscapes(value, doubleQuoteEscapable))
	case modeHeredoc:
		t.text(removeEscapes(value, heredocEscapable))
	}
}

// unescapedGlob reports a glob character a backslash does not escape.
func unescapedGlob(value string) bool {
	for i := 0; i < len(value); i++ {
		switch {
		case value[i] == '\\':
			i++
		case strings.IndexByte(globChars, value[i]) >= 0:
			return true
		}
	}

	return false
}

func (t *ShellText) addParam(pe *syntax.ParamExp, unquoted bool) {
	rendered := paramExpToString(pe)
	if pe == nil || pe.Param == nil || rendered != "${"+pe.Param.Value+"}" {
		t.gap(fmt.Sprintf(gapOperatorFormat, rendered))

		return
	}

	t.Parts = append(t.Parts, TextPart{Var: pe.Param.Value, unquoted: unquoted})
}

// addCmdSubst adds what a command substitution outputs: the body of a
// "$(cat <<EOF ... EOF)" without its trailing newlines, else a gap.
func (t *ShellText) addCmdSubst(cs *syntax.CmdSubst) {
	redir, ok := catHeredoc(cs)
	if !ok {
		t.gap(GapCommandOutput)

		return
	}

	body := heredocText(redir)
	body.trimTrailingNewlines()

	t.Parts = append(t.Parts, TextPart{cat: true})
	t.Parts = append(t.Parts, body.Parts...)
}

// catHeredoc returns the heredoc of a substitution that only copies it:
// one plain cat (or cat -) with a single heredoc or here-string.
func catHeredoc(cs *syntax.CmdSubst) (*syntax.Redirect, bool) {
	if cs == nil || len(cs.Stmts) != 1 {
		return nil, false
	}

	stmt := cs.Stmts[0]
	call := callExprOf(stmt)

	if call == nil || stmt.Negated || stmt.Background || stmt.Coprocess ||
		len(call.Assigns) > 0 || len(stmt.Redirs) != 1 || !copiesStdinVerbatim(call) {
		return nil, false
	}

	switch redir := stmt.Redirs[0]; redir.Op {
	case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		return redir, true
	default:
		return nil, false
	}
}

// heredocText splits what a heredoc or here-string feeds to stdin. A quoted
// delimiter keeps the body literal.
func heredocText(redir *syntax.Redirect) ShellText {
	var t ShellText

	switch {
	case redir.Op == syntax.WordHdoc:
		if redir.Word != nil {
			t.addParts(redir.Word.Parts, modeHereString)
		}

		t.text("\n")
	case redir.Hdoc == nil:
	case heredocQuoted(redir.Word):
		t.text(wordToString(redir.Hdoc))
	default:
		t.addParts(redir.Hdoc.Parts, modeHeredoc)
	}

	if redir.Op == syntax.DashHdoc {
		t.stripLeadingTabs()
	}

	return t
}

// stripLeadingTabs removes the tabs that start each line of a <<- heredoc.
// The shell strips them from the source lines, so a variable's value keeps
// its own.
func (t *ShellText) stripLeadingTabs() {
	lineStart := true

	for i, part := range t.Parts {
		if !part.literal() {
			lineStart = false

			continue
		}

		var b strings.Builder

		for _, r := range part.Text {
			if lineStart && r == '\t' {
				continue
			}

			lineStart = r == '\n'

			b.WriteRune(r)
		}

		t.Parts[i].Text = b.String()
	}
}

// heredocQuoted reports a heredoc delimiter with quotes or a backslash,
// which keeps the body from being expanded.
func heredocQuoted(word *syntax.Word) bool {
	if word == nil {
		return false
	}

	for _, part := range word.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok || strings.Contains(lit.Value, `\`) {
			return true
		}
	}

	return false
}

// catText marks text piped from cat, which a same-line alias or function
// named cat would change.
func catText(text ShellText) ShellText {
	return ShellText{Parts: append([]TextPart{{cat: true}}, text.Parts...)}
}

// trimTrailingNewlines drops the newlines a command substitution removes.
func (t *ShellText) trimTrailingNewlines() {
	for len(t.Parts) > 0 {
		last := &t.Parts[len(t.Parts)-1]
		if !last.literal() {
			return
		}

		last.Text = strings.TrimRight(last.Text, "\n")
		if last.Text != "" {
			return
		}

		t.Parts = t.Parts[:len(t.Parts)-1]
	}
}

// resolveText fills in the variables of t as they stand now, and turns
// what the line makes unknowable into gaps.
func (w *astWalker) resolveText(t ShellText) ShellText {
	parts := make([]TextPart, 0, len(t.Parts))

	for _, part := range t.Parts {
		switch {
		case part.cat && w.defined("cat"):
			part = TextPart{Gap: GapCatDefined}
		case part.cat:
			continue
		case part.Var != "":
			part = w.resolveVar(part)
		}

		parts = append(parts, part)
	}

	return ShellText{Parts: parts}
}

// resolveVar fills in one variable reference, or a gap when its value is
// not known or the shell would split it.
func (w *astWalker) resolveVar(part TextPart) TextPart {
	value, set, trusted := w.trustedValue(part.Var)

	switch {
	case !trusted || w.state.arithmetic:
		part.Gap = fmt.Sprintf(gapUntrustedFmt, part.Var)
	case !set:
		part.Gap = fmt.Sprintf(gapUnsetFormat, part.Var)
	case HasUnresolvedVars(value):
		part.Gap = fmt.Sprintf(gapNestedFormat, part.Var)
	case part.unquoted && (value == "" || strings.ContainsAny(value, unquotedSplit)):
		part.Gap = fmt.Sprintf(gapSplitFormat, part.Var)
	default:
		part.Text, part.resolved = value, true
	}

	return part
}

// argTexts resolves the argument words of a call, keyed by the argument as
// recorded. Two words that render alike but build differently get a gap.
func (w *astWalker) argTexts(words []*syntax.Word) map[string]ShellText {
	if len(words) == 0 {
		return nil
	}

	texts := make(map[string]ShellText, len(words))

	for _, word := range words {
		key := argWord(word)
		text := w.resolveText(wordText(word))

		if prev, seen := texts[key]; seen && !slices.Equal(prev.Parts, text.Parts) {
			text = ShellText{Parts: []TextPart{{Gap: GapAmbiguous}}}
		}

		texts[key] = text
	}

	return texts
}

// envNames are the variables a command records as they stand when it runs,
// for validators that need git's environment.
var envNames = []string{
	"GIT_EDITOR", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT",
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", pathVar,
}

// EnvValue is a variable as a command sees it. Known reports that the parser
// can tell its value; when false, Value and Set mean nothing.
type EnvValue struct {
	Value string
	Set   bool
	Known bool
}

// envSnapshot records envNames as call runs with them: from its own prefix
// assignments, else as they stand on the line. It runs before the prefix
// assignments are recorded, which marks them unknown for later commands.
func (w *astWalker) envSnapshot(call *syntax.CallExpr) map[string]EnvValue {
	env := make(map[string]EnvValue, len(envNames))

	for _, name := range envNames {
		env[name] = w.exportedValue(name)
	}

	for _, assign := range call.Assigns {
		if assign.Name == nil || !slices.Contains(envNames, assign.Name.Value) {
			continue
		}

		if assign.Append || assign.Naked || assign.Index != nil || assign.Array != nil {
			env[assign.Name.Value] = EnvValue{}

			continue
		}

		value, gap := w.resolveText(wordText(assign.Value)).Value()
		env[assign.Name.Value] = EnvValue{Value: value, Set: true, Known: gap == ""}
	}

	return env
}

// exportedValue returns name as a program the line starts sees it. A value
// assigned on the line reaches the program only when the name is exported:
// by export or declare -x, set -a, or because it came from the environment.
// PATH is unknown once the line changes it or the command table.
func (w *astWalker) exportedValue(name string) EnvValue {
	value, set, trusted := w.trustedValue(name)

	_, assigned := w.assignments[name]
	_, inEnv := w.resolver.LookupEnv(name)

	switch {
	case name == pathVar && w.state.pathChanged:
		return EnvValue{}
	case set && assigned && !inEnv && !w.state.allExport && !w.state.exported[name]:
		return EnvValue{Known: trusted}
	default:
		return EnvValue{Value: value, Set: set, Known: trusted}
	}
}

// noteExports records the names export, declare -x and typeset -x export.
func (w *astWalker) noteExports(decl *syntax.DeclClause) {
	exports := decl.Variant != nil && decl.Variant.Value == "export"

	for _, arg := range decl.Args {
		if arg.Name == nil && arg.Value != nil {
			option := wordToString(arg.Value)
			exports = exports || strings.HasPrefix(option, "-") && strings.Contains(option, "x")
		}
	}

	if !exports {
		return
	}

	if w.state.exported == nil {
		w.state.exported = make(map[string]bool)
	}

	for _, arg := range decl.Args {
		if arg.Name != nil {
			w.state.exported[arg.Name.Value] = true
		}
	}
}

// noteAllExport records set -a and set -o allexport, which export every
// variable assigned after them.
func (w *astWalker) noteAllExport(args []string) {
	for i, arg := range args {
		short := strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.Contains(arg, "a")
		long := arg == setOption && i+1 < len(args) && args[i+1] == "allexport"

		if short || long {
			w.state.allExport = true
		}
	}
}

// noteArithmetic records arithmetic anywhere on the line. It can assign
// any variable ((T=1)), let, $[T=1], a[T=1]=x, declare -i) and evaluates
// values as more arithmetic, so no variable value is trusted after it.
func (w *astWalker) noteArithmetic(node syntax.Node) {
	switch n := node.(type) {
	case *syntax.ArithmCmd, *syntax.ArithmExp, *syntax.LetClause:
		w.state.arithmetic = true
	case *syntax.ParamExp:
		w.state.arithmetic = w.state.arithmetic || !literalIndex(n.Index) || n.Slice != nil
	case *syntax.Assign:
		w.state.arithmetic = w.state.arithmetic || !literalIndex(n.Index)
	case *syntax.DeclClause:
		for _, arg := range n.Args {
			if arg.Name == nil && arg.Value != nil {
				option := wordToString(arg.Value)
				w.state.arithmetic = w.state.arithmetic ||
					strings.HasPrefix(option, "-") && strings.Contains(option, "i")
			}
		}
	}
}

// prefixGaps turns into gaps the variables of a heredoc or here-string that
// the call's own prefix assignments set: bash expands them with the new
// value and zsh with the old one.
func prefixGaps(text *ShellText, call *syntax.CallExpr) *ShellText {
	if text == nil || len(call.Assigns) == 0 {
		return text
	}

	out := ShellText{Parts: slices.Clone(text.Parts)}

	for i, part := range out.Parts {
		for _, assign := range call.Assigns {
			if part.Var != "" && assign.Name != nil && assign.Name.Value == part.Var {
				out.Parts[i] = TextPart{Var: part.Var, Gap: fmt.Sprintf(gapUntrustedFmt, part.Var)}
			}
		}
	}

	return &out
}

// literalIndex reports no subscript, or one of digits, @ or *, which assigns
// nothing.
func literalIndex(index syntax.ArithmExpr) bool {
	if index == nil {
		return true
	}

	word, ok := index.(*syntax.Word)
	if !ok {
		return false
	}

	lit := word.Lit()

	return lit != "" && strings.Trim(lit, "0123456789@*") == ""
}

// ArgText returns how the shell builds the argument recorded as arg, with
// the variables resolved as they stood when the command ran. It reports
// false when the parser kept no record for arg.
func (c *Command) ArgText(arg string) (ShellText, bool) {
	text, ok := c.argTexts[arg]

	return text, ok
}

// StdinText returns how the shell builds the heredoc, here-string or literal
// output fed to the command's stdin, when the parser captured it.
func (c *Command) StdinText() (ShellText, bool) {
	if c.stdinText == nil {
		return ShellText{}, false
	}

	return *c.stdinText, true
}

// Env returns a variable from git's environment as the command runs. A
// command started by a launcher, or a name not recorded, is unknown.
func (c *Command) Env(name string) EnvValue {
	return c.env[name]
}
