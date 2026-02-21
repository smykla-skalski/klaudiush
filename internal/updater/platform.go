// Package updater provides self-update functionality for klaudiush.
package updater

import (
	"fmt"
	"runtime"

	"github.com/cockroachdb/errors"
)

const (
	// GitHubOwner is the repository owner on GitHub.
	GitHubOwner = "smykla-skalski"
	// GitHubRepo is the repository name on GitHub.
	GitHubRepo = "klaudiush"
	// BinaryName is the name of the binary to extract from archives.
	BinaryName = "klaudiush"
	// ChecksumsFile is the name of the checksums file in releases.
	ChecksumsFile = "checksums.txt"
)

// Platform represents the current OS and architecture.
type Platform struct {
	OS   string
	Arch string
}

// DetectPlatform returns the current OS and architecture.
func DetectPlatform() Platform {
	return Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
}

// ArchiveName returns the expected archive filename for a given version tag.
// Version should be without "v" prefix (e.g. "1.13.0").
func (p Platform) ArchiveName(version string) string {
	ext := "tar.gz"
	if p.OS == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("%s_%s_%s_%s.%s", BinaryName, version, p.OS, p.Arch, ext)
}

// IsWindows returns true if the platform is Windows.
func (p Platform) IsWindows() bool {
	return p.OS == "windows"
}

// DownloadURL returns the full download URL for a release asset.
func DownloadURL(tag, filename string) string {
	return fmt.Sprintf(
		"https://github.com/%s/%s/releases/download/%s/%s",
		GitHubOwner, GitHubRepo, tag, filename,
	)
}

// ErrBrewVersionPin is returned when attempting to install a specific version via homebrew.
// Brew taps don't support @version formulas without dedicated formula files.
var ErrBrewVersionPin = errors.New(
	"version pinning not supported via homebrew; use direct install instead",
)

// ReleaseURL returns the URL to the release page.
func ReleaseURL(tag string) string {
	return fmt.Sprintf(
		"https://github.com/%s/%s/releases/tag/%s",
		GitHubOwner, GitHubRepo, tag,
	)
}
