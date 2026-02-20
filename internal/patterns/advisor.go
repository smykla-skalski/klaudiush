package patterns

import (
	"fmt"
	"slices"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// codeDescriptions maps error codes to short human-readable labels.
var codeDescriptions = map[string]string{
	"GIT004": "title too long",
	"GIT005": "body line too long",
	"GIT006": "infra scope misuse",
	"GIT013": "conventional format",
	"GIT016": "list format",
}

// Advisor generates pattern-based warnings from known failure sequences.
type Advisor struct {
	store            PatternStore
	minCount         int
	maxPerError      int
	maxTotal         int
	codeDescriptions map[string]string
}

// NewAdvisor creates an advisor with the given config.
func NewAdvisor(store PatternStore, cfg *config.PatternsConfig) *Advisor {
	return &Advisor{
		store:            store,
		minCount:         cfg.GetMinCount(),
		maxPerError:      cfg.GetMaxWarningsPerError(),
		maxTotal:         cfg.GetMaxWarningsTotal(),
		codeDescriptions: codeDescriptions,
	}
}

// Advise returns warnings for the given blocking error codes.
// Each warning describes a known follow-up pattern so Claude can
// preemptively address it.
func (a *Advisor) Advise(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}

	var warnings []string

	for _, code := range codes {
		if len(warnings) >= a.maxTotal {
			break
		}

		followUps := a.store.GetFollowUps(code, a.minCount)

		slices.SortFunc(followUps, func(a, b *FailurePattern) int {
			return b.Count - a.Count
		})

		perError := 0

		for _, fp := range followUps {
			if perError >= a.maxPerError || len(warnings) >= a.maxTotal {
				break
			}

			warnings = append(warnings, a.formatWarning(fp))
			perError++
		}
	}

	return warnings
}

func (a *Advisor) formatWarning(fp *FailurePattern) string {
	srcDesc := a.describeCode(fp.SourceCode)
	tgtDesc := a.describeCode(fp.TargetCode)

	return fmt.Sprintf(
		"Pattern hint: after fixing %s (%s), %s (%s) often follows.",
		fp.SourceCode, srcDesc, fp.TargetCode, tgtDesc,
	)
}

func (a *Advisor) describeCode(code string) string {
	if desc, ok := a.codeDescriptions[code]; ok {
		return desc
	}

	return code
}
