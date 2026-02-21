package xdg_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/smykla-skalski/klaudiush/internal/xdg"
)

func TestConfigHome(t *testing.T) {
	t.Run("respects XDG_CONFIG_HOME", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "/custom/config")

		got := xdg.ConfigHome()
		if got != "/custom/config" {
			t.Errorf("ConfigHome() = %q, want /custom/config", got)
		}
	})

	t.Run("defaults to ~/.config", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")

		got := xdg.ConfigHome()
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".config")

		if got != want {
			t.Errorf("ConfigHome() = %q, want %q", got, want)
		}
	})
}

func TestDataHome(t *testing.T) {
	t.Run("respects XDG_DATA_HOME", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "/custom/data")

		got := xdg.DataHome()
		if got != "/custom/data" {
			t.Errorf("DataHome() = %q, want /custom/data", got)
		}
	})

	t.Run("defaults to ~/.local/share", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "")

		got := xdg.DataHome()
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".local", "share")

		if got != want {
			t.Errorf("DataHome() = %q, want %q", got, want)
		}
	})
}

func TestStateHome(t *testing.T) {
	t.Run("respects XDG_STATE_HOME", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "/custom/state")

		got := xdg.StateHome()
		if got != "/custom/state" {
			t.Errorf("StateHome() = %q, want /custom/state", got)
		}
	})

	t.Run("defaults to ~/.local/state", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", "")

		got := xdg.StateHome()
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".local", "state")

		if got != want {
			t.Errorf("StateHome() = %q, want %q", got, want)
		}
	})
}

func TestCacheHome(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")

	got := xdg.CacheHome()
	if got != "/custom/cache" {
		t.Errorf("CacheHome() = %q, want /custom/cache", got)
	}
}

func TestConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")

	got := xdg.ConfigDir()
	if got != "/xdg/config/klaudiush" {
		t.Errorf("ConfigDir() = %q, want /xdg/config/klaudiush", got)
	}
}

func TestDataDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := xdg.DataDir()
	if got != "/xdg/data/klaudiush" {
		t.Errorf("DataDir() = %q, want /xdg/data/klaudiush", got)
	}
}

func TestStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")

	got := xdg.StateDir()
	if got != "/xdg/state/klaudiush" {
		t.Errorf("StateDir() = %q, want /xdg/state/klaudiush", got)
	}
}

func TestGlobalConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg/config")

	got := xdg.GlobalConfigFile()
	want := "/xdg/config/klaudiush/config.toml"

	if got != want {
		t.Errorf("GlobalConfigFile() = %q, want %q", got, want)
	}
}

func TestLogFile(t *testing.T) {
	t.Run("respects KLAUDIUSH_LOG_FILE", func(t *testing.T) {
		t.Setenv("KLAUDIUSH_LOG_FILE", "/custom/log.txt")

		got := xdg.LogFile()
		if got != "/custom/log.txt" {
			t.Errorf("LogFile() = %q, want /custom/log.txt", got)
		}
	})

	t.Run("defaults to state dir", func(t *testing.T) {
		t.Setenv("KLAUDIUSH_LOG_FILE", "")
		t.Setenv("XDG_STATE_HOME", "/xdg/state")

		got := xdg.LogFile()
		want := "/xdg/state/klaudiush/dispatcher.log"

		if got != want {
			t.Errorf("LogFile() = %q, want %q", got, want)
		}
	})
}

func TestExceptionStateFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := xdg.ExceptionStateFile()
	want := "/xdg/data/klaudiush/exceptions/state.json"

	if got != want {
		t.Errorf("ExceptionStateFile() = %q, want %q", got, want)
	}
}

func TestExceptionAuditFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")

	got := xdg.ExceptionAuditFile()
	want := "/xdg/state/klaudiush/exception_audit.jsonl"

	if got != want {
		t.Errorf("ExceptionAuditFile() = %q, want %q", got, want)
	}
}

func TestCrashDumpDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := xdg.CrashDumpDir()
	want := "/xdg/data/klaudiush/crash_dumps"

	if got != want {
		t.Errorf("CrashDumpDir() = %q, want %q", got, want)
	}
}

func TestPatternsGlobalDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := xdg.PatternsGlobalDir()
	want := "/xdg/data/klaudiush/patterns"

	if got != want {
		t.Errorf("PatternsGlobalDir() = %q, want %q", got, want)
	}
}

func TestBackupDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := xdg.BackupDir()
	want := "/xdg/data/klaudiush/backups"

	if got != want {
		t.Errorf("BackupDir() = %q, want %q", got, want)
	}
}

func TestPluginDir(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/xdg/data")

	got := xdg.PluginDir()
	want := "/xdg/data/klaudiush/plugins"

	if got != want {
		t.Errorf("PluginDir() = %q, want %q", got, want)
	}
}

func TestMigrationMarker(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg/state")

	got := xdg.MigrationMarker()
	want := "/xdg/state/klaudiush/.migration_v2"

	if got != want {
		t.Errorf("MigrationMarker() = %q, want %q", got, want)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "absolute", input: "/usr/bin", want: "/usr/bin"},
		{name: "relative", input: "foo/bar", want: "foo/bar"},
		{name: "tilde alone", input: "~", want: home},
		{name: "tilde slash", input: "~/foo/bar", want: filepath.Join(home, "foo/bar")},
		{name: "tilde user", input: "~other", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xdg.ExpandPath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ExpandPath(%q) expected error, got nil", tt.input)
				}

				return
			}

			if err != nil {
				t.Errorf("ExpandPath(%q) unexpected error: %v", tt.input, err)

				return
			}

			if got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandPathSilent(t *testing.T) {
	t.Run("returns expanded path", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		got := xdg.ExpandPathSilent("~/foo")
		want := filepath.Join(home, "foo")

		if got != want {
			t.Errorf("ExpandPathSilent(~/foo) = %q, want %q", got, want)
		}
	})

	t.Run("returns original on error", func(t *testing.T) {
		got := xdg.ExpandPathSilent("~other")
		if got != "~other" {
			t.Errorf("ExpandPathSilent(~other) = %q, want ~other", got)
		}
	})
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a", "b", "c")

	if err := xdg.EnsureDir(target); err != nil {
		t.Fatalf("EnsureDir() error: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}

	if !info.IsDir() {
		t.Error("EnsureDir() did not create a directory")
	}

	if info.Mode().Perm() != 0o700 {
		t.Errorf("EnsureDir() mode = %o, want 0700", info.Mode().Perm())
	}
}

func TestLegacyDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := xdg.LegacyDir()
	want := filepath.Join(home, ".klaudiush")

	if got != want {
		t.Errorf("LegacyDir() = %q, want %q", got, want)
	}
}

func TestLegacyLogFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := xdg.LegacyLogFile()
	want := filepath.Join(home, ".claude", "hooks", "dispatcher.log")

	if got != want {
		t.Errorf("LegacyLogFile() = %q, want %q", got, want)
	}
}
