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

// EvalSetupTools returns the programs whose printed shell setup an eval or
// sourced-stream opacity can name in Opacity.Tool, sorted.
func EvalSetupTools() []string {
	return slices.Sorted(maps.Keys(evalSetupTools))
}

// agentValueOptions are the ssh-agent options that take a value, given
// attached or as the next argument.
const agentValueOptions = "aAEOPt"

// agentFlags are the ssh-agent flags that still print a new agent's setup;
// -k, -u and -V print something else.
const agentFlags = "cdDsTUx"

// agentSetup reports ssh-agent printing the variables of a new agent: only
// options, no command to run, and no flag that prints something else.
func agentSetup(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == endOfOptions {
			return i == len(args)-1
		}

		if len(arg) < 2 || arg[0] != '-' {
			return false
		}

		takesNext, ok := agentCluster(arg[1:])
		if !ok {
			return false
		}

		if takesNext {
			if i++; i >= len(args) {
				return false
			}
		}
	}

	return true
}

// agentCluster checks one cluster of short ssh-agent options (-sk, -t60),
// reporting whether its last option takes the next argument as its value.
func agentCluster(cluster string) (takesNext, ok bool) {
	for j, opt := range cluster {
		switch {
		case strings.ContainsRune(agentValueOptions, opt):
			return j == len(cluster)-1, true
		case !strings.ContainsRune(agentFlags, opt):
			return false, false
		}
	}

	return false, true
}

// condaOperations are the conda shell.<shell> operations that print shell
// setup; "commands" prints completion words instead.
var condaOperations = nameSet("hook activate deactivate reactivate")

// condaSetup reports conda shell.<shell> followed by an operation that
// prints shell code.
func condaSetup(args []string) bool {
	idx := slices.IndexFunc(args, isOperand)
	if idx < 0 || !strings.HasPrefix(args[idx], "shell.") || len(args[idx]) == len("shell.") {
		return false
	}

	return condaOperations[firstOperand(args[idx+1:])]
}

func isOperand(arg string) bool {
	return !strings.HasPrefix(arg, "-")
}

func subcommandIn(names ...string) func([]string) bool {
	return func(args []string) bool {
		return slices.Contains(names, firstOperand(args))
	}
}

// evalSetupTool returns the tool whose shell setup an eval call runs: eval
// given one command substitution, quoted or not, of a known setup command
// (see setupTool). Anything else returns "".
func evalSetupTool(call *syntax.CallExpr) string {
	if len(call.Args) != 2 || !isLiteralWord(call.Args[0]) ||
		argWord(call.Args[0]) != evalBuiltin {
		return ""
	}

	sub := soleSubstitution(call.Args[1])
	if sub == nil {
		return ""
	}

	return setupTool(sub.Stmts)
}

// setupTool returns the tool whose shell setup stmts print: one plain call
// of a known program named by a literal word, with only literal arguments
// that make it print setup. Anything else, a computed program name or
// argument, or a name that only contains a known one (evil-mise), returns "".
func setupTool(stmts []*syntax.Stmt) string {
	if len(stmts) != 1 {
		return ""
	}

	stmt := stmts[0]
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

// knownSetupTool is setupTool, unless the tool is an alias or function from
// this line.
func (w *astWalker) knownSetupTool(stmts []*syntax.Stmt) string {
	tool := setupTool(stmts)
	if tool == "" || w.defined(tool) {
		return ""
	}

	return tool
}
