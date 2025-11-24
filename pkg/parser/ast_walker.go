package parser

import (
	"mvdan.cc/sh/v3/syntax"
)

// astWalker walks the AST and extracts commands and file operations.
type astWalker struct {
	commands   []Command
	fileWrites []FileWrite
}

// visit is called for each node in the AST.
func (w *astWalker) visit(node syntax.Node) bool {
	switch n := node.(type) {
	case *syntax.CallExpr:
		w.extractCommand(n)
	case *syntax.Stmt:
		w.extractRedirect(n)
	case *syntax.Subshell:
		// Subshells are handled recursively by syntax.Walk
		return true
	case *syntax.CmdSubst:
		// Command substitution is handled recursively
		return true
	}

	return true
}

// extractCommand extracts a command from a CallExpr node.
func (w *astWalker) extractCommand(call *syntax.CallExpr) {
	if len(call.Args) == 0 {
		return
	}

	// First word is the command name
	name := wordToString(call.Args[0])
	if name == "" {
		return
	}

	// Remaining words are arguments
	args := wordsToStrings(call.Args[1:])

	// Determine location
	loc := Location{
		Line:   call.Pos().Line(),
		Column: call.Pos().Col(),
	}

	// Determine command type (simple for now, enhanced later)
	cmdType := CmdTypeSimple

	cmd := Command{
		Name:     name,
		Args:     args,
		Location: loc,
		Type:     cmdType,
	}

	w.commands = append(w.commands, cmd)

	// Check if this is a file write command
	w.extractFileWriteCommand(cmd)
}

// extractRedirect extracts file write operations from redirections.
func (w *astWalker) extractRedirect(stmt *syntax.Stmt) {
	if stmt.Redirs == nil {
		return
	}

	// First pass: collect output redirections and heredocs separately
	var outputPath string

	var outputOp WriteOp

	var outputLoc Location

	var heredocContent string

	var heredocLoc Location

	hasOutput := false
	hasHeredoc := false

	for _, redir := range stmt.Redirs {
		if redir.Op == syntax.RdrOut || redir.Op == syntax.AppOut {
			path := wordToString(redir.Word)
			if path == "" {
				continue
			}

			outputPath = path

			outputOp = WriteOpRedirect
			if redir.Op == syntax.AppOut {
				outputOp = WriteOpAppend
			}

			outputLoc = Location{
				Line:   redir.Pos().Line(),
				Column: redir.Pos().Col(),
			}
			hasOutput = true
		}

		// Handle heredocs
		if redir.Op == syntax.Hdoc || redir.Op == syntax.DashHdoc {
			// Extract heredoc content from Hdoc field (may be empty)
			if redir.Hdoc != nil {
				heredocContent = wordToString(redir.Hdoc)
			}
			// Mark as heredoc even if content is empty
			heredocLoc = Location{
				Line:   redir.Pos().Line(),
				Column: redir.Pos().Col(),
			}
			hasHeredoc = true
		}
	}

	// Second pass: create FileWrite entries
	// If we have both output redirection and heredoc, combine them
	if hasOutput && hasHeredoc {
		fw := FileWrite{
			Path:      outputPath,
			Operation: WriteOpHeredoc,
			Content:   heredocContent,
			Location:  heredocLoc,
		}
		w.fileWrites = append(w.fileWrites, fw)
	} else if hasOutput {
		// Just output redirection without heredoc
		fw := FileWrite{
			Path:      outputPath,
			Operation: outputOp,
			Location:  outputLoc,
		}
		w.fileWrites = append(w.fileWrites, fw)
	}
	// Note: heredoc without output redirection is rare and not handled
	// (it would just pipe to stdin of a command)
}

// extractFileWriteCommand detects file write commands (tee, cp, mv).
func (w *astWalker) extractFileWriteCommand(cmd Command) {
	op, targets := getFileWriteOperation(cmd)
	if op == WriteOpNone {
		return
	}

	for _, target := range targets {
		fw := FileWrite{
			Path:      target,
			Operation: op,
			Source:    cmd.Name,
			Location:  cmd.Location,
		}

		w.fileWrites = append(w.fileWrites, fw)
	}
}

// getFileWriteOperation determines if a command writes to files.
func getFileWriteOperation(cmd Command) (WriteOp, []string) {
	switch cmd.Name {
	case "tee":
		// tee writes to all file arguments
		return WriteOpTee, extractTeeTargets(cmd.Args)

	case "cp", "copy":
		// cp writes to the last argument
		if len(cmd.Args) >= 2 { //nolint:mnd // Trivial check for minimum args (source + dest)
			return WriteOpCopy, []string{cmd.Args[len(cmd.Args)-1]}
		}

	case "mv", "move":
		// mv writes to the last argument
		if len(cmd.Args) >= 2 { //nolint:mnd // Trivial check for minimum args (source + dest)
			return WriteOpMove, []string{cmd.Args[len(cmd.Args)-1]}
		}
	}

	return WriteOpNone, nil
}

// extractTeeTargets extracts file targets from tee command arguments.
func extractTeeTargets(args []string) []string {
	targets := make([]string, 0)

	// Skip flags (starting with -)
	for _, arg := range args {
		if len(arg) > 0 && arg[0] != '-' {
			targets = append(targets, arg)
		}
	}

	return targets
}
