package evidence

import (
	"fmt"

	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// Providers lists the providers whose evidence coverage is reported.
var Providers = []hook.Provider{
	hook.ProviderClaude, hook.ProviderCodex, hook.ProviderGemini, hook.ProviderOpenCode,
}

// Coverage describes what the evidence gate can do for a provider: whether
// its completion event can be blocked at all, and whether a check run
// through its shell tool counts or only a run through the verifier does.
func Coverage(provider hook.Provider) string {
	switch {
	case !hook.IsCompletionGate(provider, hook.CanonicalEventTurnStop, ""):
		return fmt.Sprintf(
			"%s: not gated, it has no completion event klaudiush can block",
			provider,
		)
	case hook.ReportsCommandOutcome(provider):
		return fmt.Sprintf(
			"%s: gated; shell runs of a check and 'klaudiush evidence run' both count",
			provider,
		)
	default:
		return fmt.Sprintf(
			"%s: gated; only 'klaudiush evidence run' counts, hooks do not learn shell exit status",
			provider,
		)
	}
}

// CoverageLines describes every provider's coverage.
func CoverageLines() []string {
	lines := make([]string, 0, len(Providers))
	for _, provider := range Providers {
		lines = append(lines, Coverage(provider))
	}

	return lines
}

// PhaseCoverage describes what the tool phase can do for a provider: only a
// provider with a tool-selection event has its tools withheld, and only
// Gemini's tool calls are checked against the phase.
func PhaseCoverage(provider hook.Provider) string {
	if hook.FiltersTools(provider) {
		return fmt.Sprintf(
			"%s: restricted; BeforeToolSelection withholds mutation tools and "+
				"BeforeTool denies calls outside the phase",
			provider,
		)
	}

	return fmt.Sprintf(
		"%s: not restricted, it has no tool-selection event, so tool phases do not apply",
		provider,
	)
}

// PhaseCoverageLines describes every provider's tool phase coverage.
func PhaseCoverageLines() []string {
	lines := make([]string, 0, len(Providers))
	for _, provider := range Providers {
		lines = append(lines, PhaseCoverage(provider))
	}

	return lines
}
