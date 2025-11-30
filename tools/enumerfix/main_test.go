package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"
)

func TestRun(t *testing.T) {
	t.Run("returns error when no arguments provided", func(t *testing.T) {
		err := run([]string{"enumerfix"})

		if !errors.Is(err, ErrUsage) {
			t.Errorf("run() error = %v, want %v", err, ErrUsage)
		}
	})

	t.Run("returns error when file does not exist", func(t *testing.T) {
		err := run([]string{"enumerfix", "/nonexistent/file.go"})
		if err == nil {
			t.Error("run() expected error for nonexistent file")
		}

		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("run() error should wrap os.ErrNotExist, got %v", err)
		}
	})

	t.Run("successfully processes file", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.go")

		input := `package test

import "fmt"

func foo() error {
	return fmt.Errorf("error")
}
`
		if err := os.WriteFile(testFile, []byte(input), 0o644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		err := run([]string{"enumerfix", testFile})
		if err != nil {
			t.Errorf("run() unexpected error = %v", err)
		}

		// Verify the file was modified
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("failed to read result file: %v", err)
		}

		expected := `package test

import "github.com/cockroachdb/errors"

func foo() error {
	return errors.Newf("error")
}
`
		if string(content) != expected {
			t.Errorf("run() file content = %q, want %q", string(content), expected)
		}
	})

	t.Run("returns error when file is not writable", func(t *testing.T) {
		// Create temp directory
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "readonly.go")

		input := `package test

import "fmt"

func foo() error {
	return fmt.Errorf("error")
}
`
		if err := os.WriteFile(testFile, []byte(input), 0o444); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		err := run([]string{"enumerfix", testFile})
		if err == nil {
			t.Error("run() expected error for readonly file")
		}
	})
}

func TestFixEnumerFile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "replaces fmt.Errorf with errors.Newf and updates import",
			input: `package test

import "fmt"

func foo() error {
	return fmt.Errorf("error: %s", msg)
}
`,
			expected: `package test

import "github.com/cockroachdb/errors"

func foo() error {
	return errors.Newf("error: %s", msg)
}
`,
		},
		{
			name: "keeps fmt import when fmt.Sprintf is used",
			input: `package test

import (
	"fmt"
)

func foo() (string, error) {
	s := fmt.Sprintf("value: %d", val)
	return s, fmt.Errorf("error")
}
`,
			expected: `package test

import (
	"fmt"
	"github.com/cockroachdb/errors"
)

func foo() (string, error) {
	s := fmt.Sprintf("value: %d", val)
	return s, errors.Newf("error")
}
`,
		},
		{
			name: "handles content without fmt.Errorf",
			input: `package test

import "fmt"

func foo() {
	fmt.Println("hello")
}
`,
			expected: `package test

import "github.com/cockroachdb/errors"

func foo() {
	fmt.Println("hello")
}
`,
		},
		{
			name: "does not duplicate errors import",
			input: `package test

import (
	"fmt"
	"github.com/cockroachdb/errors"
)

func foo() error {
	s := fmt.Sprintf("value")
	return fmt.Errorf("error")
}
`,
			expected: `package test

import (
	"fmt"
	"github.com/cockroachdb/errors"
)

func foo() error {
	s := fmt.Sprintf("value")
	return errors.Newf("error")
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixEnumerFile([]byte(tt.input))

			if string(result) != tt.expected {
				t.Errorf("fixEnumerFile() = %q, want %q", string(result), tt.expected)
			}
		})
	}
}

func TestAddErrorsImport(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "adds errors import to existing import block",
			input: `package test

import (
	"fmt"
)
`,
			expected: `package test

import (
	"fmt"
	"github.com/cockroachdb/errors"
)
`,
		},
		{
			name: "does not add duplicate errors import",
			input: `package test

import (
	"fmt"
	"github.com/cockroachdb/errors"
)
`,
			expected: `package test

import (
	"fmt"
	"github.com/cockroachdb/errors"
)
`,
		},
		{
			name: "returns unchanged content without import block",
			input: `package test

var x = 1
`,
			expected: `package test

var x = 1
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addErrorsImport(tt.input)

			if result != tt.expected {
				t.Errorf("addErrorsImport() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestReplaceImport(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		oldImport string
		newImport string
		expected  string
	}{
		{
			name:      "replaces single-line import",
			input:     `import "fmt"`,
			oldImport: `"fmt"`,
			newImport: `"github.com/cockroachdb/errors"`,
			expected:  `import "github.com/cockroachdb/errors"`,
		},
		{
			name: "replaces import in multi-line block",
			input: `import (
	"fmt"
	"strings"
)`,
			oldImport: `"fmt"`,
			newImport: `"github.com/cockroachdb/errors"`,
			expected: `import (
	"github.com/cockroachdb/errors"
	"strings"
)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceImport(tt.input, tt.oldImport, tt.newImport)

			if result != tt.expected {
				t.Errorf("replaceImport() = %q, want %q", result, tt.expected)
			}
		})
	}
}
