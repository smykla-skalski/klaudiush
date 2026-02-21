package suggest

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const (
	// filePermissions is the permission mode for generated KLAUDIUSH.md.
	// 0644 because this file is meant to be committed to the repo.
	filePermissions = 0o644

	// dirPermissions is the permission mode for parent directories.
	dirPermissions = 0o755
)

// Generator orchestrates KLAUDIUSH.md generation.
type Generator struct {
	cfg     *config.Config
	version string
}

// NewGenerator creates a new Generator.
func NewGenerator(cfg *config.Config, ver string) *Generator {
	return &Generator{
		cfg:     cfg,
		version: ver,
	}
}

// Generate produces the full KLAUDIUSH.md content.
func (g *Generator) Generate() (string, error) {
	hash, err := ComputeHash(g.cfg)
	if err != nil {
		return "", errors.Wrap(err, "computing config hash")
	}

	data := Collect(g.cfg, g.version)
	data.Hash = hash

	content, err := Render(data)
	if err != nil {
		return "", errors.Wrap(err, "rendering template")
	}

	return content, nil
}

// Check compares the hash in an existing file against the current config hash.
// Returns true if the file is up-to-date, false if stale or missing.
func (g *Generator) Check(filePath string) (bool, error) {
	currentHash, err := ComputeHash(g.cfg)
	if err != nil {
		return false, errors.Wrap(err, "computing current hash")
	}

	existingHash, err := ExtractHash(filePath)
	if err != nil {
		// File doesn't exist or can't be read — stale
		return false, nil //nolint:nilerr // missing file means stale, not error
	}

	return currentHash == existingHash, nil
}

// WriteFile writes content to a file atomically using tmp+rename.
func (g *Generator) WriteFile(filePath, content string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return errors.Wrap(err, "creating output directory")
	}

	tmpPath := filePath + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(content), filePermissions); err != nil {
		return errors.Wrap(err, "writing temp file")
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)

		return errors.Wrap(err, "renaming temp file")
	}

	return nil
}
