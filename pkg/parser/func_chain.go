package parser

import "mvdan.cc/sh/v3/syntax"

// OpacityFunctionChain means a function definition is followed by &&, || or
// | in a shape no shell accepts, so where its body ends is unknown.
const OpacityFunctionChain OpacityCause = "function-chain"

// FunctionChainOperation names a function definition whose name is unknown.
const FunctionChainOperation = "function definition"

// splitFuncChains gives every function definition under root the body a
// shell gives it. mvdan.cc/sh folds an &&, || or | list that follows the
// body into it, so f() { :; } && cmd parses as a function whose body is
// "{ :; } && cmd". bash and zsh end the definition at its first command and
// run the rest of the list at once. The body is cut back to that command
// and the definition takes its place at the head of the list, so the rest is
// walked as code that runs now. It returns the name of the first definition
// it could not split, and false when there is one.
func splitFuncChains(root syntax.Node) (string, bool) {
	failed, ok := "", true

	syntax.Walk(root, func(node syntax.Node) bool {
		stmt, isStmt := node.(*syntax.Stmt)
		if !isStmt {
			return true
		}

		fn, isFunc := stmt.Cmd.(*syntax.FuncDecl)
		if !isFunc {
			return true
		}

		if !splitFuncChain(stmt, fn) && ok {
			failed, ok = funcName(fn), false
		}

		return true
	})

	return failed, ok
}

// splitFuncChain moves the list folded into the body of fn, defined by
// stmt, out of it, reporting false on a shape it cannot split. zsh takes a
// definition as a body, as in f() g() { :; } && cmd, where the list folds
// into the innermost definition, so that one is split first.
func splitFuncChain(stmt *syntax.Stmt, fn *syntax.FuncDecl) bool {
	chain := fn.Body
	if chain == nil {
		return true
	}

	if inner, isFunc := chain.Cmd.(*syntax.FuncDecl); isFunc && !splitFuncChain(chain, inner) {
		return false
	}

	head, isList := chain.Cmd.(*syntax.BinaryCmd)
	if !isList {
		return !chain.Negated
	}

	if !plainLink(chain) {
		return false
	}

	links := []*syntax.BinaryCmd{head}
	for {
		next, isList := head.X.Cmd.(*syntax.BinaryCmd)
		if !isList {
			break
		}

		if !plainLink(head.X) {
			return false
		}

		head = next
		links = append(links, head)
	}

	body := head.X
	if body.Cmd == nil || !plainBody(body) {
		return false
	}

	fn.Body = body
	head.X = &syntax.Stmt{Position: fn.Pos(), Cmd: fn, Negated: stmt.Negated}

	stmt.Cmd = chain.Cmd
	if stmt.Negated {
		pruneFalseAndLinks(links)
	}

	stmt.Negated = false

	return true
}

// pruneFalseAndLinks removes commands skipped after a negated definition.
func pruneFalseAndLinks(links []*syntax.BinaryCmd) {
	for i := len(links) - 1; i >= 0 && links[i].Op == syntax.AndStmt; i-- {
		links[i].Y = &syntax.Stmt{Position: links[i].Y.Pos()}
	}
}

// plainLink reports a statement inside the folded list that carries nothing
// of its own. The parser never sets these on one, and no shell accepts a
// negated function body.
func plainLink(stmt *syntax.Stmt) bool {
	return plainBody(stmt) && len(stmt.Redirs) == 0
}

// plainBody reports a body statement with no negation and no job control.
// Its redirects stay with it: a shell applies them on every call.
func plainBody(stmt *syntax.Stmt) bool {
	return !stmt.Negated && !stmt.Background && !stmt.Coprocess && !stmt.Disown
}

// funcName returns the name fn defines, or a placeholder.
func funcName(fn *syntax.FuncDecl) string {
	if fn.Name != nil && fn.Name.Value != "" {
		return fn.Name.Value
	}

	for _, name := range fn.Names {
		if name != nil && name.Value != "" {
			return name.Value
		}
	}

	return FunctionChainOperation
}
