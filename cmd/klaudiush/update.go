// Package main provides the CLI entry point for klaudiush.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/github"
	"github.com/smykla-skalski/klaudiush/internal/updater"
)

const (
	updateTimeout     = 5 * time.Minute
	percentMultiple   = 100
	commandRunTimeout = 30 * time.Second
)

var (
	updateToVersion string
	updateCheckOnly bool
	updateAll       bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update klaudiush to the latest version",
	Long: `Update klaudiush to the latest version from GitHub Releases.

Downloads the release archive, verifies the SHA256 checksum,
and atomically replaces the current binary.

Automatically detects the install method (homebrew or direct) and
uses the correct update path.

Examples:
  klaudiush update                  # Update to latest
  klaudiush update --to v1.13.0    # Update to specific version
  klaudiush update --check         # Check only, don't install
  klaudiush update --all           # Update all binaries in PATH
  klaudiush update --all --check   # List all binaries with status`,
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
	updateCmd.Flags().BoolVar(
		&updateAll,
		"all",
		false,
		"Update all klaudiush binaries found in PATH",
	)
}

func runUpdate(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	ghClient := github.NewClient()
	opts := buildUpdaterOptions()
	up := updater.NewUpdater(version, ghClient, opts...)

	if updateAll {
		return runUpdateAll(ctx, up)
	}

	if updateToVersion != "" {
		return runUpdateToVersion(ctx, up)
	}

	return runUpdateLatest(ctx, up)
}

func buildUpdaterOptions() []updater.Option {
	runner := exec.NewCommandRunner(commandRunTimeout)
	tools := exec.NewToolChecker()
	detector := updater.NewDetector(runner)

	opts := []updater.Option{
		updater.WithDetector(detector),
	}

	if tools.IsAvailable("brew") {
		opts = append(opts, updater.WithBrewUpdater(updater.NewBrewUpdater(runner)))
	}

	return opts
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
	info, _ := up.GetInstallInfo()
	if info != nil && info.Method == updater.InstallMethodHomebrew {
		return performBrewUpdate(ctx, up, tag)
	}

	return performDirectUpdate(ctx, up, tag)
}

func performBrewUpdate(ctx context.Context, up *updater.Updater, tag string) error {
	fmt.Printf("\nUpgrading via homebrew...\n")

	result, err := up.Update(ctx, tag, nil)
	if err != nil {
		if errors.Is(err, updater.ErrBrewVersionPin) {
			fmt.Printf("Version pinning not supported via homebrew.\n")
			fmt.Printf(
				"Use direct install: klaudiush update --to %s "+
					"(after uninstalling brew version)\n", tag,
			)

			return nil
		}

		return errors.Wrap(err, "update failed")
	}

	fmt.Printf("Updated %s -> %s\n", result.PreviousVersion, result.NewVersion)

	return nil
}

func performDirectUpdate(ctx context.Context, up *updater.Updater, tag string) error {
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

func runUpdateAll(ctx context.Context, up *updater.Updater) error {
	if updateCheckOnly {
		return runUpdateAllCheck(ctx, up)
	}

	return runUpdateAllPerform(ctx, up)
}

func runUpdateAllCheck(ctx context.Context, up *updater.Updater) error {
	statuses, err := up.CheckAll(ctx)
	if err != nil {
		return errors.Wrap(err, "checking binaries")
	}

	fmt.Printf("Found %d klaudiush %s:\n\n",
		len(statuses), pluralize(len(statuses), "binary", "binaries"))

	for _, s := range statuses {
		fmt.Printf("  %s (%s)\n", s.DisplayPath(), s.Method)

		if s.Outdated {
			fmt.Printf("    %s -> outdated (latest: %s)\n", s.CurrentVersion, s.LatestVersion)
		} else {
			fmt.Printf("    %s -> up to date\n", s.CurrentVersion)
		}
	}

	return nil
}

func runUpdateAllPerform(ctx context.Context, up *updater.Updater) error {
	tag, err := resolveUpdateAllTag(ctx, up)
	if err != nil {
		return err
	}

	results, err := up.UpdateAll(ctx, tag, nil)
	if err != nil {
		return errors.Wrap(err, "updating binaries")
	}

	fmt.Printf("Found %d klaudiush %s:\n\n",
		len(results), pluralize(len(results), "binary", "binaries"))

	return printUpdateAllResults(results)
}

func resolveUpdateAllTag(ctx context.Context, up *updater.Updater) (string, error) {
	if updateToVersion != "" {
		return up.ValidateTargetVersion(ctx, updateToVersion)
	}

	tag, err := up.CheckLatest(ctx)
	if err != nil && !errors.Is(err, updater.ErrAlreadyLatest) {
		return "", errors.Wrap(err, "checking latest version")
	}

	return tag, nil
}

func printUpdateAllResults(results []updater.UpdateAllResult) error {
	var hadError bool

	for i, r := range results {
		fmt.Printf("[%d/%d] %s (%s)\n", i+1, len(results), r.DisplayPath(), r.Method)

		if r.Skipped {
			fmt.Printf("  Skipped: %s\n\n", r.Err)

			continue
		}

		if r.Err != nil {
			if errors.Is(r.Err, updater.ErrAlreadyLatest) {
				fmt.Printf("  Already up to date\n\n")
			} else {
				fmt.Printf("  Error: %s\n\n", r.Err)

				hadError = true
			}

			continue
		}

		if r.Result != nil {
			fmt.Printf("  Updated %s -> %s\n\n", r.Result.PreviousVersion, r.Result.NewVersion)
		}
	}

	if hadError {
		return errors.New("some updates failed")
	}

	return nil
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}

	return plural
}
