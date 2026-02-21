// Command schema-gen writes the versioned JSON Schema to schema/.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/smykla-skalski/klaudiush/internal/schema"
)

func main() {
	data, err := schema.GenerateJSON(true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	outDir := "schema"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	outPath := filepath.Clean(
		filepath.Join(outDir, schema.Filename()),
	)

	const filePerms = 0o644

	//nolint:gosec // dev tool, outDir from CLI arg
	writeErr := os.WriteFile(outPath, data, filePerms)
	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", writeErr)
		os.Exit(1)
	}

	fmt.Println(outPath)
}
