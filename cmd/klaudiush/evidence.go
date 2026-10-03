package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"

	"github.com/smykla-skalski/klaudiush/internal/evidence"
	kexec "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

// errUnknownCheck is returned for a check name the configuration lacks.
var errUnknownCheck = errors.New("unknown evidence check")

// exitCodeError carries the exit status of a check the verifier ran, so
// klaudiush exits with it without printing an error of its own.
type exitCodeError struct {
	code int
}

func (e *exitCodeError) Error() string {
	return fmt.Sprintf("check exited with status %d", e.code)
}

// commandExitCode returns the exit status a subcommand asked klaudiush to
// exit with.
func commandExitCode(err error) (int, bool) {
	var exitErr *exitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.code, true
	}

	return 0, false
}

var evidenceCmd = &cobra.Command{
	Use:   cmdUseEvidence,
	Short: "Run required checks and inspect their results",
	Long: `Run required checks and inspect their results.

With [evidence] enabled, a session that changed files a required check covers
cannot finish until that check passed against the files as they are now.

Examples:
  klaudiush evidence status        # Show each check and whether it is satisfied
  klaudiush evidence run tests     # Run the "tests" check and record the result`,
	Args: cobra.NoArgs,
	RunE: runEvidenceStatus,
}

var evidenceStatusCmd = &cobra.Command{
	Use:   cmdUseStatus,
	Short: "Show each required check and its latest result",
	Args:  cobra.NoArgs,
	RunE:  runEvidenceStatus,
}

var evidenceRunCmd = &cobra.Command{
	Use:   "run <check>",
	Short: "Run a required check and record its result",
	Long: `Run a required check from the repository root and record its result.

klaudiush runs the check's first configured command itself, records its exit
status, and ties the result to a digest of the files the check covers. The
result counts only while those files stay unchanged. It can run in the
background: until it records a result the completion gate reports the
check as running and blocks, within its usual limit of 3 blocks a turn.`,
	Args: cobra.ExactArgs(1),
	RunE: runEvidenceRun,
}

func init() {
	rootCmd.AddCommand(evidenceCmd)

	evidenceCmd.AddCommand(evidenceStatusCmd)
	evidenceCmd.AddCommand(evidenceRunCmd)
}

// evidenceSetup is the configuration and repository an evidence command
// works on.
type evidenceSetup struct {
	cfg    *config.Config
	checks []*evidence.Check
	repo   string
}

func loadEvidenceSetup(ctx context.Context, log logger.Logger) (*evidenceSetup, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get working directory")
	}

	cfg, err := loadConfig(log, cwd)
	if err != nil {
		return nil, err
	}

	checks, err := evidence.Compile(cfg.Evidence)
	if err != nil {
		return nil, errors.Wrap(err, "invalid evidence configuration")
	}

	repo, err := evidence.RepoRoot(ctx, cwd)
	if err != nil {
		return nil, errors.Wrap(err, "evidence checks run inside a git repository")
	}

	return &evidenceSetup{cfg: cfg, checks: checks, repo: repo}, nil
}

func runEvidenceRun(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	setup, err := loadEvidenceSetup(cmd.Context(), loggerFromCmd(cmd))
	if err != nil {
		return err
	}

	check := evidence.Find(setup.checks, args[0])
	if check == nil {
		return errors.Wrapf(errUnknownCheck, "%q (configured: %s)",
			args[0], checkNames(setup.checks))
	}

	verifier := &checkVerifier{
		store:  hooksession.NewStore(),
		runner: kexec.NewCommandRunner(0),
		now:    time.Now,
		stdin:  cmd.InOrStdin(),
		stdout: cmd.OutOrStdout(),
		stderr: cmd.ErrOrStderr(),
		notify: cmd.PrintErrf,
	}

	code, err := verifier.run(cmd.Context(), setup.repo, check)
	if err != nil {
		return err
	}

	if code != 0 {
		return &exitCodeError{code: code}
	}

	return nil
}

func checkNames(checks []*evidence.Check) string {
	if len(checks) == 0 {
		return "none"
	}

	names := make([]string, 0, len(checks))
	for _, check := range checks {
		names = append(names, check.Name)
	}

	return strings.Join(names, ", ")
}

// checkVerifier runs a check itself, so its exit status comes from the
// process and not from anything the agent or the provider says about it.
// fingerprint and cleanup default to fingerprinting the work tree and to
// defaultCleanupTimeout.
type checkVerifier struct {
	store       *hooksession.Store
	runner      kexec.OptionsRunner
	now         func() time.Time
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	notify      func(format string, args ...any)
	fingerprint func(ctx context.Context, repo string, check *evidence.Check) (fingerprint, error)
	cleanup     time.Duration
}

// defaultCleanupTimeout bounds the fingerprint taken after a run, which must
// still happen when the run itself was interrupted or timed out.
const defaultCleanupTimeout = time.Minute

func worktreeFingerprint(
	ctx context.Context,
	repo string,
	check *evidence.Check,
) (fingerprint, error) {
	return checkFingerprint(ctx, &lazySnapshot{root: repo}, check)
}

// run records the check as running, runs it from the repository root, and
// records how it ended. It returns the check's exit status. The check's
// timeout bounds the starting fingerprint and the run; the end fingerprint
// gets its own deadline, since it is taken after cancellation too.
func (v *checkVerifier) run(ctx context.Context, repo string, check *evidence.Check) (int, error) {
	takeFingerprint := v.fingerprint
	if takeFingerprint == nil {
		takeFingerprint = worktreeFingerprint
	}

	runCtx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	start, err := takeFingerprint(runCtx, repo, check)
	if err != nil {
		return 0, errors.Wrap(err, "failed to fingerprint the files the check covers")
	}

	receipt := &evidence.Receipt{
		RunID:     evidence.NewRunID(),
		CheckID:   check.ID(),
		Check:     check.Name,
		Kind:      check.Kind,
		Status:    evidence.StatusRunning,
		Digest:    start.digest,
		Files:     start.files,
		Base:      start.diff.Base,
		Changed:   start.diff.Changed,
		Source:    evidence.SourceVerifier,
		Command:   check.RunCommand(),
		PID:       os.Getpid(),
		StartedAt: v.now(),
	}

	if putErr := v.store.PutReceipt(repo, receipt); putErr != nil {
		return 0, errors.Wrap(putErr, "failed to record the check start")
	}

	status, code, detail := v.execute(runCtx, repo, check)

	cleanup := v.cleanup
	if cleanup <= 0 {
		cleanup = defaultCleanupTimeout
	}

	endCtx, endCancel := context.WithTimeout(context.WithoutCancel(ctx), cleanup)
	defer endCancel()

	end, endErr := takeFingerprint(endCtx, repo, check)
	if endErr != nil {
		status = evidence.StatusUnverified
		detail = "klaudiush could not fingerprint the files after the run: " +
			firstLine(endErr.Error())
	}

	var finished evidence.Receipt

	found, err := v.store.FinishReceipt(repo, check.Name,
		func(stored *evidence.Receipt) bool { return stored.RunID == receipt.RunID },
		func(stored *evidence.Receipt) {
			stored.Finish(status, &code, end.digest, detail, v.now())
			finished = *stored
		},
	)
	if err != nil {
		return code, errors.Wrap(err, "failed to record the check result")
	}

	if !found {
		v.notify("klaudiush: a newer run of %s replaced this one; its result was not recorded\n",
			check.Name)

		return code, nil
	}

	v.notify("klaudiush: %s\n", finished.Summary())

	return code, nil
}

// execute runs the check's command until ctx, which carries the check's
// timeout, ends, and classifies how it ended.
func (v *checkVerifier) execute(
	ctx context.Context,
	repo string,
	check *evidence.Check,
) (evidence.Status, int, string) {
	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	argv := check.Commands[0]

	result := v.runner.RunWithOptions(runCtx, kexec.RunOptions{
		Dir:    repo,
		Stdin:  v.stdin,
		Stdout: v.stdout,
		Stderr: v.stderr,
	}, argv[0], argv[1:]...)

	switch {
	case result.Err == nil:
		return evidence.StatusPassed, 0, ""
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return evidence.StatusCanceled, exitStatus(result),
			"timed out after " + check.Timeout.String()
	case runCtx.Err() != nil:
		return evidence.StatusCanceled, exitStatus(result), "interrupted"
	case result.ExitCode > 0:
		return evidence.StatusFailed, result.ExitCode, ""
	default:
		return evidence.StatusFailed, 1, "could not start: " + firstLine(result.Err.Error())
	}
}

// exitStatus returns the exit status a finished command reported, or 1 when
// it reported none (it was killed or never started).
func exitStatus(result kexec.CommandResult) int {
	if result.ExitCode > 0 {
		return result.ExitCode
	}

	return 1
}

func runEvidenceStatus(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	ctx := cmd.Context()

	setup, err := loadEvidenceSetup(ctx, loggerFromCmd(cmd))
	if err != nil {
		return err
	}

	state := "disabled"
	if setup.cfg.Evidence.IsEnabled() {
		state = "enabled"
	}

	fmt.Printf("Evidence gate: %s\n", state)
	fmt.Printf("Repository: %s\n", setup.repo)

	if setup.cfg.Evidence.IsEnabled() {
		fmt.Println("Coverage:")

		for _, line := range evidence.CoverageLines() {
			fmt.Println("  " + line)
		}
	} else {
		fmt.Println("Coverage: none, nothing is gated while the gate is disabled")
	}

	if len(setup.checks) == 0 {
		fmt.Println("\nNo checks configured. See docs/EVIDENCE_GUIDE.md.")

		return nil
	}

	printf := func(format string, args ...any) { fmt.Printf(format, args...) }
	store := hooksession.NewStore()

	if err := printToolPhaseStatus(ctx, printf, setup, store, loggerFromCmd(cmd)); err != nil {
		return err
	}

	return printCheckStatus(ctx, printf, store, setup.repo, setup.checks)
}

// printToolPhaseStatus shows whether the evidence tool phase withholds
// Gemini's mutation tools in the repository now, and where it applies.
func printToolPhaseStatus(
	ctx context.Context,
	printf func(format string, args ...any),
	setup *evidenceSetup,
	store *hooksession.Store,
	log logger.Logger,
) error {
	if !setup.cfg.Evidence.GetToolPhase().IsEnabled() {
		printf("Tool phase: disabled\n")

		return nil
	}

	phase := newEvidenceGate(setup.cfg, store, failpolicy.New(setup.cfg.FailurePolicy), log).
		toolPhase()
	if phase.err != nil {
		return errors.Wrap(phase.err, "invalid evidence tool phase")
	}

	st := phase.state(ctx, &hook.Context{WorkingDir: setup.repo})

	switch {
	case st.unavailable != nil:
		printf("Tool phase: unknown, %s\n", st.unavailable.Message)
	case st.restricted:
		printf("Tool phase: restricted, waiting on %s\n", strings.Join(st.unmet, "; "))
		printf("  offered: %s\n", strings.Join(phase.phase.AllowedTools(), ", "))
	default:
		printf("Tool phase: open, %s passed on the current files\n",
			strings.Join(phase.phase.RequiredNames(), ", "))
	}

	for _, line := range evidence.PhaseCoverageLines() {
		printf("  %s\n", line)
	}

	return nil
}

// printCheckStatus shows each check's verdict the way the completion gate
// reaches it: the latest result, or a kept pass on the current content.
func printCheckStatus(
	ctx context.Context,
	printf func(format string, args ...any),
	store *hooksession.Store,
	repo string,
	checks []*evidence.Check,
) error {
	receipts, err := store.Receipts(repo)
	if err != nil {
		return errors.Wrap(err, "failed to read check results")
	}

	passes, err := store.Passes(repo)
	if err != nil {
		return errors.Wrap(err, "failed to read check results")
	}

	snap := &lazySnapshot{root: repo}

	for _, check := range checks {
		printf("\n%s (%s): %s\n", check.Name, check.Kind, check.RunCommand())

		fp, err := checkFingerprint(ctx, snap, check)
		if err != nil {
			printf("  current: unavailable (%s)\n", firstLine(err.Error()))

			continue
		}

		latest := receipts[check.Name]
		verdict := evidence.JudgeKept(
			check, latest, passes[check.Name], fp.digest, time.Now(), evidence.ProcessAlive,
		)

		printf("  current: %s (%d file(s))\n", fp.digest, fp.files)
		printf("  verdict: %s, %s\n", verdict.Status, verdict.Reason)

		if latest != nil {
			printf("  latest:  %s\n", latest.Summary())
		}

		if verdict.Receipt != nil && verdict.Receipt != latest {
			printf("  kept:    %s\n", verdict.Receipt.Summary())
		}
	}

	return nil
}
