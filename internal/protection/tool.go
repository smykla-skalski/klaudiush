package protection

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// maxInputDepth bounds how deep tool input is searched for paths.
const maxInputDepth = 16

// patchHeader matches the file lines of an apply_patch body, leniently:
// any spacing and case the patch tool might accept.
var patchHeader = regexp.MustCompile(
	`(?im)^[ \t]*\*\*\*[ \t]*(?:(?:add|update|delete)[ \t]+file|move[ \t]+to)[ \t]*:[ \t]*(.+?)[ \t\r]*$`,
)

// readOnlyToolVerbs start the names of tools that only read: read_file,
// list_directory, search_files, get_file_info.
var readOnlyToolVerbs = []string{
	"read", "get", "list", "search", programFind, "view", "show", "fetch", "query",
	"stat", "glob", "grep", "ls", "cat", "describe", "inspect", "lookup",
}

// nonFileTools are built-in tools whose input names no file they change.
var nonFileTools = map[string]bool{
	"agent": true, "askuserquestion": true, "bashoutput": true, "exitplanmode": true,
	"killbash": true, "killshell": true, "skill": true, "slashcommand": true,
	"task": true, "todoread": true, "todowrite": true, "toolsearch": true,
	"updateplan": true, "webfetch": true, "websearch": true, "workflow": true,
	"monitor": true, "googlewebsearch": true, "webfetchtool": true,
}

// ToolTargets returns the paths a non-shell tool call may change: the file
// of a write or edit, every file of a patch, and for other tools (MCP
// servers included) every path-like string in the input, unless the tool's
// name says it only reads.
func ToolTargets(ctx *hook.Context) []string {
	if ctx == nil || ctx.IsBashTool() {
		return nil
	}

	switch ctx.ToolFamily {
	case hook.ToolFamilyRead, hook.ToolFamilyGrep, hook.ToolFamilyGlob:
		return nil
	case hook.ToolFamilyWrite, hook.ToolFamilyEdit, hook.ToolFamilyMultiEdit:
		return fileToolTargets(ctx)
	case hook.ToolFamilyShell, hook.ToolFamilyUnknown:
	}

	own := toolOwnName(ctx.RawToolName)
	if nonFileTools[normalizeName(ctx.RawToolName)] ||
		(readsOnly(own) && (!ctx.IsMCPTool() || !mentionsWrite(own))) {
		return nil
	}

	targets := fileToolTargets(ctx)

	for _, raw := range ctx.ToolInput.Additional {
		targets = append(targets, pathStrings(raw, 0)...)
	}

	return targets
}

func fileToolTargets(ctx *hook.Context) []string {
	targets := append([]string{}, ctx.AffectedPaths...)

	for _, path := range []string{ctx.ToolInput.FilePath, ctx.ToolInput.Path} {
		if path != "" {
			targets = append(targets, path)
		}
	}

	for _, file := range ctx.PatchFiles {
		if file.Input.FilePath != "" {
			targets = append(targets, file.Input.FilePath)
		}
	}

	for _, key := range []string{"notebook_path", "filePath", "filepath", "target", "destination"} {
		raw, ok := ctx.ToolInput.Additional[key]
		if !ok {
			continue
		}

		var value string
		if json.Unmarshal(raw, &value) == nil && value != "" {
			targets = append(targets, value)
		}
	}

	return append(targets, patchTargets(ctx)...)
}

// patchTargets reads the file lines of a patch body wherever the tool put
// it: Codex sends it as the command, other tools as input or patch.
func patchTargets(ctx *hook.Context) []string {
	texts := []string{ctx.ToolInput.Command, ctx.ToolInput.Content}

	for _, key := range []string{"input", "patch", "patchText", "patch_text", "command"} {
		var text string
		if raw, ok := ctx.ToolInput.Additional[key]; ok && json.Unmarshal(raw, &text) == nil {
			texts = append(texts, text)
		}
	}

	targets := make([]string, 0, len(texts))

	for _, text := range texts {
		targets = append(targets, patchFiles(text)...)
	}

	return targets
}

func patchFiles(text string) []string {
	matches := patchHeader.FindAllStringSubmatch(text, -1)
	files := make([]string, 0, len(matches))

	for _, m := range matches {
		files = append(files, m[1])
	}

	return files
}

// pathStrings returns the strings in a JSON value that look like paths:
// not a URL, a slash or a dot in them, and no spaces unless they start like
// a path. Patch bodies in any field are read for their file lines too.
func pathStrings(raw json.RawMessage, depth int) []string {
	if depth > maxInputDepth {
		return nil
	}

	var text string
	if json.Unmarshal(raw, &text) == nil {
		found := patchFiles(text)
		if path := fileURIPath(text); path != "" {
			found = append(found, path)
		} else if looksLikePath(text) {
			found = append(found, text)
		}

		return found
	}

	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		var out []string
		for _, item := range list {
			out = append(out, pathStrings(item, depth+1)...)
		}

		return out
	}

	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		var out []string
		for _, item := range object {
			out = append(out, pathStrings(item, depth+1)...)
		}

		return out
	}

	return nil
}

// fileURIPath returns the path of a file:// URI, or "".
func fileURIPath(text string) string {
	rest, ok := strings.CutPrefix(text, "file://")
	if !ok {
		return ""
	}

	if parsed, err := url.Parse(text); err == nil && parsed.Path != "" {
		return parsed.Path
	}

	return rest
}

func looksLikePath(text string) bool {
	if text == "" || strings.ContainsAny(text, "\n\r") || strings.Contains(text, "://") {
		return false
	}

	if strings.ContainsAny(text, " \t") && !strings.HasPrefix(text, "/") &&
		!strings.HasPrefix(text, "~") && !strings.HasPrefix(text, ".") {
		return false
	}

	return strings.ContainsAny(text, "/\\.~")
}

// toolOwnName returns the server's own tool name of an MCP tool
// (mcp__server__write_file gives write_file), or the tool name.
func toolOwnName(raw string) string {
	if _, after, found := strings.CutLast(raw, "__"); found {
		return after
	}

	return raw
}

// writeVerbs mark a tool name that writes whatever verb it starts with,
// such as find_and_replace or read_then_overwrite. MCP servers name their
// own tools, so a read-only prefix alone does not exempt one.
var writeVerbs = []string{
	"write", "edit", "replace", "delete", "remove", "move", "rename", "create",
	"update", "put", "set", "patch", "append", "save", "upload", "copy", "mkdir",
	programRm, "unlink", "truncate", "chmod", "link", "modify", "change", "apply",
	"insert", "overwrite", "store", "exec", "run",
}

func mentionsWrite(name string) bool {
	lower := strings.ToLower(name)

	for _, verb := range writeVerbs {
		if strings.Contains(lower, verb) {
			return true
		}
	}

	return false
}

func readsOnly(name string) bool {
	lower := strings.ToLower(name)

	for _, verb := range readOnlyToolVerbs {
		if lower == verb || strings.HasPrefix(lower, verb+"_") ||
			strings.HasPrefix(lower, verb+"-") {
			return true
		}

		if len(name) > len(verb) && strings.HasPrefix(lower, verb) {
			if next := name[len(verb)]; next >= 'A' && next <= 'Z' {
				return true
			}
		}
	}

	return false
}

func normalizeName(name string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(name))
}
