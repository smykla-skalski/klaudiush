package main

import (
	"fmt"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/suggest"
)

var (
	suggestCheck  bool
	suggestDryRun bool
	suggestOutput string
)

var suggestCmd = &cobra.Command{
	Use:   "suggest",
	Short: "Generate KLAUDIUSH.md with active validation rules",
	Long: `Generate a KLAUDIUSH.md file documenting active validation rules for Claude Code.

This gives Claude Code upfront knowledge of conventions, avoiding trial-and-error
validation failures.

Examples:
  klaudiush suggest                    # Write KLAUDIUSH.md to current directory
  klaudiush suggest --dry-run          # Print to stdout without writing
  klaudiush suggest --check            # Check if KLAUDIUSH.md is up-to-date
  klaudiush suggest --output path.md   # Write to custom path`,
	RunE: runSuggest,
}

func init() {
	rootCmd.AddCommand(suggestCmd)

	suggestCmd.Flags().BoolVar(
		&suggestCheck,
		"check",
		false,
		"Check if KLAUDIUSH.md is up-to-date (exit 1 if stale)",
	)

	suggestCmd.Flags().BoolVar(
		&suggestDryRun,
		"dry-run",
		false,
		"Print generated content to stdout without writing a file",
	)

	suggestCmd.Flags().StringVar(
		&suggestOutput,
		"output",
		"",
		"Output file path (default: ./KLAUDIUSH.md)",
	)
}

func runSuggest(_ *cobra.Command, _ []string) error {
	cfg, err := setupDebugContext("suggest", "", "")
	if err != nil {
		return err
	}

	// Determine output path
	outputPath := suggestOutput
	if outputPath == "" {
		outputPath = "KLAUDIUSH.md"
	}

	gen := suggest.NewGenerator(cfg, version)

	// Check mode: compare hash and exit
	if suggestCheck {
		return runSuggestCheck(gen, outputPath)
	}

	// Generate content
	content, err := gen.Generate()
	if err != nil {
		return errors.Wrap(err, "generating KLAUDIUSH.md")
	}

	// Dry-run mode: print to stdout
	if suggestDryRun {
		fmt.Print(content)

		return nil
	}

	// Write file
	if err := gen.WriteFile(outputPath, content); err != nil {
		return errors.Wrap(err, "writing KLAUDIUSH.md")
	}

	fmt.Printf("Generated %s\n", outputPath)

	return nil
}

func runSuggestCheck(gen *suggest.Generator, outputPath string) error {
	upToDate, err := gen.Check(outputPath)
	if err != nil {
		return errors.Wrap(err, "checking KLAUDIUSH.md")
	}

	if upToDate {
		fmt.Printf("%s is up-to-date\n", outputPath)

		return nil
	}

	fmt.Fprintf(os.Stderr, "%s is stale — re-run: klaudiush suggest\n", outputPath)

	// Return an error so cobra exits with code 1
	return errors.New("KLAUDIUSH.md is stale")
}
