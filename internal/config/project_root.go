// Package config provides internal configuration loading and processing.
package config

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	internalgit "github.com/smykla-skalski/klaudiush/internal/git"
)

// ResolveProjectRoot resolves the project root for the given work directory.
// Resolution order is:
//  1. directory containing the walked-up project config
//  2. git repository root
//  3. the work directory itself
func ResolveProjectRoot(workDir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", errors.Wrap(err, "getting home directory")
	}

	return ResolveProjectRootWithDirs(homeDir, workDir)
}

// ResolveProjectRootWithDirs resolves the project root using explicit home and
// work directories. This is primarily intended for tests.
func ResolveProjectRootWithDirs(homeDir, workDir string) (string, error) {
	baseDir, err := resolveBaseWorkDir(workDir)
	if err != nil {
		return "", err
	}

	loader, err := NewKoanfLoaderWithDirs(homeDir, baseDir)
	if err != nil {
		return "", errors.Wrap(err, "creating config loader")
	}

	if projectConfigPath := loader.FindProjectConfigPath(); projectConfigPath != "" {
		return projectRootFromConfigPath(projectConfigPath), nil
	}

	if gitRoot := resolveGitProjectRoot(baseDir); gitRoot != "" {
		return gitRoot, nil
	}

	return baseDir, nil
}

func resolveBaseWorkDir(workDir string) (string, error) {
	if workDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", errors.Wrap(err, "getting working directory")
		}

		workDir = cwd
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", errors.Wrap(err, "resolving absolute working directory")
	}

	return filepath.Clean(absWorkDir), nil
}

func projectRootFromConfigPath(configPath string) string {
	cleanPath := filepath.Clean(configPath)
	configDir := filepath.Dir(cleanPath)

	if filepath.Base(configDir) == ProjectConfigDir {
		return filepath.Dir(configDir)
	}

	return configDir
}

func resolveGitProjectRoot(workDir string) string {
	repo, err := internalgit.OpenRepository(workDir)
	if err != nil {
		return ""
	}

	root, err := repo.GetRoot()
	if err != nil {
		return ""
	}

	return filepath.Clean(root)
}
