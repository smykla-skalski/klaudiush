package crashdump

import (
	"testing"
	"time"

	"github.com/cockroachdb/errors"
)

var testTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func TestFormatPanicValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "nil value",
			value:    nil,
			expected: "panic(nil)",
		},
		{
			name:     "string value",
			value:    "something went wrong",
			expected: "something went wrong",
		},
		{
			name:     "error value",
			value:    errors.New("test error"),
			expected: "test error",
		},
		{
			name:     "wrapped error",
			value:    errors.Wrap(errors.New("inner"), "outer"),
			expected: "outer: inner",
		},
		{
			name:     "int value",
			value:    42,
			expected: "42",
		},
		{
			name:     "custom struct",
			value:    struct{ msg string }{msg: "custom panic"},
			expected: "{custom panic}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPanicValue(tt.value)
			if result != tt.expected {
				t.Errorf("formatPanicValue(%v) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}

// TestFormatPanicValueWithNilPanic tests the actual panic(nil) behavior.
// This must be in a separate test to properly test the recover mechanism.
func TestFormatPanicValueWithNilPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			result := formatPanicValue(r)
			// In Go 1.21+, panic(nil) becomes *runtime.PanicNilError
			// In earlier versions, r will be nil
			expected := "panic(nil)"
			if result != expected {
				t.Errorf(
					"formatPanicValue(recovered from panic(nil)) = %q, want %q",
					result,
					expected,
				)
			}
		} else {
			t.Error("expected panic but none occurred")
		}
	}()

	panic(nil) //nolint:govet // intentional panic(nil) for testing Go 1.21+ PanicNilError handling
}

func TestCollector_Collect(t *testing.T) {
	collector := NewCollector("v1.0.0")

	tests := []struct {
		name      string
		recovered any
	}{
		{
			name:      "string panic",
			recovered: "runtime error",
		},
		{
			name:      "error panic",
			recovered: errors.New("something failed"),
		},
		{
			name:      "nil panic",
			recovered: nil,
		},
		{
			name:      "int panic",
			recovered: 123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := collector.Collect(tt.recovered, nil, nil)

			if info == nil {
				t.Fatal("expected non-nil CrashInfo")
			}

			if info.ID == "" {
				t.Error("expected non-empty crash ID")
			}

			if info.PanicValue == "" {
				t.Error("expected non-empty panic value")
			}

			if info.StackTrace == "" {
				t.Error("expected non-empty stack trace")
			}

			if info.Runtime.GOOS == "" {
				t.Error("expected non-empty GOOS")
			}

			if info.Runtime.GoVersion == "" {
				t.Error("expected non-empty Go version")
			}

			if info.Metadata.Version != "v1.0.0" {
				t.Errorf("expected version v1.0.0, got %q", info.Metadata.Version)
			}

			// Verify panic value is properly formatted
			expected := formatPanicValue(tt.recovered)
			if info.PanicValue != expected {
				t.Errorf("PanicValue = %q, want %q", info.PanicValue, expected)
			}
		})
	}
}

func TestCollector_CollectWithContext(t *testing.T) {
	// This test would require importing hook.Context and creating a mock
	// For now, we verify that nil context doesn't crash
	collector := NewCollector("v1.0.0")
	info := collector.Collect("test panic", nil, nil)

	if info.Context != nil {
		t.Error("expected nil context when no context provided")
	}
}

func TestCaptureStack(t *testing.T) {
	stack := captureStack()

	if stack == "" {
		t.Error("expected non-empty stack trace")
	}

	// Stack trace should contain function information
	if len(stack) < 100 {
		t.Errorf("stack trace seems too short: %d bytes", len(stack))
	}
}

func TestGenerateCrashID(t *testing.T) {
	tests := []struct {
		name       string
		panicValue string
	}{
		{
			name:       "simple panic",
			panicValue: "error",
		},
		{
			name:       "empty panic",
			panicValue: "",
		},
		{
			name:       "long panic",
			panicValue: string(make([]byte, 1000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := testTime
			id := generateCrashID(now, tt.panicValue)

			// Verify format: crash-{timestamp}-{shortHash}
			expectedPrefix := "crash-20250101T120000-"
			if len(id) != len(expectedPrefix)+shortIDLength {
				t.Errorf(
					"crash ID length = %d, want %d",
					len(id),
					len(expectedPrefix)+shortIDLength,
				)
			}

			if id[:len(expectedPrefix)] != expectedPrefix {
				t.Errorf("crash ID prefix = %q, want %q", id[:len(expectedPrefix)], expectedPrefix)
			}
		})
	}
}

func TestGenerateCrashID_Uniqueness(t *testing.T) {
	// Same panic value at different times should generate different IDs
	panic1 := "test error"
	panic2 := "test error"

	id1 := generateCrashID(testTime, panic1)
	id2 := generateCrashID(testTime.Add(1), panic2)

	if id1 == id2 {
		t.Error("expected different crash IDs for different timestamps")
	}

	// Different panic values at same time should generate different IDs
	panic3 := "different error"
	id3 := generateCrashID(testTime, panic3)

	if id1 == id3 {
		t.Error("expected different crash IDs for different panic values")
	}
}

// Benchmark formatPanicValue to ensure it's efficient
func BenchmarkFormatPanicValue(b *testing.B) {
	values := []any{
		nil,
		"string panic",
		errors.New("error panic"),
		42,
		errors.Wrap(errors.New("wrapped"), "formatted error"),
	}

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		formatPanicValue(values[i%len(values)])
	}
}
