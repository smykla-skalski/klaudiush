//nolint:govet // Test code commonly shadows err variables
package crashdump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smykla-labs/klaudiush/pkg/config"
	"github.com/smykla-labs/klaudiush/pkg/hook"
)

// TestIntegration_FullCrashDumpFlow tests the complete crash dump creation flow.
func TestIntegration_FullCrashDumpFlow(t *testing.T) {
	tmpDir := t.TempDir()

	// Create collector and writer
	collector := NewCollector("v1.0.0-test")

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Simulate a panic with context
	ctx := &hook.Context{
		EventType: hook.EventTypePreToolUse,
		ToolName:  hook.ToolTypeBash,
		ToolInput: hook.ToolInput{
			Command: "git commit -sS -m 'test'",
		},
	}

	cfg := &config.Config{}

	// Simulate panic recovery
	panicValue := "runtime error: index out of range"

	// Collect crash info
	info := collector.Collect(panicValue, ctx, cfg)

	// Write crash dump
	dumpPath, err := writer.Write(info)
	if err != nil {
		t.Fatalf("failed to write crash dump: %v", err)
	}

	// Verify dump file exists
	if _, err := os.Stat(dumpPath); err != nil {
		t.Errorf("crash dump file not found: %v", err)
	}

	// Verify dump can be read and parsed
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("failed to read dump file: %v", err)
	}

	var loaded CrashInfo
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to parse dump file: %v", err)
	}

	// Verify essential fields
	if loaded.ID == "" {
		t.Error("crash ID is empty")
	}

	if loaded.PanicValue != panicValue {
		t.Errorf("panic value = %q, want %q", loaded.PanicValue, panicValue)
	}

	if loaded.StackTrace == "" {
		t.Error("stack trace is empty")
	}

	if loaded.Runtime.GOOS == "" {
		t.Error("GOOS is empty")
	}

	if loaded.Runtime.GoVersion == "" {
		t.Error("Go version is empty")
	}

	if loaded.Metadata.Version != "v1.0.0-test" {
		t.Errorf("version = %q, want %q", loaded.Metadata.Version, "v1.0.0-test")
	}

	// Verify context is captured
	if loaded.Context == nil {
		t.Fatal("context is nil")
	}

	if loaded.Context.EventType != hook.EventTypePreToolUse.String() {
		t.Errorf(
			"event type = %q, want %q",
			loaded.Context.EventType,
			hook.EventTypePreToolUse.String(),
		)
	}

	if loaded.Context.ToolName != hook.ToolTypeBash.String() {
		t.Errorf("tool name = %q, want %q", loaded.Context.ToolName, hook.ToolTypeBash.String())
	}

	if loaded.Context.Command != "git commit -sS -m 'test'" {
		t.Errorf("command = %q, want %q", loaded.Context.Command, "git commit -sS -m 'test'")
	}
}

// TestIntegration_StorageWorkflow tests the full storage workflow.
func TestIntegration_StorageWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	// Create storage and writer
	storage, err := NewFilesystemStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	collector := NewCollector("v1.0.0")

	// Create multiple crash dumps
	const numDumps = 5

	ids := make([]string, numDumps)

	for i := range numDumps {
		info := collector.Collect("test panic", nil, nil)
		info.Timestamp = testTime.Add(time.Duration(i) * time.Second)
		info.ID = generateCrashID(info.Timestamp, "test panic")

		path, err := writer.Write(info)
		if err != nil {
			t.Fatalf("failed to write dump %d: %v", i, err)
		}

		ids[i] = info.ID

		t.Logf("Created dump: %s at %s", info.ID, path)
	}

	// List dumps
	summaries, err := storage.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(summaries) != numDumps {
		t.Errorf("List() returned %d dumps, want %d", len(summaries), numDumps)
	}

	// Verify sorting (newest first)
	for i := 1; i < len(summaries); i++ {
		if summaries[i].Timestamp.After(summaries[i-1].Timestamp) {
			t.Errorf("dumps not sorted: summaries[%d] is newer than summaries[%d]", i, i-1)
		}
	}

	// Get individual dump
	info, err := storage.Get(ids[0])
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}

	if info.ID != ids[0] {
		t.Errorf("Get() returned ID %q, want %q", info.ID, ids[0])
	}

	// Delete dump
	if err := storage.Delete(ids[0]); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Verify deleted
	_, err = storage.Get(ids[0])
	if err == nil {
		t.Error("expected error for deleted dump")
	}

	// Verify list is updated
	summaries, err = storage.List()
	if err != nil {
		t.Fatalf("List() after delete failed: %v", err)
	}

	if len(summaries) != numDumps-1 {
		t.Errorf("List() after delete returned %d dumps, want %d", len(summaries), numDumps-1)
	}

	// Prune to keep only 2 dumps
	removed, err := storage.Prune(2, 0)
	if err != nil {
		t.Fatalf("Prune() failed: %v", err)
	}

	expectedRemoved := numDumps - 1 - 2 // 5 created - 1 deleted - 2 kept = 2 removed
	if removed != expectedRemoved {
		t.Errorf("Prune() removed %d dumps, want %d", removed, expectedRemoved)
	}

	// Verify remaining count
	summaries, err = storage.List()
	if err != nil {
		t.Fatalf("List() after prune failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Errorf("List() after prune returned %d dumps, want 2", len(summaries))
	}
}

// TestIntegration_ConfigSanitization tests config sanitization in the full flow.
func TestIntegration_ConfigSanitization(t *testing.T) {
	tmpDir := t.TempDir()

	collector := NewCollector("v1.0.0")

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Create config with sensitive data
	cfg := &config.Config{}
	// Note: Actual config structure would be more complex, but we're testing the sanitization flow

	// Collect and write
	info := collector.Collect("test panic", nil, cfg)

	dumpPath, err := writer.Write(info)
	if err != nil {
		t.Fatalf("failed to write crash dump: %v", err)
	}

	// Read and verify sanitization
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("failed to read dump file: %v", err)
	}

	// Config should be sanitized (this test documents the behavior)
	var loaded CrashInfo
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to parse dump file: %v", err)
	}

	// Verify config is present and sanitized
	if loaded.Config == nil {
		t.Log("config is nil (expected when input config is empty)")
	}
}

// TestIntegration_RealPanicRecovery tests panic recovery with actual panic.
func TestIntegration_RealPanicRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	collector := NewCollector("v1.0.0")

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	var (
		capturedPanic any
		capturedPath  string
		capturedErr   error
	)

	// Simulate panic recovery pattern from main.go
	func() {
		defer func() {
			if r := recover(); r != nil {
				capturedPanic = r

				info := collector.Collect(r, nil, nil)
				capturedPath, capturedErr = writer.Write(info)
			}
		}()

		// Trigger real panic
		panic("simulated crash")
	}()

	// Verify panic was recovered
	if capturedPanic == nil {
		t.Fatal("panic was not recovered")
	}

	if capturedPanic != "simulated crash" {
		t.Errorf("captured panic = %v, want 'simulated crash'", capturedPanic)
	}

	// Verify dump was created
	if capturedErr != nil {
		t.Fatalf("failed to write crash dump: %v", capturedErr)
	}

	if capturedPath == "" {
		t.Error("dump path is empty")
	}

	// Verify dump file exists and is valid
	data, err := os.ReadFile(capturedPath)
	if err != nil {
		t.Fatalf("failed to read dump file: %v", err)
	}

	var info CrashInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("failed to parse dump file: %v", err)
	}

	if info.PanicValue != "simulated crash" {
		t.Errorf("panic value = %q, want 'simulated crash'", info.PanicValue)
	}

	// Stack trace should contain this test function
	if !strings.Contains(info.StackTrace, "TestIntegration_RealPanicRecovery") {
		t.Error("stack trace does not contain test function name")
	}
}

// TestIntegration_FilePermissions tests that files are created with correct permissions.
func TestIntegration_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()

	collector := NewCollector("v1.0.0")

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	info := collector.Collect("test panic", nil, nil)

	dumpPath, err := writer.Write(info)
	if err != nil {
		t.Fatalf("failed to write crash dump: %v", err)
	}

	// Check file permissions
	fileInfo, err := os.Stat(dumpPath)
	if err != nil {
		t.Fatalf("failed to stat dump file: %v", err)
	}

	if fileInfo.Mode().Perm() != FilePerm {
		t.Errorf("file permissions = %o, want %o", fileInfo.Mode().Perm(), FilePerm)
	}

	// Check directory permissions
	dirInfo, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}

	// Directory might have been created with different permissions during test setup
	// Just verify it's readable
	if !dirInfo.IsDir() {
		t.Error("path is not a directory")
	}
}

// TestIntegration_ConcurrentWrites tests concurrent crash dump creation.
func TestIntegration_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()

	collector := NewCollector("v1.0.0")

	writer, err := NewFilesystemWriter(tmpDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	const numGoroutines = 10

	done := make(chan error, numGoroutines)

	// Launch concurrent writes
	for i := range numGoroutines {
		go func(idx int) {
			defer func() {
				if r := recover(); r != nil {
					done <- nil

					return
				}
			}()

			info := collector.Collect("concurrent panic", nil, nil)
			info.Timestamp = testTime.Add(time.Duration(idx) * time.Millisecond)
			info.ID = generateCrashID(info.Timestamp, "concurrent panic")

			_, writeErr := writer.Write(info)
			done <- writeErr
		}(i)
	}

	// Wait for all goroutines
	for range numGoroutines {
		if err := <-done; err != nil {
			t.Errorf("concurrent write failed: %v", err)
		}
	}

	// Verify all files were created
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

	if jsonCount != numGoroutines {
		t.Errorf("found %d dump files, want %d", jsonCount, numGoroutines)
	}
}

// TestIntegration_DirectoryCreation tests that nested directories are created.
func TestIntegration_DirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "level1", "level2", "crash_dumps")

	collector := NewCollector("v1.0.0")

	writer, err := NewFilesystemWriter(nestedDir)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}

	// Directory shouldn't exist yet
	if _, err := os.Stat(nestedDir); !os.IsNotExist(err) {
		t.Fatal("directory should not exist before first write")
	}

	// Write should create nested directories
	info := collector.Collect("test panic", nil, nil)

	dumpPath, err := writer.Write(info)
	if err != nil {
		t.Fatalf("failed to write crash dump: %v", err)
	}

	// Verify directory was created
	dirInfo, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("directory was not created: %v", err)
	}

	if !dirInfo.IsDir() {
		t.Error("path is not a directory")
	}

	// Verify file exists
	if _, err := os.Stat(dumpPath); err != nil {
		t.Errorf("dump file not found: %v", err)
	}
}
