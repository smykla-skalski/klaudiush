package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// binaryFileMode is the permission mode for extracted binary files.
const binaryFileMode = 0o755

// ExtractBinaryFromTarGz extracts a named binary from a .tar.gz archive.
// Returns the path to the extracted binary and a cleanup function.
//
//nolint:gosec // G304: archivePath is a temp file we just downloaded, not user-controlled
func ExtractBinaryFromTarGz(archivePath, binaryName string) (string, func(), error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", nil, errors.Wrap(err, "opening archive")
	}
	defer f.Close() //nolint:errcheck // read-only file

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", nil, errors.Wrap(err, "creating gzip reader")
	}
	defer gz.Close() //nolint:errcheck // read-only decompressor

	tr := tar.NewReader(gz)

	tmpDir, err := os.MkdirTemp("", "klaudiush-update-*")
	if err != nil {
		return "", nil, errors.Wrap(err, "creating temp directory")
	}

	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			cleanup()

			return "", nil, errors.Wrap(err, "reading tar entry")
		}

		// Match the binary name at any path depth (e.g. "klaudiush" or "dist/klaudiush")
		if filepath.Base(header.Name) != binaryName {
			continue
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		dest, pathErr := safePath(tmpDir, binaryName)
		if pathErr != nil {
			cleanup()

			return "", nil, pathErr
		}

		path, writeErr := extractToFile(dest, tr)
		if writeErr != nil {
			cleanup()

			return "", nil, writeErr
		}

		return path, cleanup, nil
	}

	cleanup()

	return "", nil, errors.Errorf("binary %q not found in archive", binaryName)
}

// ExtractBinaryFromZip extracts a named binary from a .zip archive.
// Returns the path to the extracted binary and a cleanup function.
func ExtractBinaryFromZip(archivePath, binaryName string) (string, func(), error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", nil, errors.Wrap(err, "opening zip archive")
	}
	defer r.Close() //nolint:errcheck // read-only zip

	tmpDir, err := os.MkdirTemp("", "klaudiush-update-*")
	if err != nil {
		return "", nil, errors.Wrap(err, "creating temp directory")
	}

	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	// On Windows the binary name includes .exe
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if base != binaryName && base != binaryName+".exe" {
			continue
		}

		if f.FileInfo().IsDir() {
			continue
		}

		dest, pathErr := safePath(tmpDir, base)
		if pathErr != nil {
			cleanup()

			return "", nil, pathErr
		}

		rc, openErr := f.Open()
		if openErr != nil {
			cleanup()

			return "", nil, errors.Wrap(openErr, "opening zip entry")
		}

		path, writeErr := extractToFile(dest, rc)

		_ = rc.Close()

		if writeErr != nil {
			cleanup()

			return "", nil, writeErr
		}

		return path, cleanup, nil
	}

	cleanup()

	return "", nil, errors.Errorf("binary %q not found in zip archive", binaryName)
}

// safePath validates that name resolves to a path within baseDir, preventing
// path traversal (Zip Slip) attacks from crafted archive entries.
func safePath(baseDir, name string) (string, error) {
	dest := filepath.Join(baseDir, name)

	// Clean both paths and verify the destination is within baseDir.
	cleanBase := filepath.Clean(baseDir) + string(os.PathSeparator)
	cleanDest := filepath.Clean(dest)

	if !strings.HasPrefix(cleanDest, cleanBase) {
		return "", errors.Errorf("path traversal attempt: %q escapes %q", name, baseDir)
	}

	return cleanDest, nil
}

// extractToFile writes data from reader to destPath with executable permissions.
//
//nolint:gosec // G304: destPath is within our temp directory, not user-controlled
func extractToFile(destPath string, reader io.Reader) (string, error) {
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, binaryFileMode)
	if err != nil {
		return "", errors.Wrap(err, "creating extracted file")
	}

	_, copyErr := io.Copy(out, reader)

	if closeErr := out.Close(); closeErr != nil && copyErr == nil {
		return "", errors.Wrap(closeErr, "closing extracted file")
	}

	if copyErr != nil {
		return "", errors.Wrap(copyErr, "extracting binary")
	}

	return destPath, nil
}

// ReplaceBinary atomically replaces the binary at targetPath with newPath.
// Writes to a temporary file in the same directory, then renames.
//
//nolint:gosec // G304/G703: paths are internal (extracted binary, current executable)
func ReplaceBinary(newPath, targetPath string) error {
	// Read new binary
	newData, err := os.ReadFile(newPath)
	if err != nil {
		return errors.Wrap(err, "reading new binary")
	}

	// Get permissions from existing binary
	info, err := os.Stat(targetPath)
	if err != nil {
		return errors.Wrap(err, "stat target binary")
	}

	// Write to temp file in the same directory (required for atomic rename)
	dir := filepath.Dir(targetPath)
	tmpFile := filepath.Join(dir, ".klaudiush-update-tmp")

	if err := os.WriteFile(tmpFile, newData, info.Mode()); err != nil {
		return errors.Wrap(err, "writing temporary binary")
	}

	// Atomic rename
	if err := os.Rename(tmpFile, targetPath); err != nil {
		_ = os.Remove(tmpFile)

		return errors.Wrap(err, "replacing binary")
	}

	return nil
}

// CurrentBinaryPath returns the resolved path to the currently running binary.
// Resolves symlinks so that updates target the real file (e.g. for homebrew installs).
func CurrentBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", errors.Wrap(err, "getting executable path")
	}

	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", errors.Wrap(err, "resolving symlinks")
	}

	return resolved, nil
}
