package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// readToolResult reads a file as the tool left it. Once a tool ran, the file
// on disk is what matters: the tool input holds only what was asked for, and
// for an Edit only a fragment of it. The second return value reports whether
// the hook fired after the tool, in which case the caller must use this
// result (or the error) instead of the tool input.
func readToolResult(ctx *hook.Context, filePath string) (string, bool, error) {
	if !ctx.IsAfterTool() || filePath == "" {
		return "", false, nil
	}

	data, err := os.ReadFile(filepath.Clean(filePath))
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

// toolEdits lists the replacements of an Edit or MultiEdit call.
func toolEdits(ctx *hook.Context) []stringEdit {
	switch ctx.ToolName {
	case hook.ToolTypeEdit:
		return []stringEdit{{
			OldString:  ctx.ToolInput.OldString,
			NewString:  ctx.ToolInput.NewString,
			ReplaceAll: additionalBool(ctx.ToolInput.Additional, "replace_all", "replaceAll"),
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
