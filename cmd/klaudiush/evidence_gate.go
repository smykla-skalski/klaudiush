package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
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
// loadChecks returns the checks configured for another repository the
// session touched, nil when its gate is off, or the error that kept them
// from loading. configErr is why the hook's own checks did not compile.
type evidenceGate struct {
	checks     []*evidence.Check
	configErr  error
	loadChecks func(repo string) ([]*evidence.Check, error)
	store      *hooksession.Store
	policy     *failpolicy.Policy
	binary     string
	now        func() time.Time
	alive      func(pid int) bool
	log        logger.Logger
}

// newEvidenceGate returns a gate for the hook's configuration. Without
// checks of its own it still follows edits into repositories that have
// checks, and judges them at the completion gate, so stopping in a
// directory without the gate does not skip them.
func newEvidenceGate(
	cfg *config.Config,
	store *hooksession.Store,
	policy *failpolicy.Policy,
	log logger.Logger,
) *evidenceGate {
	if store == nil {
		return nil
	}

	var (
		checks    []*evidence.Check
		configErr error
	)

	if cfg != nil && cfg.Evidence.IsEnabled() {
		checks, configErr = evidence.Compile(cfg.Evidence)
		if configErr != nil {
			log.Info("evidence checks are invalid", "error", configErr)
		}
	}

	return &evidenceGate{
		checks:    checks,
		configErr: configErr,
		loadChecks: func(repo string) ([]*evidence.Check, error) {
			return repoChecks(log, repo)
		},
		store:  store,
		policy: policy,
		binary: klaudiushBinary(),
		now:    time.Now,
		alive:  evidence.ProcessAlive,
		log:    log,
	}
}

// repoChecks loads the checks configured for a repository, or nil when its
// configuration keeps the gate off. A configuration that cannot be loaded
// or compiled is an error, not a disabled gate.
func repoChecks(log logger.Logger, repo string) ([]*evidence.Check, error) {
	cfg, err := loadConfig(log, repo)
	if err != nil {
		return nil, err
	}

	if !cfg.Evidence.IsEnabled() {
		return nil, nil
	}

	checks, err := evidence.Compile(cfg.Evidence)
	if err != nil {
		return nil, errors.Wrap(err, "invalid evidence configuration")
	}

	return checks, nil
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

// repoScope is one repository a hook concerns, with the checks configured
// for it and a snapshot of its work tree. err is why its checks could not
// be loaded.
type repoScope struct {
	root   string
	checks []*evidence.Check
	err    error
	snap   *lazySnapshot
}

// apply records baselines and check runs for this hook and, at the turn's
// completion gate, adds a blocking finding for every required check without
// a passing result on the current content of every repository the session
// touched.
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
	touched := touchedDirs(hookCtx, workDir)
	stopping := hookCtx.Event == hook.CanonicalEventTurnStop

	if len(g.checks) == 0 && len(touched) == 0 && !stopping {
		return errs
	}

	scopes := newScopeSet(g)

	primary, primaryErr := "", evidence.ErrNotRepository
	if len(g.checks) > 0 {
		primary, primaryErr = evidence.RepoRoot(ctx, workDir)
		if primaryErr == nil {
			scopes.add(primary, g.checks, true)
		}
	}

	for _, dir := range touched {
		if repo, err := evidence.RepoRoot(ctx, dir); err == nil {
			scopes.add(repo, nil, false)
		}
	}

	if stopping {
		return append(errs, g.stop(ctx, hookCtx, scopes)...)
	}

	for _, scope := range scopes.list {
		_, _ = g.ensureBaselines(ctx, hookCtx, scope)
		g.markTouched(hookCtx, scope.root)
	}

	// A repository whose checks cannot load is remembered, so the completion
	// gate reports it instead of treating it as ungated.
	for _, scope := range scopes.failed {
		g.markTouched(hookCtx, scope.root)
	}

	if primaryErr != nil {
		return errs
	}

	switch hookCtx.Event {
	case hook.CanonicalEventBeforeTool:
		g.startRun(ctx, hookCtx, workDir, scopes.byRoot[primary], errs)
	case hook.CanonicalEventAfterTool:
		g.finishRun(ctx, hookCtx, scopes.byRoot[primary])
	default:
	}

	return errs
}

func (g *evidenceGate) markTouched(hookCtx *hook.Context, repo string) {
	if !isToolEvent(hookCtx) || readOnlyTool(hookCtx) {
		return
	}

	if err := g.store.MarkTouched(hookCtx.Provider, hookCtx.SessionID, repo); err != nil {
		g.log.Info("failed to record the session's edits", "error", err)
	}
}

// stop judges every repository the session recorded baselines for or
// edited, plus the one the agent stops in. Checks that cannot be loaded are
// reported as unavailable.
func (g *evidenceGate) stop(
	ctx context.Context,
	hookCtx *hook.Context,
	scopes *scopeSet,
) []*dispatcher.ValidationError {
	repos, err := g.store.EvidenceRepos(hookCtx.Provider, hookCtx.SessionID)
	if err != nil {
		return []*dispatcher.ValidationError{
			g.unavailable("read the session's evidence baselines", err, false),
		}
	}

	for _, repo := range repos {
		scopes.add(repo, nil, false)
	}

	var findings []*dispatcher.ValidationError

	if g.configErr != nil {
		findings = append(findings,
			g.configUnavailable("compile the configured evidence checks", g.configErr))
	}

	for _, scope := range scopes.failed {
		findings = append(findings,
			g.configUnavailable("load the evidence checks of "+scope.root, scope.err))
	}

	for _, scope := range scopes.list {
		baselines, err := g.ensureBaselines(ctx, hookCtx, scope)
		if err != nil {
			findings = append(findings,
				g.unavailable("read the session's evidence baselines", err, false))

			continue
		}

		findings = append(findings, g.verdicts(ctx, hookCtx, scope, baselines)...)
	}

	return findings
}

// scopeSet collects the repositories a hook concerns, loading each one's
// checks once. failed holds the repositories whose checks did not load.
type scopeSet struct {
	gate   *evidenceGate
	byRoot map[string]*repoScope
	list   []*repoScope
	failed []*repoScope
}

func newScopeSet(gate *evidenceGate) *scopeSet {
	return &scopeSet{gate: gate, byRoot: make(map[string]*repoScope)}
}

// add registers a repository with the checks the hook's configuration
// holds, or, unless loaded, with the repository's own.
func (s *scopeSet) add(root string, checks []*evidence.Check, loaded bool) {
	if _, ok := s.byRoot[root]; ok {
		return
	}

	var err error

	if !loaded && s.gate.loadChecks != nil {
		checks, err = s.gate.loadChecks(root)
	}

	scope := &repoScope{root: root, checks: checks, err: err, snap: &lazySnapshot{root: root}}
	s.byRoot[root] = scope

	switch {
	case err != nil:
		s.gate.log.Info("failed to load evidence checks", "repo", root, "error", err)
		s.failed = append(s.failed, scope)
	case len(checks) > 0:
		s.list = append(s.list, scope)
	}
}

// touchedDirs lists the existing directories holding files a file tool
// names, so an edit in a repository other than the working directory's
// still gets a baseline.
func touchedDirs(hookCtx *hook.Context, workDir string) []string {
	if !hookCtx.IsFileTool() && len(hookCtx.PatchFiles) == 0 {
		return nil
	}

	dirs := make([]string, 0, len(hookCtx.AffectedPaths))

	for _, path := range hookCtx.AffectedPaths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workDir, path)
		}

		dir := filepath.Dir(path)
		for dir != filepath.Dir(dir) {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				break
			}

			dir = filepath.Dir(dir)
		}

		dirs = append(dirs, dir)
	}

	return dirs
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
	scope *repoScope,
) (map[string]string, error) {
	repo := scope.root

	baselines, err := g.store.EvidenceBaselines(hookCtx.Provider, hookCtx.SessionID, repo)
	if err != nil {
		g.log.Info("failed to read evidence baselines", "error", err)

		return nil, err
	}

	if baselines == nil {
		baselines = make(map[string]string)
	}

	added := make(map[string]string)
	changing := isToolEvent(hookCtx) && !readOnlyTool(hookCtx)
	unknown := hookCtx.IsAfterTool() && changing

	for _, check := range scope.checks {
		if _, ok := baselines[check.ID()]; ok {
			continue
		}

		digest := hooksession.BaselineUnknown

		if !unknown && !redefined(baselines, check) {
			digest, err = contentDigest(ctx, scope.snap, check)
			if err != nil {
				g.log.Info("failed to fingerprint evidence baseline",
					"check", check.Name, "error", err)

				if !changing {
					continue
				}

				digest = hooksession.BaselineUnknown
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

// redefined reports a check the session already had a baseline for under
// an earlier definition. Its new baseline would include whatever the
// session changed before the definition changed, so the check is required.
func redefined(baselines map[string]string, check *evidence.Check) bool {
	prefix := check.Name + "@"

	for id := range baselines {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}

	return false
}

func isToolEvent(hookCtx *hook.Context) bool {
	return hookCtx.Event == hook.CanonicalEventBeforeTool ||
		hookCtx.Event == hook.CanonicalEventAfterTool
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
	workDir string,
	scope *repoScope,
	errs []*dispatcher.ValidationError,
) {
	repo, snap := scope.root, scope.snap

	if !hook.ReportsCommandOutcome(hookCtx.Provider) || !hookCtx.IsBashTool() ||
		hookCtx.IsPermissionRequest() || hookCtx.ToolUseID == "" ||
		dispatcher.ShouldBlock(errs) {
		return
	}

	check := evidence.MatchCommand(scope.checks, hookCtx.GetCommand(), workDir, repo)
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
	scope *repoScope,
) {
	repo, snap := scope.root, scope.snap

	if !hook.ReportsCommandOutcome(hookCtx.Provider) || !hookCtx.IsBashTool() ||
		hookCtx.ToolUseID == "" {
		return
	}

	sameRun := func(receipt *evidence.Receipt) bool {
		return receipt.ToolUseID == hookCtx.ToolUseID &&
			receipt.SessionID == hookCtx.SessionID &&
			receipt.Command == hookCtx.GetCommand() &&
			receipt.Status == evidence.StatusRunning
	}

	started, err := g.store.FindReceipt(repo, sameRun)
	if err != nil || started == nil {
		return
	}

	check := evidence.Find(scope.checks, started.Check)
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
	case hookCtx.ToolExitNote != "":
		return evidence.StatusFailed, "it exited non-zero (" + hookCtx.ToolExitNote + ")"
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
	scope *repoScope,
	baselines map[string]string,
) []*dispatcher.ValidationError {
	repo, snap := scope.root, scope.snap

	receipts, err := g.store.Receipts(repo)
	if err != nil {
		return []*dispatcher.ValidationError{g.unavailable("read the check results", err, false)}
	}

	passes, err := g.store.Passes(repo)
	if err != nil {
		return []*dispatcher.ValidationError{g.unavailable("read the check results", err, false)}
	}

	touched, err := g.store.Touched(hookCtx.Provider, hookCtx.SessionID, repo)
	if err != nil {
		return []*dispatcher.ValidationError{
			g.unavailable("read the session's evidence baselines", err, false),
		}
	}

	var findings []*dispatcher.ValidationError

	for _, check := range scope.checks {
		baseline, ok := baselines[check.ID()]
		if !ok {
			continue
		}

		if !touched && baseline != hooksession.BaselineUnknown {
			continue
		}

		current, err := contentDigest(ctx, snap, check)
		if err != nil {
			findings = append(findings,
				g.unavailable("fingerprint the files of check "+check.Name, err, true))

			continue
		}

		if baseline == current {
			continue
		}

		if finding := g.judge(ctx, hookCtx, scope, check, receipts, passes); finding != nil {
			findings = append(findings, finding)
		}
	}

	return findings
}

// judge returns the finding for a required check, or nil when its latest
// result, or else its kept pass, passed against the current content. A kept
// pass never outweighs a later failure.
func (g *evidenceGate) judge(
	ctx context.Context,
	hookCtx *hook.Context,
	scope *repoScope,
	check *evidence.Check,
	receipts, passes map[string]*evidence.Receipt,
) *dispatcher.ValidationError {
	fp, err := checkFingerprint(ctx, scope.snap, check)
	if err != nil {
		return g.unavailable("fingerprint the files of check "+check.Name, err, true)
	}

	verdict := abandonedRun(hookCtx, evidence.JudgeKept(
		check, receipts[check.Name], passes[check.Name], fp.digest, g.now(), g.alive,
	))

	if verdict.Satisfied() {
		g.log.Info("evidence check satisfied", "check", check.Name, "digest", fp.digest)

		return nil
	}

	return g.missing(hookCtx, scope.root, check, verdict, fp)
}

// abandonedRun turns a shell run the stopping agent itself started and never
// finished into a canceled one. A provider runs an agent's foreground tool
// calls before that agent's turn ends, so the run was denied or interrupted
// before it reported anything.
func abandonedRun(hookCtx *hook.Context, verdict evidence.Verdict) evidence.Verdict {
	receipt := verdict.Receipt
	if verdict.Status != evidence.StatusRunning || receipt == nil ||
		receipt.Source == evidence.SourceVerifier ||
		receipt.SessionID != hookCtx.SessionID || receipt.AgentID != hookCtx.AgentID {
		return verdict
	}

	verdict.Status = evidence.StatusCanceled
	verdict.Reason = "it started but never reported a result, so it was denied or interrupted"

	return verdict
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

// unavailable reports evidence klaudiush could not establish, and the
// failure policy decides whether that blocks. Files klaudiush cannot
// fingerprint block by default: an agent can make a file unreadable, and
// unknown content must not count as checked. Unreadable session state warns
// by default, since a held lock is transient. The completion gate's block
// limit keeps either from holding the agent forever.
func (g *evidenceGate) unavailable(
	what string,
	err error,
	blocks bool,
) *dispatcher.ValidationError {
	return g.unavailableFor(validator.ReasonState, what, err, blocks)
}

// configUnavailable reports checks klaudiush could not load. It blocks by
// default like unreadable files: a broken configuration must not turn the
// gate off for the repository it belongs to.
func (g *evidenceGate) configUnavailable(what string, err error) *dispatcher.ValidationError {
	return g.unavailableFor(validator.ReasonConfig, what, err, true)
}

func (g *evidenceGate) unavailableFor(
	reason validator.UnavailableReason,
	what string,
	err error,
	blocks bool,
) *dispatcher.ValidationError {
	action := g.policy.Resolve(evidenceValidator, reason, blocks)

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
		UnavailableReason: reason,
		Resource:          evidenceValidator,
	}
}
