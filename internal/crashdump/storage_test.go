//nolint:govet // Test code commonly shadows err variables
package crashdump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewFilesystemStorage(t *testing.T) {
	tests := []struct {
		name        string
		dumpDir     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid absolute path",
			dumpDir: t.TempDir(),
			wantErr: false,
		},
		{
			name:    "valid relative path",
			dumpDir: "test_dumps",
			wantErr: false,
		},
		{
			name:        "empty path",
			dumpDir:     "",
			wantErr:     true,
			errContains: "dump directory cannot be empty",
		},
		{
			name:    "home directory expansion",
			dumpDir: "~/.test_dumps",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := NewFilesystemStorage(tt.dumpDir)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}

				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)

				return
			}

			if storage == nil {
				t.Error("expected non-nil storage")
			}
		})
	}
}

func TestFilesystemStorage_ExistsAndInitialize(t *testing.T) {
	tmpDir := t.TempDir()
	dumpDir := filepath.Join(tmpDir, "crash_dumps")

	storage, err := NewFilesystemStorage(dumpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Directory doesn't exist yet
	if storage.Exists() {
		t.Error("directory should not exist yet")
	}

	// Initialize should create it
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Now it should exist
	if !storage.Exists() {
		t.Error("directory should exist after Initialize()")
	}

	// Verify directory permissions
	dirInfo, err := os.Stat(dumpDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}

	if dirInfo.Mode().Perm() != DirPerm {
		t.Errorf("directory permissions = %o, want %o", dirInfo.Mode().Perm(), DirPerm)
	}
}

func TestFilesystemStorage_List(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	// Empty directory returns empty list
	summaries, err := storage.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 0 {
		t.Errorf("expected empty list, got %d items", len(summaries))
	}

	// Create test dumps
	dumps := []struct {
		id         string
		timestamp  time.Time
		panicValue string
	}{
		{
			id:         "crash-20250101T120000-aaa",
			timestamp:  testTime,
			panicValue: "panic 1",
		},
		{
			id:         "crash-20250101T120001-bbb",
			timestamp:  testTime.Add(1 * time.Second),
			panicValue: "panic 2",
		},
		{
			id:         "crash-20250101T120002-ccc",
			timestamp:  testTime.Add(2 * time.Second),
			panicValue: "panic 3",
		},
	}

	// Initialize directory
	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Write test dumps
	for _, dump := range dumps {
		info := &CrashInfo{
			ID:         dump.id,
			Timestamp:  dump.timestamp,
			PanicValue: dump.panicValue,
			StackTrace: "test stack",
			Runtime: RuntimeInfo{
				GOOS: "linux",
			},
			Metadata: DumpMetadata{
				Version: "v1.0.0",
			},
		}

		data, err := json.Marshal(info)
		if err != nil {
			t.Fatalf("failed to marshal dump: %v", err)
		}

		filePath := filepath.Join(tmpDir, dump.id+FileExtension)
		if err := os.WriteFile(filePath, data, FilePerm); err != nil {
			t.Fatalf("failed to write dump: %v", err)
		}
	}

	// List dumps
	summaries, err = storage.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	// Verify count
	if len(summaries) != len(dumps) {
		t.Errorf("List() returned %d items, want %d", len(summaries), len(dumps))
	}

	// Verify sorting (newest first)
	for i := range summaries {
		expectedIdx := len(dumps) - 1 - i
		expected := dumps[expectedIdx]

		if summaries[i].ID != expected.id {
			t.Errorf("summaries[%d].ID = %q, want %q", i, summaries[i].ID, expected.id)
		}

		if summaries[i].PanicValue != expected.panicValue {
			t.Errorf(
				"summaries[%d].PanicValue = %q, want %q",
				i,
				summaries[i].PanicValue,
				expected.panicValue,
			)
		}

		if !summaries[i].Timestamp.Equal(expected.timestamp) {
			t.Errorf(
				"summaries[%d].Timestamp = %v, want %v",
				i,
				summaries[i].Timestamp,
				expected.timestamp,
			)
		}

		if summaries[i].Size <= 0 {
			t.Errorf("summaries[%d].Size = %d, want > 0", i, summaries[i].Size)
		}
	}
}

func TestFilesystemStorage_List_TruncatesPanicValue(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Create dump with long panic value
	longPanic := strings.Repeat("a", 150)
	info := &CrashInfo{
		ID:         "crash-20250101T120000-test",
		Timestamp:  testTime,
		PanicValue: longPanic,
		StackTrace: "test stack",
		Runtime: RuntimeInfo{
			GOOS: "linux",
		},
		Metadata: DumpMetadata{
			Version: "v1.0.0",
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal dump: %v", err)
	}

	filePath := filepath.Join(tmpDir, info.ID+FileExtension)
	if err := os.WriteFile(filePath, data, FilePerm); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	// List dumps
	summaries, err := storage.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	// Verify truncation
	if len(summaries[0].PanicValue) > 83 { // 80 chars + "..."
		t.Errorf("panic value not truncated: length = %d", len(summaries[0].PanicValue))
	}

	if !strings.HasSuffix(summaries[0].PanicValue, "...") {
		t.Error("truncated panic value should end with '...'")
	}
}

func TestFilesystemStorage_List_SkipsInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Create valid dump
	validInfo := &CrashInfo{
		ID:         "crash-valid",
		Timestamp:  testTime,
		PanicValue: "valid panic",
		StackTrace: "test",
		Runtime: RuntimeInfo{
			GOOS: "linux",
		},
		Metadata: DumpMetadata{
			Version: "v1.0.0",
		},
	}

	validData, err := json.Marshal(validInfo)
	if err != nil {
		t.Fatalf("failed to marshal valid dump: %v", err)
	}

	validPath := filepath.Join(tmpDir, "crash-valid.json")
	if err := os.WriteFile(validPath, validData, FilePerm); err != nil {
		t.Fatalf("failed to write valid dump: %v", err)
	}

	// Create invalid JSON file
	invalidPath := filepath.Join(tmpDir, "crash-invalid.json")
	if err := os.WriteFile(invalidPath, []byte("{invalid json}"), FilePerm); err != nil {
		t.Fatalf("failed to write invalid dump: %v", err)
	}

	// Create non-JSON file (should be ignored)
	readmePath := filepath.Join(tmpDir, "readme.txt")
	if err := os.WriteFile(readmePath, []byte("test"), FilePerm); err != nil {
		t.Fatalf("failed to write non-JSON file: %v", err)
	}

	// Create subdirectory (should be ignored)
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), DirPerm); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	// List should only return valid dump
	summaries, err := storage.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != 1 {
		t.Errorf("expected 1 valid summary, got %d", len(summaries))
	}

	if len(summaries) > 0 && summaries[0].ID != "crash-valid" {
		t.Errorf("summary.ID = %q, want 'crash-valid'", summaries[0].ID)
	}
}

func TestFilesystemStorage_Get(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Create test dump
	expected := &CrashInfo{
		ID:         "crash-20250101T120000-test",
		Timestamp:  testTime,
		PanicValue: "test panic",
		StackTrace: "test stack trace",
		Runtime: RuntimeInfo{
			GOOS:         "darwin",
			GOARCH:       "arm64",
			GoVersion:    "go1.23.0",
			NumGoroutine: 5,
			NumCPU:       8,
		},
		Metadata: DumpMetadata{
			Version:    "v1.0.0",
			User:       "testuser",
			Hostname:   "testhost",
			WorkingDir: "/test/dir",
		},
	}

	data, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal dump: %v", err)
	}

	filePath := filepath.Join(tmpDir, expected.ID+FileExtension)
	if err := os.WriteFile(filePath, data, FilePerm); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	// Get dump
	info, err := storage.Get(expected.ID)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	// Verify all fields
	if info.ID != expected.ID {
		t.Errorf("ID = %q, want %q", info.ID, expected.ID)
	}

	if !info.Timestamp.Equal(expected.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", info.Timestamp, expected.Timestamp)
	}

	if info.PanicValue != expected.PanicValue {
		t.Errorf("PanicValue = %q, want %q", info.PanicValue, expected.PanicValue)
	}

	if info.StackTrace != expected.StackTrace {
		t.Errorf("StackTrace = %q, want %q", info.StackTrace, expected.StackTrace)
	}

	if info.Runtime.GOOS != expected.Runtime.GOOS {
		t.Errorf("Runtime.GOOS = %q, want %q", info.Runtime.GOOS, expected.Runtime.GOOS)
	}

	if info.Metadata.Version != expected.Metadata.Version {
		t.Errorf("Metadata.Version = %q, want %q", info.Metadata.Version, expected.Metadata.Version)
	}
}

func TestFilesystemStorage_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	_, err = storage.Get("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent dump")
	}

	if !strings.Contains(err.Error(), "crash dump not found") {
		t.Errorf("error = %q, want to contain 'crash dump not found'", err.Error())
	}
}

func TestFilesystemStorage_Delete(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Create test dump
	info := &CrashInfo{
		ID:         "crash-to-delete",
		Timestamp:  testTime,
		PanicValue: "test",
		StackTrace: "test",
		Runtime: RuntimeInfo{
			GOOS: "linux",
		},
		Metadata: DumpMetadata{
			Version: "v1.0.0",
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal dump: %v", err)
	}

	filePath := filepath.Join(tmpDir, info.ID+FileExtension)
	if err := os.WriteFile(filePath, data, FilePerm); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	// Delete dump
	if err := storage.Delete(info.ID); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}

func TestFilesystemStorage_Delete_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	err = storage.Delete("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent dump")
	}

	if !strings.Contains(err.Error(), "crash dump not found") {
		t.Errorf("error = %q, want to contain 'crash dump not found'", err.Error())
	}
}

func TestFilesystemStorage_Prune_ByAge(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	now := time.Now()

	// Create dumps at different ages
	dumps := []struct {
		id        string
		timestamp time.Time
	}{
		{
			id:        "crash-old-1",
			timestamp: now.Add(-40 * 24 * time.Hour), // 40 days old
		},
		{
			id:        "crash-old-2",
			timestamp: now.Add(-35 * 24 * time.Hour), // 35 days old
		},
		{
			id:        "crash-recent",
			timestamp: now.Add(-5 * 24 * time.Hour), // 5 days old
		},
	}

	for _, dump := range dumps {
		info := &CrashInfo{
			ID:         dump.id,
			Timestamp:  dump.timestamp,
			PanicValue: "test",
			StackTrace: "test",
			Runtime: RuntimeInfo{
				GOOS: "linux",
			},
			Metadata: DumpMetadata{
				Version: "v1.0.0",
			},
		}

		data, err := json.Marshal(info)
		if err != nil {
			t.Fatalf("failed to marshal dump: %v", err)
		}

		filePath := filepath.Join(tmpDir, dump.id+FileExtension)
		if err := os.WriteFile(filePath, data, FilePerm); err != nil {
			t.Fatalf("failed to write dump: %v", err)
		}
	}

	// Prune dumps older than 30 days
	maxAge := 30 * 24 * time.Hour

	removed, err := storage.Prune(100, maxAge)
	if err != nil {
		t.Fatalf("Prune() failed: %v", err)
	}

	// Should remove 2 old dumps
	if removed != 2 {
		t.Errorf("Prune() removed %d dumps, want 2", removed)
	}

	// Verify recent dump still exists
	if _, err := storage.Get("crash-recent"); err != nil {
		t.Errorf("recent dump should still exist: %v", err)
	}

	// Verify old dumps are gone
	if _, err := storage.Get("crash-old-1"); err == nil {
		t.Error("old dump 1 should be deleted")
	}

	if _, err := storage.Get("crash-old-2"); err == nil {
		t.Error("old dump 2 should be deleted")
	}
}

func TestFilesystemStorage_Prune_ByCount(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	now := time.Now()

	// Create 5 dumps
	const totalDumps = 5

	for i := range totalDumps {
		info := &CrashInfo{
			ID:         generateCrashID(now.Add(time.Duration(i)*time.Second), "test"),
			Timestamp:  now.Add(time.Duration(i) * time.Second),
			PanicValue: "test",
			StackTrace: "test",
			Runtime: RuntimeInfo{
				GOOS: "linux",
			},
			Metadata: DumpMetadata{
				Version: "v1.0.0",
			},
		}

		data, err := json.Marshal(info)
		if err != nil {
			t.Fatalf("failed to marshal dump: %v", err)
		}

		filePath := filepath.Join(tmpDir, info.ID+FileExtension)
		if err := os.WriteFile(filePath, data, FilePerm); err != nil {
			t.Fatalf("failed to write dump: %v", err)
		}
	}

	// Keep only 3 newest dumps
	maxDumps := 3

	removed, err := storage.Prune(maxDumps, 0)
	if err != nil {
		t.Fatalf("Prune() failed: %v", err)
	}

	// Should remove 2 oldest dumps
	if removed != 2 {
		t.Errorf("Prune() removed %d dumps, want 2", removed)
	}

	// Verify remaining count
	summaries, err := storage.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != maxDumps {
		t.Errorf("List() returned %d dumps, want %d", len(summaries), maxDumps)
	}
}

func TestFilesystemStorage_Prune_EmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if err := storage.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Prune empty directory should not fail
	removed, err := storage.Prune(10, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Prune() failed: %v", err)
	}

	if removed != 0 {
		t.Errorf("Prune() removed %d dumps, want 0", removed)
	}
}

func TestFilesystemStorage_GetDumpDir(t *testing.T) {
	dumpDir := "/test/dumps"

	storage, err := NewFilesystemStorage(dumpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	if got := storage.GetDumpDir(); got != dumpDir {
		t.Errorf("GetDumpDir() = %q, want %q", got, dumpDir)
	}
}
