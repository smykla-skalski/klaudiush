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

	const filePerms = 0o644

	root, err := os.OpenRoot(outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	writeErr := root.WriteFile(schema.Filename(), data, filePerms)
	_ = root.Close()

	if writeErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", writeErr)
		os.Exit(1)
	}

	fmt.Println(filepath.Join(outDir, schema.Filename()))
}
