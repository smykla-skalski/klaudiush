package main

import (
	"bytes"
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/smykla-skalski/klaudiush/internal/dispatcher"
	"github.com/smykla-skalski/klaudiush/internal/evidence"
	kexec "github.com/smykla-skalski/klaudiush/internal/exec"
	"github.com/smykla-skalski/klaudiush/internal/failpolicy"
	"github.com/smykla-skalski/klaudiush/internal/hooksession"
	"github.com/smykla-skalski/klaudiush/internal/validator"
	"github.com/smykla-skalski/klaudiush/pkg/config"
	"github.com/smykla-skalski/klaudiush/pkg/hook"
	"github.com/smykla-skalski/klaudiush/pkg/logger"
)

func evidenceRepo() string {
	repo := GinkgoT().TempDir()

	run := func(args ...string) {
		cmd := osexec.Command("git", args...)
		cmd.Dir = repo

		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		)
		out, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))
	}

	run("init", "-q")
	Expect(os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n"), 0o600)).To(Succeed())
	run("add", "-A")
	run("commit", "-qm", "init")

	resolved, err := filepath.EvalSymlinks(repo)
	Expect(err).NotTo(HaveOccurred())

	return resolved
}

func evidenceConfig(checks ...*config.EvidenceCheckConfig) *config.Config {
	enabled := true

	return &config.Config{Evidence: &config.EvidenceConfig{Enabled: &enabled, Checks: checks}}
}

func evidenceCtx(event hook.CanonicalEvent, raw, repo string) *hook.Context {
	return &hook.Context{
		Provider:     hook.ProviderClaude,
		Event:        event,
		RawEventName: raw,
		SessionID:    "s1",
		WorkingDir:   repo,
		ToolName:     hook.ToolTypeBash,
		ToolFamily:   hook.ToolFamilyShell,
		ToolUseID:    "t1",
		ToolInput:    hook.ToolInput{Command: "make test"},
	}
}

var _ = Describe("evidenceGate", func() {
	var (
		repo  string
		store *hooksession.Store
		log   logger.Logger
		check *config.EvidenceCheckConfig
	)

	BeforeEach(func() {
		repo = evidenceRepo()
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(GinkgoT().TempDir(), "state.json")),
		)
		log = logger.NewNoOpLogger()
		check = &config.EvidenceCheckConfig{Name: "tests", Commands: []string{"make test"}}
	})

	It("stays off unless enabled with valid checks", func() {
		Expect(newEvidenceGate(nil, store, nil, log)).To(BeNil())
		Expect(newEvidenceGate(&config.Config{}, store, nil, log)).To(BeNil())
		Expect(newEvidenceGate(evidenceConfig(check), nil, nil, log)).To(BeNil())
		Expect(newEvidenceGate(evidenceConfig(), store, nil, log)).To(BeNil())
		Expect(newEvidenceGate(
			evidenceConfig(&config.EvidenceCheckConfig{Name: "x"}), store, nil, log,
		)).To(BeNil())

		var gate *evidenceGate

		errs := []*dispatcher.ValidationError{{Message: "kept"}}
		Expect(gate.apply(context.Background(), &hook.Context{}, errs)).To(Equal(errs))
	})

	It("ignores hooks without a session or outside a repository", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		Expect(gate).NotTo(BeNil())

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", GinkgoT().TempDir())
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())

		stop.SessionID = ""
		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())
		Expect(gate.apply(context.Background(), nil, nil)).To(BeEmpty())
	})

	It("does not record runs klaudiush blocked or approval prompts", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		pre := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)

		blocked := []*dispatcher.ValidationError{{ShouldBlock: true, Message: "no"}}
		gate.apply(context.Background(), pre, blocked)

		permission := evidenceCtx(hook.CanonicalEventBeforeTool, "PermissionRequest", repo)
		gate.apply(context.Background(), permission, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts).To(BeEmpty())
	})

	It("ignores a result for a run it did not see start", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.ToolSucceeded = true

		gate.apply(context.Background(), post, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts).To(BeEmpty())
	})

	It("leaves a run alone when the check definition changed", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		changed := newEvidenceGate(evidenceConfig(&config.EvidenceCheckConfig{
			Name:     "tests",
			Commands: []string{"make test"},
			Paths:    []string{"*.go"},
		}), store, nil, log)
		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.ToolSucceeded = true
		changed.apply(context.Background(), post, nil)

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].Status).To(Equal(evidence.StatusRunning))
	})

	It("lets a pass from another session on the same content count", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		Expect(
			os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600),
		).To(Succeed())

		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		Expect(gate.apply(context.Background(), stop, nil)).To(HaveLen(1))

		other := evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo)
		other.SessionID, other.ToolUseID = "s2", "t9"
		gate.apply(context.Background(), other, nil)

		post := evidenceCtx(hook.CanonicalEventAfterTool, "PostToolUse", repo)
		post.SessionID, post.ToolUseID, post.ToolSucceeded = "s2", "t9", true
		gate.apply(context.Background(), post, nil)

		Expect(gate.apply(context.Background(), stop, nil)).To(BeEmpty())
	})

	It("reports evidence it cannot read according to the failure policy", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		err := os.ErrDeadlineExceeded

		warn := gate.unavailable("read things", err)
		Expect(warn.ShouldBlock).To(BeFalse())
		Expect(warn.Unavailable).To(BeTrue())
		Expect(warn.Reference).To(Equal(validator.RefValidationUnavailable))
		Expect(warn.Message).To(ContainSubstring("could not read things"))

		gate.policy = failpolicy.New(&config.FailurePolicyConfig{Critical: []string{"evidence"}})
		Expect(gate.unavailable("read things", err).ShouldBlock).To(BeTrue())
	})

	It("reports a fingerprint failure at the gate", func() {
		review := &config.EvidenceCheckConfig{
			Name:     "review",
			Kind:     config.EvidenceKindReview,
			Commands: []string{"review"},
			Base:     "no-such-base",
		}
		gate := newEvidenceGate(evidenceConfig(review), store, nil, log)
		gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventBeforeTool, "PreToolUse", repo),
			nil,
		)

		Expect(
			os.WriteFile(filepath.Join(repo, "a.go"), []byte("package b\n"), 0o600),
		).To(Succeed())

		errs := gate.apply(
			context.Background(),
			evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo),
			nil,
		)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Unavailable).To(BeTrue())
		Expect(errs[0].Message).To(ContainSubstring("no-such-base"))
	})

	It("reports a work tree it cannot list at the gate", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		stop := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		snap := &lazySnapshot{root: GinkgoT().TempDir()}

		errs := gate.verdicts(context.Background(), stop, repo, snap,
			map[string]string{gate.checks[0].ID(): hooksession.BaselineUnknown})
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Unavailable).To(BeTrue())
	})

	It("formats repairs and file lists", func() {
		gate := newEvidenceGate(evidenceConfig(check), store, nil, log)
		gate.binary = "/opt/my tools/klaudiush"
		codex := evidenceCtx(hook.CanonicalEventTurnStop, "Stop", repo)
		codex.Provider = hook.ProviderCodex

		repair := gate.repair(
			codex,
			repo,
			gate.checks[0],
			evidence.Verdict{Status: evidence.StatusMissing},
		)
		Expect(repair).To(HavePrefix("Run `'/opt/my tools/klaudiush' evidence run tests`"))
		Expect(repair).To(ContainSubstring("Codex does not tell hooks"))

		running := gate.repair(
			codex,
			repo,
			gate.checks[0],
			evidence.Verdict{Status: evidence.StatusRunning},
		)
		Expect(running).To(HavePrefix("Wait for the running check"))

		Expect(gate.repair(codex, repo, gate.checks[0], evidence.Verdict{Status: "other"})).
			To(HavePrefix("Run "))

		files := make([]string, 12)
		for i := range files {
			files[i] = "f.go"
		}

		Expect(listFiles(files)).To(HaveSuffix("and 2 more"))
		Expect(listFiles([]string{"a", "b"})).To(Equal("a, b"))
		Expect(providerTitle("")).To(Equal("this provider"))
		Expect(upperFirst("")).To(BeEmpty())
		Expect(klaudiushBinary()).NotTo(BeEmpty())
	})

	It("uses the process directory when the provider sends none", func() {
		Expect(evidenceWorkDir(&hook.Context{WorkingDir: "/x"})).To(Equal("/x"))

		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(evidenceWorkDir(&hook.Context{})).To(Equal(cwd))
	})
})

type fakeOptionsRunner struct {
	result kexec.CommandResult
	wait   bool
	opts   kexec.RunOptions
}

func (f *fakeOptionsRunner) RunWithOptions(
	ctx context.Context,
	opts kexec.RunOptions,
	_ string,
	_ ...string,
) kexec.CommandResult {
	f.opts = opts

	if f.wait {
		<-ctx.Done()
	}

	return f.result
}

var _ = Describe("checkVerifier", func() {
	var (
		repo  string
		store *hooksession.Store
		check *evidence.Check
	)

	BeforeEach(func() {
		repo = evidenceRepo()
		store = hooksession.NewStore(
			hooksession.WithStateFile(filepath.Join(GinkgoT().TempDir(), "state.json")),
		)

		checks, err := evidence.Compile(&config.EvidenceConfig{
			Checks: []*config.EvidenceCheckConfig{{
				Name:     "tests",
				Commands: []string{"make test"},
				Timeout:  config.Duration(50 * time.Millisecond),
			}},
		})
		Expect(err).NotTo(HaveOccurred())

		check = checks[0]
	})

	verify := func(runner *fakeOptionsRunner) (int, *evidence.Receipt, string) {
		var notices bytes.Buffer

		verifier := &checkVerifier{
			store:  store,
			runner: runner,
			now:    time.Now,
			notify: func(format string, args ...any) {
				notices.WriteString(strings.TrimSpace(format))

				for range args {
					notices.WriteString(" ")
				}
			},
		}

		code, err := verifier.run(context.Background(), repo, check)
		Expect(err).NotTo(HaveOccurred())

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())

		return code, receipts["tests"], notices.String()
	}

	It("records a pass from the repository root", func() {
		runner := &fakeOptionsRunner{}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(0))
		Expect(runner.opts.Dir).To(Equal(repo))
		Expect(receipt.Status).To(Equal(evidence.StatusPassed))
		Expect(receipt.Source).To(Equal(evidence.SourceVerifier))
		Expect(*receipt.ExitCode).To(Equal(0))
	})

	It("records a failure with the exit status", func() {
		runner := &fakeOptionsRunner{result: kexec.CommandResult{ExitCode: 4, Err: os.ErrInvalid}}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(4))
		Expect(receipt.Status).To(Equal(evidence.StatusFailed))
	})

	It("records a command that could not start", func() {
		runner := &fakeOptionsRunner{result: kexec.CommandResult{Err: os.ErrNotExist}}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(1))
		Expect(receipt.Status).To(Equal(evidence.StatusFailed))
		Expect(receipt.Detail).To(ContainSubstring("could not start"))
	})

	It("records a timeout as canceled", func() {
		runner := &fakeOptionsRunner{wait: true, result: kexec.CommandResult{Err: os.ErrClosed}}
		code, receipt, _ := verify(runner)

		Expect(code).To(Equal(1))
		Expect(receipt.Status).To(Equal(evidence.StatusCanceled))
		Expect(receipt.Detail).To(ContainSubstring("timed out"))
	})

	It("does not overwrite a newer run", func() {
		runner := &fakeOptionsRunner{}
		runner.result = kexec.CommandResult{}

		replacing := &replacingRunner{store: store, repo: repo}

		var notices bytes.Buffer

		verifier := &checkVerifier{
			store:  store,
			runner: replacing,
			now:    time.Now,
			notify: func(format string, _ ...any) { notices.WriteString(format) },
		}

		_, err := verifier.run(context.Background(), repo, check)
		Expect(err).NotTo(HaveOccurred())
		Expect(notices.String()).To(ContainSubstring("replaced this one"))

		receipts, err := store.Receipts(repo)
		Expect(err).NotTo(HaveOccurred())
		Expect(receipts["tests"].RunID).To(Equal("newer"))
	})

	It("refuses a work tree it cannot fingerprint", func() {
		verifier := &checkVerifier{store: store, runner: &fakeOptionsRunner{}, now: time.Now}

		_, err := verifier.run(context.Background(), GinkgoT().TempDir(), check)
		Expect(err).To(MatchError(ContainSubstring("failed to fingerprint")))
	})

	It("passes exit codes through", func() {
		code, ok := commandExitCode(&exitCodeError{code: 7})
		Expect(ok).To(BeTrue())
		Expect(code).To(Equal(7))
		Expect((&exitCodeError{code: 7}).Error()).To(ContainSubstring("7"))

		_, ok = commandExitCode(os.ErrInvalid)
		Expect(ok).To(BeFalse())

		Expect(checkNames(nil)).To(Equal("none"))
	})
})

type replacingRunner struct {
	store *hooksession.Store
	repo  string
}

func (r *replacingRunner) RunWithOptions(
	context.Context,
	kexec.RunOptions,
	string,
	...string,
) kexec.CommandResult {
	Expect(r.store.PutReceipt(r.repo, &evidence.Receipt{RunID: "newer", Check: "tests"})).
		To(Succeed())

	return kexec.CommandResult{}
}
