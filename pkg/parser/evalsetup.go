package parser

import (
	"maps"
	"path"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// evalSetupTools are programs whose printed shell setup is commonly run
// through eval ("$(mise activate bash)"), each with a check of the
// arguments that make it print setup.
var evalSetupTools = map[string]func(args []string) bool{
	"ssh-agent": agentSetup,
	"mise":      subcommandIn("activate", "env", "hook-env"),
	"direnv":    subcommandIn("export", "hook"),
	"rbenv":     subcommandIn("init"),
	"pyenv":     subcommandIn("init", "virtualenv-init"),
	"nodenv":    subcommandIn("init"),
	"conda":     condaSetup,
	"brew":      subcommandIn("shellenv"),
	"starship":  subcommandIn("init"),
	"zoxide":    subcommandIn("init"),
	"fnm":       subcommandIn("env"),
}

// EvalSetupTools returns the programs whose printed shell setup an eval
// opacity can name in Opacity.Tool, sorted.
func EvalSetupTools() []string {
	return slices.Sorted(maps.Keys(evalSetupTools))
}

// agentSetup reports ssh-agent printing the variables of a new agent: only
// options, and not -k, which prints the commands that forget one.
func agentSetup(args []string) bool {
	return (len(args) == 0 || strings.HasPrefix(args[0], "-")) && !slices.Contains(args, "-k")
}

// condaSetup reports conda shell.<shell> with a command such as hook or
// activate, which prints shell code.
func condaSetup(args []string) bool {
	sub := firstOperand(args)

	return strings.HasPrefix(sub, "shell.") && len(sub) > len("shell.")
}

func subcommandIn(names ...string) func([]string) bool {
	return func(args []string) bool {
		return slices.Contains(names, firstOperand(args))
	}
}

// evalSetupTool returns the tool whose shell setup an eval call runs: eval
// given one command substitution, quoted or not, of a known program named by
// a literal word, with only literal arguments that make it print setup.
// Anything else, a computed program name or argument, or a name that only
// contains a known one (evil-mise), returns "".
func evalSetupTool(call *syntax.CallExpr) string {
	if len(call.Args) != 2 || !isLiteralWord(call.Args[0]) ||
		argWord(call.Args[0]) != evalBuiltin {
		return ""
	}

	sub := soleSubstitution(call.Args[1])
	if sub == nil || len(sub.Stmts) != 1 {
		return ""
	}

	stmt := sub.Stmts[0]
	inner := callExprOf(stmt)

	if inner == nil || stmt.Negated || stmt.Background || stmt.Coprocess ||
		len(inner.Args) == 0 {
		return ""
	}

	args, literal := literalArgs(inner.Args[1:])
	if !literal || !isLiteralWord(inner.Args[0]) {
		return ""
	}

	name := path.Base(argWord(inner.Args[0]))

	prints, ok := evalSetupTools[name]
	if !ok || !prints(args) {
		return ""
	}

	return name
}

// soleSubstitution returns the command substitution that is all of word,
// inside double quotes or not.
func soleSubstitution(word *syntax.Word) *syntax.CmdSubst {
	parts := word.Parts
	if len(parts) == 1 {
		if quoted, ok := parts[0].(*syntax.DblQuoted); ok {
			parts = quoted.Parts
		}
	}

	if len(parts) != 1 {
		return nil
	}

	sub, _ := parts[0].(*syntax.CmdSubst)

	return sub
}

// noteEvalSetup remembers the setup tool of the eval call recorded at seq,
// unless eval or the tool is an alias or function from this line.
func (w *astWalker) noteEvalSetup(call *syntax.CallExpr, seq int) {
	tool := evalSetupTool(call)
	if tool == "" || w.defined(evalBuiltin) || w.defined(tool) {
		return
	}

	if w.state.evalSetups == nil {
		w.state.evalSetups = make(map[int]string)
	}

	w.state.evalSetups[seq] = tool
}
