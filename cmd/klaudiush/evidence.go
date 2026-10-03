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
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/pkg/config"
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
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

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
type checkVerifier struct {
	store  *hooksession.Store
	runner kexec.OptionsRunner
	now    func() time.Time
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	notify func(format string, args ...any)
}

// run records the check as running, runs it from the repository root, and
// records how it ended. It returns the check's exit status.
func (v *checkVerifier) run(ctx context.Context, repo string, check *evidence.Check) (int, error) {
	start, err := checkFingerprint(ctx, &lazySnapshot{root: repo}, check)
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

	status, code, detail := v.execute(ctx, repo, check)

	end, endErr := checkFingerprint(context.WithoutCancel(ctx), &lazySnapshot{root: repo}, check)
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

// execute runs the check's command and classifies how it ended.
func (v *checkVerifier) execute(
	ctx context.Context,
	repo string,
	check *evidence.Check,
) (evidence.Status, int, string) {
	runCtx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	runCtx, stop := signal.NotifyContext(runCtx, os.Interrupt, syscall.SIGTERM)
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

	receipts, err := hooksession.NewStore().Receipts(setup.repo)
	if err != nil {
		return errors.Wrap(err, "failed to read check results")
	}

	snap := &lazySnapshot{root: setup.repo}

	for _, check := range setup.checks {
		fmt.Printf("\n%s (%s): %s\n", check.Name, check.Kind, check.RunCommand())

		fp, err := checkFingerprint(ctx, snap, check)
		if err != nil {
			fmt.Printf("  current: unavailable (%s)\n", firstLine(err.Error()))

			continue
		}

		verdict := evidence.Judge(
			check,
			receipts[check.Name],
			fp.digest,
			time.Now(),
			evidence.ProcessAlive,
		)
		fmt.Printf("  current: %s (%d file(s))\n", fp.digest, fp.files)
		fmt.Printf("  verdict: %s, %s\n", verdict.Status, verdict.Reason)

		if verdict.Receipt != nil {
			fmt.Printf("  latest:  %s\n", verdict.Receipt.Summary())
		}
	}

	return nil
}
