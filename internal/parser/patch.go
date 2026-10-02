package parser

import (
	"encoding/json"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// moveApplyPatchText handles Codex apply_patch, which carries the patch text in
// tool_input.command. A patch is not a shell command, so it moves to the
// "input" key the path extraction reads, keeping shell-command predicates from
// matching patch text.
func moveApplyPatchText(rawToolName string, toolInput *hook.ToolInput) {
	if normalizeToolName(rawToolName) != toolApplyPatch || toolInput.Command == "" {
		return
	}

	if _, ok := toolInput.Additional[patchInputKey]; !ok {
		encoded, err := json.Marshal(toolInput.Command)
		if err == nil {
			toolInput.Additional[patchInputKey] = encoded
		}
	}

	toolInput.Command = ""
}

// applyPatchFiles shapes each file an apply_patch touches as a Write or Edit,
// so file validators see the edit before it lands:
//   - an added file reads as a Write of its full content
//   - a single-hunk update reads as an Edit with old and new text
//   - other updates and deletions expose their added lines as NewString
//
// A move yields entries for both the source and the destination path. When
// exactly one file results, it is also copied onto toolInput.
func applyPatchFiles(rawToolName string, toolInput *hook.ToolInput) []hook.PatchFile {
	if normalizeToolName(rawToolName) != toolApplyPatch ||
		toolInput.Content != "" || toolInput.NewString != "" {
		return nil
	}

	var files []hook.PatchFile

	for _, section := range parsePatch(patchInputText(toolInput.Additional)) {
		files = append(files, section.files()...)
	}

	if len(files) == 1 {
		input := files[0].Input
		toolInput.FilePath = input.FilePath
		toolInput.Content = input.Content
		toolInput.OldString = input.OldString
		toolInput.NewString = input.NewString
	}

	return files
}

type patchSectionKind int

const (
	patchAddFile patchSectionKind = iota + 1
	patchUpdateFile
	patchDeleteFile
)

type patchSection struct {
	kind     patchSectionKind
	path     string
	movePath string
	hunks    int
	added    []string
	old      []string
	new      []string
}

func parsePatch(patchText string) []patchSection {
	var (
		sections []patchSection
		current  *patchSection
	)

	for line := range strings.SplitSeq(patchText, "\n") {
		line = strings.TrimSuffix(line, "\r")

		if kind, path, ok := patchHeader(line); ok {
			sections = append(sections, patchSection{kind: kind, path: path})
			current = &sections[len(sections)-1]

			continue
		}

		if current == nil {
			continue
		}

		if dest, ok := strings.CutPrefix(line, "*** Move to: "); ok {
			current.movePath = strings.TrimSpace(dest)

			continue
		}

		if strings.HasPrefix(line, "*** ") {
			continue
		}

		current.addLine(line)
	}

	return sections
}

func patchHeader(line string) (patchSectionKind, string, bool) {
	headers := []struct {
		prefix string
		kind   patchSectionKind
	}{
		{"*** Add File: ", patchAddFile},
		{"*** Update File: ", patchUpdateFile},
		{"*** Delete File: ", patchDeleteFile},
	}

	for _, header := range headers {
		if path, ok := strings.CutPrefix(line, header.prefix); ok {
			return header.kind, strings.TrimSpace(path), true
		}
	}

	return 0, "", false
}

func (s *patchSection) addLine(line string) {
	switch {
	case strings.HasPrefix(line, "@@"):
		s.hunks++
	case strings.HasPrefix(line, "+"):
		text := strings.TrimPrefix(line, "+")
		s.added = append(s.added, text)
		s.new = append(s.new, text)
	case strings.HasPrefix(line, "-"):
		s.old = append(s.old, strings.TrimPrefix(line, "-"))
	case strings.HasPrefix(line, " "):
		text := strings.TrimPrefix(line, " ")
		s.old = append(s.old, text)
		s.new = append(s.new, text)
	}
}

func (s *patchSection) files() []hook.PatchFile {
	file := s.file(s.path)
	if s.movePath == "" {
		return []hook.PatchFile{file}
	}

	return []hook.PatchFile{file, s.file(s.movePath)}
}

func (s *patchSection) file(path string) hook.PatchFile {
	switch {
	case s.kind == patchAddFile:
		return hook.PatchFile{
			ToolName:   hook.ToolTypeWrite,
			ToolFamily: hook.ToolFamilyWrite,
			Input: hook.ToolInput{
				FilePath: path,
				Content:  strings.Join(s.added, "\n"),
			},
		}
	case s.kind == patchUpdateFile && s.movePath == "" && s.hunks == 1 && len(s.old) > 0:
		return hook.PatchFile{
			ToolName:   hook.ToolTypeEdit,
			ToolFamily: hook.ToolFamilyEdit,
			Input: hook.ToolInput{
				FilePath:  path,
				OldString: strings.Join(s.old, "\n"),
				NewString: strings.Join(s.new, "\n"),
			},
		}
	default:
		return hook.PatchFile{
			ToolName:   hook.ToolTypeEdit,
			ToolFamily: hook.ToolFamilyEdit,
			Input: hook.ToolInput{
				FilePath:  path,
				NewString: strings.Join(s.added, "\n"),
			},
		}
	}
}
