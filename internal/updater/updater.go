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

// Option configures an Updater.
type Option func(*Updater)

// WithDetector sets the install method detector.
func WithDetector(d *Detector) Option {
	return func(u *Updater) {
		u.detector = d
	}
}

// WithBrewUpdater sets the homebrew updater.
func WithBrewUpdater(b *BrewUpdater) Option {
	return func(u *Updater) {
		u.brew = b
	}
}

// Updater orchestrates the self-update process.
type Updater struct {
	currentVersion string
	ghClient       github.Client
	downloader     *Downloader
	platform       Platform
	detector       *Detector
	brew           *BrewUpdater
}

// NewUpdater creates a new Updater.
func NewUpdater(
	currentVersion string,
	ghClient github.Client,
	opts ...Option,
) *Updater {
	u := &Updater{
		currentVersion: currentVersion,
		ghClient:       ghClient,
		downloader:     NewDownloader(nil),
		platform:       DetectPlatform(),
	}

	for _, opt := range opts {
		opt(u)
	}

	return u
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

// GetInstallInfo returns install info for the current binary.
// Returns nil if no detector is configured - callers should fall back to direct.
func (u *Updater) GetInstallInfo() (*InstallInfo, error) {
	if u.detector == nil {
		return nil, nil //nolint:nilnil // nil means "no detection available"
	}

	return u.detector.DetectCurrent()
}

// Update performs the full update to the given tag.
// When a detector is configured, dispatches to the correct update method.
// Without a detector, uses direct update (backward compat).
func (u *Updater) Update(
	ctx context.Context,
	tag string,
	progress ProgressFunc,
) (*UpdateResult, error) {
	info, err := u.GetInstallInfo()
	if err != nil {
		return nil, errors.Wrap(err, "detecting install method")
	}

	if info != nil && info.Method == InstallMethodHomebrew {
		return u.updateViaBrew(ctx, tag)
	}

	return u.updateDirect(ctx, tag, "", progress)
}

// UpdateAt updates a specific binary path to the given tag.
func (u *Updater) UpdateAt(
	ctx context.Context,
	tag string,
	info InstallInfo,
	progress ProgressFunc,
) (*UpdateResult, error) {
	if info.Method == InstallMethodHomebrew {
		return u.updateViaBrew(ctx, tag)
	}

	return u.updateDirect(ctx, tag, info.Path, progress)
}

// UpdateAll discovers all klaudiush binaries and updates each using the correct method.
// Continues on error and collects all results.
func (u *Updater) UpdateAll(
	ctx context.Context,
	tag string,
	progress func(index int, info InstallInfo, p ProgressFunc) ProgressFunc,
) ([]UpdateAllResult, error) {
	if u.detector == nil {
		return nil, errors.New("detector required for UpdateAll")
	}

	infos, err := u.detector.FindAll(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding binaries")
	}

	results := make([]UpdateAllResult, 0, len(infos))

	for i, info := range infos {
		r := UpdateAllResult{InstallInfo: info}

		// For --to with homebrew, skip with a warning.
		if tag != "" && info.Method == InstallMethodHomebrew {
			if isSpecificVersion(tag) {
				r.Skipped = true
				r.Err = ErrBrewVersionPin
				results = append(results, r)

				continue
			}
		}

		var pf ProgressFunc
		if progress != nil {
			pf = progress(i, info, nil)
		}

		result, updateErr := u.UpdateAt(ctx, tag, info, pf)
		r.Result = result
		r.Err = updateErr

		results = append(results, r)
	}

	return results, nil
}

// CheckAll discovers all binaries and checks version status for each.
func (u *Updater) CheckAll(ctx context.Context) ([]InstallStatus, error) {
	if u.detector == nil {
		return nil, errors.New("detector required for CheckAll")
	}

	infos, err := u.detector.FindAll(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "finding binaries")
	}

	statuses := make([]InstallStatus, 0, len(infos))

	for _, info := range infos {
		status := u.checkInstallStatus(ctx, info)
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// checkInstallStatus checks version status for a single binary.
func (u *Updater) checkInstallStatus(ctx context.Context, info InstallInfo) InstallStatus {
	status := InstallStatus{InstallInfo: info}

	if info.Method == InstallMethodHomebrew && u.brew != nil {
		current, latest, outdated, err := u.brew.CheckOutdated(ctx)
		if err == nil {
			status.CurrentVersion = current
			status.LatestVersion = latest
			status.Outdated = outdated
		}

		return status
	}

	status.CurrentVersion = u.currentVersion

	tag, err := u.CheckLatest(ctx)
	if err != nil {
		if errors.Is(err, ErrAlreadyLatest) {
			status.LatestVersion = u.currentVersion
		}

		return status
	}

	status.LatestVersion = strings.TrimPrefix(tag, "v")
	status.Outdated = true

	return status
}

// updateViaBrew delegates the update to homebrew.
func (u *Updater) updateViaBrew(ctx context.Context, tag string) (*UpdateResult, error) {
	if u.brew == nil {
		// Brew not available - fall back to direct.
		return u.updateDirect(ctx, tag, "", nil)
	}

	if tag != "" && isSpecificVersion(tag) {
		return nil, u.brew.UpgradeToVersion(ctx, tag)
	}

	return u.brew.Upgrade(ctx)
}

// updateDirect downloads from GitHub and replaces the binary.
// If binaryPath is empty, uses CurrentBinaryPath().
func (u *Updater) updateDirect(
	ctx context.Context,
	tag, binaryPath string,
	progress ProgressFunc,
) (*UpdateResult, error) {
	ver := strings.TrimPrefix(tag, "v")
	archiveName := u.platform.ArchiveName(ver)

	tmpPath, err := u.downloadAndVerify(ctx, tag, archiveName, ver, progress)
	if err != nil {
		return nil, err
	}

	defer removeTempFile(tmpPath)

	extractedPath, cleanup, extractErr := u.extractBinary(tmpPath)
	if extractErr != nil {
		return nil, extractErr
	}

	defer cleanup()

	if binaryPath == "" {
		binaryPath, err = CurrentBinaryPath()
		if err != nil {
			return nil, err
		}
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

// isSpecificVersion returns true if the tag looks like a specific version (v1.X.Y)
// rather than empty or "latest".
func isSpecificVersion(tag string) bool {
	return tag != "" && tag != "latest"
}
