package protection_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/parser"
)

type env struct {
	root    string
	project string
	home    string
	opts    protection.Options
}

func newEnv(root string, goos string, cfg *config.ProtectionConfig) *env {
	resolved, err := filepath.EvalSymlinks(root)
	Expect(err).NotTo(HaveOccurred())

	e := &env{
		root:    resolved,
		project: filepath.Join(resolved, "project"),
		home:    filepath.Join(resolved, "home"),
	}

	e.write("project/.klaudiush/config.toml", "[protection]\nenabled = true\n")
	e.write("project/.claude/settings.json", "{}")
	e.write("project/main.go", "package main\n")
	e.write("home/.config/klaudiush/config.toml", "")
	e.write("home/.local/state/klaudiush/hook_sessions/state.json", "{}")
	Expect(os.MkdirAll(filepath.Join(e.project, ".git"), 0o755)).To(Succeed())

	e.opts = protection.Options{
		WorkDir:     e.project,
		ProjectRoot: e.project,
		Home:        e.home,
		ConfigDir:   filepath.Join(e.home, ".config", "klaudiush"),
		StateDir:    filepath.Join(e.home, ".local", "state", "klaudiush"),
		DataDir:     filepath.Join(e.home, ".local", "share", "klaudiush"),
		LegacyDir:   filepath.Join(e.home, ".klaudiush"),
		Executables: []string{filepath.Join(e.home, "bin", "klaudiush")},
		Config:      cfg,
		LookupEnv: func(name string) (string, bool) {
			if name == "HOME" {
				return e.home, true
			}

			if name == "PATH" {
				return filepath.Join(e.home, "bin") + ":relative", true
			}

			return "", false
		},
		GOOS: goos,
	}

	return e
}

func (e *env) write(rel, content string) string {
	path := filepath.Join(e.root, rel)
	Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())

	return path
}

func (e *env) set() *protection.Set {
	set, err := protection.NewSet(e.opts)
	Expect(err).NotTo(HaveOccurred())

	return set
}

func commandNamed(name string, args ...string) parser.Command {
	return parser.Command{Name: name, Invoked: name, Args: args}
}
