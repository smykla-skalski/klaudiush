package patterns

import (
	"fmt"
	"maps"
	"slices"

	"github.com/smykla-skalski/klaudiush/pkg/config"
)

// codeDescriptions maps error codes to short human-readable labels.
var codeDescriptions = map[string]string{
	// Git
	"GIT001": "missing signoff",
	"GIT002": "missing GPG sign",
	"GIT003": "no staged files",
	"GIT004": "title too long",
	"GIT005": "body line too long",
	"GIT006": "infra scope misuse",
	"GIT007": "missing remote",
	"GIT008": "missing branch",
	"GIT009": "file not found",
	"GIT010": "missing flags",
	"GIT011": "PR ref in commit",
	"GIT012": "Claude attribution",
	"GIT013": "conventional format",
	"GIT014": "forbidden pattern",
	"GIT015": "signoff mismatch",
	"GIT016": "list format",
	"GIT017": "merge message",
	"GIT018": "merge signoff",
	"GIT019": "blocked files",
	"GIT020": "branch naming",
	"GIT021": "no-verify flag",
	"GIT022": "Kong org push",
	"GIT023": "PR validation",
	"GIT024": "fetch no remote",
	"GIT025": "blocked remote",
	// File
	"FILE001": "shellcheck",
	"FILE002": "terraform fmt",
	"FILE003": "tflint",
	"FILE004": "actionlint",
	"FILE005": "markdown lint",
	"FILE006": "gofumpt",
	"FILE007": "ruff",
	"FILE008": "oxlint",
	"FILE009": "rustfmt",
	"FILE010": "linter ignore",
	// Security
	"SEC001": "API key detected",
	"SEC002": "password detected",
	"SEC003": "private key detected",
	"SEC004": "token detected",
	"SEC005": "connection string",
	// Shell
	"SHELL001": "backtick substitution",
	// GitHub
	"GH001": "issue validation",
	// Plugin
	"PLUG001": "path traversal",
	"PLUG002": "path not allowed",
	"PLUG003": "invalid extension",
	"PLUG004": "insecure remote",
	"PLUG005": "dangerous chars",
	// Session
	"SESS001": "session poisoned",
}

// CodeDescriptions returns a copy of the code descriptions map.
func CodeDescriptions() map[string]string {
	result := make(map[string]string, len(codeDescriptions))
	maps.Copy(result, codeDescriptions)

	return result
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
