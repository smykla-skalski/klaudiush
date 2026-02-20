// Package main provides the CLI entry point for klaudiush.
package main

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/schema"
)

var (
	schemaOutput  string
	schemaCompact bool
)

var debugSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Generate JSON Schema for configuration",
	Long: `Generate a JSON Schema (Draft 2020-12) for the klaudiush configuration format.

The schema is derived from the Go config types and includes type constraints,
enum values, and descriptions for all configuration options.

Examples:
  klaudiush debug schema                           # Print to stdout
  klaudiush debug schema --output schema.json      # Write to file
  klaudiush debug schema --compact                 # Compact output`,
	RunE: runDebugSchema,
}

func init() {
	debugSchemaCmd.Flags().StringVarP(
		&schemaOutput,
		"output", "o",
		"",
		"Write schema to file instead of stdout",
	)

	debugSchemaCmd.Flags().BoolVar(
		&schemaCompact,
		"compact",
		false,
		"Output compact JSON without indentation",
	)
}

func runDebugSchema(_ *cobra.Command, _ []string) error {
	data, err := schema.GenerateJSON(!schemaCompact)
	if err != nil {
		return errors.Wrap(err, "generating schema")
	}

	if schemaOutput != "" {
		const filePerms = 0o644

		if writeErr := os.WriteFile(schemaOutput, data, filePerms); writeErr != nil {
			return errors.Wrap(writeErr, "writing schema file")
		}

		fmt.Printf("Schema written to %s\n", schemaOutput)

		return nil
	}

	fmt.Print(string(data))

	return nil
}
