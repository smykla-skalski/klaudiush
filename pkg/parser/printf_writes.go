package parser

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// printfNames returns the variables a printf call sets. The words up to and
// including the format are read with their values: "$f" holding -vNAME or
// %n makes printf assign. It reports false when one of them may hold an
// option or a format klaudiush cannot know; an unknown last word is left
// alone, since printf assigns nothing without a word after its format.
func (w *astWalker) printfNames(cmd Command) ([]string, bool) {
	args := make([]string, 0, len(cmd.Args))

	for i := 0; i < len(cmd.Args); i++ {
		arg := cmd.Args[i]
		last := i == len(cmd.Args)-1

		value, ok := w.printfWordValue(cmd, arg)
		if !ok && !last {
			return nil, false
		}

		if !ok {
			value = arg
		}

		args = append(args, value)

		switch {
		case value == endOfOptions:
			continue
		case value == "-v" && !last:
			i++
			args = append(args, cmd.Args[i])

			continue
		case strings.HasPrefix(value, "-") && value != "-":
			continue
		}

		args = append(args, cmd.Args[i+1:]...)

		break
	}

	return writtenVars(Command{Name: printfBuiltin, Args: args}), true
}

// printfWordValue expands the variables in one rendered printf word, also
// trusting a variable no pass of the enclosing loop can change. A word that
// may split into several, or holds command output, is unknown.
func (w *astWalker) printfWordValue(cmd Command, arg string) (string, bool) {
	if marked(arg) {
		return "", false
	}

	if !strings.Contains(arg, "$") {
		return arg, true
	}

	known := true

	value := expandVars(arg, func(name string) (string, bool) {
		value, set, trusted := w.printfValue(name)
		if !trusted {
			known = false

			return "", false
		}

		return value, set
	})

	if !known || HasUnresolvedVars(value) || marked(value) {
		return "", false
	}

	if cmd.quoting[arg]&splitWord != 0 &&
		(value == "" || strings.ContainsAny(value, " \t\n"+globChars)) {
		return "", false
	}

	return value, true
}

// printfValue returns a variable's value for a printf word: one trusted as
// any word is, or one this shell assigned a literal that, in a loop, no pass
// changes. In a script run by a new shell no inherited value is trusted,
// but the script's own assignments still hold.
func (w *astWalker) printfValue(name string) (string, bool, bool) {
	if value, set, trusted := w.trustedValue(name); trusted {
		return value, set, true
	}

	if w.state.untrusted || w.state.dynamicVars[name] || w.unknownVars[name] ||
		(w.distrust && !w.shellAssigned[name]) {
		return "", false, false
	}

	value, set := w.assignments[name]
	if !set {
		return "", false, false
	}

	if stable, ok := w.loopStable[name]; w.inLoop && (!ok || value != stable) {
		return "", false, false
	}

	return value, true, true
}

// shellSetVars are variables the shell itself sets while a loop runs, with
// no name on the line: read and mapfile defaults, getopts, cd and pushd,
// coproc, and the counters the shell updates.
var shellSetVars = nameSet(`REPLY MAPFILE OPTARG OPTIND OPTERR PWD OLDPWD
	DIRSTACK COPROC _ RANDOM SRANDOM SECONDS LINENO HISTCMD EPOCHREALTIME
	EPOCHSECONDS PIPESTATUS FUNCNAME GROUPS COLUMNS LINES SHLVL`)

// identifier matches every name-like token in literal text.
var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// loopScope returns the variables, assigned a literal earlier on the line,
// that no pass of loop can change, with the values they hold before it. A
// name the loop, or a same-line definition it calls, writes appears in it
// as literal text; one it may write without naming it (eval, source, a
// computed target or command word, arithmetic on values) leaves none, and
// so does a write the loop scan cannot name even with these values known.
// The loop must be the whole statement, so these are the values it starts
// with.
func (w *astWalker) loopScope(loop syntax.Node) map[string]string {
	if w.state.untrusted || w.state.namesUnknown || w.state.arithmetic ||
		len(w.assignments) == 0 {
		return nil
	}

	scan := loopScan{w: w, mentioned: make(map[string]bool), seen: make(map[string]bool)}
	scan.node(loop)

	if scan.unknown {
		return nil
	}

	stable := make(map[string]string)

	for name, value := range w.assignments {
		if scan.mentioned[name] || shellSetVars[name] || strings.HasPrefix(name, "BASH_") ||
			strings.HasPrefix(name, "COMP_") || strings.HasPrefix(name, "READLINE_") ||
			w.unknownVars[name] || w.state.dynamicVars[name] || w.namerefNamed(name) ||
			(w.distrust && !w.shellAssigned[name]) {
			continue
		}

		stable[name] = value
	}

	if len(stable) == 0 {
		return nil
	}

	saved := w.loopStable
	w.loopStable = stable

	defer func() { w.loopStable = saved }()

	unknown := false

	walkLoop(loop, func(node syntax.Node, param bool) {
		unknown = unknown || allStartupVars(w.loopStartupNames(node, param, make(map[string]bool)))
	})

	if unknown {
		return nil
	}

	return stable
}

// namerefNamed reports a name that is a nameref or the target of one, which
// an assignment to another name changes.
func (w *astWalker) namerefNamed(name string) bool {
	if _, ok := w.namerefs[name]; ok {
		return true
	}

	for _, target := range w.namerefs {
		if target == name {
			return true
		}
	}

	return false
}

// allStartupVars reports names holding every startup variable, which the
// loop scan returns for a write it cannot name.
func allStartupVars(names []string) bool {
	for name := range startupVars {
		found := false

		for _, n := range names {
			found = found || n == name
		}

		if !found {
			return false
		}
	}

	return true
}

// loopScan collects the names written as literal text in a loop and the
// same-line definitions it calls.
type loopScan struct {
	w         *astWalker
	mentioned map[string]bool
	seen      map[string]bool
	unknown   bool
}

func (s *loopScan) node(root syntax.Node) {
	walkLoop(root, func(node syntax.Node, param bool) {
		if arithmeticNode(node) {
			s.unknown = true
		}

		switch n := node.(type) {
		case *syntax.Lit:
			if !param {
				s.mention(n.Value)
			}
		case *syntax.SglQuoted:
			s.mention(n.Value)
		case *syntax.BinaryTest:
			s.unknown = s.unknown || arithmeticTest(n.Op)
		case *syntax.CallExpr:
			s.call(n)
		}
	})
}

func (s *loopScan) mention(text string) {
	for _, name := range identifier.FindAllString(text, -1) {
		s.mentioned[name] = true
	}
}

// call scans the definitions a call may run. A computed command word may
// run anything.
func (s *loopScan) call(call *syntax.CallExpr) {
	inner := runWrapped(call.Args)
	if len(inner) == 0 {
		return
	}

	if computedWord(inner[0]) {
		s.unknown = true

		return
	}

	name := argWord(inner[0])

	var texts []string

	if value, ok := s.w.aliases[name]; ok {
		texts = append(texts, value)
	}

	if body, ok := s.w.funcs[name]; ok {
		texts = append(texts, body)
	}

	texts = append(texts, s.w.stmtFuncs[name]...)

	for _, text := range texts {
		if s.seen[text] {
			continue
		}

		s.seen[text] = true

		if len(s.seen) > maxDefinitionScans {
			s.unknown = true

			return
		}

		file, err := syntax.NewParser().Parse(strings.NewReader(text), "")
		if err != nil {
			s.unknown = true

			return
		}

		s.node(file)
	}
}

// arithmeticTest reports a [[ ]] operator that evaluates both sides as
// arithmetic, which may assign: a='G=1'; [[ $a -eq 1 ]] sets G.
func arithmeticTest(op syntax.BinTestOperator) bool {
	switch op {
	case syntax.TsEql, syntax.TsNeq, syntax.TsLeq, syntax.TsGeq, syntax.TsLss, syntax.TsGtr:
		return true
	default:
		return false
	}
}

// loopLiteral returns a printf word in a loop as literal text when its
// expansions are quoted plain references to loop-stable variables, so the
// loop scan can tell whether it holds an option or %n.
func (w *astWalker) loopLiteral(word *syntax.Word) *syntax.Word {
	if w.loopStable == nil || isLiteralWord(word) {
		return word
	}

	var text strings.Builder

	for _, part := range word.Parts {
		switch p := part.(type) {
		case *syntax.SglQuoted:
			if p.Dollar {
				return word
			}

			text.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				switch q := inner.(type) {
				case *syntax.Lit:
					text.WriteString(renderLit(q.Value, true, doubleQuoteEscapable))
				case *syntax.ParamExp:
					value, ok := w.stableParam(q)
					if !ok {
						return word
					}

					text.WriteString(value)
				default:
					return word
				}
			}
		default:
			return word
		}
	}

	return &syntax.Word{Parts: []syntax.WordPart{&syntax.SglQuoted{Value: text.String()}}}
}

// stableParam returns the value of a plain $name or ${name} reference to a
// loop-stable variable.
func (w *astWalker) stableParam(pe *syntax.ParamExp) (string, bool) {
	if pe.Param == nil || pe.Excl || pe.Length || pe.Width || pe.Index != nil ||
		pe.Slice != nil || pe.Repl != nil || pe.Names != 0 || pe.Exp != nil {
		return "", false
	}

	value, ok := w.loopStable[pe.Param.Value]

	return value, ok
}

// leadingLoop returns the loop a statement starts with, directly or as the
// first statement of a group such as a function body, so nothing on the
// statement runs before it.
func leadingLoop(stmt *syntax.Stmt) syntax.Command {
	for stmt != nil {
		switch cmd := stmt.Cmd.(type) {
		case *syntax.ForClause, *syntax.WhileClause:
			return cmd
		case *syntax.Block:
			stmt = firstStmt(cmd.Stmts)
		default:
			return nil
		}
	}

	return nil
}

func firstStmt(stmts []*syntax.Stmt) *syntax.Stmt {
	if len(stmts) == 0 {
		return nil
	}

	return stmts[0]
}
