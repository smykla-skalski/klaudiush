// Package main provides the CLI entry point for klaudiush.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/github"
	"github.com/smykla-skalski/klaudiush/internal/updater"
)

const (
	updateTimeout   = 5 * time.Minute
	percentMultiple = 100
)

var (
	updateToVersion string
	updateCheckOnly bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update klaudiush to the latest version",
	Long: `Update klaudiush to the latest version from GitHub Releases.

Downloads the release archive, verifies the SHA256 checksum,
and atomically replaces the current binary.

Examples:
  klaudiush update                  # Update to latest
  klaudiush update --to v1.13.0    # Update to specific version
  klaudiush update --check         # Check only, don't install`,
	RunE: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)

	updateCmd.Flags().StringVar(
		&updateToVersion,
		"to",
		"",
		"Update to a specific version (e.g. v1.13.0)",
	)
	updateCmd.Flags().BoolVar(
		&updateCheckOnly,
		"check",
		false,
		"Only check for updates, don't install",
	)
}

func runUpdate(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	ghClient := github.NewClient()
	up := updater.NewUpdater(version, ghClient)

	if updateToVersion != "" {
		return runUpdateToVersion(ctx, up)
	}

	return runUpdateLatest(ctx, up)
}

func runUpdateLatest(ctx context.Context, up *updater.Updater) error {
	tag, err := up.CheckLatest(ctx)
	if err != nil {
		if errors.Is(err, updater.ErrAlreadyLatest) {
			fmt.Printf("Already up to date (version %s)\n", version)

			return nil
		}

		return errors.Wrap(err, "checking for updates")
	}

	fmt.Printf("Current version: %s\n", version)
	fmt.Printf("Latest version:  %s\n", tag)

	if updateCheckOnly {
		fmt.Printf("\nRun 'klaudiush update' to install\n")

		return nil
	}

	return performUpdate(ctx, up, tag)
}

func runUpdateToVersion(ctx context.Context, up *updater.Updater) error {
	tag, err := up.ValidateTargetVersion(ctx, updateToVersion)
	if err != nil {
		return err
	}

	fmt.Printf("Current version: %s\n", version)
	fmt.Printf("Target version:  %s\n", tag)

	if updateCheckOnly {
		fmt.Printf("\nRelease %s exists and is available for install\n", tag)

		return nil
	}

	return performUpdate(ctx, up, tag)
}

func performUpdate(ctx context.Context, up *updater.Updater, tag string) error {
	fmt.Printf("\nDownloading %s...\n", tag)

	result, err := up.Update(ctx, tag, func(received, total int64) {
		if total > 0 {
			pct := float64(received) / float64(total) * percentMultiple
			fmt.Fprintf(os.Stderr, "\r  %.0f%% (%d / %d bytes)", pct, received, total)
		} else {
			fmt.Fprintf(os.Stderr, "\r  %d bytes", received)
		}
	})
	if err != nil {
		return errors.Wrap(err, "update failed")
	}

	// Clear progress line
	fmt.Fprintf(os.Stderr, "\r%60s\r", "")

	fmt.Printf("Updated %s -> %s\n", result.PreviousVersion, result.NewVersion)
	fmt.Printf("Binary: %s\n", result.BinaryPath)

	return nil
}
