package xdg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

func TestResolveFile(t *testing.T) {
	dir := t.TempDir()

	xdgPath := filepath.Join(dir, "xdg", "config.toml")
	legacyPath := filepath.Join(dir, "legacy", "config.toml")

	t.Run("neither exists returns XDG", func(t *testing.T) {
		got := xdg.ResolveFile(xdgPath, legacyPath)
		if got != xdgPath {
			t.Errorf("ResolveFile() = %q, want %q", got, xdgPath)
		}
	})

	t.Run("legacy exists returns legacy", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(legacyPath, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}

		got := xdg.ResolveFile(xdgPath, legacyPath)
		if got != legacyPath {
			t.Errorf("ResolveFile() = %q, want %q", got, legacyPath)
		}
	})

	t.Run("both exist returns XDG", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Dir(xdgPath), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(xdgPath, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}

		got := xdg.ResolveFile(xdgPath, legacyPath)
		if got != xdgPath {
			t.Errorf("ResolveFile() = %q, want %q", got, xdgPath)
		}
	})

	t.Run("XDG exists returns XDG", func(t *testing.T) {
		_ = os.Remove(legacyPath)

		got := xdg.ResolveFile(xdgPath, legacyPath)
		if got != xdgPath {
			t.Errorf("ResolveFile() = %q, want %q", got, xdgPath)
		}
	})
}

func TestResolveDir(t *testing.T) {
	dir := t.TempDir()

	xdgDir := filepath.Join(dir, "xdg", "plugins")
	legacyDir := filepath.Join(dir, "legacy", "plugins")

	t.Run("neither exists returns XDG", func(t *testing.T) {
		got := xdg.ResolveDir(xdgDir, legacyDir)
		if got != xdgDir {
			t.Errorf("ResolveDir() = %q, want %q", got, xdgDir)
		}
	})

	t.Run("legacy exists returns legacy", func(t *testing.T) {
		if err := os.MkdirAll(legacyDir, 0o700); err != nil {
			t.Fatal(err)
		}

		got := xdg.ResolveDir(xdgDir, legacyDir)
		if got != legacyDir {
			t.Errorf("ResolveDir() = %q, want %q", got, legacyDir)
		}
	})

	t.Run("both exist returns XDG", func(t *testing.T) {
		if err := os.MkdirAll(xdgDir, 0o700); err != nil {
			t.Fatal(err)
		}

		got := xdg.ResolveDir(xdgDir, legacyDir)
		if got != xdgDir {
			t.Errorf("ResolveDir() = %q, want %q", got, xdgDir)
		}
	})
}
