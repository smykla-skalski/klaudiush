package hookresponse

import (
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// outcome is what a validation error means for the action, told apart in
// both the agent and the human message.
type outcome int

const (
	outcomeBlocked outcome = iota
	outcomeRepairRequired
	outcomeExceptionAccepted
	outcomeUnavailable
	outcomeWarning
)

func (o outcome) label() string {
	switch o {
	case outcomeBlocked:
		return "Blocked"
	case outcomeRepairRequired:
		return "Repair required"
	case outcomeExceptionAccepted:
		return "Exception accepted"
	case outcomeUnavailable:
		return "Validation unavailable"
	case outcomeWarning:
		return "Warning"
	default:
		return "Warning"
	}
}

func (o outcome) icon() string {
	switch o {
	case outcomeBlocked:
		return "❌"
	case outcomeRepairRequired:
		return "\U0001F527"
	case outcomeExceptionAccepted:
		return "✅"
	case outcomeUnavailable, outcomeWarning:
		return "⚠️"
	default:
		return "⚠️"
	}
}

// outcomeOf classifies an error for the event the hook received. After a tool
// ran nothing was stopped, so every unwaived finding asks for a repair.
func outcomeOf(hookCtx *hook.Context, e *dispatcher.ValidationError) outcome {
	switch {
	case e.Bypassed:
		return outcomeExceptionAccepted
	case e.Unavailable:
		return outcomeUnavailable
	case hookCtx != nil && hookCtx.IsAfterTool():
		return outcomeRepairRequired
	case e.ShouldBlock:
		return outcomeBlocked
	default:
		return outcomeWarning
	}
}

// Output budgets in bytes, measured per field. Claude caps each hook string
// at 10,000 characters and Codex caps additionalContext near 2,500 tokens;
// past the cap the text goes to a file the agent is not told to read, so
// everything actionable has to fit. contextReserve leaves room for text
// appended after the context is built, such as the agent summary instruction.
const (
	claudeAgentBudget  = 9000
	codexAgentBudget   = 6000
	defaultAgentBudget = 9000
	humanBudget        = 9000
	contextReserve     = 512
)

func agentBudgetFor(hookCtx *hook.Context) int {
	if hookCtx == nil {
		return defaultAgentBudget
	}

	switch hookCtx.Provider {
	case hook.ProviderCodex:
		return codexAgentBudget
	case hook.ProviderClaude, hook.ProviderUnknown:
		return claudeAgentBudget
	case hook.ProviderGemini, hook.ProviderOpenCode:
		return defaultAgentBudget
	default:
		return defaultAgentBudget
	}
}

// detail is how much supplementary text an agent entry carries. Higher
// levels drop supplementary detail first; repairs are always kept.
type detail int

const (
	detailFull detail = iota
	detailShortActual
	detailNoActual
	detailNoRequired
	detailRepairOnly
)

const (
	shortActualRunes  = 40
	shortSummaryRunes = 160
)

// renderWithin renders at the most detailed level that fits the budget, and
// cuts the least detailed rendering at a rune boundary as a last resort.
func renderWithin(budget int, render func(detail) string) string {
	var text string

	for level := detailFull; level <= detailRepairOnly; level++ {
		text = render(level)
		if len(text) <= budget {
			return text
		}
	}

	return fitBudget(text, budget)
}

// agentEntries renders errors for the model, one entry per error, each with
// every finding and repair.
func agentEntries(errs []*dispatcher.ValidationError, level detail) string {
	parts := make([]string, 0, len(errs))

	for _, e := range errs {
		parts = append(parts, agentEntry(e, level))
	}

	return strings.Join(parts, "\n")
}

// agentEntry renders one error: [CODE] summary, then a line per finding.
// Without findings the fix hint is the repair.
func agentEntry(e *dispatcher.ValidationError, level detail) string {
	var b strings.Builder

	code := extractCode(e.Reference)
	if code != "" {
		b.WriteString("[")
		b.WriteString(code)
		b.WriteString("] ")
	}

	if e.Unavailable {
		b.WriteString("Validation unavailable: ")
	}

	summary := summarizeMessage(e.Message)
	if level >= detailShortActual {
		summary = truncateRunes(summary, shortSummaryRunes)
	}

	b.WriteString(summary)

	if len(e.Findings) == 0 {
		if e.FixHint != "" {
			if !strings.HasSuffix(summary, ".") {
				b.WriteString(".")
			}

			b.WriteString(" ")
			b.WriteString(e.FixHint)
		}

		if level < detailNoActual {
			for _, line := range messageDetailLines(e.Message) {
				b.WriteString("\n  ")
				b.WriteString(line)
			}
		}

		return sanitizeText(b.String())
	}

	for _, f := range e.Findings {
		b.WriteString("\n  - ")
		b.WriteString(agentFinding(f, code, level))
	}

	return sanitizeText(b.String())
}

// agentFinding renders: [CODE] location: message (actual: ...; required:
// ...). Repair: ... The code is shown only when it differs from the entry's.
func agentFinding(f validator.Finding, entryCode string, level detail) string {
	var b strings.Builder

	if code := f.Code(); code != "" && code != entryCode {
		b.WriteString("[")
		b.WriteString(code)
		b.WriteString("] ")
	}

	if f.Location != "" {
		b.WriteString(f.Location)
		b.WriteString(": ")
	}

	if level < detailRepairOnly || f.Repair == "" {
		b.WriteString(oneLine(f.Message))
	}

	var facts []string

	if f.Actual != "" && level < detailNoActual {
		actual := oneLine(f.Actual)
		if level >= detailShortActual {
			actual = truncateRunes(actual, shortActualRunes)
		}

		facts = append(facts, "actual: '"+actual+"'")
	}

	if f.Required != "" && level < detailNoRequired {
		facts = append(facts, "required: "+oneLine(f.Required))
	}

	if len(facts) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(facts, "; "))
		b.WriteString(")")
	}

	if f.Repair != "" {
		if level < detailRepairOnly {
			b.WriteString(".")
		}

		b.WriteString(" Repair: ")
		b.WriteString(oneLine(f.Repair))
	}

	return strings.TrimSpace(b.String())
}

// humanFindings renders a finding list for the user: where and what, and the
// repair, one line each.
func humanFindings(b *strings.Builder, findings []validator.Finding, entryCode string) {
	for _, f := range findings {
		b.WriteString("  - ")

		if code := f.Code(); code != "" && code != entryCode {
			b.WriteString(code)
			b.WriteString(" ")
		}

		if f.Location != "" {
			b.WriteString(f.Location)
			b.WriteString(": ")
		}

		b.WriteString(oneLine(f.Message))

		if f.Repair != "" {
			b.WriteString(". Fix: ")
			b.WriteString(oneLine(f.Repair))
		}

		b.WriteString("\n")
	}
}

// oneLine folds whitespace runs, newlines included, into single spaces.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
