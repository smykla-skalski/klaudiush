package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

const evidenceValidator = cmdUseEvidence

// maxListedFiles bounds the changed files a review finding names.
const maxListedFiles = 10

const backgroundDetail = "it ran in the background, and hooks never learn " +
	"a background command's exit status"

// evidenceGate keeps the agent from finishing while a required check has no
// passing result for the files as they are now. It records the content each
// session started from, ties every observed check run to the content it ran
// against, and judges those results at the turn's completion gate.
type evidenceGate struct {
	checks []*evidence.Check
	store  *hooksession.Store
	policy *failpolicy.Policy
	binary string
	now    func() time.Time
	alive  func(pid int) bool
	log    logger.Logger
}

// newEvidenceGate returns nil unless evidence is enabled with valid checks.
func newEvidenceGate(
	cfg *config.Config,
	store *hooksession.Store,
	policy *failpolicy.Policy,
	log logger.Logger,
) *evidenceGate {
	if cfg == nil || store == nil || !cfg.Evidence.IsEnabled() {
		return nil
	}

	checks, err := evidence.Compile(cfg.Evidence)
	if err != nil {
		log.Info("evidence checks are invalid", "error", err)

		return nil
	}

	if len(checks) == 0 {
		return nil
	}

	return &evidenceGate{
		checks: checks,
		store:  store,
		policy: policy,
		binary: klaudiushBinary(),
		now:    time.Now,
		alive:  evidence.ProcessAlive,
		log:    log,
	}
}

func klaudiushBinary() string {
	path, err := os.Executable()
	if err != nil || path == "" {
		return "klaudiush"
	}

	return path
}

// lazySnapshot lists the work tree once per hook, on first use.
type lazySnapshot struct {
	root string
	snap *evidence.Snapshot
	err  error
	done bool
}

func (l *lazySnapshot) get(ctx context.Context) (*evidence.Snapshot, error) {
	if !l.done {
		l.snap, l.err = evidence.TakeSnapshot(ctx, l.root)
		l.done = true
	}

	return l.snap, l.err
}

// apply records baselines and check runs for this hook and, at the turn's
// completion gate, adds a blocking finding for every required check without
// a passing result on the current content.
func (g *evidenceGate) apply(
	ctx context.Context,
	hookCtx *hook.Context,
	errs []*dispatcher.ValidationError,
) []*dispatcher.ValidationError {
	if g == nil || hookCtx == nil || hookCtx.Provider == hook.ProviderUnknown ||
		hookCtx.SessionID == "" {
		return errs
	}

	workDir := evidenceWorkDir(hookCtx)

	repo, err := evidence.RepoRoot(ctx, workDir)
	if err != nil {
		g.log.Debug("evidence gate skipped outside a git repository", "dir", workDir)

		return errs
	}

	snap := &lazySnapshot{root: repo}

	baselines, baselineErr := g.ensureBaselines(ctx, hookCtx, repo, snap)

	switch hookCtx.Event {
	case hook.CanonicalEventBeforeTool:
		g.startRun(ctx, hookCtx, workDir, repo, snap, errs)
	case hook.CanonicalEventAfterTool:
		g.finishRun(ctx, hookCtx, repo, snap)
	case hook.CanonicalEventTurnStop:
		if baselineErr != nil {
			return append(errs, g.unavailable("read the session's evidence baselines", baselineErr))
		}

		return append(errs, g.verdicts(ctx, hookCtx, repo, snap, baselines)...)
	default:
	}

	return errs
}

func evidenceWorkDir(hookCtx *hook.Context) string {
	if dir := hookCtx.GetWorkingDir(); dir != "" {
		return dir
	}

	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	return dir
}

// ensureBaselines records, for every check the session has no baseline for
// yet, the content the check covers now. When the first hook the session
// shows klaudiush comes after a tool already ran, that content may include
// the session's own change, so the check is required regardless.
func (g *evidenceGate) ensureBaselines(
	ctx context.Context,
	hookCtx *hook.Context,
	repo string,
	snap *lazySnapshot,
) (map[string]string, error) {
	baselines, err := g.store.EvidenceBaselines(hookCtx.Provider, hookCtx.SessionID, repo)
	if err != nil {
		g.log.Info("failed to read evidence baselines", "error", err)

		return nil, err
	}

	if baselines == nil {
		baselines = make(map[string]string)
	}

	added := make(map[string]string)
	unknown := hookCtx.IsAfterTool() && !readOnlyTool(hookCtx)

	for _, check := range g.checks {
		if _, ok := baselines[check.ID()]; ok {
			continue
		}

		digest := hooksession.BaselineUnknown

		if !unknown {
			digest, err = contentDigest(ctx, snap, check)
			if err != nil {
				g.log.Info("failed to fingerprint evidence baseline",
					"check", check.Name, "error", err)

				continue
			}
		}

		added[check.ID()] = digest
		baselines[check.ID()] = digest
	}

	if err := g.store.AddEvidenceBaselines(
		hookCtx.Provider, hookCtx.SessionID, repo, added,
	); err != nil {
		g.log.Info("failed to save evidence baselines", "error", err)
	}

	return baselines, nil
}

func readOnlyTool(hookCtx *hook.Context) bool {
	switch hookCtx.ToolFamily {
	case hook.ToolFamilyRead, hook.ToolFamilyGrep, hook.ToolFamilyGlob:
		return true
	default:
		return false
	}
}

func contentDigest(
	ctx context.Context,
	snap *lazySnapshot,
	check *evidence.Check,
) (string, error) {
	tree, err := snap.get(ctx)
	if err != nil {
		return "", err
	}

	digest, _, err := tree.ContentDigest(ctx, check)

	return digest, err
}

// fingerprint is what a check's result is tied to: the covered content of a
// test check, the exact diff of a review check.
type fingerprint struct {
	digest string
	files  int
	diff   evidence.Diff
}

func checkFingerprint(
	ctx context.Context,
	snap *lazySnapshot,
	check *evidence.Check,
) (fingerprint, error) {
	tree, err := snap.get(ctx)
	if err != nil {
		return fingerprint{}, err
	}

	if check.IsReview() {
		diff, diffErr := tree.ReviewDiff(ctx, check)

		return fingerprint{digest: diff.Digest, files: len(diff.Changed), diff: diff}, diffErr
	}

	digest, files, err := tree.ContentDigest(ctx, check)

	return fingerprint{digest: digest, files: files}, err
}

// startRun records a check the provider is about to run through its shell
// tool. Only providers that report the command's exit status afterwards
// count, and only when klaudiush let the command through.
func (g *evidenceGate) startRun(
	ctx context.Context,
	hookCtx *hook.Context,
	workDir, repo string,
	snap *lazySnapshot,
	errs []*dispatcher.ValidationError,
) {
	if !hook.ReportsCommandOutcome(hookCtx.Provider) || !hookCtx.IsBashTool() ||
		hookCtx.IsPermissionRequest() || hookCtx.ToolUseID == "" ||
		dispatcher.ShouldBlock(errs) {
		return
	}

	check := evidence.MatchCommand(g.checks, hookCtx.GetCommand(), workDir, repo)
	if check == nil {
		return
	}

	receipt := &evidence.Receipt{
		RunID:     evidence.NewRunID(),
		CheckID:   check.ID(),
		Check:     check.Name,
		Kind:      check.Kind,
		Status:    evidence.StatusRunning,
		Source:    string(hookCtx.Provider),
		Command:   hookCtx.GetCommand(),
		SessionID: hookCtx.SessionID,
		AgentID:   hookCtx.AgentID,
		ToolUseID: hookCtx.ToolUseID,
		StartedAt: g.now(),
	}

	fp, err := checkFingerprint(ctx, snap, check)
	if err != nil {
		receipt.Status = evidence.StatusUnverified
		receipt.Detail = "klaudiush could not fingerprint the files it ran against: " +
			firstLine(err.Error())
	}

	receipt.Digest, receipt.Files = fp.digest, fp.files
	receipt.Base, receipt.Changed = fp.diff.Base, fp.diff.Changed

	if hookCtx.ToolInput.RunInBackground() {
		receipt.Status = evidence.StatusUnverified
		receipt.Detail = backgroundDetail
	}

	if err := g.store.PutReceipt(repo, receipt); err != nil {
		g.log.Info("failed to record check start", "check", check.Name, "error", err)
	}
}

// finishRun records how a check run started in startRun ended, from the
// provider's own report: Claude PostToolUse after success, PostToolUseFailure
// after failure. The files are fingerprinted again, so a run during which
// they changed proves nothing.
func (g *evidenceGate) finishRun(
	ctx context.Context,
	hookCtx *hook.Context,
	repo string,
	snap *lazySnapshot,
) {
	if !hook.ReportsCommandOutcome(hookCtx.Provider) || !hookCtx.IsBashTool() ||
		hookCtx.ToolUseID == "" {
		return
	}

	sameRun := func(receipt *evidence.Receipt) bool {
		return receipt.ToolUseID == hookCtx.ToolUseID &&
			receipt.SessionID == hookCtx.SessionID &&
			receipt.Status == evidence.StatusRunning
	}

	started, err := g.store.FindReceipt(repo, sameRun)
	if err != nil || started == nil {
		return
	}

	check := evidence.Find(g.checks, started.Check)
	if check == nil || check.ID() != started.CheckID {
		return
	}

	status, detail := runOutcome(hookCtx)

	fp, err := checkFingerprint(ctx, snap, check)
	if err != nil {
		status = evidence.StatusUnverified
		detail = "klaudiush could not fingerprint the files after the run: " +
			firstLine(err.Error())
	}

	_, err = g.store.FinishReceipt(repo, check.Name, sameRun, func(receipt *evidence.Receipt) {
		receipt.Finish(status, nil, fp.digest, detail, g.now())
	})
	if err != nil {
		g.log.Info("failed to record check result", "check", check.Name, "error", err)
	}
}

func runOutcome(hookCtx *hook.Context) (evidence.Status, string) {
	switch {
	case hookCtx.ToolBackground:
		return evidence.StatusUnverified, backgroundDetail
	case hookCtx.ToolInterrupted:
		return evidence.StatusCanceled, "the command was interrupted"
	case hookCtx.ToolSucceeded:
		return evidence.StatusPassed, ""
	default:
		return evidence.StatusFailed, firstLine(hookCtx.ToolError)
	}
}

// verdicts judges every required check at the completion gate. A check is
// required when the files it covers differ from the session's baseline, so
// read-only work and changes the check does not cover never gate.
func (g *evidenceGate) verdicts(
	ctx context.Context,
	hookCtx *hook.Context,
	repo string,
	snap *lazySnapshot,
	baselines map[string]string,
) []*dispatcher.ValidationError {
	receipts, err := g.store.Receipts(repo)
	if err != nil {
		return []*dispatcher.ValidationError{g.unavailable("read the check results", err)}
	}

	var findings []*dispatcher.ValidationError

	for _, check := range g.checks {
		baseline, ok := baselines[check.ID()]
		if !ok {
			continue
		}

		current, err := contentDigest(ctx, snap, check)
		if err != nil {
			findings = append(findings,
				g.unavailable("fingerprint the files of check "+check.Name, err))

			continue
		}

		if baseline == current {
			continue
		}

		fp, err := checkFingerprint(ctx, snap, check)
		if err != nil {
			findings = append(findings,
				g.unavailable("fingerprint the files of check "+check.Name, err))

			continue
		}

		verdict := evidence.Judge(check, receipts[check.Name], fp.digest, g.now(), g.alive)
		if verdict.Satisfied() {
			g.log.Info("evidence check satisfied", "check", check.Name, "digest", fp.digest)

			continue
		}

		findings = append(findings, g.missing(hookCtx, repo, check, verdict, fp))
	}

	return findings
}

func (g *evidenceGate) missing(
	hookCtx *hook.Context,
	repo string,
	check *evidence.Check,
	verdict evidence.Verdict,
	fp fingerprint,
) *dispatcher.ValidationError {
	what := "a passing result"
	if check.IsReview() {
		what = "a passing review of the exact current diff"
	}

	lines := []string{"Repository: " + repo}

	if check.IsReview() {
		lines = append(lines,
			"Review base: "+fp.diff.Base,
			"Changed files: "+listFiles(fp.diff.Changed),
		)
	}

	details := map[string]string{evidenceValidator: strings.Join(lines, "\n")}

	return &dispatcher.ValidationError{
		Validator: evidenceValidator,
		Message: fmt.Sprintf(
			"Required check %q (%s) has no passing result for the files as they are now: %s.",
			check.Name, check.Kind, verdict.Reason,
		),
		Details:     details,
		ShouldBlock: true,
		Reference:   validator.RefEvidenceMissing,
		FixHint:     validator.GetSuggestion(validator.RefEvidenceMissing),
		Resource:    "evidence:" + check.Name,
		Findings: []validator.Finding{{
			Reference: validator.RefEvidenceMissing,
			Location:  "check " + check.Name,
			Message:   verdict.Reason,
			Actual:    string(verdict.Status),
			Required:  what + " for " + fp.digest,
			Repair:    g.repair(hookCtx, repo, check, verdict),
		}},
	}
}

func (g *evidenceGate) repair(
	hookCtx *hook.Context,
	repo string,
	check *evidence.Check,
	verdict evidence.Verdict,
) string {
	verifier := fmt.Sprintf("%s evidence run %s", shellQuote(g.binary), check.Name)

	var repair string

	if hook.ReportsCommandOutcome(hookCtx.Provider) {
		repair = fmt.Sprintf(
			"run `%s` from %s as its own foreground command (no pipes, `;`, `||`, "+
				"`&` or variable prefixes), or run `%s`",
			check.RunCommand(), repo, verifier,
		)
	} else {
		repair = fmt.Sprintf(
			"run `%s` from %s; %s does not tell hooks whether a shell command "+
				"succeeded, so running `%s` directly does not count",
			verifier, repo, providerTitle(hookCtx.Provider), check.RunCommand(),
		)
	}

	switch verdict.Status {
	case evidence.StatusRunning:
		return "Wait for the running check to finish, then stop again; if it was abandoned, " +
			repair
	case evidence.StatusFailed:
		return "Fix what the check reports, then " + repair
	case evidence.StatusPassed, evidence.StatusCanceled, evidence.StatusStale,
		evidence.StatusMissing, evidence.StatusUnverified:
		return upperFirst(repair)
	default:
		return upperFirst(repair)
	}
}

func providerTitle(provider hook.Provider) string {
	if provider == "" {
		return "this provider"
	}

	return upperFirst(string(provider))
}

func upperFirst(text string) string {
	if text == "" {
		return text
	}

	return strings.ToUpper(text[:1]) + text[1:]
}

func shellQuote(value string) string {
	quoted, err := syntax.Quote(value, syntax.LangBash)
	if err != nil {
		return value
	}

	return quoted
}

func listFiles(files []string) string {
	if len(files) <= maxListedFiles {
		return strings.Join(files, ", ")
	}

	return fmt.Sprintf("%s and %d more",
		strings.Join(files[:maxListedFiles], ", "), len(files)-maxListedFiles)
}

// unavailable reports evidence klaudiush could not establish. The failure
// policy decides whether that blocks; by default it warns, like other state
// klaudiush cannot read.
func (g *evidenceGate) unavailable(what string, err error) *dispatcher.ValidationError {
	action := g.policy.Resolve(evidenceValidator, validator.ReasonState, false)

	return &dispatcher.ValidationError{
		Validator: evidenceValidator,
		Message: fmt.Sprintf(
			"klaudiush could not %s, so required checks were not verified: %s",
			what, firstLine(err.Error()),
		),
		ShouldBlock:       action == failpolicy.ActionBlock,
		Reference:         validator.RefValidationUnavailable,
		FixHint:           validator.GetSuggestion(validator.RefValidationUnavailable),
		Unavailable:       true,
		UnavailableReason: validator.ReasonState,
		Resource:          evidenceValidator,
	}
}
