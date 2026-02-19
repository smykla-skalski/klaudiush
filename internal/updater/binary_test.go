package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractBinaryFromTarGz(t *testing.T) {
	t.Run("extracts binary at root level", func(t *testing.T) {
		archivePath := createTestTarGz(t, "klaudiush", []byte("binary-content"))

		path, cleanup, err := ExtractBinaryFromTarGz(archivePath, "klaudiush")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		defer cleanup()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading extracted file: %v", err)
		}

		if string(data) != "binary-content" {
			t.Errorf("extracted content = %q, want %q", string(data), "binary-content")
		}
	})

	t.Run("extracts binary in subdirectory", func(t *testing.T) {
		archivePath := createTestTarGzWithPath(t, "dist/klaudiush", []byte("nested-binary"))

		path, cleanup, err := ExtractBinaryFromTarGz(archivePath, "klaudiush")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		defer cleanup()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading extracted file: %v", err)
		}

		if string(data) != "nested-binary" {
			t.Errorf("extracted content = %q, want %q", string(data), "nested-binary")
		}
	})

	t.Run("binary not found", func(t *testing.T) {
		archivePath := createTestTarGz(t, "other-binary", []byte("content"))

		_, _, err := ExtractBinaryFromTarGz(archivePath, "klaudiush")
		if err == nil {
			t.Fatal("expected error for missing binary")
		}

		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("error = %q, want to contain 'not found'", err.Error())
		}
	})

	t.Run("invalid archive", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "bad.tar.gz")
		if err := os.WriteFile(tmpFile, []byte("not a tar.gz"), 0o644); err != nil {
			t.Fatal(err)
		}

		_, _, err := ExtractBinaryFromTarGz(tmpFile, "klaudiush")
		if err == nil {
			t.Fatal("expected error for invalid archive")
		}
	})
}

func TestExtractBinaryFromZip(t *testing.T) {
	t.Run("extracts binary", func(t *testing.T) {
		archivePath := createTestZip(t, "klaudiush.exe", []byte("exe-content"))

		path, cleanup, err := ExtractBinaryFromZip(archivePath, "klaudiush")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		defer cleanup()

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading extracted file: %v", err)
		}

		if string(data) != "exe-content" {
			t.Errorf("extracted content = %q, want %q", string(data), "exe-content")
		}
	})

	t.Run("binary not found", func(t *testing.T) {
		archivePath := createTestZip(t, "other.exe", []byte("content"))

		_, _, err := ExtractBinaryFromZip(archivePath, "klaudiush")
		if err == nil {
			t.Fatal("expected error for missing binary")
		}
	})
}

func TestReplaceBinary(t *testing.T) {
	t.Run("replaces binary atomically", func(t *testing.T) {
		dir := t.TempDir()

		// Create "existing" binary
		target := filepath.Join(dir, "klaudiush")
		if err := os.WriteFile(target, []byte("old-binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		// Create "new" binary
		newBin := filepath.Join(dir, "new-klaudiush")
		if err := os.WriteFile(newBin, []byte("new-binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := ReplaceBinary(newBin, target); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("reading replaced file: %v", err)
		}

		if string(data) != "new-binary" {
			t.Errorf("content = %q, want %q", string(data), "new-binary")
		}

		// Verify the file is executable
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode()&0o111 == 0 {
			t.Error("replaced binary is not executable")
		}
	})

	t.Run("target does not exist", func(t *testing.T) {
		dir := t.TempDir()
		newBin := filepath.Join(dir, "new")

		if err := os.WriteFile(newBin, []byte("content"), 0o755); err != nil {
			t.Fatal(err)
		}

		err := ReplaceBinary(newBin, filepath.Join(dir, "nonexistent"))
		if err == nil {
			t.Error("expected error when target does not exist")
		}
	})
}

// createTestTarGz creates a tar.gz archive with a single file.
func createTestTarGz(t *testing.T, name string, content []byte) string {
	t.Helper()

	return createTestTarGzWithPath(t, name, content)
}

// createTestTarGzWithPath creates a tar.gz archive with a file at the given path.
func createTestTarGzWithPath(t *testing.T, name string, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.tar.gz")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	header := &tar.Header{
		Name: name,
		Size: int64(len(content)),
		Mode: 0o755,
	}

	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}

	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	return path
}

// createTestZip creates a zip archive with a single file.
func createTestZip(t *testing.T, name string, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.zip")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}

	return path
}
