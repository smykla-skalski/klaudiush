// Package main provides the CLI entry point for klaudiush.
package main

import (
	"github.com/cockroachdb/errors"

	internalconfig "github.com/smykla-skalski/klaudiush/internal/config"
	"github.com/smykla-skalski/klaudiush/internal/patterns"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func resolvePatternProjectDir(
	workDir string,
	log logger.Logger,
) (string, error) {
	projectDir, err := internalconfig.ResolveProjectRoot(workDir)
	if err != nil {
		return "", errors.Wrap(err, "resolving pattern project root")
	}

	log.Debug("resolved pattern project root",
		"workDir", workDir,
		"projectDir", projectDir,
	)

	return projectDir, nil
}

func loadPatternStore(
	cfg *config.PatternsConfig,
	workDir string,
	log logger.Logger,
) (*patterns.FilePatternStore, error) {
	projectDir, err := resolvePatternProjectDir(workDir, log)
	if err != nil {
		return nil, err
	}

	store := patterns.NewFilePatternStore(cfg, projectDir)
	if err := store.Load(); err != nil {
		log.Debug("failed to load pattern store", "error", err)
	}

	if cfg.IsUseSeedData() {
		if err := patterns.EnsureSeedData(store); err != nil {
			log.Debug("failed to ensure seed data", "error", err)
		}
	}

	return store, nil
}
