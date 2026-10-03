// Package evidence checks that the evidence gate is configured so it can
// see completion attempts, and reports which providers can supply results.
package evidence

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/doctor"
	"github.com/smykla-skalski/klaudiush/internal/doctor/settings"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/pkg/config"
)

const claudeStopEvent = "Stop"

// GateChecker verifies the evidence configuration compiles and that every
// Claude settings file running klaudiush also runs it on Stop, the event the
// gate blocks. The installer registers Codex Stop and Gemini AfterAgent but
// not Claude Stop.
type GateChecker struct {
	cfg       *config.Config
	locations func() []settings.SettingsLocation
}

// NewGateChecker creates a GateChecker for cfg.
func NewGateChecker(cfg *config.Config) *GateChecker {
	return &GateChecker{cfg: cfg, locations: settings.GetAllSettingsPaths}
}

// NewGateCheckerWithLocations creates a GateChecker reading the given
// settings locations.
func NewGateCheckerWithLocations(
	cfg *config.Config,
	locations func() []settings.SettingsLocation,
) *GateChecker {
	return &GateChecker{cfg: cfg, locations: locations}
}

// Name returns the name of the check.
func (*GateChecker) Name() string {
	return "Evidence gate reaches completion"
}

// Category returns the category of the check.
func (*GateChecker) Category() doctor.Category {
	return doctor.CategoryEvidence
}

// Check reports configuration errors, Claude settings without a Stop hook,
// and how each provider can supply check results.
func (c *GateChecker) Check(context.Context) doctor.CheckResult {
	var evidenceCfg *config.EvidenceConfig
	if c.cfg != nil {
		evidenceCfg = c.cfg.Evidence
	}

	if !evidenceCfg.IsEnabled() {
		return doctor.Skip(c.Name(), "Evidence gate disabled")
	}

	checks, err := evidence.Compile(evidenceCfg)
	if err != nil {
		result := doctor.FailError(c.Name(), "Evidence checks are invalid, so nothing is gated")
		result.Details = []string{err.Error()}

		return result
	}

	if len(checks) == 0 {
		return doctor.FailWarning(c.Name(),
			"Evidence gate enabled without checks, so nothing is gated")
	}

	missingStop := c.settingsWithoutStop()
	coverage := evidence.CoverageLines()

	if len(missingStop) > 0 {
		result := doctor.FailWarning(c.Name(), fmt.Sprintf(
			"%d Claude settings file(s) run klaudiush but not on Stop, so Claude "+
				"sessions finish without the evidence gate",
			len(missingStop),
		))
		details := slices.Concat(missingStop, []string{
			`Add a Stop hook running "klaudiush --provider claude --event Stop"`,
		}, coverage)
		result.Details = details

		return result
	}

	result := doctor.Pass(
		c.Name(),
		fmt.Sprintf("%d required check(s) gate completion", len(checks)),
	)
	result.Details = coverage

	return result
}

func (c *GateChecker) settingsWithoutStop() []string {
	var missing []string

	for _, location := range c.locations() {
		if !location.Exists {
			continue
		}

		parsed, err := settings.NewSettingsParser(location.Path).Parse()
		if err != nil {
			continue
		}

		if runsKlaudiush(parsed, "") && !runsKlaudiush(parsed, claudeStopEvent) {
			missing = append(missing, fmt.Sprintf("%s settings (%s)", location.Type, location.Path))
		}
	}

	return missing
}

// runsKlaudiush reports whether klaudiush runs on event, or on any event
// when event is empty.
func runsKlaudiush(parsed *settings.ClaudeSettings, event string) bool {
	for name, groups := range parsed.Hooks {
		if event != "" && name != event {
			continue
		}

		for _, group := range groups {
			for _, hookCmd := range group.Hooks {
				if isKlaudiushCommand(hookCmd.Command) {
					return true
				}
			}
		}
	}

	return false
}

func isKlaudiushCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}

	name := filepath.Base(fields[0])

	return strings.Contains(name, "klaudiush") || name == "dispatcher"
}
