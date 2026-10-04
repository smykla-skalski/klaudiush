package dispatcher

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

// maxChangedFileChecks caps how many provider-reported changed files one
// shell command gets validated for, keeping the hook inside its timeout.
const maxChangedFileChecks = 10

// fileWriteTarget is one file a shell command writes. Before the command runs
// content is what the parser recovered; afterwards it is empty so validators
// read the file from disk. captured holds the exact bytes pre-tool validation
// saw, when the parser could recover them.
type fileWriteTarget struct {
	path        string
	content     string
	captured    string
	hasCaptured bool
}

// bashWriteTargets lists the files a shell command writes. After the command
// ran, the files the provider reports as changed and the files with unresolved
// findings that changed on disk are added to the parsed ones.
func bashWriteTargets(
	bashCtx *hook.Context,
	writes []parser.FileWrite,
	resolver parser.Resolver,
) []fileWriteTarget {
	writes = slices.DeleteFunc(slices.Clone(writes), func(fw parser.FileWrite) bool {
		return fw.TargetUnknown
	})

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
		path := resolveWritePath(bashCtx.WorkingDir, fw, resolver)

		if i, ok := seen[path]; ok {
			targets[i].hasCaptured = false

			continue
		}

		seen[path] = len(targets)
		targets = append(targets, fileWriteTarget{
			path:        path,
			captured:    fw.Content,
			hasCaptured: fw.ContentCaptured,
		})
	}

	added := 0

	for _, changed := range bashCtx.ChangedFiles {
		path := canonicalPath(changed)
		if _, ok := seen[path]; ok || added >= maxChangedFileChecks {
			continue
		}

		seen[path] = len(targets)
		targets = append(targets, fileWriteTarget{path: path})
		added++
	}

	for _, path := range bashCtx.RecheckFiles {
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
// after the command ran and matched against the provider's changed files. A
// ~ in the target or in the directory a cd moved to is the home directory,
// not a directory under the hook's one.
func resolveWritePath(workingDir string, fw parser.FileWrite, resolver parser.Resolver) string {
	path := parser.ExpandHome(fw.Path, resolver)
	if filepath.IsAbs(path) {
		return canonicalPath(path)
	}

	base := parser.ExpandHome(fw.WorkingDirectory, resolver)
	if !filepath.IsAbs(base) && !strings.HasPrefix(base, "~") {
		base = filepath.Join(workingDir, base)
	}

	if !filepath.IsAbs(base) || strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}

	return canonicalPath(filepath.Join(base, path))
}

// canonicalPath resolves symlinks, so a path reached through a symlinked
// directory matches the real path the provider reports.
func canonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}

	return resolved
}

// unchangedSinceBeforeTool reports whether the file holds exactly the bytes
// pre-tool validation already checked, so checking it again would only repeat
// those findings.
func unchangedSinceBeforeTool(target fileWriteTarget) bool {
	if !target.hasCaptured {
		return false
	}

	info, err := os.Stat(target.path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}

	data, err := os.ReadFile(target.path)
	if err != nil {
		return false
	}

	return bytes.Equal(data, []byte(target.captured))
}

// namedAfter prefixes each finding with the file it is about. Linters report
// a temporary copy, and one command can change several files.
func namedAfter(path string, errs []*ValidationError) []*ValidationError {
	named := make([]*ValidationError, 0, len(errs))

	for _, verr := range errs {
		withPath := *verr
		withPath.Message = path + ": " + verr.Message
		named = append(named, &withPath)
	}

	return named
}

// advisory turns blocking findings into warnings. Findings about a file as a
// tool left it are advisory: the change already happened, and with no record
// of the file before it there is no telling which problems the tool caused.
func advisory(errs []*ValidationError) []*ValidationError {
	result := make([]*ValidationError, 0, len(errs))

	for _, verr := range errs {
		if !verr.ShouldBlock {
			result = append(result, verr)

			continue
		}

		downgraded := *verr
		downgraded.ShouldBlock = false
		result = append(result, &downgraded)
	}

	return result
}
