package updater

import (
	"context"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/github"
)

// ErrAlreadyLatest is returned when the current version is already the latest.
var ErrAlreadyLatest = errors.New("already up to date")

// UpdateResult contains the outcome of an update operation.
type UpdateResult struct {
	PreviousVersion string
	NewVersion      string
	BinaryPath      string
}

// Updater orchestrates the self-update process.
type Updater struct {
	currentVersion string
	ghClient       github.Client
	downloader     *Downloader
	platform       Platform
}

// NewUpdater creates a new Updater.
func NewUpdater(currentVersion string, ghClient github.Client) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		ghClient:       ghClient,
		downloader:     NewDownloader(nil),
		platform:       DetectPlatform(),
	}
}

// CheckLatest returns the latest release tag, or ErrAlreadyLatest if current >= latest.
// Dev builds (version="dev") always return the latest tag.
func (u *Updater) CheckLatest(ctx context.Context) (string, error) {
	release, err := u.ghClient.GetLatestRelease(ctx, GitHubOwner, GitHubRepo)
	if err != nil {
		return "", errors.Wrap(err, "checking latest release")
	}

	tag := release.TagName

	// Dev builds always get the latest
	if u.currentVersion == "dev" {
		return tag, nil
	}

	latestVer, err := semver.NewVersion(strings.TrimPrefix(tag, "v"))
	if err != nil {
		return "", errors.Wrapf(err, "parsing latest version %q", tag)
	}

	currentVer, err := semver.NewVersion(strings.TrimPrefix(u.currentVersion, "v"))
	if err != nil {
		return "", errors.Wrapf(err, "parsing current version %q", u.currentVersion)
	}

	if !currentVer.LessThan(latestVer) {
		return "", ErrAlreadyLatest
	}

	return tag, nil
}

// ValidateTargetVersion normalizes and validates a version string.
// Accepts both "v1.13.0" and "1.13.0" formats, always returns "v"-prefixed tag.
// Verifies the release exists on GitHub.
func (u *Updater) ValidateTargetVersion(
	ctx context.Context,
	version string,
) (string, error) {
	// Normalize: strip "v" prefix for semver parsing
	stripped := strings.TrimPrefix(version, "v")

	if _, err := semver.NewVersion(stripped); err != nil {
		return "", errors.Errorf("invalid version %q: must be valid semver (e.g. v1.13.0)", version)
	}

	tag := "v" + stripped

	// Verify the release exists
	if _, err := u.ghClient.GetReleaseByTag(ctx, GitHubOwner, GitHubRepo, tag); err != nil {
		if errors.Is(err, github.ErrRepositoryNotFound) {
			return "", errors.Errorf("release %s not found", tag)
		}

		return "", errors.Wrapf(err, "checking release %s", tag)
	}

	return tag, nil
}

// Update performs the full update to the given tag.
// Steps: download checksums -> download archive -> verify checksum -> extract -> replace binary.
func (u *Updater) Update(
	ctx context.Context,
	tag string,
	progress ProgressFunc,
) (*UpdateResult, error) {
	// Strip "v" prefix for archive naming (goreleaser uses bare version)
	ver := strings.TrimPrefix(tag, "v")
	archiveName := u.platform.ArchiveName(ver)

	// Download checksums + archive, verify, extract, replace
	tmpPath, err := u.downloadAndVerify(ctx, tag, archiveName, ver, progress)
	if err != nil {
		return nil, err
	}

	defer removeTempFile(tmpPath)

	// Extract binary
	extractedPath, cleanup, extractErr := u.extractBinary(tmpPath)
	if extractErr != nil {
		return nil, extractErr
	}

	defer cleanup()

	// Replace current binary
	binaryPath, err := CurrentBinaryPath()
	if err != nil {
		return nil, err
	}

	if replaceErr := ReplaceBinary(extractedPath, binaryPath); replaceErr != nil {
		return nil, replaceErr
	}

	return &UpdateResult{
		PreviousVersion: u.currentVersion,
		NewVersion:      ver,
		BinaryPath:      binaryPath,
	}, nil
}

// downloadAndVerify downloads the checksums and archive, then verifies the checksum.
func (u *Updater) downloadAndVerify(
	ctx context.Context,
	tag, archiveName, _ string,
	progress ProgressFunc,
) (string, error) {
	checksumsURL := DownloadURL(tag, ChecksumsFile)

	checksumsContent, err := u.downloader.DownloadToString(ctx, checksumsURL)
	if err != nil {
		return "", errors.Wrap(err, "downloading checksums")
	}

	checksums := ParseChecksums(checksumsContent)

	expectedHash, ok := checksums[archiveName]
	if !ok {
		return "", errors.Errorf(
			"no checksum found for %s in release %s",
			archiveName, tag,
		)
	}

	// Download archive to temp file
	tmpFile, err := os.CreateTemp("", "klaudiush-archive-*")
	if err != nil {
		return "", errors.Wrap(err, "creating temp file for archive")
	}

	tmpPath := tmpFile.Name()

	if closeErr := tmpFile.Close(); closeErr != nil {
		return "", errors.Wrap(closeErr, "closing temp file")
	}

	archiveURL := DownloadURL(tag, archiveName)

	if dlErr := u.downloader.DownloadToFile(ctx, archiveURL, tmpPath, progress); dlErr != nil {
		removeTempFile(tmpPath)

		return "", errors.Wrap(dlErr, "downloading archive")
	}

	if verifyErr := VerifyFileChecksum(tmpPath, expectedHash); verifyErr != nil {
		removeTempFile(tmpPath)

		return "", errors.Wrap(verifyErr, "verifying archive checksum")
	}

	return tmpPath, nil
}

// extractBinary extracts the binary from the downloaded archive.
func (u *Updater) extractBinary(archivePath string) (string, func(), error) {
	if u.platform.IsWindows() {
		return ExtractBinaryFromZip(archivePath, BinaryName)
	}

	return ExtractBinaryFromTarGz(archivePath, BinaryName)
}

// removeTempFile removes a temporary file, ignoring errors.
//
//nolint:gosec // G703: path is from os.CreateTemp, not user-controlled
func removeTempFile(path string) {
	_ = os.Remove(path)
}
