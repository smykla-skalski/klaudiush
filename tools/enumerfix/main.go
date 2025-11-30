// Package main provides a tool to fix enumer-generated files to use cockroachdb/errors.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

const (
	minArgs         = 2
	filePermissions = 0o644
)

// ErrUsage indicates incorrect usage of the tool.
var ErrUsage = errors.New("usage: enumerfix <file>")

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// run executes the enumerfix logic with the given arguments.
func run(args []string) error {
	if len(args) < minArgs {
		return ErrUsage
	}

	filename := args[1]

	//nolint:gosec // G304: File path from CLI argument is expected
	content, err := os.ReadFile(filename)
	if err != nil {
		return errors.Wrap(err, "reading file")
	}

	fixed := fixEnumerFile(content)

	if err := os.WriteFile(filename, fixed, filePermissions); err != nil {
		return errors.Wrap(err, "writing file")
	}

	return nil
}

func fixEnumerFile(content []byte) []byte {
	result := string(content)

	// Replace fmt.Errorf with errors.Newf
	result = strings.ReplaceAll(result, "fmt.Errorf", "errors.Newf")

	// Check if fmt is still needed (for Sprintf, Stringer, etc.)
	fmtStillNeeded := strings.Contains(result, "fmt.Sprintf") ||
		strings.Contains(result, "fmt.Stringer") ||
		strings.Contains(result, "fmt.Fprintf") ||
		strings.Contains(result, "fmt.Printf")

	// Add errors import and potentially remove fmt
	if fmtStillNeeded {
		// Add errors import alongside fmt
		result = addErrorsImport(result)
	} else {
		// Replace fmt import with errors import
		result = replaceImport(result, `"fmt"`, `"github.com/cockroachdb/errors"`)
	}

	return []byte(result)
}

func addErrorsImport(content string) string {
	// Find the import block and add errors import
	importPattern := regexp.MustCompile(`import \(\n([\s\S]*?)\n\)`)
	match := importPattern.FindStringSubmatch(content)

	if match == nil {
		return content
	}

	imports := match[1]

	// Check if errors is already imported
	if strings.Contains(imports, `"github.com/cockroachdb/errors"`) {
		return content
	}

	// Add errors import after the import block opener
	newImports := imports + "\n\t\"github.com/cockroachdb/errors\""

	return importPattern.ReplaceAllString(content, "import (\n"+newImports+"\n)")
}

func replaceImport(content, oldImport, newImport string) string {
	// Handle single-line import
	singleImportPattern := regexp.MustCompile(`import ` + regexp.QuoteMeta(oldImport))
	if singleImportPattern.MatchString(content) {
		return singleImportPattern.ReplaceAllString(content, "import "+newImport)
	}

	// Handle multi-line import block
	return strings.Replace(content, "\t"+oldImport, "\t"+newImport, 1)
}
