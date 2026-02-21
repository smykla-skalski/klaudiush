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

func TestNewFilesystemWriter(t *testing.T) {
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
			writer, err := NewFilesystemWriter(tt.dumpDir)

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

			if writer == nil {
				t.Error("expected non-nil writer")
			}

			if tt.dumpDir[0] == '~' {
				// Verify home directory was expanded
				if strings.HasPrefix(writer.GetDumpDir(), "~") {
					t.Error("home directory was not expanded")
				}
			}
		})
	}
}

func TestFilesystemWriter_Write(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	info := &CrashInfo{
		ID:         "crash-20250101T120000-abc123",
		Timestamp:  testTime,
		PanicValue: "test panic",
		StackTrace: "goroutine 1 [running]:\nmain.main()\n\t/test/main.go:10",
		Runtime: RuntimeInfo{
			GOOS:         "darwin",
			GOARCH:       "arm64",
			GoVersion:    "go1.23.0",
			NumGoroutine: 1,
			NumCPU:       8,
		},
		Metadata: DumpMetadata{
			Version: "v1.0.0",
		},
	}

	path, err := writer.Write(info)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Verify file path
	expectedPath := filepath.Join(tmpDir, info.ID+FileExtension)
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file does not exist: %v", err)
	}

	// Verify file permissions
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if fileInfo.Mode().Perm() != FilePerm {
		t.Errorf("file permissions = %o, want %o", fileInfo.Mode().Perm(), FilePerm)
	}

	// Verify file content is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var loaded CrashInfo
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify content matches
	if loaded.ID != info.ID {
		t.Errorf("loaded.ID = %q, want %q", loaded.ID, info.ID)
	}

	if loaded.PanicValue != info.PanicValue {
		t.Errorf("loaded.PanicValue = %q, want %q", loaded.PanicValue, info.PanicValue)
	}

	// Verify JSON is indented (human-readable)
	if !strings.Contains(string(data), "\n  ") {
		t.Error("JSON is not indented")
	}
}

func TestFilesystemWriter_Write_NilInfo(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	_, err = writer.Write(nil)
	if err == nil {
		t.Error("expected error for nil crash info")
	}

	if !strings.Contains(err.Error(), "crash info is nil") {
		t.Errorf("error message = %q, want to contain 'crash info is nil'", err.Error())
	}
}

func TestFilesystemWriter_Write_AtomicOperation(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	info := &CrashInfo{
		ID:         "crash-20250101T120000-test",
		Timestamp:  testTime,
		PanicValue: "atomic test",
		StackTrace: "test stack",
		Runtime: RuntimeInfo{
			GOOS: "linux",
		},
		Metadata: DumpMetadata{
			Version: "v1.0.0",
		},
	}

	// Write the file
	path, err := writer.Write(info)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Verify temp file doesn't exist after successful write
	tempPath := path + TempSuffix
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("temporary file still exists after write")
	}

	// Verify final file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("final file does not exist: %v", err)
	}
}

func TestFilesystemWriter_Write_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dumpDir := filepath.Join(tmpDir, "nested", "crash_dumps")

	// Directory doesn't exist yet
	if _, err := os.Stat(dumpDir); !os.IsNotExist(err) {
		t.Fatal("directory should not exist yet")
	}

	writer, err := NewFilesystemWriter(dumpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	info := &CrashInfo{
		ID:         "crash-20250101T120000-test",
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

	// Write should create the directory
	_, err = writer.Write(info)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// Verify directory exists with correct permissions
	dirInfo, err := os.Stat(dumpDir)
	if err != nil {
		t.Fatalf("directory was not created: %v", err)
	}

	if !dirInfo.IsDir() {
		t.Error("path is not a directory")
	}

	if dirInfo.Mode().Perm() != DirPerm {
		t.Errorf("directory permissions = %o, want %o", dirInfo.Mode().Perm(), DirPerm)
	}
}

func TestFilesystemWriter_Write_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Write multiple crash dumps
	const numDumps = 5

	for i := range numDumps {
		info := &CrashInfo{
			ID:         generateCrashID(testTime.Add(time.Duration(i)*time.Second), "test panic"),
			Timestamp:  testTime.Add(time.Duration(i) * time.Second),
			PanicValue: "test panic",
			StackTrace: "test stack",
			Runtime: RuntimeInfo{
				GOOS: "linux",
			},
			Metadata: DumpMetadata{
				Version: "v1.0.0",
			},
		}

		_, err := writer.Write(info)
		if err != nil {
			t.Errorf("Write() #%d failed: %v", i, err)
		}
	}

	// Verify all files exist
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	jsonCount := 0

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), FileExtension) {
			jsonCount++
		}
	}

	if jsonCount != numDumps {
		t.Errorf("found %d dump files, want %d", jsonCount, numDumps)
	}
}

func TestFilesystemWriter_GetDumpDir(t *testing.T) {
	dumpDir := "/test/dumps"

	writer, err := NewFilesystemWriter(dumpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	if got := writer.GetDumpDir(); got != dumpDir {
		t.Errorf("GetDumpDir() = %q, want %q", got, dumpDir)
	}
}

// TestFilesystemWriter_Write_InvalidJSON tests handling of crash info that can't be marshaled.
func TestFilesystemWriter_Write_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Create crash info with a channel, which can't be marshaled to JSON
	info := &CrashInfo{
		ID:         "crash-test",
		Timestamp:  testTime,
		PanicValue: "test",
		StackTrace: "test",
		Runtime: RuntimeInfo{
			GOOS: "linux",
		},
		Metadata: DumpMetadata{
			Version: "v1.0.0",
		},
		Config: map[string]any{
			"invalid": make(chan int), // channels can't be marshaled to JSON
		},
	}

	_, err = writer.Write(info)
	if err == nil {
		t.Error("expected error for invalid JSON content")
	}

	if !strings.Contains(err.Error(), "failed to marshal") {
		t.Errorf("error = %q, want to contain 'failed to marshal'", err.Error())
	}
}

// BenchmarkFilesystemWriter_Write measures write performance.
func BenchmarkFilesystemWriter_Write(b *testing.B) {
	tmpDir := b.TempDir()

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		b.Fatalf("failed to create writer: %v", err)
	}

	info := &CrashInfo{
		ID:         "crash-bench",
		Timestamp:  testTime,
		PanicValue: "benchmark panic",
		StackTrace: strings.Repeat("goroutine stack line\n", 100),
		Runtime: RuntimeInfo{
			GOOS:         "darwin",
			GOARCH:       "arm64",
			GoVersion:    "go1.23.0",
			NumGoroutine: 10,
			NumCPU:       8,
		},
		Metadata: DumpMetadata{
			Version:    "v1.0.0",
			User:       "benchuser",
			Hostname:   "bench-host",
			WorkingDir: "/bench/dir",
		},
		Config: map[string]any{
			"key1": "value1",
			"key2": 42,
			"key3": true,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		info.ID = generateCrashID(time.Now(), info.PanicValue)

		_, err := writer.Write(info)
		if err != nil {
			b.Fatalf("Write() failed: %v", err)
		}
	}
}
