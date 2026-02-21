package updater

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cockroachdb/errors"

	"github.com/smykla-skalski/klaudiush/internal/exec"
)

const brewFormula = "smykla-skalski/tap/klaudiush"

// BrewUpdater handles updates via homebrew.
type BrewUpdater struct {
	runner exec.CommandRunner
}

// NewBrewUpdater creates a new BrewUpdater.
func NewBrewUpdater(runner exec.CommandRunner) *BrewUpdater {
	return &BrewUpdater{runner: runner}
}

// Upgrade runs brew upgrade for the klaudiush formula.
// Returns ErrAlreadyLatest if already at the latest version.
func (b *BrewUpdater) Upgrade(ctx context.Context) (*UpdateResult, error) {
	// Get current version before upgrade.
	current, latest, _, checkErr := b.CheckOutdated(ctx)
	if checkErr != nil {
		return nil, errors.Wrap(checkErr, "checking current version")
	}

	result := b.runner.Run(ctx, "brew", "upgrade", brewFormula)

	// brew upgrade exits 0 even when already up to date, so check output.
	if result.Failed() {
		return nil, errors.Wrapf(result.Err, "brew upgrade failed: %s", result.Stderr)
	}

	output := result.Stdout + result.Stderr

	if strings.Contains(output, "already installed") ||
		strings.Contains(output, "already up-to-date") {
		return nil, ErrAlreadyLatest
	}

	// Re-check version after upgrade to get the actual new version.
	_, newLatest, _, recheckErr := b.CheckOutdated(ctx)
	if recheckErr != nil {
		// Upgrade succeeded but can't determine new version - use what we had.
		newLatest = latest
	}

	return &UpdateResult{
		PreviousVersion: current,
		NewVersion:      strings.TrimPrefix(newLatest, "v"),
	}, nil
}

// CheckOutdated checks if the homebrew formula is outdated.
// Returns current installed version, latest available version, whether it's outdated, and any error.
func (b *BrewUpdater) CheckOutdated(
	ctx context.Context,
) (current, latest string, outdated bool, err error) {
	result := b.runner.Run(ctx, "brew", "info", "--json=v2", brewFormula)
	if result.Failed() {
		return "", "", false, errors.Wrapf(result.Err, "brew info failed: %s", result.Stderr)
	}

	return parseBrewInfo(result.Stdout)
}

// UpgradeToVersion returns ErrBrewVersionPin. Brew taps don't support @version formulas
// without dedicated formula files.
func (*BrewUpdater) UpgradeToVersion(_ context.Context, _ string) error {
	return ErrBrewVersionPin
}

// brewInfoResponse is the top-level brew info --json=v2 structure.
type brewInfoResponse struct {
	Formulae []brewFormulae `json:"formulae"`
}

// brewFormulae represents a formula in brew info output.
type brewFormulae struct {
	Versions  brewVersions    `json:"versions"`
	Installed []brewInstalled `json:"installed"`
}

// brewVersions holds the available versions.
type brewVersions struct {
	Stable string `json:"stable"`
}

// brewInstalled holds an installed version entry.
type brewInstalled struct {
	Version string `json:"version"`
}

// parseBrewInfo extracts version info from brew info --json=v2 output.
func parseBrewInfo(jsonOutput string) (current, latest string, outdated bool, err error) {
	var resp brewInfoResponse
	if err := json.Unmarshal([]byte(jsonOutput), &resp); err != nil {
		return "", "", false, errors.Wrap(err, "parsing brew info JSON")
	}

	if len(resp.Formulae) == 0 {
		return "", "", false, errors.New("no formula info in brew output")
	}

	formula := resp.Formulae[0]
	latest = formula.Versions.Stable

	if len(formula.Installed) > 0 {
		current = formula.Installed[0].Version
	}

	outdated = current != latest

	return current, latest, outdated, nil
}
