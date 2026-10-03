package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/internal/hookresponse"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
)

// toolPhase withholds Gemini's mutation tools until the prerequisite
// evidence checks pass on the current content. BeforeToolSelection narrows
// the tools the model is offered, and BeforeTool denies any call outside
// that set, since another hook's selection can widen the offer again. The
// phase has no state of its own: it is judged from evidence receipts every
// time, so a passing check opens it and a later change closes it again.
type toolPhase struct {
	gate  *evidenceGate
	phase *evidence.Phase
	err   error
}

// newToolPhase returns the tool phase the configuration enables, or nil. A
// phase that does not compile still restricts, with the default read-only
// tools, and reports why.
func newToolPhase(cfg *config.Config, gate *evidenceGate) *toolPhase {
	if gate == nil || cfg == nil || !cfg.Evidence.GetToolPhase().IsEnabled() {
		return nil
	}

	phase, err := evidence.CompilePhase(cfg.Evidence, gate.checks)
	if gate.configErr != nil {
		err = gate.configErr
	}

	if err != nil {
		gate.log.Info("evidence tool phase is invalid", "error", err)

		phase = &evidence.Phase{ReadOnlyTools: config.DefaultToolPhaseReadOnlyTools}
	}

	return &toolPhase{gate: gate, phase: phase, err: err}
}

// phaseState is the phase as judged for one hook: whether mutation tools
// are withheld, which prerequisites are unmet and why, and the finding for
// a phase klaudiush could not judge. Outside a repository there is no
// content to judge, so the phase stays open there, as the evidence gate does.
type phaseState struct {
	repo        string
	restricted  bool
	unmet       []string
	unavailable *dispatcher.ValidationError
}

func (p *toolPhase) state(ctx context.Context, hookCtx *hook.Context) phaseState {
	if p.err != nil {
		finding := p.gate.configUnavailable("compile the evidence tool phase", p.err)

		return phaseState{restricted: finding.ShouldBlock, unavailable: finding}
	}

	repo, err := evidence.RepoRoot(ctx, evidenceWorkDir(hookCtx))
	if err != nil {
		return phaseState{}
	}

	st := phaseState{repo: repo}

	receipts, err := p.gate.store.Receipts(repo)
	if err == nil {
		var passes map[string]*evidence.Receipt

		passes, err = p.gate.store.Passes(repo)
		if err == nil {
			p.judge(ctx, &st, receipts, passes)

			return st
		}
	}

	st.unavailable = p.gate.unavailable("read the check results", err, true)
	st.restricted = st.unavailable.ShouldBlock

	return st
}

func (p *toolPhase) judge(
	ctx context.Context,
	st *phaseState,
	receipts, passes map[string]*evidence.Receipt,
) {
	snap := &lazySnapshot{root: st.repo}

	for _, check := range p.phase.Requires {
		fp, err := checkFingerprint(ctx, snap, check)
		if err != nil {
			st.unavailable = p.gate.unavailable(
				"fingerprint the files of check "+check.Name, err, true,
			)
			st.restricted = st.restricted || st.unavailable.ShouldBlock

			continue
		}

		verdict := evidence.JudgeKept(
			check, receipts[check.Name], passes[check.Name], fp.digest,
			p.gate.now(), p.gate.alive,
		)
		if !verdict.Satisfied() {
			st.unmet = append(st.unmet, fmt.Sprintf("%s (%s)", check.Name, verdict.Reason))
			st.restricted = true
		}
	}
}

// selection answers Gemini BeforeToolSelection: while the phase is
// restricted, only the phase's tools are offered. Nothing is written once
// every prerequisite passed, so the model sees every tool again.
func (p *toolPhase) selection(ctx context.Context, hookCtx *hook.Context) any {
	if p == nil || !hook.FiltersTools(hookCtx.Provider) ||
		hookCtx.Event != hook.CanonicalEventToolSelection {
		return nil
	}

	st := p.state(ctx, hookCtx)
	if !st.restricted {
		p.gate.log.Info("tool phase open, offering every tool")

		return nil
	}

	p.gate.log.Info("tool phase restricted", "unmet", st.unmet, "repo", st.repo)

	return hookresponse.BuildGeminiToolSelection(hookCtx.EventName(), p.phase.AllowedTools())
}

// apply checks a Gemini tool call against the phase. A tool the phase
// withholds is denied even when another hook's selection offered it, or the
// model called it anyway. A call the restricted phase permits never blocks,
// even when the phase could not be judged, so the verifier that opens the
// phase always runs.
func (p *toolPhase) apply(
	ctx context.Context,
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
) []*dispatcher.ValidationError {
	if p == nil || !hook.FiltersTools(hookCtx.Provider) ||
		hookCtx.Event != hook.CanonicalEventBeforeTool {
		return errs
	}

	st := p.state(ctx, hookCtx)

	switch {
	case !st.restricted:
		if st.unavailable != nil {
			errs = append(errs, st.unavailable)
		}

		return errs
	case p.permits(hookCtx, st.repo):
		return errs
	}

	if st.unavailable != nil {
		errs = append(errs, st.unavailable)
	}

	return append(errs, p.locked(hookCtx, st))
}

// permits reports whether a restricted phase lets the call through: a
// read-only tool, the verifier for a prerequisite through the shell, or a
// file tool changing only writable paths.
func (p *toolPhase) permits(hookCtx *hook.Context, repo string) bool {
	switch {
	case p.phase.AllowsReadOnly(hookCtx.ToolNameString()):
		return true
	case hookCtx.IsBashTool():
		return p.phase.AllowsVerifier(hookCtx.GetCommand(), p.gate.binary)
	case hookCtx.IsFileTool():
		return p.writesOnlyWritable(hookCtx, repo)
	default:
		return false
	}
}

func (p *toolPhase) writesOnlyWritable(hookCtx *hook.Context, repo string) bool {
	paths := hookCtx.AffectedPaths
	if len(paths) == 0 {
		paths = []string{hookCtx.GetFilePath()}
	}

	workDir := evidenceWorkDir(hookCtx)

	for _, path := range paths {
		if path == "" {
			return false
		}

		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}

		if !p.phase.Writable(repo, path) {
			return false
		}
	}

	return true
}

func (p *toolPhase) locked(hookCtx *hook.Context, st phaseState) *dispatcher.ValidationError {
	tool := hookCtx.ToolNameString()
	names := p.phase.RequiredNames()

	reason := "klaudiush could not judge the evidence tool phase"
	if len(st.unmet) > 0 {
		reason = "these prerequisite checks lack a passing result for the current files: " +
			strings.Join(st.unmet, "; ")
	}

	return &dispatcher.ValidationError{
		Validator:   evidenceValidator,
		Message:     fmt.Sprintf("Tool %q is withheld for now: %s.", tool, reason),
		ShouldBlock: true,
		Reference:   validator.RefToolPhaseLocked,
		FixHint:     validator.GetSuggestion(validator.RefToolPhaseLocked),
		Resource:    "tool_phase:" + tool,
		Findings: []validator.Finding{{
			Reference: validator.RefToolPhaseLocked,
			Location:  "tool " + tool,
			Message:   reason,
			Actual:    tool,
			Required:  "read-only tools until " + strings.Join(names, ", ") + " pass",
			Repair:    p.repair(st, names),
		}},
	}
}

func (p *toolPhase) repair(st phaseState, names []string) string {
	if len(names) == 0 {
		return "Fix the evidence tool phase configuration; until then only read-only tools run"
	}

	runs := make([]string, 0, len(names))
	for _, name := range names {
		runs = append(runs, fmt.Sprintf("`%s evidence run %s`", shellQuote(p.gate.binary), name))
	}

	repair := "Run " + strings.Join(runs, " and ") + " as its own shell command"
	if st.repo != "" {
		repair += " from " + st.repo
	}

	repair += ", then retry"

	if len(p.phase.WritablePaths) > 0 {
		repair += "; until then write_file and replace may change only " +
			strings.Join(p.phase.WritablePaths, ", ")
	}

	return repair
}
