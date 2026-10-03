package parser

import "mvdan.cc/sh/v3/syntax"

// certainty says when a statement is sure to have run: everywhere after it
// (unbounded), only before "until" (it runs only on a branch that ends
// there), or never for sure (background, pipelines, function bodies).
type certainty struct {
	until   Location
	bounded bool
	never   bool
}

// markCertainStmts records, for each statement under root, when a later
// consumer can rely on it having run.
func markCertainStmts(root *syntax.Stmt, out map[*syntax.Stmt]certainty) {
	if out == nil {
		return
	}

	walkCertain(root, certainty{}, out)
}

func walkCertain(stmt *syntax.Stmt, c certainty, out map[*syntax.Stmt]certainty) {
	if stmt == nil {
		return
	}

	if stmt.Background || stmt.Coprocess {
		c.never = true
	}

	out[stmt] = c

	walkCertainCmd(stmt.Cmd, c, out)
}

func walkCertainStmts(stmts []*syntax.Stmt, c certainty, out map[*syntax.Stmt]certainty) {
	for _, stmt := range stmts {
		walkCertain(stmt, c, out)
	}
}

func walkCertainCmd(cmd syntax.Command, c certainty, out map[*syntax.Stmt]certainty) {
	switch x := cmd.(type) {
	case *syntax.BinaryCmd:
		switch x.Op {
		case syntax.AndStmt:
			walkAndChain(x, c, x, out)
		case syntax.OrStmt:
			walkCertain(x.X, c, out)
			walkCertain(x.Y, boundedBy(c, x), out)
		case syntax.Pipe, syntax.PipeAll:
			walkCertain(x.X, certainty{never: true}, out)
			walkCertain(x.Y, certainty{never: true}, out)
		}
	case *syntax.Block:
		walkCertainStmts(x.Stmts, c, out)
	case *syntax.Subshell:
		walkCertainStmts(x.Stmts, c, out)
	case *syntax.TimeClause:
		walkCertain(x.Stmt, c, out)
	case *syntax.IfClause:
		walkCertainStmts(x.Cond, c, out)
		walkCertainStmts(x.Then, boundedBy(c, x), out)

		if x.Else != nil {
			walkCertainCmd(x.Else, boundedBy(c, x), out)
		}
	case *syntax.WhileClause:
		walkCertainStmts(x.Cond, c, out)
		walkCertainStmts(x.Do, boundedBy(c, x), out)
	case *syntax.ForClause:
		walkCertainStmts(x.Do, boundedBy(c, x), out)
	case *syntax.CaseClause:
		for _, item := range x.Items {
			walkCertainStmts(item.Stmts, boundedBy(c, x), out)
		}
	case *syntax.FuncDecl:
		walkCertain(x.Body, certainty{never: true}, out)
	}
}

// boundedBy limits c to consumers that come before node ends.
func boundedBy(c certainty, node syntax.Node) certainty {
	end := Location{Line: node.End().Line(), Column: node.End().Col()}
	if !c.bounded || locationBefore(end, c.until) {
		c.until, c.bounded = end, true
	}

	return c
}

// markCertainty sets, on the writes added since index from, whether they
// are sure to have run, from the certainty of the statement that made them.
// Writes in nested scripts are never sure: the script itself may not run.
func (w *astWalker) markCertainty(stmt *syntax.Stmt, from int) {
	c, ok := w.certain[stmt]
	if !ok || w.depth > 0 || w.certain == nil {
		c = certainty{never: true}
	}

	for i := from; i < len(w.fileWrites); i++ {
		w.fileWrites[i].Unconditional = !c.never && !c.bounded
		if !c.never && c.bounded {
			w.fileWrites[i].CertainUntil = c.until
		}
	}
}

// Certain reports whether the write is sure to have run by the time a
// consumer at source position "at" runs.
func (f FileWrite) Certain(at Location) bool {
	if f.Unconditional {
		return true
	}

	return f.CertainUntil != (Location{}) && locationBefore(at, f.CertainUntil)
}

// walkAndChain walks a && chain. Every link after the first runs only when
// the ones before it succeeded, so it is certain for the rest of the chain:
// in a && b && c, b has run whenever c runs.
func walkAndChain(
	x *syntax.BinaryCmd,
	c certainty,
	chain syntax.Node,
	out map[*syntax.Stmt]certainty,
) {
	inner, ok := x.X.Cmd.(*syntax.BinaryCmd)
	if ok && inner.Op == syntax.AndStmt && !x.X.Background && !x.X.Coprocess && !x.X.Negated {
		out[x.X] = c
		walkAndChain(inner, c, chain, out)
	} else {
		walkCertain(x.X, c, out)
	}

	walkCertain(x.Y, boundedBy(c, chain), out)
}
