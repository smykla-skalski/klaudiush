package xdg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func TestNeedsMigration(t *testing.T) {
	t.Run("no legacy dir means no migration needed", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

		if xdg.NeedsMigration() {
			t.Error("NeedsMigration() = true, want false (no legacy dir)")
		}
	})

	t.Run("legacy dir exists means migration needed", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

		if err := os.MkdirAll(filepath.Join(tmp, ".klaudiush"), 0o700); err != nil {
			t.Fatal(err)
		}

		if !xdg.NeedsMigration() {
			t.Error("NeedsMigration() = false, want true (legacy dir exists)")
		}
	})

	t.Run("v2 marker means no migration needed", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

		// Create legacy dir
		if err := os.MkdirAll(filepath.Join(tmp, ".klaudiush"), 0o700); err != nil {
			t.Fatal(err)
		}

		// Create v2 marker
		markerDir := filepath.Join(tmp, "state", "klaudiush")
		if err := os.MkdirAll(markerDir, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(markerDir, ".migration_v2"),
			[]byte("v2"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		if xdg.NeedsMigration() {
			t.Error("NeedsMigration() = true, want false (marker exists)")
		}
	})
}

func TestMigrate(t *testing.T) {
	log := logger.NewNoOpLogger()

	t.Run("fresh install just writes marker", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
		t.Setenv("KLAUDIUSH_LOG_FILE", "")

		result, err := xdg.Migrate(log)
		if err != nil {
			t.Fatalf("Migrate() error: %v", err)
		}

		if result.Moved != 0 {
			t.Errorf("Moved = %d, want 0", result.Moved)
		}

		// Marker should exist
		marker := filepath.Join(tmp, "state", "klaudiush", ".migration_v2")
		if _, err := os.Stat(marker); err != nil {
			t.Errorf("migration marker not created: %v", err)
		}
	})

	t.Run("moves config file", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
		t.Setenv("KLAUDIUSH_LOG_FILE", "")

		// Create legacy config
		legacyDir := filepath.Join(tmp, ".klaudiush")
		if err := os.MkdirAll(legacyDir, 0o700); err != nil {
			t.Fatal(err)
		}

		legacyConfig := filepath.Join(legacyDir, "config.toml")
		if err := os.WriteFile(legacyConfig, []byte("version = 1"), 0o600); err != nil {
			t.Fatal(err)
		}

		result, err := xdg.Migrate(log)
		if err != nil {
			t.Fatalf("Migrate() error: %v", err)
		}

		if result.Moved < 1 {
			t.Errorf("Moved = %d, want >= 1", result.Moved)
		}

		// Config should be at XDG location
		xdgConfig := filepath.Join(tmp, "config", "klaudiush", "config.toml")
		if _, err := os.Stat(xdgConfig); err != nil {
			t.Errorf("config not at XDG location: %v", err)
		}
	})

	t.Run("moves data directories", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
		t.Setenv("KLAUDIUSH_LOG_FILE", "")

		// Create legacy crash dumps
		legacyDir := filepath.Join(tmp, ".klaudiush", "crash_dumps")
		if err := os.MkdirAll(legacyDir, 0o700); err != nil {
			t.Fatal(err)
		}

		dumpFile := filepath.Join(legacyDir, "crash-test.json")
		if err := os.WriteFile(dumpFile, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}

		result, err := xdg.Migrate(log)
		if err != nil {
			t.Fatalf("Migrate() error: %v", err)
		}

		if result.Moved < 1 {
			t.Errorf("Moved = %d, want >= 1", result.Moved)
		}

		// Crash dump should be at XDG location
		xdgDump := filepath.Join(tmp, "data", "klaudiush", "crash_dumps", "crash-test.json")
		if _, err := os.Stat(xdgDump); err != nil {
			t.Errorf("crash dump not at XDG location: %v", err)
		}
	})

	t.Run("idempotent - second run is no-op", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
		t.Setenv("KLAUDIUSH_LOG_FILE", "")

		// Create legacy dir
		legacyDir := filepath.Join(tmp, ".klaudiush")
		if err := os.MkdirAll(legacyDir, 0o700); err != nil {
			t.Fatal(err)
		}

		// First run
		_, err := xdg.Migrate(log)
		if err != nil {
			t.Fatalf("First Migrate() error: %v", err)
		}

		// Second run should be no-op
		result, err := xdg.Migrate(log)
		if err != nil {
			t.Fatalf("Second Migrate() error: %v", err)
		}

		if result.Moved != 0 {
			t.Errorf("Second Migrate() moved = %d, want 0", result.Moved)
		}
	})

	t.Run("skips when dest already exists", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
		t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
		t.Setenv("KLAUDIUSH_LOG_FILE", "")

		// Create legacy config
		legacyDir := filepath.Join(tmp, ".klaudiush")
		if err := os.MkdirAll(legacyDir, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(legacyDir, "config.toml"),
			[]byte("old"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		// Create XDG config (already exists)
		xdgDir := filepath.Join(tmp, "config", "klaudiush")
		if err := os.MkdirAll(xdgDir, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(xdgDir, "config.toml"),
			[]byte("new"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}

		_, err := xdg.Migrate(log)
		if err != nil {
			t.Fatalf("Migrate() error: %v", err)
		}

		// XDG config should still have "new" content
		data, _ := os.ReadFile(filepath.Join(xdgDir, "config.toml"))
		if string(data) != "new" {
			t.Errorf("XDG config content = %q, want %q", string(data), "new")
		}
	})
}
