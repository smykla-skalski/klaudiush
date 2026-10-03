// Package policy provides validators that keep the agent from weakening the
// policy enforced on it: protected policy files and trusted MCP servers.
package policy

import (
	"os"
	"path/filepath"

	"github.com/smykla-skalski/klaudiush/internal/protection"
	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// maxRootSearch bounds how many directories are climbed looking for the
// repository root.
const maxRootSearch = 64

// Locator says where the policy files live for one hook.
type Locator func(hookCtx *hook.Context) protection.Options

// NewLocator returns the Locator for the running system: XDG directories,
// the home directory, the running binary and the configured hook, plugin
// and evidence files.
func NewLocator(cfg *config.Config) Locator {
	return func(hookCtx *hook.Context) protection.Options {
		workDir := hookCtx.GetWorkingDir()
		if workDir == "" {
			workDir, _ = os.Getwd()
		}

		home, _ := os.UserHomeDir()

		return protection.Options{
			WorkDir:          workDir,
			ProjectRoot:      ProjectRoot(workDir),
			Home:             home,
			ConfigDir:        xdg.ConfigDir(),
			StateDir:         xdg.StateDir(),
			DataDir:          xdg.DataDir(),
			LegacyDir:        xdg.LegacyDir(),
			XDGConfigHome:    xdg.ConfigHome(),
			HookFiles:        hookFiles(cfg),
			Executables:      executables(home),
			OpenCodePlugin:   openCodePlugin(cfg),
			EvidenceCommands: evidenceCommands(cfg),
			PluginPaths:      pluginPaths(cfg),
			Config:           protectionConfig(cfg),
		}
	}
}

// ProjectRoot returns the closest directory at or above dir holding .git,
// or dir when there is none.
func ProjectRoot(dir string) string {
	current := filepath.Clean(dir)

	for range maxRootSearch {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}

		current = parent
	}

	return dir
}

func protectionConfig(cfg *config.Config) *config.ProtectionConfig {
	if cfg == nil {
		return nil
	}

	return cfg.Protection
}

func executables(home string) []string {
	var paths []string

	if exe, err := os.Executable(); err == nil {
		paths = append(paths, exe)
	}

	if home != "" {
		paths = append(paths, filepath.Join(home, ".claude", "hooks", "dispatcher"))
	}

	return paths
}

func hookFiles(cfg *config.Config) []string {
	if cfg == nil || cfg.Providers == nil {
		return nil
	}

	var files []string

	if cfg.Providers.Codex.HasHooksConfigPath() {
		files = append(files, xdg.ExpandPathSilent(cfg.Providers.Codex.HooksConfigPath))
	}

	if cfg.Providers.Gemini.HasSettingsPath() {
		files = append(files, xdg.ExpandPathSilent(cfg.Providers.Gemini.SettingsPath))
	}

	return files
}

func openCodePlugin(cfg *config.Config) string {
	if cfg == nil || cfg.Providers == nil || !cfg.Providers.OpenCode.HasPluginPath() {
		return ""
	}

	return xdg.ExpandPathSilent(cfg.Providers.OpenCode.PluginPath)
}

func evidenceCommands(cfg *config.Config) []string {
	if cfg == nil || cfg.Evidence == nil {
		return nil
	}

	var commands []string

	for _, check := range cfg.Evidence.Checks {
		if check != nil {
			commands = append(commands, check.Commands...)
		}
	}

	return commands
}

func pluginPaths(cfg *config.Config) []string {
	if cfg == nil || cfg.Plugins == nil {
		return nil
	}

	var paths []string

	for _, plugin := range cfg.Plugins.Plugins {
		if plugin != nil && plugin.Path != "" {
			paths = append(paths, xdg.ExpandPathSilent(plugin.Path))
		}
	}

	return paths
}
