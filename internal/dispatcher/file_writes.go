package dispatcher

import (
	"path/filepath"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// maxChangedFileChecks caps how many provider-reported changed files one
// shell command gets validated for, keeping the hook inside its timeout.
const maxChangedFileChecks = 50

// fileWriteTarget is one file a shell command writes. checkedBeforeTool
// reports whether pre-tool validation already saw the exact bytes written.
type fileWriteTarget struct {
	path              string
	content           string
	checkedBeforeTool bool
}

// bashWriteTargets lists the files a shell command writes. Before the tool
// runs, each target carries the content the parser recovered from the
// command. Afterwards the content is left empty so validators read the file
// as the command left it, and the files the provider reports as changed are
// added to the parsed ones.
func bashWriteTargets(bashCtx *hook.Context, writes []parser.FileWrite) []fileWriteTarget {
	if !bashCtx.IsAfterTool() {
		targets := make([]fileWriteTarget, 0, len(writes))

		for _, fw := range writes {
			targets = append(targets, fileWriteTarget{path: fw.Path, content: fw.Content})
		}

		return targets
	}

	targets := make([]fileWriteTarget, 0, len(writes)+len(bashCtx.ChangedFiles))
	seen := make(map[string]int, cap(targets))

	for _, fw := range writes {
		path := resolveWritePath(bashCtx.WorkingDir, fw)

		if i, ok := seen[path]; ok {
			targets[i].checkedBeforeTool = targets[i].checkedBeforeTool && fw.ContentCaptured

			continue
		}

		seen[path] = len(targets)
		targets = append(targets, fileWriteTarget{
			path:              path,
			checkedBeforeTool: fw.ContentCaptured,
		})
	}

	added := 0

	for _, changed := range bashCtx.ChangedFiles {
		path := filepath.Clean(changed)
		if _, ok := seen[path]; ok || added >= maxChangedFileChecks {
			continue
		}

		seen[path] = len(targets)
		targets = append(targets, fileWriteTarget{path: path})
		added++
	}

	return targets
}

// resolveWritePath makes a parsed write target absolute, so it can be read
// after the command ran and matched against the provider's changed files.
func resolveWritePath(workingDir string, fw parser.FileWrite) string {
	if filepath.IsAbs(fw.Path) {
		return filepath.Clean(fw.Path)
	}

	base := fw.WorkingDirectory
	if !filepath.IsAbs(base) {
		base = filepath.Join(workingDir, base)
	}

	if !filepath.IsAbs(base) {
		return filepath.Clean(fw.Path)
	}

	return filepath.Join(base, fw.Path)
}

// repeatsBeforeTool reports whether findings for a target only repeat what
// Claude's PreToolUse already showed. Claude denies a call on a blocking
// pre-tool finding, so once the command succeeded only its warnings can come
// back, unless the file changed after pre-tool validation saw it.
func repeatsBeforeTool(bashCtx *hook.Context, target fileWriteTarget) bool {
	return bashCtx.Provider == hook.ProviderClaude &&
		bashCtx.IsAfterTool() &&
		!bashCtx.ToolFailed() &&
		target.checkedBeforeTool
}

// blockingOnly drops non-blocking findings.
func blockingOnly(errs []*ValidationError) []*ValidationError {
	kept := make([]*ValidationError, 0, len(errs))

	for _, verr := range errs {
		if verr.ShouldBlock {
			kept = append(kept, verr)
		}
	}

	return kept
}
