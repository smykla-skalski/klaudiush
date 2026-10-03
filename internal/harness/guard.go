package harness

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
)

// guardedFiles are the real-home files a leaking harness run would touch:
// hook registrations and harness and klaudiush configuration. Files that a
// running agent session rewrites on its own (~/.claude.json, credential
// files a token refresh rewrites, transcripts, the klaudiush log) are left
// out so the guard does not report the caller's own activity.
var guardedFiles = []string{
	".claude/settings.json",
	".claude/settings.local.json",
	".codex/config.toml",
	".codex/hooks.json",
	".klaudiush/config.toml",
	".gemini/settings.json",
}

// guardedConfigFiles live under XDG_CONFIG_HOME.
var guardedConfigFiles = []string{
	"klaudiush/config.toml",
	"opencode/opencode.json",
	"opencode/plugin/klaudiush.ts",
}

type fileStamp struct {
	exists  bool
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

// HomeGuard detects changes to the real home directory's harness and
// klaudiush configuration across a live run. It compares file metadata
// only and never opens the files.
type HomeGuard struct {
	stamps map[string]fileStamp
}

// GuardHome records the guarded files under home and configHome (the real
// XDG_CONFIG_HOME, or home/.config when empty).
func GuardHome(home, configHome string) (*HomeGuard, error) {
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}

	paths := make([]string, 0, len(guardedFiles)+len(guardedConfigFiles))
	for _, rel := range guardedFiles {
		paths = append(paths, filepath.Join(home, rel))
	}

	for _, rel := range guardedConfigFiles {
		paths = append(paths, filepath.Join(configHome, rel))
	}

	guard := &HomeGuard{stamps: make(map[string]fileStamp, len(paths))}

	for _, path := range paths {
		stamp, err := stampFile(path)
		if err != nil {
			return nil, err
		}

		guard.stamps[path] = stamp
	}

	return guard, nil
}

// Paths lists the guarded files.
func (g *HomeGuard) Paths() []string {
	paths := make([]string, 0, len(g.stamps))
	for path := range g.stamps {
		paths = append(paths, path)
	}

	slices.Sort(paths)

	return paths
}

// Changed lists guarded files created, removed or modified since GuardHome.
func (g *HomeGuard) Changed() ([]string, error) {
	var changed []string

	for _, path := range g.Paths() {
		stamp, err := stampFile(path)
		if err != nil {
			return nil, err
		}

		if stamp != g.stamps[path] {
			changed = append(changed, path)
		}
	}

	return changed, nil
}

func stampFile(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fileStamp{}, nil
	}

	if err != nil {
		return fileStamp{}, errors.Wrapf(err, "stat %s", path)
	}

	return fileStamp{
		exists:  true,
		size:    info.Size(),
		mode:    info.Mode(),
		modTime: info.ModTime(),
	}, nil
}
