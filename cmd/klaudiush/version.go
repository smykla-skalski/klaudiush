package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

const shortCommitLength = 12

// Build information set by ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print detailed version and build information for klaudiush.",
	Run:   runVersion,
}

// versionRequested is set by the --version/-v flag.
var versionRequested bool

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.Flags().BoolVarP(
		&versionRequested,
		"version",
		"v",
		false,
		"Print version information",
	)
}

func checkVersionFlag() {
	if versionRequested {
		fmt.Print(versionString())
		os.Exit(0)
	}
}

func runVersion(_ *cobra.Command, _ []string) {
	fmt.Print(versionString())
}

func versionString() string {
	var b strings.Builder

	fmt.Fprintf(&b, "klaudiush %s\n", version)
	fmt.Fprintf(&b, "  commit:    %s\n", commit)
	fmt.Fprintf(&b, "  built:     %s\n", date)
	fmt.Fprintf(&b, "  go:        %s\n", runtime.Version())
	fmt.Fprintf(&b, "  os/arch:   %s/%s\n", runtime.GOOS, runtime.GOARCH)

	if info, ok := debug.ReadBuildInfo(); ok {
		fmt.Fprintf(&b, "  module:    %s\n", info.Main.Path)

		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && setting.Value != "" {
				if commit == "unknown" {
					fmt.Fprintf(&b,
						"  vcs.rev:   %s\n",
						setting.Value[:min(shortCommitLength, len(setting.Value))],
					)
				}
			}

			if setting.Key == "vcs.modified" && setting.Value == "true" {
				b.WriteString("  modified:  true\n")
			}
		}
	}

	return b.String()
}
