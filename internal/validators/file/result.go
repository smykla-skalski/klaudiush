package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/linters"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// coverage is how much of a file one validator run checked.
type coverage uint8

const (
	// coverPart is a fragment of the file, or nothing of it.
	coverPart coverage = iota
	// coverProposed is the whole file a Write would leave, before it ran.
	coverProposed
	// coverResult is the whole file as the tool left it.
	coverResult
)

// fileCoverage is what checking the hook's content covers: the file as the
// tool left it when toolResult holds, the whole file a Write proposes
// before it ran, and otherwise only part of it.
func fileCoverage(hookCtx *hook.Context, toolResult bool) coverage {
	switch {
	case toolResult:
		return coverResult
	case validator.ProposedWrite(hookCtx):
		return coverProposed
	default:
		return coverPart
	}
}

// only keeps the coverage when the run's verdict holds for the whole file.
func (c coverage) only(complete bool) coverage {
	if complete {
		return c
	}

	return coverPart
}

// mark records on result how much of the file the run checked.
func (c coverage) mark(result *validator.Result) *validator.Result {
	switch c {
	case coverResult:
		return result.MarkInspected()
	case coverProposed:
		return result.MarkProposed()
	case coverPart:
		return result
	default:
		return result
	}
}

// lintUnavailable reports why a linter run checked nothing: the tool is not
// installed, ran out of time, was canceled, or failed without reporting
// anything. It returns nil when the run's verdict can be trusted.
func lintUnavailable(
	lintCtx context.Context,
	tool string,
	result *linters.LintResult,
) *validator.Result {
	switch {
	case result == nil:
		return validator.Unavailable(validator.ReasonError, tool+" returned no result")
	case result.Skipped:
		return validator.Unavailable(
			validator.ReasonMissingTool,
			tool+" is not installed, so this file was not checked",
		)
	}

	if reason := validator.ReasonFromContext(lintCtx); reason != "" {
		return validator.Unavailable(
			reason,
			fmt.Sprintf("%s %s before it finished checking this file", tool, reason.Describe()),
		)
	}

	if !result.Success && len(result.Findings) == 0 && strings.TrimSpace(result.RawOut) == "" {
		return validator.Unavailable(
			validator.ReasonError,
			fmt.Sprintf("%s failed without reporting a finding: %v", tool, result.Err),
		)
	}

	return nil
}

// readToolResult reads a file as the tool left it. Once a tool ran, the file
// on disk is what matters: the tool input holds only what was asked for, and
// for an Edit only a fragment of it. The second return value reports whether
// the hook fired after the tool, in which case the caller must use this
// result (or the error) instead of the tool input. A relative path is read
// against the hook's working directory, not the process's.
func readToolResult(ctx *hook.Context, filePath string) (string, bool, error) {
	if !ctx.IsAfterTool() || filePath == "" {
		return "", false, nil
	}

	path := hook.CanonicalFilePath(ctx.WorkingDir, filePath)

	info, err := os.Stat(path)
	if err != nil {
		return "", true, errors.Wrap(err, "reading file after tool")
	}

	if !info.Mode().IsRegular() {
		return "", true, errors.Newf("not a regular file: %s", path)
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", true, errors.Wrap(err, "reading file after tool")
	}

	return string(data), true, nil
}

// stringEdit is one replacement of an Edit or MultiEdit call.
type stringEdit struct {
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// proposedEditContent returns the file an Edit or MultiEdit call would leave
// behind, applying its replacements in order to the original content the way
// the tools do. An empty old_string on an empty file creates it. It returns
// false when a replacement cannot be applied, which the tool would reject.
func proposedEditContent(ctx *hook.Context, original string) (string, bool) {
	edits := toolEdits(ctx)
	if len(edits) == 0 {
		return "", false
	}

	content := original

	for _, edit := range edits {
		switch {
		case edit.OldString == "" && content == "":
			content = edit.NewString
		case edit.OldString == "" || !strings.Contains(content, edit.OldString):
			return "", false
		case edit.ReplaceAll:
			content = strings.ReplaceAll(content, edit.OldString, edit.NewString)
		default:
			content = strings.Replace(content, edit.OldString, edit.NewString, 1)
		}
	}

	return content, true
}

// originalBeforeEdit reverts an Edit or MultiEdit on the file it produced,
// undoing the replacements in reverse order. It returns false when a
// replacement cannot be found in the result.
func originalBeforeEdit(ctx *hook.Context, result string) (string, bool) {
	edits := toolEdits(ctx)
	if len(edits) == 0 {
		return "", false
	}

	content := result

	for _, edit := range slices.Backward(edits) {
		switch {
		case edit.NewString == "" || !strings.Contains(content, edit.NewString):
			return "", false
		case edit.ReplaceAll:
			content = strings.ReplaceAll(content, edit.NewString, edit.OldString)
		default:
			content = strings.Replace(content, edit.NewString, edit.OldString, 1)
		}
	}

	return content, true
}

// toolEdits lists the replacements of an Edit or MultiEdit call.
func toolEdits(ctx *hook.Context) []stringEdit {
	switch ctx.ToolName {
	case hook.ToolTypeEdit:
		return []stringEdit{{
			OldString: ctx.ToolInput.OldString,
			NewString: ctx.ToolInput.NewString,
			ReplaceAll: additionalBool(ctx.ToolInput.Additional, "replace_all", "replaceAll") ||
				expectsSeveralReplacements(ctx.ToolInput.Additional),
		}}
	case hook.ToolTypeMultiEdit:
		raw, ok := ctx.ToolInput.Additional["edits"]
		if !ok {
			return nil
		}

		var edits []stringEdit
		if err := json.Unmarshal(raw, &edits); err != nil {
			return nil
		}

		return edits
	case hook.ToolTypeUnknown, hook.ToolTypeBash, hook.ToolTypeWrite,
		hook.ToolTypeGrep, hook.ToolTypeRead, hook.ToolTypeGlob:
		return nil
	default:
		return nil
	}
}

// expectsSeveralReplacements reports a Gemini replace call that replaces every
// match (expected_replacements above one).
func expectsSeveralReplacements(additional map[string]json.RawMessage) bool {
	raw, ok := additional["expected_replacements"]
	if !ok {
		return false
	}

	var count int
	if err := json.Unmarshal(raw, &count); err != nil {
		return false
	}

	return count > 1
}

// additionalBool reads the first of keys that holds a JSON boolean.
func additionalBool(additional map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		raw, ok := additional[key]
		if !ok {
			continue
		}

		var value bool
		if err := json.Unmarshal(raw, &value); err == nil {
			return value
		}
	}

	return false
}
