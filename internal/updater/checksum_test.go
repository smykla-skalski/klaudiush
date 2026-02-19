package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestParseChecksums(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name: "standard checksums.txt",
			content: `abc123def456  klaudiush_1.13.0_darwin_arm64.tar.gz
789012fed345  klaudiush_1.13.0_linux_amd64.tar.gz
deadbeef0000  klaudiush_1.13.0_windows_amd64.zip`,
			want: map[string]string{
				"klaudiush_1.13.0_darwin_arm64.tar.gz": "abc123def456",
				"klaudiush_1.13.0_linux_amd64.tar.gz":  "789012fed345",
				"klaudiush_1.13.0_windows_amd64.zip":   "deadbeef0000",
			},
		},
		{
			name:    "empty content",
			content: "",
			want:    map[string]string{},
		},
		{
			name:    "whitespace only",
			content: "  \n  \n  ",
			want:    map[string]string{},
		},
		{
			name:    "single space separator is ignored",
			content: "abc123 filename.tar.gz",
			want:    map[string]string{},
		},
		{
			name:    "trailing newline",
			content: "abc123  file.tar.gz\n",
			want: map[string]string{
				"file.tar.gz": "abc123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseChecksums(tt.content)

			if len(got) != len(tt.want) {
				t.Errorf("ParseChecksums() returned %d entries, want %d", len(got), len(tt.want))
			}

			for key, wantVal := range tt.want {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("missing key %q", key)

					continue
				}

				if gotVal != wantVal {
					t.Errorf("key %q = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestVerifyFileChecksum(t *testing.T) {
	// Create a temp file with known content
	content := []byte("hello world\n")
	h := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(h[:])

	tmpFile := writeTestFile(t, content)

	t.Run("valid checksum", func(t *testing.T) {
		if err := VerifyFileChecksum(tmpFile, expectedHex); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("valid checksum uppercase", func(t *testing.T) {
		if err := VerifyFileChecksum(tmpFile, strings.ToUpper(expectedHex)); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("invalid checksum", func(t *testing.T) {
		wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

		err := VerifyFileChecksum(tmpFile, wrongHash)
		if err == nil {
			t.Error("expected error for mismatched checksum")
		}

		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("error = %q, want to contain 'checksum mismatch'", err.Error())
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		err := VerifyFileChecksum("/nonexistent/path", expectedHex)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func writeTestFile(t *testing.T, content []byte) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "checksum-test-*")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}

	if _, err := f.Write(content); err != nil {
		f.Close()
		t.Fatalf("writing temp file: %v", err)
	}

	f.Close()

	return f.Name()
}
